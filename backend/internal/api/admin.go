package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type adminCaseListItem struct {
	model.AdminCase
	PostTitle        string `json:"postTitle"`
	TargetNickname   string `json:"targetNickname"`
	ReporterNickname string `json:"reporterNickname"`
	ResolverNickname string `json:"resolverNickname"`
}

type adminCaseDetail struct {
	Case             adminCaseListItem                `json:"case"`
	Post             *model.Post                      `json:"post,omitempty"`
	TargetUser       *model.User                      `json:"targetUser,omitempty"`
	Reporter         *model.User                      `json:"reporter,omitempty"`
	Resolver         *model.User                      `json:"resolver,omitempty"`
	Settlement       *model.PostParticipantSettlement `json:"settlement,omitempty"`
	Decision         *model.CaseDecision              `json:"decision,omitempty"`
	AgentRun         *adminAgentRunSummary             `json:"agentRun,omitempty"`
	Timeline         []adminCaseTimelineItem          `json:"timeline"`
	CreditComparison adminCaseCreditComparison        `json:"creditComparison"`
}

type adminAgentRunSummary struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StepCount int    `json:"stepCount"`
	StartedAt int64  `json:"startedAt"`
}

type adminCaseResolveReq struct {
	Resolution string `json:"resolution"`
	Note       string `json:"note"`
}

type adminCreditAdjustReq struct {
	Delta  int    `json:"delta"`
	Note   string `json:"note"`
	PostID string `json:"postId"`
}

type adminUserDetail struct {
	User          model.User `json:"user"`
	InitiatedPost int64      `json:"initiatedPostCount"`
	JoinedPost    int64      `json:"joinedPostCount"`
	ReviewCount   int64      `json:"reviewCount"`
}

type adminPostDetail struct {
	Post              model.Post        `json:"post"`
	Author            model.User        `json:"author"`
	ParticipantCount  int64             `json:"participantCount"`
	ChatMessageCount  int64             `json:"chatMessageCount"`
	ReviewCount       int64             `json:"reviewCount"`
	SettlementPending int64             `json:"settlementPendingCount"`
	Reviews           []adminReviewItem `json:"reviews"`
}

type adminCaseTimelineItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Time        int64  `json:"time"`
}

type adminCaseCreditDelta struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	Label     string `json:"label"`
	Before    int    `json:"before"`
	After     int    `json:"after"`
	Delta     int    `json:"delta"`
	PostTitle string `json:"postTitle"`
}

type adminCaseCreditComparison struct {
	Target   adminCaseCreditDelta `json:"target"`
	Reporter adminCaseCreditDelta `json:"reporter"`
}

type adminReviewItem struct {
	model.Review
	PostTitle    string `json:"postTitle"`
	FromNickname string `json:"fromNickname"`
	ToNickname   string `json:"toNickname"`
}

func parsePageParams(c *gin.Context) (int, int) {
	page := 1
	pageSize := 20
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			page = value
		}
	}
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			pageSize = value
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func applyTimeRange(query *gorm.DB, column string, c *gin.Context) *gorm.DB {
	if from := strings.TrimSpace(c.Query("timeFrom")); from != "" {
		if value, err := strconv.ParseInt(from, 10, 64); err == nil && value > 0 {
			query = query.Where(column+" >= ?", value)
		}
	}
	if to := strings.TrimSpace(c.Query("timeTo")); to != "" {
		if value, err := strconv.ParseInt(to, 10, 64); err == nil && value > 0 {
			query = query.Where(column+" <= ?", value)
		}
	}
	return query
}

func paginate[T any](query *gorm.DB, page, pageSize int, out *[]T) (int64, error) {
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}
	if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(out).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Server) listAdminCasesResponse(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	status := strings.TrimSpace(c.Query("status"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	query := s.DB.Model(&model.AdminCase{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	query = applyTimeRange(query, "created_at", c)

	var rows []model.AdminCase
	if err := query.Order("updated_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query admin cases failed"})
		return
	}

	postIDs := make([]string, 0, len(rows))
	userIDs := make([]string, 0, len(rows)*3)
	for _, row := range rows {
		postIDs = append(postIDs, row.PostID)
		userIDs = append(userIDs, row.TargetUserID, row.ReporterUserID, row.ResolverUserID)
	}
	postMap, _ := s.postsByIDIncludingDeleted(postIDs)
	userMap, _ := s.usersByIDsIncludingDeleted(userIDs)

	items := make([]adminCaseListItem, 0, len(rows))
	for _, row := range rows {
		item := adminCaseListItem{
			AdminCase:        row,
			PostTitle:        postMap[row.PostID].Title,
			TargetNickname:   userMap[row.TargetUserID].Nickname,
			ReporterNickname: userMap[row.ReporterUserID].Nickname,
			ResolverNickname: userMap[row.ResolverUserID].Nickname,
		}
		if keyword != "" {
			all := strings.ToLower(strings.Join([]string{item.Summary, item.PostTitle, item.TargetNickname, item.ReporterNickname}, " "))
			if !strings.Contains(all, strings.ToLower(keyword)) {
				continue
			}
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftRank := adminCaseStatusRank(items[i].Status)
		rightRank := adminCaseStatusRank(items[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})

	total := int64(len(items))
	c.JSON(http.StatusOK, gin.H{
		"items":    paginateSlice(items, page, pageSize),
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

func (s *Server) adminDashboardSummaryResponse(c *gin.Context) {
	var openCases int64
	var inReviewCases int64
	var investigatingCases int64
	var underReviewCases int64
	var disputedSettlements int64
	var pendingReviews int64
	var recentCreditDeltas int64
	var totalUsers int64
	var totalPosts int64
	var closedPosts int64
	now := time.Now().UnixMilli()

	_ = s.DB.Model(&model.AdminCase{}).Where("status = ?", "open").Count(&openCases).Error
	_ = s.DB.Model(&model.AdminCase{}).Where("status = ?", "in_review").Count(&inReviewCases).Error
	_ = s.DB.Model(&model.AdminCase{}).Where("status = ?", "investigating").Count(&investigatingCases).Error
	_ = s.DB.Model(&model.AdminCase{}).Where("status = ?", "under_review").Count(&underReviewCases).Error
	_ = s.DB.Model(&model.PostParticipantSettlement{}).Where("final_status = ?", score.SettlementDisputed).Count(&disputedSettlements).Error
	_ = s.DB.Model(&model.ActivityScore{}).Where("expected_review_count > completed_review_count").Count(&pendingReviews).Error
	_ = s.DB.Model(&model.CreditLedger{}).Where("created_at >= ?", now-int64(7*24*time.Hour/time.Millisecond)).Count(&recentCreditDeltas).Error
	_ = activeUsersQuery(s.DB.Model(&model.User{})).Count(&totalUsers).Error
	_ = activePostsQuery(s.DB.Model(&model.Post{})).Count(&totalPosts).Error
	_ = activePostsQuery(s.DB.Model(&model.Post{})).Where("status = ?", "closed").Count(&closedPosts).Error

	// Agent stats
	var totalRuns int64
	var completedRuns int64
	var failedRuns int64
	_ = s.DB.Model(&model.AgentRun{}).Count(&totalRuns).Error
	_ = s.DB.Model(&model.AgentRun{}).Where("status = ?", "completed").Count(&completedRuns).Error
	_ = s.DB.Model(&model.AgentRun{}).Where("status = ?", "failed").Count(&failedRuns).Error

	var avgDurationMs float64
	s.DB.Model(&model.AgentRun{}).
		Where("status = ? AND completed_at > 0 AND started_at > 0", "completed").
		Select("COALESCE(AVG(completed_at - started_at), 0)").
		Scan(&avgDurationMs)

	var totalDecisions int64
	var autoApprovedDecisions int64
	_ = s.DB.Model(&model.CaseDecision{}).Count(&totalDecisions).Error
	_ = s.DB.Model(&model.CaseDecision{}).Where("approved_by = ?", "system:auto-approve").Count(&autoApprovedDecisions).Error

	c.JSON(http.StatusOK, gin.H{
		"openCases":           openCases,
		"inReviewCases":       inReviewCases,
		"investigatingCases":  investigatingCases,
		"underReviewCases":    underReviewCases,
		"disputedSettlements": disputedSettlements,
		"pendingReviews":      pendingReviews,
		"recentCreditDeltas":  recentCreditDeltas,
		"totalUsers":          totalUsers,
		"totalPosts":          totalPosts,
		"closedPosts":         closedPosts,
		"agent": gin.H{
			"totalRuns":         totalRuns,
			"completedRuns":     completedRuns,
			"failedRuns":        failedRuns,
			"avgDurationMs":     avgDurationMs,
			"totalDecisions":    totalDecisions,
			"autoApproved":      autoApprovedDecisions,
			"successRate":       safePercent(completedRuns, totalRuns),
			"autoApproveRate":   safePercent(autoApprovedDecisions, totalDecisions),
		},
	})
}

func (s *Server) GetAdminCase(c *gin.Context) {
	caseID := strings.TrimSpace(c.Param("id"))
	var row model.AdminCase
	if err := s.DB.First(&row, "id = ?", caseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "admin case not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query admin case failed"})
		return
	}

	postMap, _ := s.postsByIDIncludingDeleted([]string{row.PostID})
	userMap, _ := s.usersByIDsIncludingDeleted([]string{row.TargetUserID, row.ReporterUserID, row.ResolverUserID})

	payload := adminCaseDetail{
		Case: adminCaseListItem{
			AdminCase:        row,
			PostTitle:        postMap[row.PostID].Title,
			TargetNickname:   userMap[row.TargetUserID].Nickname,
			ReporterNickname: userMap[row.ReporterUserID].Nickname,
			ResolverNickname: userMap[row.ResolverUserID].Nickname,
		},
	}
	if post, ok := postMap[row.PostID]; ok {
		payload.Post = &post
	}
	if user, ok := userMap[row.TargetUserID]; ok {
		normalizeUserModel(&user)
		payload.TargetUser = &user
	}
	if user, ok := userMap[row.ReporterUserID]; ok {
		normalizeUserModel(&user)
		payload.Reporter = &user
	}
	if user, ok := userMap[row.ResolverUserID]; ok {
		normalizeUserModel(&user)
		payload.Resolver = &user
	}

	if row.CaseType == score.AdminCaseSettlementDispute {
		var settlement model.PostParticipantSettlement
		if err := s.DB.First(&settlement, "post_id = ? AND user_id = ?", row.PostID, row.TargetUserID).Error; err == nil {
			payload.Settlement = &settlement
		}
	}

	// Fetch latest CaseDecision for this case (proposed or executed)
	var decision model.CaseDecision
	if err := s.DB.Where("case_id = ?", caseID).Order("created_at DESC").First(&decision).Error; err == nil {
		payload.Decision = &decision
	}

	// Fetch agent run summary if the case has one
	if row.AgentRunID != "" {
		var run model.AgentRun
		if err := s.DB.First(&run, "id = ?", row.AgentRunID).Error; err == nil {
			payload.AgentRun = &adminAgentRunSummary{
				ID:        run.ID,
				Status:    run.Status,
				StepCount: run.StepCount,
				StartedAt: run.StartedAt,
			}
		}
	}

	payload.Timeline = buildAdminCaseTimeline(payload.Case, payload.Settlement)
	payload.CreditComparison = s.buildAdminCaseCreditComparison(payload.Case, payload.TargetUser, payload.Reporter)

	c.JSON(http.StatusOK, payload)
}

func (s *Server) ResolveAdminCase(c *gin.Context) {
	caseID := strings.TrimSpace(c.Param("id"))
	adminUserID := mustUserID(c)
	var req adminCaseResolveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	resolution := strings.TrimSpace(req.Resolution)
	if resolution != score.SettlementCompleted && resolution != score.SettlementCancelled && resolution != score.SettlementNoShow {
		fail(c, http.StatusBadRequest, "INVALID_RESOLUTION", "resolution must be completed, cancelled, or no_show")
		return
	}
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var item model.AdminCase
		if err := tx.First(&item, "id = ?", caseID).Error; err != nil {
			return err
		}
		if item.CaseType != score.AdminCaseSettlementDispute {
			return errors.New("only settlement dispute cases can be resolved")
		}

		var post model.Post
		if err := tx.First(&post, "id = ?", item.PostID).Error; err != nil {
			return err
		}

		// Record the ruling in admin_resolution rather than rewriting the two
		// sides' decisions: those stay as evidence of what each party claimed.
		// The ruling survives recalculation while the project and participation
		// remain active; a later cancellation still ends the settlement.
		updates := map[string]any{
			"author_confirmed_at": now,
			"updated_at":          now,
			"settled_at":          now,
			"final_status":        resolution,
			"admin_resolution":    resolution,
		}
		if strings.TrimSpace(req.Note) != "" {
			updates["author_note"] = strings.TrimSpace(req.Note)
		}
		if err := tx.Model(&model.PostParticipantSettlement{}).
			Where("post_id = ? AND user_id = ?", item.PostID, item.TargetUserID).
			Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.AdminCase{}).
			Where("id = ?", item.ID).
			Updates(map[string]any{
				"status":           "resolved",
				"resolver_user_id": adminUserID,
				"resolution":       resolution,
				"resolution_note":  strings.TrimSpace(req.Note),
				"decision":         resolution,
				"decision_reason":  strings.TrimSpace(req.Note),
				"resolved_at":      now,
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}
		payload, _ := json.Marshal(req)
		if err := tx.Create(&model.CaseEvent{CaseID: item.ID, EventType: "decision_made", ActorID: adminUserID, Payload: string(payload), CreatedAt: now}).Error; err != nil {
			return err
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "case or settlement not found"})
		default:
			fail(c, http.StatusBadRequest, "RESOLVE_CASE_FAILED", err.Error())
		}
		return
	}

	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) ListAdminUsers(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	role := model.NormalizeUserRole(c.Query("role"))
	status := strings.TrimSpace(c.Query("status"))
	if strings.TrimSpace(c.Query("role")) == "" {
		role = ""
	}

	query := applySoftDeleteStatus(s.DB.Model(&model.User{}), "deleted_at", status)
	if role != "" {
		query = query.Where("role = ?", role)
	}
	if keyword != "" {
		query = query.Where("nickname LIKE ? OR id LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	query = applyTimeRange(query, "created_at", c)

	var users []model.User
	total, err := paginate(query.Order("updated_at DESC"), page, pageSize, &users)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query users failed"})
		return
	}
	for index := range users {
		normalizeUserModel(&users[index])
	}
	c.JSON(http.StatusOK, gin.H{"items": users, "page": page, "pageSize": pageSize, "total": total})
}

func (s *Server) GetAdminUser(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	var user model.User
	if err := s.DB.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query user failed"})
		return
	}
	normalizeUserModel(&user)

	var initiatedCount int64
	var joinedCount int64
	var reviewCount int64
	_ = s.DB.Model(&model.Post{}).Where("author_id = ? AND deleted_at = 0", userID).Count(&initiatedCount).Error
	_ = s.DB.Model(&model.PostParticipant{}).Where("user_id = ?", userID).Count(&joinedCount).Error
	_ = s.DB.Model(&model.Review{}).Where("to_user_id = ?", userID).Count(&reviewCount).Error

	c.JSON(http.StatusOK, adminUserDetail{
		User:          user,
		InitiatedPost: initiatedCount,
		JoinedPost:    joinedCount,
		ReviewCount:   reviewCount,
	})
}

func (s *Server) GetAdminUserCreditLedger(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	page, pageSize := parsePageParams(c)
	items, total, err := s.creditLedgerItemsForUser(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query credit ledger failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "pageSize": pageSize, "total": total})
}

func (s *Server) AdminAdjustUserCredit(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	adminUserID := mustUserID(c)
	var req adminCreditAdjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Delta == 0 {
		fail(c, http.StatusBadRequest, "INVALID_DELTA", "delta cannot be zero")
		return
	}
	now := time.Now().UnixMilli()
	postID := strings.TrimSpace(req.PostID)
	if postID == "" {
		postID = "manual_" + strings.ToLower(strconv.FormatInt(now, 36))
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		row := model.CreditLedger{
			UserID:         userID,
			PostID:         postID,
			SourceType:     score.LedgerManualCreditAdjust,
			Delta:          req.Delta,
			Status:         "settled",
			Note:           strings.TrimSpace(req.Note),
			OperatorUserID: adminUserID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return score.RecalculateUsersFromActivityScoresTx(tx, []string{userID}, now)
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		default:
			fail(c, http.StatusBadRequest, "CREDIT_ADJUST_FAILED", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) ListAdminPosts(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))

	query := applySoftDeleteStatus(s.DB.Model(&model.Post{}), "deleted_at", status)
	if status == "open" || status == "closed" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ? OR address LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	query = applyTimeRange(query, "created_at", c)

	var posts []model.Post
	total, err := paginate(query.Order("created_at DESC"), page, pageSize, &posts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query posts failed"})
		return
	}
	views, err := s.buildPostViews(posts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build post views failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views, "page": page, "pageSize": pageSize, "total": total})
}

func (s *Server) GetAdminPost(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post failed"})
		return
	}
	authors, _ := s.usersByIDsIncludingDeleted([]string{post.AuthorID})
	author := authors[post.AuthorID]
	normalizeUserModel(&author)

	var participantCount int64
	var chatCount int64
	var reviewCount int64
	var settlementPending int64
	var reviews []model.Review
	_ = s.DB.Model(&model.PostParticipant{}).Where("post_id = ? AND status = ?", postID, score.ParticipantStatusActive).Count(&participantCount).Error
	_ = s.DB.Model(&model.ChatMessage{}).Where("post_id = ?", postID).Count(&chatCount).Error
	_ = s.DB.Model(&model.Review{}).Where("post_id = ?", postID).Count(&reviewCount).Error
	_ = s.DB.Model(&model.PostParticipantSettlement{}).Where("post_id = ? AND final_status IN ?", postID, []string{score.SettlementPending, score.SettlementDisputed}).Count(&settlementPending).Error
	_ = s.DB.Where("post_id = ?", postID).Order("created_at DESC").Limit(20).Find(&reviews).Error

	reviewItems := make([]adminReviewItem, 0, len(reviews))
	if len(reviews) > 0 {
		userIDs := make([]string, 0, len(reviews)*2)
		for _, row := range reviews {
			userIDs = append(userIDs, row.FromUserID, row.ToUserID)
		}
		userMap, _ := s.usersByIDsIncludingDeleted(userIDs)
		for _, row := range reviews {
			reviewItems = append(reviewItems, adminReviewItem{
				Review:       row,
				PostTitle:    post.Title,
				FromNickname: userMap[row.FromUserID].Nickname,
				ToNickname:   userMap[row.ToUserID].Nickname,
			})
		}
	}

	c.JSON(http.StatusOK, adminPostDetail{
		Post:              post,
		Author:            author,
		ParticipantCount:  participantCount,
		ChatMessageCount:  chatCount,
		ReviewCount:       reviewCount,
		SettlementPending: settlementPending,
		Reviews:           reviewItems,
	})
}

func (s *Server) ListAdminReviews(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	keyword := strings.TrimSpace(c.Query("keyword"))

	query := s.DB.Model(&model.Review{})
	query = applyTimeRange(query, "created_at", c)

	var reviews []model.Review
	total, err := paginate(query.Order("created_at DESC"), page, pageSize, &reviews)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query reviews failed"})
		return
	}
	postIDs := make([]string, 0, len(reviews))
	userIDs := make([]string, 0, len(reviews)*2)
	for _, row := range reviews {
		postIDs = append(postIDs, row.PostID)
		userIDs = append(userIDs, row.FromUserID, row.ToUserID)
	}
	postMap, _ := s.postsByIDIncludingDeleted(postIDs)
	userMap, _ := s.usersByIDsIncludingDeleted(userIDs)

	items := make([]adminReviewItem, 0, len(reviews))
	for _, row := range reviews {
		item := adminReviewItem{
			Review:       row,
			PostTitle:    postMap[row.PostID].Title,
			FromNickname: userMap[row.FromUserID].Nickname,
			ToNickname:   userMap[row.ToUserID].Nickname,
		}
		if keyword != "" {
			all := strings.ToLower(strings.Join([]string{item.PostTitle, item.FromNickname, item.ToNickname, item.Comment}, " "))
			if !strings.Contains(all, strings.ToLower(keyword)) {
				continue
			}
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "pageSize": pageSize, "total": total})
}

func (s *Server) ListAdminAccounts(c *gin.Context) {
	c.Request.URL.RawQuery = mergeAdminRoleQuery(c.Request.URL.RawQuery)
	s.ListAdminUsers(c)
}

func mergeAdminRoleQuery(raw string) string {
	if strings.Contains(raw, "role=") {
		return raw
	}
	if strings.TrimSpace(raw) == "" {
		return "role=admin"
	}
	return raw + "&role=admin"
}

func (s *Server) creditLedgerItemsForUser(userID string, page, pageSize int) ([]creditLedgerView, int64, error) {
	query := s.DB.Model(&model.CreditLedger{}).Where("user_id = ?", userID)
	var rows []model.CreditLedger
	total, err := paginate(query.Order("created_at DESC"), page, pageSize, &rows)
	if err != nil {
		return nil, 0, err
	}
	postIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if !strings.HasPrefix(row.PostID, "manual_") {
			postIDs = append(postIDs, row.PostID)
		}
	}
	postMap, err := s.postsByIDIncludingDeleted(postIDs)
	if err != nil {
		return nil, 0, err
	}
	items := make([]creditLedgerView, 0, len(rows))
	for _, row := range rows {
		postTitle := postMap[row.PostID].Title
		if postTitle == "" && strings.HasPrefix(row.PostID, "manual_") {
			postTitle = "管理员手动调分"
		}
		items = append(items, creditLedgerView{
			PostID:     row.PostID,
			SourceType: row.SourceType,
			Delta:      row.Delta,
			Status:     row.Status,
			Note:       row.Note,
			CreatedAt:  row.CreatedAt,
			PostTitle:  postTitle,
		})
	}
	return items, total, nil
}

func adminCaseStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case "open":
		return 0
	case "in_review":
		return 1
	case "resolved":
		return 2
	default:
		return 9
	}
}

func buildAdminCaseTimeline(item adminCaseListItem, settlement *model.PostParticipantSettlement) []adminCaseTimelineItem {
	timeline := make([]adminCaseTimelineItem, 0, 8)
	if item.CreatedAt > 0 {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "创建争议案例",
			Description: fallbackText(item.Summary, "系统检测到该活动履约确认出现分歧，已自动生成争议案例。"),
			Time:        item.CreatedAt,
		})
	}
	if settlement != nil && settlement.ParticipantConfirmedAt > 0 {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "参与者提交结论",
			Description: settlementDecisionLabel(settlement.ParticipantDecision) + joinNote(settlement.ParticipantNote),
			Time:        settlement.ParticipantConfirmedAt,
		})
	}
	if settlement != nil && settlement.AuthorConfirmedAt > 0 {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "发起人提交结论",
			Description: settlementDecisionLabel(settlement.AuthorDecision) + joinNote(settlement.AuthorNote),
			Time:        settlement.AuthorConfirmedAt,
		})
	}
	if settlement != nil && settlement.FinalStatus == score.SettlementDisputed {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "系统标记为争议",
			Description: "参与者和发起人的结论不一致，系统已冻结自动结算并等待管理员介入。",
			Time:        maxInt64(settlement.AuthorConfirmedAt, settlement.ParticipantConfirmedAt),
		})
	}
	if item.Status == "in_review" {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "管理员开始处理",
			Description: "案例已从待处理转为处理中，说明管理员已经开始查看证据和结论。",
			Time:        item.UpdatedAt,
		})
	}
	if item.Status == "resolved" && item.ResolvedAt > 0 {
		timeline = append(timeline, adminCaseTimelineItem{
			Title:       "管理员完成结案",
			Description: "结案结果：" + caseResolutionLabel(item.Resolution) + joinNote(item.ResolutionNote),
			Time:        item.ResolvedAt,
		})
	}
	sort.SliceStable(timeline, func(i, j int) bool {
		return timeline[i].Time < timeline[j].Time
	})
	return timeline
}

func (s *Server) buildAdminCaseCreditComparison(item adminCaseListItem, targetUser, reporter *model.User) adminCaseCreditComparison {
	comparison := adminCaseCreditComparison{
		Target: adminCaseCreditDelta{
			UserID:    item.TargetUserID,
			Nickname:  item.TargetNickname,
			Label:     "目标用户",
			PostTitle: item.PostTitle,
		},
		Reporter: adminCaseCreditDelta{
			UserID:    item.ReporterUserID,
			Nickname:  item.ReporterNickname,
			Label:     "发起人",
			PostTitle: item.PostTitle,
		},
	}
	if targetUser != nil {
		comparison.Target.After = targetUser.CreditScore
	}
	if reporter != nil {
		comparison.Reporter.After = reporter.CreditScore
	}

	var ledgers []model.CreditLedger
	_ = s.DB.Where("post_id = ? AND user_id IN ?", item.PostID, []string{item.TargetUserID, item.ReporterUserID}).
		Order("created_at ASC").
		Find(&ledgers).Error
	for _, row := range ledgers {
		switch row.UserID {
		case item.TargetUserID:
			comparison.Target.Delta += row.Delta
		case item.ReporterUserID:
			comparison.Reporter.Delta += row.Delta
		}
	}
	comparison.Target.Before = clampAdminCredit(comparison.Target.After - comparison.Target.Delta)
	comparison.Reporter.Before = clampAdminCredit(comparison.Reporter.After - comparison.Reporter.Delta)
	return comparison
}

func paginateSlice[T any](items []T, page, pageSize int) []T {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func clampAdminCredit(value int) int {
	if value < 60 {
		return 60
	}
	if value > 100 {
		return 100
	}
	return value
}

func fallbackText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func joinNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return "，备注：" + note
}

func settlementDecisionLabel(value string) string {
	switch strings.TrimSpace(value) {
	case score.DecisionCompleted:
		return "确认到场"
	case score.DecisionCancelled:
		return "确认取消"
	case score.DecisionNoShow:
		return "标记爽约"
	case score.DecisionDisputed:
		return "活动异常"
	default:
		return "待提交"
	}
}

func caseResolutionLabel(value string) string {
	switch strings.TrimSpace(value) {
	case score.SettlementCompleted:
		return "确认完成"
	case score.SettlementCancelled:
		return "确认取消"
	case score.SettlementNoShow:
		return "确认爽约"
	case "auto_closed":
		return "系统自动关闭"
	default:
		if value == "" {
			return "未结案"
		}
		return value
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func safePercent(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}
