package recommend

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
)

const (
	ModelKeyLRRanker = "lr_ranker_v1"
	modelNameBGE     = "backend-model"
)

type tagAccumulator struct {
	Weight        float64
	EvidenceCount int
	LastEventAt   int64
}

func DefaultRankingWeights() map[string]float64 {
	return map[string]float64{
		"embedding_cosine":                     0.46,
		"sub_category_match":                   0.18,
		"category_match":                       0.08,
		"city_match":                           0.06,
		"location_proximity":                   0.24,
		"author_quality":                       0.18,
		"interaction_heat":                     0.06,
		"freshness":                            0.08,
		"joinability":                          0.06,
		"author_rating_score":                  0.07,
		"author_credit_score":                  0.06,
		"author_activity_score_count":          0.03,
		"post_current_count":                   0.02,
		"post_chat_count":                      0.03,
		"post_review_count":                    0.03,
		"post_age_hours":                       -0.02,
		"fixed_time_distance_hours":            -0.02,
		"viewer_clicked_same_subcategory_7d":   0.08,
		"viewer_unclicked_same_subcategory_7d": -0.05,
	}
}

func EnsureDefaultRecommendationModel(db *gorm.DB, nowMS int64) error {
	weightsJSON, err := json.Marshal(DefaultRankingWeights())
	if err != nil {
		return fmt.Errorf("marshal default ranking weights: %w", err)
	}
	statsJSON, err := json.Marshal(map[string]any{
		"exposureCount": 0,
		"clickCount":    0,
		"auc":           0,
		"logLoss":       0,
		"fallback":      true,
	})
	if err != nil {
		return fmt.Errorf("marshal default training stats: %w", err)
	}
	row := model.RecommendationModel{
		ModelKey:      ModelKeyLRRanker,
		Version:       1,
		Intercept:     0,
		FeatureJSON:   string(weightsJSON),
		TrainingStats: string(statsJSON),
		TrainedAt:     nowMS,
		CreatedAt:     nowMS,
		UpdatedAt:     nowMS,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"feature_json":   row.FeatureJSON,
			"training_stats": row.TrainingStats,
			"updated_at":     row.UpdatedAt,
		}),
	}).Create(&row).Error
}

func RebuildUserTags(db *gorm.DB, userIDs []string, nowMS int64) error {
	uniqueIDs := uniqueStrings(userIDs)
	for _, userID := range uniqueIDs {
		if err := rebuildSingleUserTags(db, userID, nowMS); err != nil {
			return err
		}
	}
	return nil
}

func DefaultModelName() string {
	return modelNameBGE
}

func rebuildSingleUserTags(db *gorm.DB, userID string, nowMS int64) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}

	tagMap := map[string]*tagAccumulator{}
	addTags := func(tagType, tagValue string, baseWeight float64, eventAt int64) {
		tagValue = strings.TrimSpace(tagValue)
		if tagValue == "" || baseWeight == 0 {
			return
		}
		key := tagType + "\x00" + tagValue
		acc, ok := tagMap[key]
		if !ok {
			acc = &tagAccumulator{}
			tagMap[key] = acc
		}
		weight := baseWeight
		if eventAt >= nowMS-int64(30*24*time.Hour/time.Millisecond) {
			weight *= 1.2
		}
		acc.Weight += weight
		acc.EvidenceCount++
		if eventAt > acc.LastEventAt {
			acc.LastEventAt = eventAt
		}
	}

	var authoredPosts []model.Post
	if err := db.Where("author_id = ?", userID).Find(&authoredPosts).Error; err != nil {
		return fmt.Errorf("query authored posts for %s: %w", userID, err)
	}
	for _, post := range authoredPosts {
		addTags("sub_category", post.SubCategory, 5, post.CreatedAt)
		addTags("category", post.Category, 3, post.CreatedAt)
		addTags("city", cityFromAddress(post.Address), 2, post.CreatedAt)
	}

	var joinedRelations []model.PostParticipant
	if err := db.Where("user_id = ? AND status = ?", userID, "active").Find(&joinedRelations).Error; err != nil {
		return fmt.Errorf("query joined relations for %s: %w", userID, err)
	}
	joinedPostIDs := make([]string, 0, len(joinedRelations))
	for _, relation := range joinedRelations {
		joinedPostIDs = append(joinedPostIDs, relation.PostID)
	}
	joinedPosts, err := postsByIDs(db, joinedPostIDs)
	if err != nil {
		return err
	}
	for _, relation := range joinedRelations {
		post, ok := joinedPosts[relation.PostID]
		if !ok {
			continue
		}
		addTags("sub_category", post.SubCategory, 4, relation.JoinedAt)
		addTags("category", post.Category, 2, relation.JoinedAt)
		addTags("city", cityFromAddress(post.Address), 1, relation.JoinedAt)
	}

	var clicks []model.FeedClick
	if err := db.Where("user_id = ?", userID).Find(&clicks).Error; err != nil {
		return fmt.Errorf("query feed clicks for %s: %w", userID, err)
	}
	clickPostIDs := make([]string, 0, len(clicks))
	for _, click := range clicks {
		clickPostIDs = append(clickPostIDs, click.PostID)
	}
	clickPosts, err := postsByIDs(db, clickPostIDs)
	if err != nil {
		return err
	}
	for _, click := range clicks {
		post, ok := clickPosts[click.PostID]
		if !ok {
			continue
		}
		addTags("sub_category", post.SubCategory, 1.5, click.CreatedAt)
		addTags("category", post.Category, 1, click.CreatedAt)
		addTags("city", cityFromAddress(post.Address), 0.5, click.CreatedAt)
	}

	var chatMessages []model.ChatMessage
	if err := db.Where("sender_id = ?", userID).Order("created_at ASC").Find(&chatMessages).Error; err != nil {
		return fmt.Errorf("query chat messages for %s: %w", userID, err)
	}
	seenChatPosts := make(map[string]struct{}, len(chatMessages))
	chatPostIDs := make([]string, 0, len(chatMessages))
	for _, msg := range chatMessages {
		if _, ok := seenChatPosts[msg.PostID]; ok {
			continue
		}
		seenChatPosts[msg.PostID] = struct{}{}
		chatPostIDs = append(chatPostIDs, msg.PostID)
	}
	chatPosts, err := postsByIDs(db, chatPostIDs)
	if err != nil {
		return err
	}
	for _, msg := range chatMessages {
		if _, ok := seenChatPosts[msg.PostID]; !ok {
			continue
		}
		delete(seenChatPosts, msg.PostID)
		post, ok := chatPosts[msg.PostID]
		if !ok {
			continue
		}
		addTags("sub_category", post.SubCategory, 1, msg.CreatedAt)
		addTags("category", post.Category, 0.5, msg.CreatedAt)
	}

	exposureCutoff := nowMS - int64(7*24*time.Hour/time.Millisecond)
	var exposures []model.FeedExposure
	if err := db.Where("user_id = ? AND created_at >= ?", userID, exposureCutoff).Find(&exposures).Error; err != nil {
		return fmt.Errorf("query feed exposures for %s: %w", userID, err)
	}
	exposurePostIDs := make([]string, 0, len(exposures))
	for _, exposure := range exposures {
		exposurePostIDs = append(exposurePostIDs, exposure.PostID)
	}
	exposurePosts, err := postsByIDs(db, exposurePostIDs)
	if err != nil {
		return err
	}
	exposureCountBySub := map[string]int{}
	clickCountBySub := map[string]int{}
	for _, exposure := range exposures {
		post, ok := exposurePosts[exposure.PostID]
		if !ok {
			continue
		}
		subCategory := strings.TrimSpace(post.SubCategory)
		if subCategory == "" {
			continue
		}
		exposureCountBySub[subCategory]++
	}
	for _, click := range clicks {
		if click.CreatedAt < exposureCutoff {
			continue
		}
		post, ok := clickPosts[click.PostID]
		if !ok {
			continue
		}
		subCategory := strings.TrimSpace(post.SubCategory)
		if subCategory == "" {
			continue
		}
		clickCountBySub[subCategory]++
	}
	for key, acc := range tagMap {
		if !strings.HasPrefix(key, "sub_category\x00") {
			continue
		}
		subCategory := strings.TrimPrefix(key, "sub_category\x00")
		if exposureCountBySub[subCategory] >= 5 && clickCountBySub[subCategory] == 0 {
			acc.Weight *= 0.9
		}
	}

	rows := make([]model.UserTag, 0, len(tagMap))
	for key, acc := range tagMap {
		if acc.Weight <= 0.05 {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		rows = append(rows, model.UserTag{
			UserID:        userID,
			TagType:       parts[0],
			TagValue:      parts[1],
			Weight:        acc.Weight,
			EvidenceCount: acc.EvidenceCount,
			LastEventAt:   acc.LastEventAt,
			CreatedAt:     nowMS,
			UpdatedAt:     nowMS,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Weight == rows[j].Weight {
			return rows[i].LastEventAt > rows[j].LastEventAt
		}
		return rows[i].Weight > rows[j].Weight
	})

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserTag{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func postsByIDs(db *gorm.DB, postIDs []string) (map[string]model.Post, error) {
	ids := uniqueStrings(postIDs)
	if len(ids) == 0 {
		return map[string]model.Post{}, nil
	}
	var posts []model.Post
	if err := db.Where("id IN ?", ids).Find(&posts).Error; err != nil {
		return nil, fmt.Errorf("query posts by ids: %w", err)
	}
	result := make(map[string]model.Post, len(posts))
	for _, post := range posts {
		result[post.ID] = post
	}
	return result, nil
}

func cityFromAddress(address string) string {
	value := strings.TrimSpace(address)
	if value == "" {
		return ""
	}
	cities := []string{
		"\u4e0a\u6d77",
		"\u5317\u4eac",
		"\u5e7f\u5dde",
		"\u6df1\u5733",
		"\u676d\u5dde",
		"\u6210\u90fd",
		"\u6b66\u6c49",
		"\u5357\u4eac",
		"\u82cf\u5dde",
		"\u897f\u5b89",
		"\u91cd\u5e86",
		"\u5929\u6d25",
	}
	for _, city := range cities {
		if strings.Contains(value, city) {
			return city
		}
	}
	return ""
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, item := range input {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
