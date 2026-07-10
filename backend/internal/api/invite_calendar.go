package api

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"make_friends/backend/internal/model"
)

const (
	inviteCalendarDefaultLimit       = 20
	inviteCalendarMaxLimit           = 50
	inviteCandidateThreshold         = 0.54
	inviteCandidateCategoryThreshold = 0.16
	inviteCandidateWeekdayThreshold  = 0.58
)

type inviteCalendarDayView struct {
	Date           string  `json:"date"`
	CandidateCount int     `json:"candidateCount"`
	Score          float64 `json:"score"`
	FireLevel      int     `json:"fireLevel"`
	FireText       string  `json:"fireText"`
	Highlighted    bool    `json:"highlighted"`
	Reason         string  `json:"reason"`
}

type inviteCalendarCandidateView struct {
	User       userBrief `json:"user"`
	MatchScore float64   `json:"matchScore"`
	ReasonTags []string  `json:"reasonTags"`
	ReasonText string    `json:"reasonText"`
	Selected   bool      `json:"selected"`
}

type inviteCalendarCandidateContext struct {
	User    model.User
	Profile calendarBehaviorProfile
	Tags    map[string]float64
}

type inviteCalendarCandidateScore struct {
	Context    inviteCalendarCandidateContext
	Score      float64
	ReasonTags []string
}

func (s *Server) InviteCalendarHeatmap(c *gin.Context) {
	userID := mustUserID(c)
	startDate, days := parseCalendarRange(c)
	endDate := startDate.AddDate(0, 0, days-1)
	query := parseInviteCalendarQuery(c)

	contexts, err := s.loadInviteCalendarCandidateContexts(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query invite candidates failed"})
		return
	}

	dayViews := make([]inviteCalendarDayView, 0, days)
	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i)
		scored := filterInviteCandidateScores(scoreInviteCandidatesForDate(contexts, date, query), inviteCandidateThreshold)
		dayViews = append(dayViews, buildInviteCalendarDayView(date, scored))
	}
	applyInviteCalendarHeatNormalization(dayViews)

	c.JSON(http.StatusOK, gin.H{
		"startDate": startDate.Format("2006-01-02"),
		"endDate":   endDate.Format("2006-01-02"),
		"days":      dayViews,
	})
}

func (s *Server) InviteCalendarCandidates(c *gin.Context) {
	userID := mustUserID(c)
	date, ok := parseCalendarDate(c.Query("date"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required"})
		return
	}
	query := parseInviteCalendarQuery(c)
	selectedIDs := parseSelectedInviteeIDs(c.Query("selectedIds"))
	limit := queryIntOrDefault(c.Query("limit"), inviteCalendarDefaultLimit)
	if limit <= 0 || limit > inviteCalendarMaxLimit {
		limit = inviteCalendarDefaultLimit
	}

	contexts, err := s.loadInviteCalendarCandidateContexts(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query invite candidates failed"})
		return
	}
	scored := scoreInviteCandidatesForDate(contexts, date, query)
	scored = filterInviteCandidateScores(scored, inviteCandidateThreshold)
	sortInviteCandidateScores(scored)
	if len(scored) > limit {
		scored = scored[:limit]
	}

	views := make([]inviteCalendarCandidateView, 0, len(scored))
	for _, item := range scored {
		user := item.Context.User
		normalizeUserModel(&user)
		views = append(views, inviteCalendarCandidateView{
			User:       toUserBrief(user),
			MatchScore: roundCalendarScore(item.Score),
			ReasonTags: item.ReasonTags,
			ReasonText: inviteCandidateReasonText(item.ReasonTags),
			Selected:   selectedIDs[user.ID],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"date":       date.Format("2006-01-02"),
		"candidates": views,
	})
}

func parseInviteCalendarQuery(c *gin.Context) calendarQuery {
	query := parseCalendarQuery(c)
	textIntent := parseCalendarTextIntent(query.Raw)
	if query.Raw != "" && textIntent.HasActivity {
		query.Category = textIntent.Category
		query.SubCategory = textIntent.SubCategory
		if textIntent.AddressKeyword != "" {
			query.AddressKeyword = textIntent.AddressKeyword
		}
		if textIntent.Period != "" {
			query.Period = textIntent.Period
		}
	}
	if address := strings.TrimSpace(c.Query("address")); address != "" && query.AddressKeyword == "" {
		query.AddressKeyword = address
	}
	if period := normalizeCalendarPeriod(c.Query("timePeriod")); period != "" && query.Period == "" {
		query.Period = period
	}
	if point := queryInviteCalendarGeoPoint(c); point != nil {
		query.UserLat = point.Latitude
		query.UserLng = point.Longitude
		query.HasUserCoords = true
	}
	query.HasIntent = query.Raw != "" || query.Category != "" || query.SubCategory != "" || query.AddressKeyword != "" || query.Period != ""
	return query
}

func queryInviteCalendarGeoPoint(c *gin.Context) *geoPoint {
	if point := queryGeoPoint(c); point != nil {
		return point
	}
	lat, latOK := queryFloat(c.Query("lat"))
	lng, lngOK := queryFloat(c.Query("lng"))
	if !latOK || !lngOK || !validGeoPoint(lat, lng) {
		return nil
	}
	return &geoPoint{Latitude: lat, Longitude: lng}
}

func (s *Server) loadInviteCalendarCandidateContexts(viewerID string) ([]inviteCalendarCandidateContext, error) {
	var users []model.User
	if err := activeUsersQuery(s.DB.Model(&model.User{})).
		Where("id <> ? AND role = ?", viewerID, model.UserRoleUser).
		Order("updated_at DESC").
		Limit(500).
		Find(&users).Error; err != nil {
		return nil, err
	}
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	tagMap, err := s.inviteCalendarTagsByUser(userIDs)
	if err != nil {
		return nil, err
	}

	contexts := make([]inviteCalendarCandidateContext, 0, len(users))
	for _, user := range users {
		normalizeUserModel(&user)
		contexts = append(contexts, inviteCalendarCandidateContext{
			User:    user,
			Profile: s.loadCalendarBehaviorProfile(user.ID),
			Tags:    tagMap[user.ID],
		})
	}
	return contexts, nil
}

func (s *Server) inviteCalendarTagsByUser(userIDs []string) (map[string]map[string]float64, error) {
	result := map[string]map[string]float64{}
	uniqueIDs := uniqueStrings(userIDs)
	if len(uniqueIDs) == 0 {
		return result, nil
	}
	var rows []model.UserTag
	if err := s.DB.Where("user_id IN ?", uniqueIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if result[row.UserID] == nil {
			result[row.UserID] = map[string]float64{}
		}
		key := inviteCalendarTagKey(row.TagType, row.TagValue)
		weight := clamp(row.Weight/6, 0, 1)
		if weight > result[row.UserID][key] {
			result[row.UserID][key] = weight
		}
	}
	return result, nil
}

func inviteCalendarTagKey(tagType, value string) string {
	return strings.TrimSpace(tagType) + "\x00" + strings.TrimSpace(value)
}

func scoreInviteCandidatesForDate(contexts []inviteCalendarCandidateContext, date time.Time, query calendarQuery) []inviteCalendarCandidateScore {
	scored := make([]inviteCalendarCandidateScore, 0, len(contexts))
	for _, context := range contexts {
		scoreValue, reasons := scoreInviteCandidateForDate(context, date, query)
		scored = append(scored, inviteCalendarCandidateScore{
			Context:    context,
			Score:      scoreValue,
			ReasonTags: reasons,
		})
	}
	return scored
}

func scoreInviteCandidateForDate(context inviteCalendarCandidateContext, date time.Time, query calendarQuery) (float64, []string) {
	profile := context.Profile
	reasons := []string{}
	scoreValue := 0.12

	categoryScore, categoryReason := inviteCandidateCategoryScore(context, query)
	scoreValue += categoryScore * 0.28
	if categoryReason != "" {
		reasons = append(reasons, categoryReason)
	}
	if query.HasIntent && (query.Category != "" || query.SubCategory != "") && categoryScore < inviteCandidateCategoryThreshold {
		return 0, []string{}
	}

	weekdayScore := profileWeekdayScore(date, profile)
	if weekdayScore < inviteCandidateWeekdayThreshold {
		return 0, []string{}
	}
	scoreValue += weekdayScore * 0.18
	if weekdayScore >= 0.72 {
		reasons = append(reasons, inviteWeekdayLabel(date)+"活跃")
	}

	periodScore := inviteCandidatePeriodScore(profile, query.Period)
	scoreValue += periodScore * 0.16
	if query.Period != "" && periodScore >= 0.72 {
		reasons = append(reasons, invitePeriodLabel(query.Period)+"常在线")
	}

	locationScore := inviteCandidateLocationScore(profile, query)
	scoreValue += locationScore * 0.18
	if locationScore >= 0.72 {
		reasons = append(reasons, "地点相近")
	}

	acceptRate := clamp(profile.InviteAcceptRate, 0, 1)
	scoreValue += acceptRate * 0.10
	if acceptRate >= 0.62 {
		reasons = append(reasons, "接受邀请率高")
	}

	reliability := clamp(profile.ReliabilityScore, 0, 1)
	if reliability == 0 {
		reliability = clamp(float64(context.User.CreditScore)/100, 0, 1)
	}
	scoreValue += reliability * 0.10
	if reliability >= 0.82 || context.User.CreditScore >= 92 {
		reasons = append(reasons, "履约稳定")
	}

	activity := math.Max(profile.WeeklyActiveScore, profile.WeeklyParticipation)
	scoreValue += clamp(activity, 0, 1) * 0.08
	if activity >= 0.72 {
		reasons = append(reasons, "最近活跃")
	}

	userQuality := (clamp(float64(context.User.CreditScore)/100, 0, 1) + clamp(context.User.RatingScore/5, 0, 1)) / 2
	scoreValue += userQuality * 0.08

	if query.HasIntent && categoryScore < inviteCandidateCategoryThreshold {
		scoreValue -= 0.10
	}
	return clamp(scoreValue, 0, 1.4), limitInviteReasonTags(reasons, 4)
}

func inviteCandidateCategoryScore(context inviteCalendarCandidateContext, query calendarQuery) (float64, string) {
	profile := context.Profile
	best := 0.0
	reason := ""
	if query.SubCategory != "" {
		key := query.Category + "/" + query.SubCategory
		best = math.Max(best, profile.SubCategoryWeights[key])
		best = math.Max(best, context.Tags[inviteCalendarTagKey("sub_category", query.SubCategory)])
		if best >= 0.48 {
			reason = "常参加" + query.SubCategory
		}
	}
	if query.Category != "" {
		categoryScore := math.Max(profile.CategoryWeights[query.Category], context.Tags[inviteCalendarTagKey("category", query.Category)]) * 0.82
		if categoryScore > best {
			best = categoryScore
			if best >= 0.42 {
				reason = "偏好" + query.Category
			}
		}
	}
	if query.Category == "" && query.SubCategory == "" {
		best = 0.34 + profile.ExplorationScore*0.24
		if best >= 0.48 {
			reason = "兴趣较广"
		}
	}
	return clamp(best, 0, 1), reason
}

func inviteCandidatePeriodScore(profile calendarBehaviorProfile, period string) float64 {
	if period == "" {
		best := 0.36
		for _, item := range profile.ActivePeriods {
			if item.Weight > best {
				best = item.Weight
			}
		}
		return clamp(best, 0, 1)
	}
	best := 0.20
	for _, item := range profile.ActivePeriods {
		if item.Period == period && item.Weight > best {
			best = item.Weight
		}
	}
	return clamp(best, 0, 1)
}

func inviteCandidateLocationScore(profile calendarBehaviorProfile, query calendarQuery) float64 {
	best := 0.32
	addressKeyword := normalizeCalendarText(query.AddressKeyword)
	for _, loc := range profile.PreferredLocations {
		if addressKeyword != "" {
			haystack := normalizeCalendarText(loc.Name + loc.Address)
			if strings.Contains(haystack, addressKeyword) || strings.Contains(addressKeyword, haystack) {
				best = math.Max(best, 0.86*math.Max(loc.Weight, 0.3))
			}
		}
		if query.HasUserCoords && loc.Lat != 0 && loc.Lng != 0 {
			best = math.Max(best, distanceToScore(geoDistanceKm(query.UserLat, query.UserLng, loc.Lat, loc.Lng))*math.Max(loc.Weight, 0.35))
		}
	}
	return clamp(best, 0, 1)
}

func filterInviteCandidateScores(scored []inviteCalendarCandidateScore, threshold float64) []inviteCalendarCandidateScore {
	out := make([]inviteCalendarCandidateScore, 0, len(scored))
	for _, item := range scored {
		if item.Score >= threshold {
			out = append(out, item)
		}
	}
	return out
}

func sortInviteCandidateScores(scored []inviteCalendarCandidateScore) {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Context.User.UpdatedAt > scored[j].Context.User.UpdatedAt
		}
		return scored[i].Score > scored[j].Score
	})
}

func buildInviteCalendarDayView(date time.Time, scored []inviteCalendarCandidateScore) inviteCalendarDayView {
	total := 0.0
	for _, item := range scored {
		total += item.Score
	}
	avg := 0.0
	if len(scored) > 0 {
		avg = total / float64(len(scored))
	}
	return inviteCalendarDayView{
		Date:           date.Format("2006-01-02"),
		CandidateCount: len(scored),
		Score:          roundCalendarScore(avg),
	}
}

func applyInviteCalendarHeatNormalization(days []inviteCalendarDayView) {
	activeTotal := 0
	activeDays := 0
	for _, day := range days {
		if day.CandidateCount <= 0 {
			continue
		}
		activeTotal += day.CandidateCount
		activeDays++
	}

	activeAverage := 0.0
	if activeDays > 0 {
		activeAverage = float64(activeTotal) / float64(activeDays)
	}
	for i := range days {
		level := calendarFireLevel(days[i].CandidateCount, activeAverage)
		days[i].FireLevel = level
		days[i].FireText = strings.Repeat("\U0001F525", level)
		days[i].Highlighted = level == 3
		days[i].Reason = inviteCalendarDayReason(days[i].CandidateCount, level)
	}
}

func inviteCalendarDayReason(count, level int) string {
	if count == 0 {
		return "暂无合适候选"
	}
	if level >= 3 {
		return "明显高于近期平均"
	}
	if level == 2 {
		return "接近近期平均"
	}
	return "有少量合适候选"
}

func parseSelectedInviteeIDs(raw string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		id := strings.TrimSpace(item)
		if id != "" {
			result[id] = true
		}
	}
	return result
}

func inviteCandidateReasonText(tags []string) string {
	if len(tags) == 0 {
		return "综合活跃度、信用分和历史偏好较匹配"
	}
	return strings.Join(tags, "、")
}

func limitInviteReasonTags(tags []string, limit int) []string {
	if limit <= 0 || len(tags) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func inviteWeekdayLabel(date time.Time) string {
	switch date.Weekday() {
	case time.Monday:
		return "周一"
	case time.Tuesday:
		return "周二"
	case time.Wednesday:
		return "周三"
	case time.Thursday:
		return "周四"
	case time.Friday:
		return "周五"
	case time.Saturday:
		return "周六"
	default:
		return "周日"
	}
}

func invitePeriodLabel(period string) string {
	switch period {
	case "morning":
		return "上午"
	case "afternoon":
		return "下午"
	case "night":
		return "夜间"
	default:
		return "晚上"
	}
}
