package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type settlementStateView struct {
	CanParticipantConfirm bool    `json:"canParticipantConfirm"`
	CanAuthorConfirm      bool    `json:"canAuthorConfirm"`
	CanCancelAll          bool    `json:"canCancelAll"`
	CanOpenFlow           bool    `json:"canOpenFlow"`
	ProjectCancelled      bool    `json:"projectCancelled"`
	FinalStatus           string  `json:"finalStatus"`
	HasDispute            bool    `json:"hasDispute"`
	ParticipantDecision   string  `json:"participantDecision"`
	AuthorDecision        string  `json:"authorDecision"`
	ReviewDeadlineAt      int64   `json:"reviewDeadlineAt"`
	PendingMemberCount    int     `json:"pendingMemberCount"`
	FlowLabel             string  `json:"flowLabel"`
	MyReviewStars         float64 `json:"myReviewStars"`
	AverageStars          float64 `json:"averageStars"`
}

type settlementMemberView struct {
	User                   userBrief           `json:"user"`
	RelationStatus         string              `json:"relationStatus"`
	ParticipantDecision    string              `json:"participantDecision"`
	AuthorDecision         string              `json:"authorDecision"`
	FinalStatus            string              `json:"finalStatus"`
	ParticipantNote        string              `json:"participantNote"`
	AuthorNote             string              `json:"authorNote"`
	ParticipantConfirmedAt int64               `json:"participantConfirmedAt"`
	AuthorConfirmedAt      int64               `json:"authorConfirmedAt"`
	SettledAt              int64               `json:"settledAt"`
	State                  settlementStateView `json:"state"`
}

type settlementReviewTargetView struct {
	User userBrief `json:"user"`
}

type creditLedgerView struct {
	PostID     string `json:"postId"`
	SourceType string `json:"sourceType"`
	Delta      int    `json:"delta"`
	Status     string `json:"status"`
	Note       string `json:"note"`
	CreatedAt  int64  `json:"createdAt"`
	PostTitle  string `json:"postTitle"`
}

type participantSettlementReq struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

type authorSettlementReq struct {
	UserID   string `json:"userId"`
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

func (s *Server) CancelParticipation(c *gin.Context) {
	userID := mustUserID(c)
	postID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()

	var responsePost model.Post
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.AuthorID == userID {
			return errors.New("author cannot cancel as participant")
		}
		if post.Status == "closed" {
			return errors.New("closed post must use settlement flow")
		}

		var relation model.PostParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&relation, "post_id = ? AND user_id = ?", postID, userID).Error; err != nil {
			return err
		}
		if score.NormalizedParticipantStatus(relation.Status) != score.ParticipantStatusActive {
			return errors.New("participant already cancelled")
		}

		relation.Status = score.ParticipantStatusCancelled
		relation.CancelledAt = now
		if err := tx.Save(&relation).Error; err != nil {
			return err
		}
		if post.CurrentCount > 1 {
			post.CurrentCount--
		}
		post.UpdatedAt = now
		if err := tx.Save(&post).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"participant_decision":     score.DecisionCancelled,
				"final_status":             score.SettlementCancelled,
				"participant_note":         "\u53c2\u4e0e\u8005\u5df2\u4e3b\u52a8\u53d6\u6d88\u53c2\u52a0\u6d3b\u52a8",
				"participant_confirmed_at": now,
				"settled_at":               now,
				"updated_at":               now,
			}),
		}).Create(&model.PostParticipantSettlement{
			PostID:                 postID,
			UserID:                 userID,
			ParticipantDecision:    score.DecisionCancelled,
			FinalStatus:            score.SettlementCancelled,
			ParticipantNote:        "\u53c2\u4e0e\u8005\u5df2\u4e3b\u52a8\u53d6\u6d88\u53c2\u52a0\u6d3b\u52a8",
			ParticipantConfirmedAt: now,
			SettledAt:              now,
			CreatedAt:              now,
			UpdatedAt:              now,
		}).Error; err != nil {
			return err
		}
		if err := score.RecalculatePostActivityScoresTx(tx, post, now); err != nil {
			return err
		}
		responsePost = post
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post or participant relation not found"})
		case err.Error() == "author cannot cancel as participant", err.Error() == "closed post must use settlement flow", err.Error() == "participant already cancelled":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true, "post": responsePost})
}

func (s *Server) GetSettlement(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)

	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		writeRecordError(c, err, "post not found", "query post failed")
		return
	}
	if err := s.refreshClosedPostDerivedState(post); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "repair settlement state failed"})
		return
	}
	if post.Status == "closed" {
		if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reload post failed"})
			return
		}
	}

	relations, settlements, err := s.loadSettlementBundle(postID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query settlement failed"})
		return
	}
	userIDs := make([]string, 0, len(relations))
	for _, relation := range relations {
		userIDs = append(userIDs, relation.UserID)
	}
	userMap, err := s.usersByIDs(userIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query settlement users failed"})
		return
	}

	var reviews []model.Review
	if err := s.DB.Where("post_id = ?", postID).Find(&reviews).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query settlement reviews failed"})
		return
	}

	viewerIsAuthor := viewerID != "" && viewerID == post.AuthorID
	pendingMemberCount := pendingSettlementCount(relations, settlements)
	viewItems := make([]settlementMemberView, 0, len(relations))
	for _, relation := range relations {
		settlement := settlements[relation.UserID]
		state := buildSettlementItemState(post, relation.UserID, viewerID, settlement)
		user := userMap[relation.UserID]
		item := settlementMemberView{
			User:                   toUserBrief(user),
			RelationStatus:         relation.Status,
			ParticipantDecision:    settlement.ParticipantDecision,
			AuthorDecision:         settlement.AuthorDecision,
			FinalStatus:            settlement.FinalStatus,
			ParticipantNote:        settlement.ParticipantNote,
			AuthorNote:             settlement.AuthorNote,
			ParticipantConfirmedAt: settlement.ParticipantConfirmedAt,
			AuthorConfirmedAt:      settlement.AuthorConfirmedAt,
			SettledAt:              settlement.SettledAt,
			State:                  state,
		}
		if viewerIsAuthor {
			if settlementNeedsAttention(settlement) {
				viewItems = append(viewItems, item)
			}
			continue
		}
		if relation.UserID == viewerID {
			viewItems = append(viewItems, item)
		}
	}

	sort.SliceStable(viewItems, func(i, j int) bool {
		if viewItems[i].FinalStatus != viewItems[j].FinalStatus {
			return settlementStatusRank(viewItems[i].FinalStatus) < settlementStatusRank(viewItems[j].FinalStatus)
		}
		return viewItems[i].User.Nickname < viewItems[j].User.Nickname
	})

	reviewTargets := s.buildSettlementReviewTargets(post, viewerID, viewerIsAuthor, relations, settlements, reviews, userMap)
	reviewState := buildReviewState(post, viewerID, relations, settlements, reviews)
	stage := settlementStage(post, viewerID, viewerIsAuthor, relations, settlements, reviewTargets)
	flowLabel := settlementFlowLabel(viewerIsAuthor, stage)

	c.JSON(http.StatusOK, gin.H{
		"postId":             post.ID,
		"postTitle":          post.Title,
		"viewerIsAuthor":     viewerIsAuthor,
		"reviewDeadlineAt":   score.ReviewDeadlineAt(post),
		"projectCancelled":   post.CancelledAt > 0,
		"canCancelAll":       viewerIsAuthor && post.Status == "closed" && post.CancelledAt == 0,
		"stage":              stage,
		"flowLabel":          flowLabel,
		"pendingMemberCount": pendingMemberCount,
		"items":              viewItems,
		"reviewTargets":      reviewTargets,
		"reviewState":        reviewState,
	})
}

func (s *Server) UpsertParticipantSettlement(c *gin.Context) {
	userID := mustUserID(c)
	postID := strings.TrimSpace(c.Param("id"))
	var req participantSettlementReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	decision := strings.TrimSpace(req.Decision)
	if !score.IsParticipantDecision(decision) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant decision"})
		return
	}
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.Status != "closed" {
			return errors.New("settlement is available only after post is closed")
		}
		if post.CancelledAt > 0 {
			return errors.New("project already cancelled")
		}
		var relation model.PostParticipant
		if err := tx.First(&relation, "post_id = ? AND user_id = ?", postID, userID).Error; err != nil {
			return err
		}
		row := model.PostParticipantSettlement{
			PostID:                 postID,
			UserID:                 userID,
			ParticipantDecision:    decision,
			ParticipantNote:        strings.TrimSpace(req.Note),
			ParticipantConfirmedAt: now,
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"participant_decision":     row.ParticipantDecision,
				"participant_note":         row.ParticipantNote,
				"participant_confirmed_at": row.ParticipantConfirmedAt,
				"updated_at":               row.UpdatedAt,
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post or relation not found"})
		case err.Error() == "settlement is available only after post is closed", err.Error() == "project already cancelled":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) UpsertAuthorSettlement(c *gin.Context) {
	userID := mustUserID(c)
	postID := strings.TrimSpace(c.Param("id"))
	var req authorSettlementReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	targetUserID := strings.TrimSpace(req.UserID)
	decision := strings.TrimSpace(req.Decision)
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}
	if !score.IsAuthorDecision(decision) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author decision"})
		return
	}
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.AuthorID != userID {
			return errors.New("no permission")
		}
		if post.Status != "closed" {
			return errors.New("settlement is available only after post is closed")
		}
		if post.CancelledAt > 0 {
			return errors.New("project already cancelled")
		}
		var relation model.PostParticipant
		if err := tx.First(&relation, "post_id = ? AND user_id = ?", postID, targetUserID).Error; err != nil {
			return err
		}
		row := model.PostParticipantSettlement{
			PostID:            postID,
			UserID:            targetUserID,
			AuthorDecision:    decision,
			AuthorNote:        strings.TrimSpace(req.Note),
			AuthorConfirmedAt: now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"author_decision":     row.AuthorDecision,
				"author_note":         row.AuthorNote,
				"author_confirmed_at": row.AuthorConfirmedAt,
				"updated_at":          row.UpdatedAt,
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post or relation not found"})
		case err.Error() == "no permission":
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case err.Error() == "settlement is available only after post is closed", err.Error() == "project already cancelled":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) CancelAllSettlement(c *gin.Context) {
	userID := mustUserID(c)
	postID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.AuthorID != userID {
			return errors.New("no permission")
		}
		if post.Status != "closed" {
			return errors.New("settlement is available only after post is closed")
		}
		if post.CancelledAt > 0 {
			return errors.New("project already cancelled")
		}
		post.CancelledAt = now
		post.UpdatedAt = now
		if err := tx.Save(&post).Error; err != nil {
			return err
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		case err.Error() == "no permission":
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case err.Error() == "settlement is available only after post is closed", err.Error() == "project already cancelled":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) GetCreditLedger(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	viewerRole := ""
	if viewerID != "" {
		viewerRole = s.resolveUserRole(viewerID)
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}
	if viewerID == "" || (viewerID != userID && viewerRole != model.UserRoleAdmin) {
		fail(c, http.StatusUnauthorized, "AUTH_REQUIRED", "login required")
		return
	}

	var rows []model.CreditLedger
	if err := s.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(50).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query credit ledger failed"})
		return
	}
	postIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		postIDs = append(postIDs, row.PostID)
	}
	postMap, err := s.postsByID(postIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query ledger posts failed"})
		return
	}
	items := make([]creditLedgerView, 0, len(rows))
	for _, row := range rows {
		items = append(items, creditLedgerView{
			PostID:     row.PostID,
			SourceType: row.SourceType,
			Delta:      row.Delta,
			Status:     row.Status,
			Note:       row.Note,
			CreatedAt:  row.CreatedAt,
			PostTitle:  postMap[row.PostID].Title,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) ListAdminCases(c *gin.Context) {
	s.listAdminCasesResponse(c)
}

func (s *Server) AdminDashboardSummary(c *gin.Context) {
	s.adminDashboardSummaryResponse(c)
}

func (s *Server) loadSettlementBundle(postID string) ([]model.PostParticipant, map[string]model.PostParticipantSettlement, error) {
	var relations []model.PostParticipant
	if err := s.DB.Where("post_id = ?", postID).Order("joined_at ASC").Find(&relations).Error; err != nil {
		return nil, nil, err
	}
	var settlements []model.PostParticipantSettlement
	if err := s.DB.Where("post_id = ?", postID).Find(&settlements).Error; err != nil {
		return nil, nil, err
	}
	settlementMap := make(map[string]model.PostParticipantSettlement, len(settlements))
	for _, row := range settlements {
		settlementMap[row.UserID] = row
	}
	return relations, settlementMap, nil
}

func buildSettlementItemState(post model.Post, subjectUserID, viewerID string, row model.PostParticipantSettlement) settlementStateView {
	viewer := strings.TrimSpace(viewerID)
	subject := strings.TrimSpace(subjectUserID)
	finalStatus := strings.TrimSpace(row.FinalStatus)
	projectCancelled := post.CancelledAt > 0
	canParticipantConfirm := viewer != "" && viewer == subject && post.Status == "closed" && !projectCancelled && settlementNeedsAttention(row)
	canAuthorConfirm := viewer != "" && viewer == post.AuthorID && viewer != subject && post.Status == "closed" && !projectCancelled && settlementNeedsAttention(row)
	return settlementStateView{
		CanParticipantConfirm: canParticipantConfirm,
		CanAuthorConfirm:      canAuthorConfirm,
		CanCancelAll:          false,
		CanOpenFlow:           canParticipantConfirm || canAuthorConfirm,
		ProjectCancelled:      projectCancelled,
		FinalStatus:           finalStatus,
		HasDispute:            finalStatus == score.SettlementDisputed,
		ParticipantDecision:   strings.TrimSpace(row.ParticipantDecision),
		AuthorDecision:        strings.TrimSpace(row.AuthorDecision),
		ReviewDeadlineAt:      score.ReviewDeadlineAt(post),
	}
}

func settlementNeedsAttention(row model.PostParticipantSettlement) bool {
	status := strings.TrimSpace(row.FinalStatus)
	return status == "" || status == score.SettlementPending || status == score.SettlementDisputed
}

func settlementStage(post model.Post, viewerID string, viewerIsAuthor bool, relations []model.PostParticipant, settlements map[string]model.PostParticipantSettlement, reviewTargets []settlementReviewTargetView) string {
	if post.CancelledAt > 0 {
		return "cancelled"
	}
	if viewerIsAuthor {
		if hasAuthorSettlementWork(relations, settlements) {
			return "settlement"
		}
		if len(reviewTargets) > 0 {
			return "review"
		}
		return "done"
	}
	if strings.TrimSpace(viewerID) == "" {
		return "done"
	}
	if hasParticipantSettlementWork(viewerID, settlements) {
		return "settlement"
	}
	if len(reviewTargets) > 0 {
		return "review"
	}
	return "done"
}

func settlementFlowLabel(viewerIsAuthor bool, stage string) string {
	switch stage {
	case "settlement":
		if viewerIsAuthor {
			return "\u7ba1\u7406\u5c65\u7ea6"
		}
		return "\u5c65\u7ea6\u786e\u8ba4"
	case "review":
		return "\u7ee7\u7eed\u8bc4\u5206"
	case "cancelled":
		if viewerIsAuthor {
			return "\u9879\u76ee\u5df2\u53d6\u6d88"
		}
		return "\u6d3b\u52a8\u5df2\u53d6\u6d88"
	default:
		return "\u5df2\u5b8c\u6210"
	}
}

func (s *Server) buildSettlementReviewTargets(
	post model.Post,
	viewerID string,
	viewerIsAuthor bool,
	relations []model.PostParticipant,
	settlements map[string]model.PostParticipantSettlement,
	reviews []model.Review,
	userMap map[string]model.User,
) []settlementReviewTargetView {
	if strings.TrimSpace(viewerID) == "" || post.CancelledAt > 0 {
		return []settlementReviewTargetView{}
	}
	if viewerIsAuthor && hasAuthorSettlementWork(relations, settlements) {
		return []settlementReviewTargetView{}
	}
	alreadyReviewed := make(map[string]struct{})
	for _, review := range reviews {
		if strings.TrimSpace(review.FromUserID) != viewerID {
			continue
		}
		alreadyReviewed[strings.TrimSpace(review.ToUserID)] = struct{}{}
	}
	result := make([]settlementReviewTargetView, 0)
	if viewerIsAuthor {
		for _, relation := range relations {
			if strings.TrimSpace(relation.UserID) == "" {
				continue
			}
			row := settlements[relation.UserID]
			if strings.TrimSpace(row.FinalStatus) != score.SettlementCompleted {
				continue
			}
			if _, ok := alreadyReviewed[relation.UserID]; ok {
				continue
			}
			user := userMap[relation.UserID]
			result = append(result, settlementReviewTargetView{User: toUserBrief(user)})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].User.Nickname < result[j].User.Nickname
		})
		return result
	}
	row := settlements[viewerID]
	if strings.TrimSpace(row.FinalStatus) != score.SettlementCompleted {
		return []settlementReviewTargetView{}
	}
	if _, ok := alreadyReviewed[post.AuthorID]; ok {
		return []settlementReviewTargetView{}
	}
	author := userMap[post.AuthorID]
	if strings.TrimSpace(author.ID) == "" {
		author = model.User{ID: post.AuthorID, Nickname: "\u53d1\u8d77\u4eba", AvatarURL: avatarURLFromSeed(post.AuthorID), CreditScore: 100, RatingScore: 5}
	}
	return []settlementReviewTargetView{{User: toUserBrief(author)}}
}

func settlementStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case score.SettlementPending:
		return 0
	case score.SettlementDisputed:
		return 1
	case score.SettlementCompleted:
		return 2
	case score.SettlementNoShow:
		return 3
	case score.SettlementCancelled:
		return 4
	default:
		return 9
	}
}
