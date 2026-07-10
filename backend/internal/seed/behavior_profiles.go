package seed

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
)

type weightedWeekday struct {
	Weekday int     `json:"weekday"`
	Weight  float64 `json:"weight"`
}

type weightedPeriod struct {
	Period string  `json:"period"`
	Weight float64 `json:"weight"`
}

type weightedLocation struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Weight  float64 `json:"weight"`
}

var personaActiveWeekdays = [][]int{
	{2, 4, 6},
	{3, 6, 7},
	{5, 6, 7},
	{5, 6, 7},
	{1, 3, 7},
	{2, 4, 6},
	{3, 6, 7},
	{5, 6, 7},
	{2, 5, 6},
	{1, 4, 7},
	{4, 6, 7},
	{3, 6, 7},
}

var personaJoinWeekdays = [][]int{
	{2, 5, 6},
	{3, 6, 7},
	{5, 6, 7},
	{5, 6, 7},
	{1, 3, 6},
	{2, 4, 7},
	{3, 6, 7},
	{5, 6, 7},
	{2, 5, 7},
	{1, 4, 6},
	{4, 6, 7},
	{3, 6, 7},
}

var personaActivePeriods = [][]string{
	{"evening", "afternoon"},
	{"morning", "evening"},
	{"evening", "night"},
	{"afternoon", "evening"},
	{"morning", "afternoon"},
	{"evening", "night"},
	{"afternoon", "morning"},
	{"evening", "afternoon"},
	{"evening", "afternoon"},
	{"morning", "evening"},
	{"afternoon", "evening"},
	{"morning", "afternoon"},
}

func buildSeedBehaviorProfiles(users []model.User, personaByUserID map[string]int, now int64) []model.UserBehaviorProfile {
	profiles := make([]model.UserBehaviorProfile, 0, len(users))
	for index, user := range users {
		if user.Role != model.UserRoleUser {
			continue
		}
		personaIndex := personaByUserID[user.ID]
		persona := fullPersonas[positiveModulo(personaIndex, len(fullPersonas))]
		profiles = append(profiles, buildSeedBehaviorProfile(user, persona, personaIndex, index, now))
	}
	return profiles
}

func BackfillBehaviorProfiles(db *gorm.DB, now int64) (int, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	var users []model.User
	if err := db.Where("role = ? AND deleted_at = 0", model.UserRoleUser).
		Order("id ASC").
		Find(&users).Error; err != nil {
		return 0, fmt.Errorf("query users for behavior profiles: %w", err)
	}
	if len(users) == 0 {
		return 0, nil
	}

	profiles := buildSeedBehaviorProfiles(users, buildPersonaByUser(users), now)
	if len(profiles) == 0 {
		return 0, nil
	}

	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"active_weekdays_json",
			"join_weekdays_json",
			"active_periods_json",
			"category_weights_json",
			"sub_category_weights_json",
			"preferred_locations_json",
			"invite_accept_rate",
			"reliability_score",
			"exploration_score",
			"weekly_active_score",
			"weekly_participation_score",
			"updated_at",
		}),
	}).CreateInBatches(profiles, 200).Error; err != nil {
		return 0, fmt.Errorf("upsert behavior profiles: %w", err)
	}
	return len(profiles), nil
}

func buildSeedBehaviorProfile(user model.User, persona fullPersona, personaIndex, userIndex int, now int64) model.UserBehaviorProfile {
	return model.UserBehaviorProfile{
		UserID:                   user.ID,
		ActiveWeekdaysJSON:       mustJSON(weightedWeekdays(personaActiveWeekdays, personaIndex, userIndex)),
		JoinWeekdaysJSON:         mustJSON(weightedWeekdays(personaJoinWeekdays, personaIndex, userIndex+1)),
		ActivePeriodsJSON:        mustJSON(weightedPeriods(personaActivePeriods, personaIndex, userIndex)),
		CategoryWeightsJSON:      mustJSON(categoryWeightsForPersona(persona)),
		SubCategoryWeightsJSON:   mustJSON(subCategoryWeightsForPersona(persona)),
		PreferredLocationsJSON:   mustJSON(preferredLocationsForPersona(persona, userIndex)),
		InviteAcceptRate:         round2(0.46 + float64(positiveModulo(userIndex+personaIndex*3, 26))/100),
		ReliabilityScore:         round2(0.70 + float64(user.CreditScore-70)/100*0.18 + float64(positiveModulo(userIndex, 5))/100),
		ExplorationScore:         round2(0.18 + float64(positiveModulo(userIndex+personaIndex, 18))/100),
		WeeklyActiveScore:        round2(0.54 + float64(positiveModulo(userIndex*7+personaIndex, 32))/100),
		WeeklyParticipationScore: round2(0.42 + float64(positiveModulo(userIndex*5+personaIndex*2, 34))/100),
		CreatedAt:                now,
		UpdatedAt:                now,
	}
}

func weightedWeekdays(source [][]int, personaIndex, seed int) []weightedWeekday {
	values := source[positiveModulo(personaIndex, len(source))]
	result := make([]weightedWeekday, 0, len(values))
	for idx, weekday := range values {
		result = append(result, weightedWeekday{
			Weekday: weekday,
			Weight:  round2(0.92 - float64(idx)*0.12 + float64(positiveModulo(seed+idx, 5))*0.01),
		})
	}
	return result
}

func weightedPeriods(source [][]string, personaIndex, seed int) []weightedPeriod {
	values := source[positiveModulo(personaIndex, len(source))]
	result := make([]weightedPeriod, 0, len(values))
	for idx, period := range values {
		result = append(result, weightedPeriod{
			Period: period,
			Weight: round2(0.90 - float64(idx)*0.16 + float64(positiveModulo(seed, 4))*0.01),
		})
	}
	return result
}

func categoryWeightsForPersona(persona fullPersona) map[string]float64 {
	result := make(map[string]float64)
	addActivityTemplateWeights(result, persona.PrimaryTemplates, 0.96, activityTemplateCategoryKey)
	addActivityTemplateWeights(result, persona.SecondaryTemplates, 0.68, activityTemplateCategoryKey)
	addActivityTemplateWeights(result, persona.ExploreTemplates, 0.36, activityTemplateCategoryKey)
	return result
}

func subCategoryWeightsForPersona(persona fullPersona) map[string]float64 {
	result := make(map[string]float64)
	addActivityTemplateWeights(result, persona.PrimaryTemplates, 0.96, activityTemplateSubCategoryKey)
	addActivityTemplateWeights(result, persona.SecondaryTemplates, 0.68, activityTemplateSubCategoryKey)
	addActivityTemplateWeights(result, persona.ExploreTemplates, 0.36, activityTemplateSubCategoryKey)
	return result
}

func addActivityTemplateWeights(target map[string]float64, templateIndexes []int, weight float64, keyFn func(seedActivityTemplate) string) {
	for _, templateIndex := range templateIndexes {
		template := seedActivityTemplates[positiveModulo(templateIndex, len(seedActivityTemplates))]
		key := keyFn(template)
		current := target[key]
		if weight > current {
			target[key] = weight
		}
	}
}

func activityTemplateCategoryKey(template seedActivityTemplate) string {
	return template.Category
}

func activityTemplateSubCategoryKey(template seedActivityTemplate) string {
	return fmt.Sprintf("%s/%s", template.Category, template.SubCategory)
}

func preferredLocationsForPersona(persona fullPersona, seed int) []weightedLocation {
	result := make([]weightedLocation, 0, len(persona.CityLocations))
	for idx, locationIndex := range persona.CityLocations {
		location := seedLocations[positiveModulo(locationIndex, len(seedLocations))]
		result = append(result, weightedLocation{
			Name:    location.Name,
			Address: location.Address,
			Lat:     location.Lat,
			Lng:     location.Lng,
			Weight:  round2(0.92 - float64(idx)*0.12 + float64(positiveModulo(seed+idx, 4))*0.01),
		})
	}
	return result
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(raw)
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
