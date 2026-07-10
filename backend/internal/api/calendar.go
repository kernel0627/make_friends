package api

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"make_friends/backend/internal/model"
)

const (
	calendarMaxDays     = 30
	calendarDefaultDays = 30
)

type calendarDayView struct {
	Date          string  `json:"date"`
	ActivityCount int     `json:"activityCount"`
	Score         float64 `json:"score"`
	FireLevel     int     `json:"fireLevel"`
	FireText      string  `json:"fireText"`
	Highlighted   bool    `json:"highlighted"`
	Reason        string  `json:"reason"`
}

type calendarPostScore struct {
	Post  model.Post
	Score float64
}

type calendarQuery struct {
	Raw            string
	Category       string
	SubCategory    string
	AddressKeyword string
	Period         string
	HasIntent      bool
	UserLat        float64
	UserLng        float64
	HasUserCoords  bool
}

type calendarBehaviorProfile struct {
	ActiveWeekdays      []calendarWeightedWeekday
	JoinWeekdays        []calendarWeightedWeekday
	ActivePeriods       []calendarWeightedPeriod
	CategoryWeights     map[string]float64
	SubCategoryWeights  map[string]float64
	PreferredLocations  []calendarWeightedLocation
	InviteAcceptRate    float64
	ReliabilityScore    float64
	ExplorationScore    float64
	WeeklyActiveScore   float64
	WeeklyParticipation float64
}

type calendarWeightedWeekday struct {
	Weekday int     `json:"weekday"`
	Weight  float64 `json:"weight"`
}

type calendarWeightedPeriod struct {
	Period string  `json:"period"`
	Weight float64 `json:"weight"`
}

type calendarWeightedLocation struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Weight  float64 `json:"weight"`
}

var calendarActivityAliases = []struct {
	Category    string
	SubCategory string
	Keywords    []string
}{
	{Category: "运动", SubCategory: "羽毛球", Keywords: []string{"羽毛球", "羽球", "打球"}},
	{Category: "运动", SubCategory: "篮球", Keywords: []string{"篮球", "半场", "全场"}},
	{Category: "运动", SubCategory: "跑步", Keywords: []string{"跑步", "夜跑", "慢跑"}},
	{Category: "运动", SubCategory: "骑行", Keywords: []string{"骑行", "单车", "自行车"}},
	{Category: "娱乐", SubCategory: "电影", Keywords: []string{"电影", "看电影", "新片"}},
	{Category: "娱乐", SubCategory: "桌游", Keywords: []string{"桌游", "剧本杀", "狼人杀", "卡牌"}},
	{Category: "娱乐", SubCategory: "KTV", Keywords: []string{"ktv", "KTV", "唱歌", "k歌", "K歌"}},
	{Category: "娱乐", SubCategory: "摄影", Keywords: []string{"摄影", "拍照", "写真"}},
	{Category: "学习", SubCategory: "自习", Keywords: []string{"自习", "学习", "复习"}},
	{Category: "学习", SubCategory: "读书", Keywords: []string{"读书", "阅读"}},
	{Category: "学习", SubCategory: "编程", Keywords: []string{"编程", "代码", "项目"}},
	{Category: "学习", SubCategory: "英语", Keywords: []string{"英语", "口语"}},
	{Category: "其他", SubCategory: "探店", Keywords: []string{"探店", "吃饭", "咖啡", "打卡"}},
	{Category: "其他", SubCategory: "宠物", Keywords: []string{"宠物", "猫", "狗", "遛狗"}},
	{Category: "其他", SubCategory: "志愿", Keywords: []string{"志愿", "公益"}},
	{Category: "其他", SubCategory: "逛展", Keywords: []string{"逛展", "展览", "看展"}},
}

func (s *Server) CalendarActivityHeatmap(c *gin.Context) {
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	startDate, days := parseCalendarRange(c)
	endDate := startDate.AddDate(0, 0, days-1)
	query := parseCalendarQuery(c)
	profile := s.loadCalendarBehaviorProfile(viewerID)

	posts, err := s.queryCalendarCandidatePosts(startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query calendar posts failed"})
		return
	}

	dayViews := make([]calendarDayView, 0, days)
	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i)
		scored := scoreCalendarPostsForDate(posts, date, query, profile)
		dayViews = append(dayViews, buildCalendarDayView(date, scored))
	}
	applyCalendarHeatNormalization(dayViews)

	c.JSON(http.StatusOK, gin.H{
		"startDate": startDate.Format("2006-01-02"),
		"endDate":   endDate.Format("2006-01-02"),
		"days":      dayViews,
	})
}

func (s *Server) CalendarActivityPosts(c *gin.Context) {
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	date, ok := parseCalendarDate(c.Query("date"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required"})
		return
	}
	query := parseCalendarQuery(c)
	profile := s.loadCalendarBehaviorProfile(viewerID)

	posts, err := s.queryCalendarCandidatePosts(date, date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query calendar posts failed"})
		return
	}
	scored := scoreCalendarPostsForDate(posts, date, query, profile)
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Post.CreatedAt > scored[j].Post.CreatedAt
		}
		return scored[i].Score > scored[j].Score
	})

	limit := queryIntOrDefault(c.Query("limit"), 20)
	if limit <= 0 || limit > 30 {
		limit = 20
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}

	resultPosts := make([]model.Post, 0, len(scored))
	scoreByPostID := make(map[string]float64, len(scored))
	for _, item := range scored {
		resultPosts = append(resultPosts, item.Post)
		scoreByPostID[item.Post.ID] = item.Score
	}

	views, err := s.buildPostViewsForViewer(resultPosts, viewerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post author failed"})
		return
	}
	for i := range views {
		scoreValue := scoreByPostID[views[i].ID]
		views[i].Recommendation = recommendationView{
			Strategy: "calendar_activity",
			Score:    roundCalendarScore(scoreValue),
			Reason:   calendarPostReason(views[i].Post, scoreValue),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"date":  date.Format("2006-01-02"),
		"posts": views,
	})
}

func parseCalendarRange(c *gin.Context) (time.Time, int) {
	startDate, ok := parseCalendarDate(c.Query("startDate"))
	if !ok {
		now := time.Now()
		startDate = dateOnly(now)
	}
	days := queryIntOrDefault(c.Query("days"), calendarDefaultDays)
	if days <= 0 {
		days = calendarDefaultDays
	}
	if days > calendarMaxDays {
		days = calendarMaxDays
	}
	return startDate, days
}

func parseCalendarDate(raw string) (time.Time, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}, false
	}
	date, err := time.ParseInLocation("2006-01-02", text, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return dateOnly(date), true
}

func dateOnly(value time.Time) time.Time {
	local := value.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
}

type calendarTextIntent struct {
	Category       string
	SubCategory    string
	AddressKeyword string
	Period         string
	HasActivity    bool
}

func parseCalendarQuery(c *gin.Context) calendarQuery {
	raw := strings.TrimSpace(firstNonEmpty(c.Query("query"), c.Query("keyword"), c.Query("q")))
	intent := parseCalendarTextIntent(raw)
	query := calendarQuery{Raw: raw}
	query.Period = normalizeCalendarPeriod(firstNonEmpty(c.Query("period"), intent.Period))
	query.AddressKeyword = strings.TrimSpace(c.Query("addressKeyword"))
	query.Category = strings.TrimSpace(c.Query("category"))
	query.SubCategory = strings.TrimSpace(c.Query("subCategory"))

	if query.Category == "" && intent.Category != "" {
		query.Category = intent.Category
	}
	if query.SubCategory == "" && intent.SubCategory != "" {
		query.SubCategory = intent.SubCategory
	}
	if query.AddressKeyword == "" {
		query.AddressKeyword = intent.AddressKeyword
	}
	if point := queryGeoPoint(c); point != nil {
		query.UserLat = point.Latitude
		query.UserLng = point.Longitude
		query.HasUserCoords = true
	}
	query.HasIntent = query.Raw != "" || query.Category != "" || query.SubCategory != "" || query.AddressKeyword != "" || query.Period != ""
	return query
}

func parseCalendarTextIntent(raw string) calendarTextIntent {
	raw = strings.TrimSpace(raw)
	text := normalizeCalendarText(raw)
	if text == "" {
		return calendarTextIntent{}
	}

	intent := calendarTextIntent{
		Period: normalizeCalendarPeriod(detectCalendarPeriod(text)),
	}

	for _, alias := range calendarActivityAliases {
		for _, keyword := range alias.Keywords {
			if strings.Contains(text, normalizeCalendarText(keyword)) {
				intent.Category = alias.Category
				intent.SubCategory = alias.SubCategory
				intent.HasActivity = true
				break
			}
		}
		if intent.HasActivity {
			break
		}
	}
	if intent.Category == "" {
		switch {
		case strings.Contains(text, "运动"):
			intent.Category = "运动"
			intent.HasActivity = true
		case strings.Contains(text, "娱乐"):
			intent.Category = "娱乐"
			intent.HasActivity = true
		case strings.Contains(text, "学习"):
			intent.Category = "学习"
			intent.HasActivity = true
		case strings.Contains(text, "其他"):
			intent.Category = "其他"
			intent.HasActivity = true
		}
	}

	addressQuery := calendarQuery{
		Raw:         raw,
		Category:    intent.Category,
		SubCategory: intent.SubCategory,
		Period:      intent.Period,
	}
	intent.AddressKeyword = detectCalendarAddressKeyword(raw)
	if intent.AddressKeyword == "" {
		intent.AddressKeyword = inferCalendarAddressKeyword(raw, addressQuery)
	}
	return intent
}

func (s *Server) queryCalendarCandidatePosts(startDate, endDate time.Time) ([]model.Post, error) {
	var posts []model.Post
	now := time.Now()
	nowDate := dateOnly(now)
	maxRangeDays := int(endDate.Sub(nowDate).Hours()/24) + 1
	if maxRangeDays < 1 {
		maxRangeDays = 1
	}
	err := activePostsQuery(s.DB.Model(&model.Post{})).
		Where("status = ? AND cancelled_at = 0", "open").
		Where("(time_mode = ? AND time_days >= ?) OR (time_mode = ? AND fixed_time <> '')", "range", 1, "fixed").
		Find(&posts).Error
	if err != nil {
		return nil, err
	}

	filtered := make([]model.Post, 0, len(posts))
	for _, post := range posts {
		if post.TimeMode == "range" && post.TimeDays > maxRangeDays+7 {
			filtered = append(filtered, post)
			continue
		}
		if postMayTouchCalendarRange(post, startDate, endDate, nowDate) {
			filtered = append(filtered, post)
		}
	}
	return filtered, nil
}

func postMayTouchCalendarRange(post model.Post, startDate, endDate, nowDate time.Time) bool {
	if post.TimeMode == "fixed" {
		ts, err := parseFixedTimeToMS(post.FixedTime)
		if err != nil {
			return false
		}
		date := dateOnly(time.UnixMilli(ts))
		return !date.Before(startDate) && !date.After(endDate)
	}
	days := post.TimeDays
	if days <= 0 {
		days = 7
	}
	lastDate := nowDate.AddDate(0, 0, days-1)
	return !lastDate.Before(startDate) && !nowDate.After(endDate)
}

func scoreCalendarPostsForDate(posts []model.Post, date time.Time, query calendarQuery, profile calendarBehaviorProfile) []calendarPostScore {
	out := make([]calendarPostScore, 0, len(posts))
	for _, post := range posts {
		if !postMatchesCalendarDate(post, date) {
			continue
		}
		if !postMatchesCalendarQuery(post, query) {
			continue
		}
		scoreValue := scoreCalendarPost(post, date, query, profile)
		out = append(out, calendarPostScore{Post: post, Score: scoreValue})
	}
	return out
}

func postMatchesCalendarDate(post model.Post, date time.Time) bool {
	if post.TimeMode == "fixed" {
		ts, err := parseFixedTimeToMS(post.FixedTime)
		if err != nil {
			return false
		}
		return dateOnly(time.UnixMilli(ts)).Equal(date)
	}
	days := post.TimeDays
	if days <= 0 {
		days = 7
	}
	nowDate := dateOnly(time.Now())
	return !date.Before(nowDate) && !date.After(nowDate.AddDate(0, 0, days-1))
}

func postMatchesCalendarQuery(post model.Post, query calendarQuery) bool {
	if query.Category != "" && post.Category != query.Category {
		return false
	}
	if query.SubCategory != "" && post.SubCategory != query.SubCategory {
		return false
	}
	if query.AddressKeyword != "" && !strings.Contains(normalizeCalendarText(post.Address), normalizeCalendarText(query.AddressKeyword)) {
		return false
	}
	if query.Raw != "" && query.Category == "" && query.SubCategory == "" && query.AddressKeyword == "" && query.Period == "" {
		haystack := normalizeCalendarText(post.Title + " " + post.Description + " " + post.Address + " " + post.Category + " " + post.SubCategory)
		if !strings.Contains(haystack, normalizeCalendarText(query.Raw)) {
			return false
		}
	}
	return true
}

func scoreCalendarPost(post model.Post, date time.Time, query calendarQuery, profile calendarBehaviorProfile) float64 {
	scoreValue := 0.25
	if query.HasIntent {
		scoreValue += 0.18
	}
	scoreValue += math.Min(float64(post.CurrentCount)/math.Max(float64(post.MaxCount), 1), 1) * 0.18
	scoreValue += profileCategoryScore(post, profile, query) * 0.32
	scoreValue += profileWeekdayScore(date, profile) * 0.18
	scoreValue += profilePeriodScore(postPeriod(post), profile, query) * 0.18
	scoreValue += locationCalendarScore(post, query, profile) * 0.18
	scoreValue += math.Min(profile.WeeklyParticipation, 1) * 0.08
	if post.TimeMode == "fixed" {
		scoreValue += 0.08
	}
	if post.CurrentCount >= post.MaxCount {
		scoreValue -= 0.3
	}
	return scoreValue
}

func profileCategoryScore(post model.Post, profile calendarBehaviorProfile, query calendarQuery) float64 {
	if query.Category != "" && post.Category == query.Category {
		if query.SubCategory != "" && post.SubCategory == query.SubCategory {
			return 1
		}
		return 0.82
	}
	if query.Category != "" {
		return 0
	}
	key := post.Category + "/" + post.SubCategory
	if v := profile.SubCategoryWeights[key]; v > 0 {
		return v
	}
	if v := profile.CategoryWeights[post.Category]; v > 0 {
		return v
	}
	return 0.36 + profile.ExplorationScore*0.2
}

func profileWeekdayScore(date time.Time, profile calendarBehaviorProfile) float64 {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return maxWeekdayWeight(profile.JoinWeekdays, weekday, maxWeekdayWeight(profile.ActiveWeekdays, weekday, 0.38))
}

func maxWeekdayWeight(list []calendarWeightedWeekday, weekday int, fallback float64) float64 {
	best := fallback
	for _, item := range list {
		if item.Weekday == weekday && item.Weight > best {
			best = item.Weight
		}
	}
	return best
}

func profilePeriodScore(period string, profile calendarBehaviorProfile, query calendarQuery) float64 {
	if query.Period != "" {
		if period == "" || period == query.Period {
			return 1
		}
		return 0.18
	}
	if period == "" {
		return 0.42
	}
	best := 0.3
	for _, item := range profile.ActivePeriods {
		if item.Period == period && item.Weight > best {
			best = item.Weight
		}
	}
	return best
}

func locationCalendarScore(post model.Post, query calendarQuery, profile calendarBehaviorProfile) float64 {
	if query.HasUserCoords && hasPostCoords(post) {
		return distanceToScore(geoDistanceKm(query.UserLat, query.UserLng, post.Lat, post.Lng))
	}
	best := 0.34
	if !hasPostCoords(post) {
		return best
	}
	for _, loc := range profile.PreferredLocations {
		if loc.Lat == 0 && loc.Lng == 0 {
			continue
		}
		scoreValue := distanceToScore(geoDistanceKm(loc.Lat, loc.Lng, post.Lat, post.Lng)) * math.Max(loc.Weight, 0.2)
		if scoreValue > best {
			best = scoreValue
		}
	}
	return best
}

func hasPostCoords(post model.Post) bool {
	return post.Lat != 0 || post.Lng != 0
}

func distanceToScore(km float64) float64 {
	switch {
	case km <= 3:
		return 1
	case km <= 10:
		return 0.82
	case km <= 30:
		return 0.56
	case km <= 80:
		return 0.32
	default:
		return 0.14
	}
}

func buildCalendarDayView(date time.Time, scored []calendarPostScore) calendarDayView {
	total := 0.0
	for _, item := range scored {
		total += item.Score
	}
	activityCount := len(scored)
	avg := 0.0
	if activityCount > 0 {
		avg = total / float64(activityCount)
	}
	return calendarDayView{
		Date:          date.Format("2006-01-02"),
		ActivityCount: activityCount,
		Score:         roundCalendarScore(avg),
	}
}

func applyCalendarHeatNormalization(days []calendarDayView) {
	activeTotal := 0
	activeDays := 0
	for _, day := range days {
		if day.ActivityCount <= 0 {
			continue
		}
		activeTotal += day.ActivityCount
		activeDays++
	}

	activeAverage := 0.0
	if activeDays > 0 {
		activeAverage = float64(activeTotal) / float64(activeDays)
	}

	for i := range days {
		level := calendarFireLevel(days[i].ActivityCount, activeAverage)
		days[i].FireLevel = level
		days[i].FireText = strings.Repeat("\U0001F525", level)
		days[i].Highlighted = level == 3
		days[i].Reason = calendarDayReason(days[i].ActivityCount, level)
	}
}

func calendarFireLevel(count int, activeAverage float64) int {
	if count <= 0 {
		return 0
	}
	if activeAverage <= 0 {
		return 1
	}
	ratio := float64(count) / activeAverage
	switch {
	case ratio >= 1.35:
		return 3
	case ratio >= 0.75:
		return 2
	default:
		return 1
	}
}

func calendarDayReason(count int, level int) string {
	if count == 0 {
		return "\u6682\u65e0\u5339\u914d\u6d3b\u52a8"
	}
	if level >= 3 {
		return "\u660e\u663e\u9ad8\u4e8e\u8fd1\u671f\u5e73\u5747"
	}
	if level == 2 {
		return "\u63a5\u8fd1\u8fd1\u671f\u5e73\u5747"
	}
	return "\u6709\u5c11\u91cf\u5339\u914d\u6d3b\u52a8"
}

func calendarPostReason(post model.Post, scoreValue float64) string {
	if scoreValue >= 1.2 {
		return "与你的兴趣、时间或地点偏好较匹配"
	}
	if post.TimeMode == "fixed" {
		return "这天有明确时间的活动"
	}
	return "这天可参加的活动"
}

func (s *Server) loadCalendarBehaviorProfile(userID string) calendarBehaviorProfile {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return calendarBehaviorProfile{}
	}
	var row model.UserBehaviorProfile
	if err := s.DB.First(&row, "user_id = ?", userID).Error; err != nil {
		return calendarBehaviorProfile{}
	}
	out := calendarBehaviorProfile{
		InviteAcceptRate:    row.InviteAcceptRate,
		ReliabilityScore:    row.ReliabilityScore,
		ExplorationScore:    row.ExplorationScore,
		WeeklyActiveScore:   row.WeeklyActiveScore,
		WeeklyParticipation: row.WeeklyParticipationScore,
	}
	_ = json.Unmarshal([]byte(row.ActiveWeekdaysJSON), &out.ActiveWeekdays)
	_ = json.Unmarshal([]byte(row.JoinWeekdaysJSON), &out.JoinWeekdays)
	_ = json.Unmarshal([]byte(row.ActivePeriodsJSON), &out.ActivePeriods)
	_ = json.Unmarshal([]byte(row.CategoryWeightsJSON), &out.CategoryWeights)
	_ = json.Unmarshal([]byte(row.SubCategoryWeightsJSON), &out.SubCategoryWeights)
	_ = json.Unmarshal([]byte(row.PreferredLocationsJSON), &out.PreferredLocations)
	if out.CategoryWeights == nil {
		out.CategoryWeights = map[string]float64{}
	}
	if out.SubCategoryWeights == nil {
		out.SubCategoryWeights = map[string]float64{}
	}
	return out
}

func postPeriod(post model.Post) string {
	if post.TimeMode != "fixed" || strings.TrimSpace(post.FixedTime) == "" {
		return ""
	}
	ts, err := parseFixedTimeToMS(post.FixedTime)
	if err != nil {
		return ""
	}
	hour := time.UnixMilli(ts).In(time.Local).Hour()
	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 18:
		return "afternoon"
	case hour >= 18 && hour < 23:
		return "evening"
	default:
		return "night"
	}
}

func normalizeCalendarPeriod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "morning", "上午", "早上", "早晨":
		return "morning"
	case "afternoon", "下午", "午后":
		return "afternoon"
	case "evening", "晚上", "晚间", "今晚":
		return "evening"
	case "night", "夜间", "夜里", "深夜":
		return "night"
	default:
		return ""
	}
}

func detectCalendarPeriod(text string) string {
	switch {
	case strings.Contains(text, "早上") || strings.Contains(text, "上午") || strings.Contains(text, "早晨"):
		return "morning"
	case strings.Contains(text, "下午") || strings.Contains(text, "午后"):
		return "afternoon"
	case strings.Contains(text, "晚上") || strings.Contains(text, "今晚") || strings.Contains(text, "晚间"):
		return "evening"
	case strings.Contains(text, "夜间") || strings.Contains(text, "深夜") || strings.Contains(text, "夜里"):
		return "night"
	default:
		return ""
	}
}

func detectCalendarAddressKeyword(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	markers := []string{"在", "去", "到"}
	for _, marker := range markers {
		idx := strings.LastIndex(text, marker)
		if idx < 0 || idx+len(marker) >= len(text) {
			continue
		}
		candidate := strings.TrimSpace(text[idx+len(marker):])
		for _, alias := range calendarActivityAliases {
			for _, keyword := range alias.Keywords {
				candidate = strings.ReplaceAll(candidate, keyword, "")
			}
		}
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && len([]rune(candidate)) <= 16 {
			return candidate
		}
	}
	return ""
}

func inferCalendarAddressKeyword(raw string, query calendarQuery) string {
	candidate := normalizeCalendarText(raw)
	if candidate == "" {
		return ""
	}
	for _, alias := range calendarActivityAliases {
		candidate = strings.ReplaceAll(candidate, normalizeCalendarText(alias.Category), "")
		candidate = strings.ReplaceAll(candidate, normalizeCalendarText(alias.SubCategory), "")
		for _, keyword := range alias.Keywords {
			candidate = strings.ReplaceAll(candidate, normalizeCalendarText(keyword), "")
		}
	}
	for _, token := range calendarQueryNoiseTokens(query) {
		candidate = strings.ReplaceAll(candidate, normalizeCalendarText(token), "")
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	runes := []rune(candidate)
	if len(runes) < 2 || len(runes) > 18 {
		return ""
	}
	return candidate
}

func calendarQueryNoiseTokens(query calendarQuery) []string {
	tokens := []string{
		"上午", "早上", "早晨", "下午", "午后", "晚上", "晚间", "今晚", "夜间", "夜里", "深夜",
		"morning", "afternoon", "evening", "night",
		"今天", "明天", "后天", "大后天", "周一", "周二", "周三", "周四", "周五", "周六", "周日", "周天",
		"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日", "星期天",
		"想", "找", "参加", "报名", "活动", "一起", "附近", "有", "吗", "的", "在", "去", "到",
	}
	if query.Category != "" {
		tokens = append(tokens, query.Category)
	}
	if query.SubCategory != "" {
		tokens = append(tokens, query.SubCategory)
	}
	return tokens
}

func normalizeCalendarText(raw string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func roundCalendarScore(value float64) float64 {
	return math.Round(value*100) / 100
}
