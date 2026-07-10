package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/recommend"
)

const (
	recommendationEventsStream = "zgbe:rec:events"
	recommendationJobsStream   = "zgbe:rec:jobs"
)

type interestTagView struct {
	Type   string  `json:"type"`
	Value  string  `json:"value"`
	Weight float64 `json:"weight"`
}

type recommendationView struct {
	Strategy    string   `json:"strategy"`
	Score       float64  `json:"score"`
	Reason      string   `json:"reason"`
	MatchedTags []string `json:"matchedTags"`
}

type feedExposureItemReq struct {
	PostID   string  `json:"postId"`
	Position int     `json:"position"`
	Strategy string  `json:"strategy"`
	Score    float64 `json:"score"`
}

type feedExposureReq struct {
	FeedRequestID string                `json:"feedRequestId"`
	SessionID     string                `json:"sessionId"`
	Items         []feedExposureItemReq `json:"items"`
}

type feedClickReq struct {
	FeedRequestID string  `json:"feedRequestId"`
	SessionID     string  `json:"sessionId"`
	PostID        string  `json:"postId"`
	Position      int     `json:"position"`
	Strategy      string  `json:"strategy"`
	Score         float64 `json:"score"`
}

type rankingModel struct {
	Intercept float64
	Weights   map[string]float64
	Trained   bool
}

type viewerTagWeights struct {
	Category    map[string]float64
	SubCategory map[string]float64
	City        map[string]float64
	TopValues   []string
}

type viewerSubcategoryStats struct {
	Clicked   map[string]float64
	Unclicked map[string]float64
}

type postSignal struct {
	Post         model.Post
	Score        float64
	ExploreScore float64
	Recommend    recommendationView
}

type geoPoint struct {
	Latitude  float64
	Longitude float64
}

func (s *Server) ReportRecommendationExposures(c *gin.Context) {
	var req feedExposureReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.FeedRequestID) == "" || len(req.Items) == 0 {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "feedRequestId and items are required")
		return
	}

	now := time.Now().UnixMilli()
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	sessionID := normalizedSessionID(req.SessionID)
	rows := make([]model.FeedExposure, 0, len(req.Items))
	for _, item := range req.Items {
		postID := strings.TrimSpace(item.PostID)
		if postID == "" {
			continue
		}
		rows = append(rows, model.FeedExposure{
			RequestID: req.FeedRequestID,
			UserID:    viewerID,
			PostID:    postID,
			Position:  item.Position,
			Strategy:  strings.TrimSpace(item.Strategy),
			Score:     item.Score,
			SessionID: sessionID,
			CreatedAt: now,
		})
	}
	if len(rows) == 0 {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "no valid exposure items")
		return
	}
	if err := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "SAVE_EXPOSURES_FAILED", "save exposures failed")
		return
	}
	if s.UseRedis && s.RedisClient != nil {
		for _, row := range rows {
			s.pushRecommendationEvent(c.Request.Context(), "feed_exposure", map[string]any{
				"requestId": row.RequestID,
				"userId":    row.UserID,
				"postId":    row.PostID,
				"position":  row.Position,
				"strategy":  row.Strategy,
				"score":     row.Score,
				"sessionId": row.SessionID,
				"createdAt": row.CreatedAt,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(rows)})
}

func (s *Server) ReportRecommendationClick(c *gin.Context) {
	var req feedClickReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.FeedRequestID) == "" || strings.TrimSpace(req.PostID) == "" {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "feedRequestId and postId are required")
		return
	}

	now := time.Now().UnixMilli()
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	row := model.FeedClick{
		RequestID: req.FeedRequestID,
		UserID:    viewerID,
		PostID:    strings.TrimSpace(req.PostID),
		Position:  req.Position,
		Strategy:  strings.TrimSpace(req.Strategy),
		Score:     req.Score,
		SessionID: normalizedSessionID(req.SessionID),
		CreatedAt: now,
	}
	if err := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		fail(c, http.StatusInternalServerError, "SAVE_CLICK_FAILED", "save click failed")
		return
	}
	if viewerID != "" {
		_ = recommend.RebuildUserTags(s.DB, []string{viewerID}, now)
	}
	if s.UseRedis && s.RedisClient != nil {
		s.pushRecommendationEvent(c.Request.Context(), "feed_click", map[string]any{
			"requestId": row.RequestID,
			"userId":    row.UserID,
			"postId":    row.PostID,
			"position":  row.Position,
			"strategy":  row.Strategy,
			"score":     row.Score,
			"sessionId": row.SessionID,
			"createdAt": row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) TriggerRecommendationRebuild(c *gin.Context) {
	now := time.Now().UnixMilli()
	if err := recommend.EnsureDefaultRecommendationModel(s.DB, now); err != nil {
		fail(c, http.StatusInternalServerError, "MODEL_INIT_FAILED", "init recommendation model failed")
		return
	}
	var users []model.User
	if err := activeUsersQuery(s.DB).Find(&users).Error; err != nil {
		fail(c, http.StatusInternalServerError, "QUERY_USERS_FAILED", "query users failed")
		return
	}
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	if err := recommend.RebuildUserTags(s.DB, userIDs, now); err != nil {
		fail(c, http.StatusInternalServerError, "TAG_REBUILD_FAILED", "rebuild user tags failed")
		return
	}
	if s.UseRedis && s.RedisClient != nil {
		s.pushRecommendationJob(c.Request.Context(), "rebuild_all_embeddings", map[string]any{"requestedAt": now})
		s.pushRecommendationJob(c.Request.Context(), "train_ranking_model", map[string]any{"requestedAt": now})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "queued": s.UseRedis, "userCount": len(userIDs)})
}

func (s *Server) interestTagsForUser(userID string, limit int) ([]interestTagView, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || limit <= 0 {
		return []interestTagView{}, nil
	}
	var rows []model.UserTag
	if err := s.DB.Where("user_id = ?", userID).
		Order("weight DESC").
		Order("last_event_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]interestTagView, 0, len(rows))
	for _, row := range rows {
		result = append(result, interestTagView{
			Type:   row.TagType,
			Value:  row.TagValue,
			Weight: round2(row.Weight),
		})
	}
	return result, nil
}

func (s *Server) pushRecommendationEvent(ctx context.Context, eventType string, payload map[string]any) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	values := map[string]any{
		"type": eventType,
	}
	for key, value := range payload {
		values[key] = value
	}
	if err := s.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: recommendationEventsStream,
		Values: values,
	}).Err(); err != nil {
		// keep recommendation side effects best-effort
	}
}

func (s *Server) pushRecommendationJob(ctx context.Context, jobType string, payload map[string]any) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	values := map[string]any{
		"type": jobType,
	}
	for key, value := range payload {
		values[key] = value
	}
	if err := s.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: recommendationJobsStream,
		Values: values,
	}).Err(); err != nil {
	}
}

func (s *Server) rebuildUserTagsForUsers(userIDs []string) {
	now := time.Now().UnixMilli()
	_ = recommend.EnsureDefaultRecommendationModel(s.DB, now)
	_ = recommend.RebuildUserTags(s.DB, userIDs, now)
}

func normalizedSessionID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "session_" + strings.ToLower(uuid.NewString()[:12])
	}
	return value
}

func queryGeoPoint(c *gin.Context) *geoPoint {
	lat, latOK := queryFloat(c.Query("userLat"))
	lng, lngOK := queryFloat(c.Query("userLng"))
	if !latOK {
		lat, latOK = queryFloat(c.Query("latitude"))
	}
	if !lngOK {
		lng, lngOK = queryFloat(c.Query("longitude"))
	}
	if !latOK || !lngOK || !validGeoPoint(lat, lng) {
		return nil
	}
	return &geoPoint{Latitude: lat, Longitude: lng}
}

func queryFloat(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func validGeoPoint(latitude, longitude float64) bool {
	return latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func usablePostGeoPoint(latitude, longitude float64) bool {
	return validGeoPoint(latitude, longitude) && !(latitude == 0 && longitude == 0)
}

func (s *Server) loadRankingModel() rankingModel {
	result := rankingModel{
		Intercept: 0,
		Weights:   recommend.DefaultRankingWeights(),
		Trained:   false,
	}

	var row model.RecommendationModel
	if err := s.DB.First(&row, "model_key = ?", recommend.ModelKeyLRRanker).Error; err != nil {
		return result
	}

	var featureWeights map[string]float64
	if err := json.Unmarshal([]byte(row.FeatureJSON), &featureWeights); err == nil && len(featureWeights) > 0 {
		result.Weights = recommend.DefaultRankingWeights()
		for name, weight := range featureWeights {
			result.Weights[name] = weight
		}
	}
	result.Intercept = row.Intercept

	var stats map[string]any
	if err := json.Unmarshal([]byte(row.TrainingStats), &stats); err == nil {
		exposureCount := int64(valueAsFloat(stats["exposureCount"]))
		clickCount := int64(valueAsFloat(stats["clickCount"]))
		fallback := valueAsBool(stats["fallback"])
		result.Trained = !fallback && exposureCount >= 1000 && clickCount >= 100
		if fallback {
			result.Intercept = 0
			result.Weights = recommend.DefaultRankingWeights()
		}
	}
	return result
}

func (s *Server) buildRecommendedFeed(posts []model.Post, viewerID string, viewerLocation *geoPoint, nowMS int64) ([]model.Post, map[string]recommendationView, string, error) {
	if len(posts) == 0 {
		return []model.Post{}, map[string]recommendationView{}, "", nil
	}

	viewerTags, _ := s.loadViewerTagWeights(viewerID)
	viewerStats, _ := s.loadViewerSubcategoryStats(viewerID, nowMS)
	viewerEmbedding, _ := s.loadUserEmbedding(viewerID)
	postEmbeddings, _ := s.loadPostEmbeddings(posts)
	modelWeights := s.loadRankingModel()

	authorIDs := make([]string, 0, len(posts))
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		authorIDs = append(authorIDs, post.AuthorID)
		postIDs = append(postIDs, post.ID)
	}
	authors, err := s.usersByIDs(authorIDs)
	if err != nil {
		return nil, nil, "", err
	}
	chatCounts, err := s.countChatMessagesByPost(postIDs)
	if err != nil {
		return nil, nil, "", err
	}
	reviewCounts, err := s.countReviewsByPost(postIDs)
	if err != nil {
		return nil, nil, "", err
	}
	exposureCounts, err := s.countExposuresByPost(postIDs)
	if err != nil {
		return nil, nil, "", err
	}
	activityCounts, err := s.countActivityScoresByUser(authorIDs)
	if err != nil {
		return nil, nil, "", err
	}

	personalized := make([]postSignal, 0, len(posts))
	exploration := make([]postSignal, 0, len(posts))
	tail := make([]postSignal, 0, len(posts))

	for _, post := range posts {
		signal := s.scorePostSignal(post, authors[post.AuthorID], viewerID, viewerTags, viewerStats, viewerEmbedding, postEmbeddings[post.ID], chatCounts[post.ID], reviewCounts[post.ID], exposureCounts[post.ID], activityCounts[post.AuthorID], modelWeights, viewerLocation, nowMS)
		if strings.TrimSpace(viewerID) != "" && post.AuthorID == viewerID {
			signal.Recommend.Strategy = "self_tail"
			tail = append(tail, signal)
			continue
		}
		if post.Status == "closed" || post.CurrentCount >= post.MaxCount {
			signal.Recommend.Strategy = "closed_tail"
			tail = append(tail, signal)
			continue
		}
		personalized = append(personalized, signal)
		if signal.ExploreScore > 0 {
			exploration = append(exploration, signal)
		}
	}

	sort.SliceStable(personalized, func(i, j int) bool {
		if personalized[i].Score == personalized[j].Score {
			return personalized[i].Post.CreatedAt > personalized[j].Post.CreatedAt
		}
		return personalized[i].Score > personalized[j].Score
	})
	sort.SliceStable(exploration, func(i, j int) bool {
		if exploration[i].ExploreScore == exploration[j].ExploreScore {
			return exploration[i].Post.CreatedAt > exploration[j].Post.CreatedAt
		}
		return exploration[i].ExploreScore > exploration[j].ExploreScore
	})
	sort.SliceStable(tail, func(i, j int) bool {
		if tail[i].Post.Status != tail[j].Post.Status {
			return tail[i].Post.Status != "closed"
		}
		return tail[i].Post.CreatedAt > tail[j].Post.CreatedAt
	})

	ordered := make([]model.Post, 0, len(posts))
	recommendationMap := make(map[string]recommendationView, len(posts))
	used := make(map[string]struct{}, len(posts))
	appendSignal := func(signal postSignal, strategy string) {
		if _, ok := used[signal.Post.ID]; ok {
			return
		}
		used[signal.Post.ID] = struct{}{}
		signal.Recommend.Strategy = strategy
		recommendationMap[signal.Post.ID] = signal.Recommend
		ordered = append(ordered, signal.Post)
	}

	if strings.TrimSpace(viewerID) != "" {
		pIdx := 0
		eIdx := 0
		for len(ordered) < len(posts) && (pIdx < len(personalized) || eIdx < len(exploration)) {
			for i := 0; i < 2 && pIdx < len(personalized); i++ {
				appendSignal(personalized[pIdx], "personalized")
				pIdx++
			}
			if eIdx < len(exploration) {
				appendSignal(exploration[eIdx], "exploration")
				eIdx++
			}
			if pIdx >= len(personalized) && eIdx >= len(exploration) {
				break
			}
		}
		for pIdx < len(personalized) {
			appendSignal(personalized[pIdx], "personalized")
			pIdx++
		}
		for eIdx < len(exploration) {
			appendSignal(exploration[eIdx], "exploration")
			eIdx++
		}
	} else {
		for _, signal := range personalized {
			appendSignal(signal, "global")
		}
	}
	for _, signal := range tail {
		appendSignal(signal, signal.Recommend.Strategy)
	}

	return ordered, recommendationMap, "feed_" + strings.ToLower(uuid.NewString()[:12]), nil
}

func (s *Server) scorePostSignal(
	post model.Post,
	author model.User,
	viewerID string,
	viewerTags viewerTagWeights,
	viewerStats viewerSubcategoryStats,
	viewerEmbedding []float64,
	postEmbedding []float64,
	chatCount int64,
	reviewCount int64,
	exposureCount int64,
	authorActivityCount int64,
	modelWeights rankingModel,
	viewerLocation *geoPoint,
	nowMS int64,
) postSignal {
	categoryMatch, subCategoryMatch, cityMatch, matchedTags := viewerTags.match(post)
	embeddingCosine := cosineSimilarity(viewerEmbedding, postEmbedding)
	postAgeHours := math.Max(0, float64(nowMS-post.CreatedAt)/float64(time.Hour/time.Millisecond))
	fixedDistanceHours := fixedTimeDistanceHours(post, nowMS)
	freshness := freshnessScore(post, nowMS)
	joinability := joinabilityScore(post)
	authorQuality := authorQualityScore(author, authorActivityCount)
	interactionHeat := saturate(float64(post.CurrentCount) + 0.3*float64(chatCount) + 0.5*float64(reviewCount))
	locationProximity := locationProximityScore(viewerLocation, post)
	clickedSameSub := viewerStats.Clicked[strings.TrimSpace(post.SubCategory)]
	unclickedSameSub := viewerStats.Unclicked[strings.TrimSpace(post.SubCategory)]

	features := map[string]float64{
		"embedding_cosine":                     embeddingCosine,
		"category_match":                       categoryMatch,
		"sub_category_match":                   subCategoryMatch,
		"city_match":                           cityMatch,
		"location_proximity":                   locationProximity,
		"author_quality":                       authorQuality,
		"author_rating_score":                  clamp(author.RatingScore/5, 0, 1),
		"author_credit_score":                  clamp(float64(author.CreditScore)/100, 0, 1),
		"author_activity_score_count":          clamp(float64(authorActivityCount)/6, 0, 1),
		"post_current_count":                   clamp(float64(post.CurrentCount)/10, 0, 1),
		"post_chat_count":                      clamp(float64(chatCount)/10, 0, 1),
		"post_review_count":                    clamp(float64(reviewCount)/5, 0, 1),
		"post_age_hours":                       clamp(postAgeHours/168, 0, 1),
		"fixed_time_distance_hours":            clamp(fixedDistanceHours/72, 0, 1),
		"freshness":                            freshness,
		"interaction_heat":                     interactionHeat,
		"joinability":                          joinability,
		"joinability_ratio":                    joinability,
		"viewer_clicked_same_subcategory_7d":   clamp(clickedSameSub/5, 0, 1),
		"viewer_unclicked_same_subcategory_7d": clamp(unclickedSameSub/8, 0, 1),
	}

	score := modelWeights.Intercept
	for name, value := range features {
		score += modelWeights.Weights[name] * value
	}
	exploreScore := 0.36*freshness + 0.22*authorQuality + 0.18*(1-clamp(float64(exposureCount)/40, 0, 1)) + 0.10*(1-clamp(float64(post.CurrentCount)/10, 0, 1)) + 0.14*locationProximity
	reason := recommendationReason(matchedTags, authorQuality, freshness, joinability, locationProximity)
	return postSignal{
		Post:         post,
		Score:        score,
		ExploreScore: exploreScore,
		Recommend: recommendationView{
			Strategy:    "personalized",
			Score:       round4(score),
			Reason:      reason,
			MatchedTags: matchedTags,
		},
	}
}

func (s *Server) loadViewerTagWeights(userID string) (viewerTagWeights, error) {
	result := viewerTagWeights{
		Category:    map[string]float64{},
		SubCategory: map[string]float64{},
		City:        map[string]float64{},
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return result, nil
	}
	var rows []model.UserTag
	if err := s.DB.Where("user_id = ?", userID).Order("weight DESC").Find(&rows).Error; err != nil {
		return result, err
	}
	maxWeightByType := map[string]float64{}
	for _, row := range rows {
		if row.Weight > maxWeightByType[row.TagType] {
			maxWeightByType[row.TagType] = row.Weight
		}
	}
	for _, row := range rows {
		denom := maxWeightByType[row.TagType]
		value := 0.0
		if denom > 0 {
			value = row.Weight / denom
		}
		switch row.TagType {
		case "category":
			result.Category[row.TagValue] = value
		case "sub_category":
			result.SubCategory[row.TagValue] = value
		case "city":
			result.City[row.TagValue] = value
		}
		if len(result.TopValues) < 3 {
			result.TopValues = append(result.TopValues, row.TagValue)
		}
	}
	return result, nil
}

func (v viewerTagWeights) match(post model.Post) (float64, float64, float64, []string) {
	matched := make([]string, 0, 3)
	subScore := v.SubCategory[strings.TrimSpace(post.SubCategory)]
	if subScore > 0 {
		matched = append(matched, strings.TrimSpace(post.SubCategory))
	}
	categoryScore := v.Category[strings.TrimSpace(post.Category)]
	if categoryScore > 0 && len(matched) < 3 {
		matched = append(matched, strings.TrimSpace(post.Category))
	}
	city := recommendCityFromAddress(post.Address)
	cityScore := v.City[city]
	if cityScore > 0 && len(matched) < 3 {
		matched = append(matched, city)
	}
	return categoryScore, subScore, cityScore, matched
}

func (s *Server) loadViewerSubcategoryStats(userID string, nowMS int64) (viewerSubcategoryStats, error) {
	stats := viewerSubcategoryStats{
		Clicked:   map[string]float64{},
		Unclicked: map[string]float64{},
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return stats, nil
	}
	cutoff := nowMS - int64(7*24*time.Hour/time.Millisecond)
	var exposures []model.FeedExposure
	if err := s.DB.Where("user_id = ? AND created_at >= ?", userID, cutoff).Find(&exposures).Error; err != nil {
		return stats, err
	}
	var clicks []model.FeedClick
	if err := s.DB.Where("user_id = ? AND created_at >= ?", userID, cutoff).Find(&clicks).Error; err != nil {
		return stats, err
	}
	postIDs := make([]string, 0, len(exposures)+len(clicks))
	for _, exposure := range exposures {
		postIDs = append(postIDs, exposure.PostID)
	}
	for _, click := range clicks {
		postIDs = append(postIDs, click.PostID)
	}
	postMap, err := s.postsByID(postIDs)
	if err != nil {
		return stats, err
	}
	clickedRequests := make(map[string]struct{}, len(clicks))
	clickedBySub := map[string]int{}
	for _, click := range clicks {
		clickedRequests[click.RequestID+"\x00"+click.PostID] = struct{}{}
		post, ok := postMap[click.PostID]
		if !ok {
			continue
		}
		clickedBySub[strings.TrimSpace(post.SubCategory)]++
	}
	exposureBySub := map[string]int{}
	for _, exposure := range exposures {
		post, ok := postMap[exposure.PostID]
		if !ok {
			continue
		}
		subCategory := strings.TrimSpace(post.SubCategory)
		exposureBySub[subCategory]++
		if _, ok := clickedRequests[exposure.RequestID+"\x00"+exposure.PostID]; !ok {
			stats.Unclicked[subCategory] = stats.Unclicked[subCategory] + 1
		}
	}
	for subCategory, count := range clickedBySub {
		stats.Clicked[subCategory] = float64(count)
	}
	for subCategory, count := range exposureBySub {
		if stats.Unclicked[subCategory] > float64(count) {
			stats.Unclicked[subCategory] = float64(count)
		}
	}
	return stats, nil
}

func (s *Server) loadPostEmbeddings(posts []model.Post) (map[string][]float64, error) {
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}
	postIDs = uniqueStrings(postIDs)
	if len(postIDs) == 0 {
		return map[string][]float64{}, nil
	}
	var rows []model.PostEmbedding
	if err := s.DB.Where("post_id IN ? AND model_name = ?", postIDs, recommend.DefaultModelName()).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string][]float64, len(rows))
	for _, row := range rows {
		result[row.PostID] = parseEmbedding(row.EmbeddingJSON)
	}
	return result, nil
}

func (s *Server) loadUserEmbedding(userID string) ([]float64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	var row model.UserEmbedding
	if err := s.DB.First(&row, "user_id = ? AND model_name = ?", userID, recommend.DefaultModelName()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return parseEmbedding(row.EmbeddingJSON), nil
}

type groupedCountRow struct {
	GroupKey string `gorm:"column:group_key"`
	Count    int64
}

func (s *Server) countByColumn(table any, column string, ids []string) (map[string]int64, error) {
	uniqueIDs := uniqueStrings(ids)
	if len(uniqueIDs) == 0 {
		return map[string]int64{}, nil
	}
	rows := []groupedCountRow{}
	if err := s.DB.Model(table).
		Select(column+" AS group_key, COUNT(*) AS count").
		Where(column+" IN ?", uniqueIDs).
		Group(column).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.GroupKey] = item.Count
	}
	return result, nil
}

func (s *Server) countChatMessagesByPost(postIDs []string) (map[string]int64, error) {
	return s.countByColumn(&model.ChatMessage{}, "post_id", postIDs)
}

func (s *Server) countReviewsByPost(postIDs []string) (map[string]int64, error) {
	return s.countByColumn(&model.Review{}, "post_id", postIDs)
}

func (s *Server) countExposuresByPost(postIDs []string) (map[string]int64, error) {
	return s.countByColumn(&model.FeedExposure{}, "post_id", postIDs)
}

func (s *Server) countActivityScoresByUser(userIDs []string) (map[string]int64, error) {
	return s.countByColumn(&model.ActivityScore{}, "user_id", userIDs)
}

func (s *Server) postsByID(postIDs []string) (map[string]model.Post, error) {
	return s.postsByIDInternal(postIDs, false)
}

func (s *Server) postsByIDIncludingDeleted(postIDs []string) (map[string]model.Post, error) {
	return s.postsByIDInternal(postIDs, true)
}

func (s *Server) postsByIDInternal(postIDs []string, includeDeleted bool) (map[string]model.Post, error) {
	ids := uniqueStrings(postIDs)
	if len(ids) == 0 {
		return map[string]model.Post{}, nil
	}
	query := s.DB
	if !includeDeleted {
		query = activePostsQuery(query)
	}
	var posts []model.Post
	if err := query.Where("id IN ?", ids).Find(&posts).Error; err != nil {
		return nil, err
	}
	return indexByStringKey(posts, func(post model.Post) string {
		return post.ID
	}), nil
}

func parseEmbedding(raw string) []float64 {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var out []float64
	if err := json.Unmarshal([]byte(value), &out); err == nil && len(out) > 0 {
		return out
	}
	return nil
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return 0
	}
	sum := 0.0
	for i := range left {
		sum += left[i] * right[i]
	}
	return clamp(sum, -1, 1)
}

func authorQualityScore(author model.User, activityCount int64) float64 {
	base := 0.6*clamp(author.RatingScore/5, 0, 1) + 0.4*clamp(float64(author.CreditScore)/100, 0, 1)
	confidence := 0.4 + 0.6*clamp(float64(activityCount)/5, 0, 1)
	return clamp(base*confidence, 0, 1)
}

func freshnessScore(post model.Post, nowMS int64) float64 {
	ageHours := math.Max(0, float64(nowMS-post.CreatedAt)/float64(time.Hour/time.Millisecond))
	score := 1 - clamp(ageHours/168, 0, 1)
	if ts, ok := homeFixedTime(post); ok {
		diffHours := float64(ts-nowMS) / float64(time.Hour/time.Millisecond)
		if diffHours >= 0 && diffHours <= 72 {
			score += 0.15 * (1 - clamp(diffHours/72, 0, 1))
		}
	}
	return clamp(score, 0, 1)
}

func fixedTimeDistanceHours(post model.Post, nowMS int64) float64 {
	ts, ok := homeFixedTime(post)
	if !ok {
		return 72
	}
	diff := float64(ts-nowMS) / float64(time.Hour/time.Millisecond)
	if diff < 0 {
		return 72
	}
	return diff
}

func joinabilityScore(post model.Post) float64 {
	if post.MaxCount <= 0 {
		return 0
	}
	if post.CurrentCount >= post.MaxCount {
		return 0
	}
	return clamp(float64(post.MaxCount-post.CurrentCount)/float64(post.MaxCount), 0, 1)
}

func locationProximityScore(viewerLocation *geoPoint, post model.Post) float64 {
	if viewerLocation == nil || !validGeoPoint(viewerLocation.Latitude, viewerLocation.Longitude) || !usablePostGeoPoint(post.Lat, post.Lng) {
		return 0
	}
	distanceKm := geoDistanceKm(viewerLocation.Latitude, viewerLocation.Longitude, post.Lat, post.Lng)
	if distanceKm <= 1 {
		return 1
	}
	if distanceKm >= 120 {
		return 0
	}
	return clamp(1/(1+distanceKm/8), 0, 1)
}

func geoDistanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371
	dLat := toRadians(lat2 - lat1)
	dLng := toRadians(lng2 - lng1)
	rLat1 := toRadians(lat1)
	rLat2 := toRadians(lat2)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	a = clamp(a, 0, 1)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

func toRadians(value float64) float64 {
	return value * math.Pi / 180
}

func saturate(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return 1 - math.Exp(-value/5)
}

func recommendationReason(matchedTags []string, authorQuality, freshness, joinability, locationProximity float64) string {
	if locationProximity >= 0.45 {
		return "\u79bb\u4f60\u66f4\u8fd1\uff0c\u53c2\u52a0\u8d77\u6765\u66f4\u65b9\u4fbf"
	}
	if len(matchedTags) > 0 {
		return "\u56e0\u4e3a\u4f60\u6700\u8fd1\u5e38\u770b\u8fd9\u7c7b\u6d3b\u52a8"
	}
	if authorQuality >= 0.7 {
		return "\u53d1\u8d77\u4eba\u8bc4\u4ef7\u548c\u4fe1\u8a89\u5206\u66f4\u7a33"
	}
	if freshness >= 0.7 {
		return "\u6d3b\u52a8\u65f6\u95f4\u8fd1\uff0c\u5185\u5bb9\u4e5f\u6bd4\u8f83\u65b0"
	}
	if joinability >= 0.5 {
		return "\u540d\u989d\u5145\u8db3\uff0c\u62a5\u540d\u6210\u529f\u7387\u66f4\u9ad8"
	}
	return "\u7efc\u5408\u5174\u8da3\u3001\u8d28\u91cf\u548c\u70ed\u5ea6\u63a8\u8350"
}

func recommendCityFromAddress(address string) string {
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
		if strings.Contains(address, city) {
			return city
		}
	}
	return ""
}

func valueAsFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		v, _ := typed.Float64()
		return v
	default:
		return 0
	}
}

func valueAsBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
