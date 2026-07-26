package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

const (
	defaultInvitationMessage = "我觉得你可能会对这个活动感兴趣，一起来吗？"
	maxInvitationBatchSize   = 30
)

var (
	errJoinAuthorOwnPost = errors.New("author cannot join own post")
	errJoinPostClosed    = errors.New("post is closed")
	errJoinPostFull      = errors.New("post is full")
	errJoinAlreadyJoined = errors.New("already joined")
)

type postInvitationCreateReq struct {
	InviteeID string `json:"inviteeId"`
	Message   string `json:"message"`
}

type postInvitationView struct {
	ID          string     `json:"id"`
	Message     string     `json:"message"`
	Status      string     `json:"status"`
	RespondedAt int64      `json:"respondedAt"`
	CreatedAt   int64      `json:"createdAt"`
	UpdatedAt   int64      `json:"updatedAt"`
	Inviter     userBrief  `json:"inviter"`
	Invitee     userBrief  `json:"invitee"`
	Post        model.Post `json:"post"`
}

func (req postUpsertReq) invitationInputs() []postInvitationCreateReq {
	out := make([]postInvitationCreateReq, 0, len(req.Invitations)+len(req.InviteeIDs))
	for _, item := range req.Invitations {
		out = append(out, item)
	}
	for _, inviteeID := range req.InviteeIDs {
		out = append(out, postInvitationCreateReq{
			InviteeID: inviteeID,
			Message:   req.InviteMessage,
		})
	}
	return out
}

func (s *Server) createPostInvitationsTx(tx *gorm.DB, postID, inviterID string, inputs []postInvitationCreateReq, now int64) error {
	inviteeIDs := make([]string, 0, len(inputs))
	messageByInvitee := map[string]string{}
	seen := map[string]struct{}{}
	for _, input := range inputs {
		inviteeID := strings.TrimSpace(input.InviteeID)
		if inviteeID == "" || inviteeID == inviterID {
			continue
		}
		if _, ok := seen[inviteeID]; ok {
			continue
		}
		seen[inviteeID] = struct{}{}
		inviteeIDs = append(inviteeIDs, inviteeID)
		messageByInvitee[inviteeID] = normalizeInvitationMessage(input.Message)
		if len(inviteeIDs) >= maxInvitationBatchSize {
			break
		}
	}
	if len(inviteeIDs) == 0 {
		return nil
	}

	var users []model.User
	if err := activeUsersQuery(tx).Where("id IN ?", inviteeIDs).Find(&users).Error; err != nil {
		return err
	}
	activeInvitees := make(map[string]struct{}, len(users))
	for _, user := range users {
		activeInvitees[user.ID] = struct{}{}
	}

	rows := make([]model.PostInvitation, 0, len(inviteeIDs))
	for _, inviteeID := range inviteeIDs {
		if _, ok := activeInvitees[inviteeID]; !ok {
			continue
		}
		rows = append(rows, model.PostInvitation{
			ID:        "invite_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:12]), "-", ""),
			PostID:    postID,
			InviterID: inviterID,
			InviteeID: inviteeID,
			Message:   messageByInvitee[inviteeID],
			Status:    model.InvitationStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func normalizeInvitationMessage(raw string) string {
	message := strings.TrimSpace(raw)
	if message == "" {
		return defaultInvitationMessage
	}
	runes := []rune(message)
	if len(runes) > 120 {
		message = string(runes[:120])
	}
	return message
}

func (s *Server) SearchUsers(c *gin.Context) {
	userID := mustUserID(c)
	keyword := strings.TrimSpace(c.Query("q"))
	limit := queryIntOrDefault(c.Query("limit"), 10)
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	query := activeUsersQuery(s.DB.Model(&model.User{})).Where("id <> ?", userID)
	if keyword != "" {
		query = query.Where("nickname LIKE ?", "%"+keyword+"%")
	}

	var users []model.User
	if err := query.Order("updated_at DESC").Limit(limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query users failed"})
		return
	}

	views := make([]userBrief, 0, len(users))
	for _, user := range users {
		normalizeUserModel(&user)
		views = append(views, toUserBrief(user))
	}
	c.JSON(http.StatusOK, gin.H{"users": views})
}

func (s *Server) ListInvitations(c *gin.Context) {
	s.listInvitationViews(c, "invitee_id")
}

func (s *Server) ListSentInvitations(c *gin.Context) {
	s.listInvitationViews(c, "inviter_id")
}

func (s *Server) listInvitationViews(c *gin.Context, userColumn string) {
	userID := mustUserID(c)
	var invitations []model.PostInvitation
	if err := s.DB.Where(userColumn+" = ?", userID).
		Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, created_at DESC").
		Limit(50).
		Find(&invitations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query invitations failed"})
		return
	}

	postIDs := make([]string, 0, len(invitations))
	userIDs := make([]string, 0, len(invitations)*2)
	for _, invitation := range invitations {
		postIDs = append(postIDs, invitation.PostID)
		userIDs = append(userIDs, invitation.InviterID, invitation.InviteeID)
	}
	postMap, err := s.postsByIDIncludingDeleted(postIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query invitation posts failed"})
		return
	}
	userMap, err := s.usersByIDsIncludingDeleted(userIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query invitation users failed"})
		return
	}

	views := make([]postInvitationView, 0, len(invitations))
	for _, invitation := range invitations {
		post, ok := postMap[invitation.PostID]
		if !ok {
			continue
		}
		inviter := userMap[invitation.InviterID]
		invitee := userMap[invitation.InviteeID]
		views = append(views, buildInvitationView(invitation, post, inviter, invitee))
	}
	c.JSON(http.StatusOK, gin.H{"invitations": views})
}

func buildInvitationView(invitation model.PostInvitation, post model.Post, inviter model.User, invitee model.User) postInvitationView {
	return postInvitationView{
		ID:          invitation.ID,
		Message:     invitation.Message,
		Status:      displayInvitationStatus(invitation, post),
		RespondedAt: invitation.RespondedAt,
		CreatedAt:   invitation.CreatedAt,
		UpdatedAt:   invitation.UpdatedAt,
		Inviter:     toUserBrief(inviter),
		Invitee:     toUserBrief(invitee),
		Post:        post,
	}
}

func displayInvitationStatus(invitation model.PostInvitation, post model.Post) string {
	if invitation.Status != model.InvitationStatusPending {
		return invitation.Status
	}
	if post.DeletedAt > 0 || post.CancelledAt > 0 || post.Status == "closed" {
		return model.InvitationStatusExpired
	}
	if post.MaxCount > 0 && post.CurrentCount >= post.MaxCount {
		return model.InvitationStatusExpired
	}
	return model.InvitationStatusPending
}

func (s *Server) AcceptInvitation(c *gin.Context) {
	userID := mustUserID(c)
	invitationID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()
	var responsePost model.Post

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var invitation model.PostInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invitation, "id = ?", invitationID).Error; err != nil {
			return err
		}
		if invitation.InviteeID != userID {
			return errors.New("no permission")
		}
		if invitation.Status != model.InvitationStatusPending {
			return errors.New("invitation already handled")
		}

		post, err := s.joinPostTx(tx, invitation.PostID, userID, now)
		if err != nil && !errors.Is(err, errJoinAlreadyJoined) {
			return err
		}
		if errors.Is(err, errJoinAlreadyJoined) {
			if reloadErr := tx.First(&post, "id = ?", invitation.PostID).Error; reloadErr != nil {
				return reloadErr
			}
		}

		invitation.Status = model.InvitationStatusAccepted
		invitation.RespondedAt = now
		invitation.UpdatedAt = now
		if err := tx.Save(&invitation).Error; err != nil {
			return err
		}
		responsePost = post
		return nil
	})
	if err != nil {
		writeInvitationActionError(c, err)
		return
	}

	s.invalidatePostsCache(c.Request.Context())
	s.rebuildUserTagsForUsers([]string{userID})
	s.pushRecommendationEvent(c.Request.Context(), "post_joined", map[string]any{
		"postId":       responsePost.ID,
		"userId":       userID,
		"authorId":     responsePost.AuthorID,
		"currentCount": responsePost.CurrentCount,
		"updatedAt":    responsePost.UpdatedAt,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "post": responsePost})
}

func (s *Server) RejectInvitation(c *gin.Context) {
	userID := mustUserID(c)
	invitationID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var invitation model.PostInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invitation, "id = ?", invitationID).Error; err != nil {
			return err
		}
		if invitation.InviteeID != userID {
			return errors.New("no permission")
		}
		if invitation.Status != model.InvitationStatusPending {
			return errors.New("invitation already handled")
		}
		invitation.Status = model.InvitationStatusRejected
		invitation.RespondedAt = now
		invitation.UpdatedAt = now
		return tx.Save(&invitation).Error
	})
	if err != nil {
		writeInvitationActionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) CancelInvitation(c *gin.Context) {
	userID := mustUserID(c)
	invitationID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var invitation model.PostInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invitation, "id = ?", invitationID).Error; err != nil {
			return err
		}
		if invitation.InviterID != userID {
			return errors.New("no permission")
		}
		if invitation.Status != model.InvitationStatusPending {
			return errors.New("invitation already handled")
		}
		invitation.Status = model.InvitationStatusCancelled
		invitation.RespondedAt = now
		invitation.UpdatedAt = now
		return tx.Save(&invitation).Error
	})
	if err != nil {
		writeInvitationActionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeInvitationActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation or post not found"})
	case errors.Is(err, errJoinAuthorOwnPost), errors.Is(err, errJoinPostClosed), errors.Is(err, errJoinPostFull):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, errJoinAlreadyJoined), err.Error() == "invitation already handled":
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err.Error() == "no permission":
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	default:
		serverError(c, err)
	}
}

func (s *Server) joinPostTx(tx *gorm.DB, postID, userID string, now int64) (model.Post, error) {
	var post model.Post
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&post, "id = ?", postID).Error; err != nil {
		return post, err
	}
	if post.AuthorID == userID {
		return post, errJoinAuthorOwnPost
	}
	if post.DeletedAt > 0 || post.CancelledAt > 0 || post.Status == "closed" {
		return post, errJoinPostClosed
	}

	var existed model.PostParticipant
	rejoining := false
	err := tx.First(&existed, "post_id = ? AND user_id = ?", postID, userID).Error
	switch {
	case err == nil:
		// A previously cancelled participant is allowed back in; only an
		// active relation counts as "already joined".
		if score.NormalizedParticipantStatus(existed.Status) == score.ParticipantStatusActive {
			return post, errJoinAlreadyJoined
		}
		rejoining = true
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return post, err
	}

	if post.CurrentCount >= post.MaxCount {
		return post, errJoinPostFull
	}

	if rejoining {
		if err := tx.Model(&model.PostParticipant{}).
			Where("post_id = ? AND user_id = ?", postID, userID).
			Updates(map[string]any{
				"status":       score.ParticipantStatusActive,
				"joined_at":    now,
				"cancelled_at": 0,
			}).Error; err != nil {
			return post, err
		}
		// Clear the settlement row left behind by the cancellation so the
		// rejoined participant starts from pending again.
		if err := tx.Model(&model.PostParticipantSettlement{}).
			Where("post_id = ? AND user_id = ?", postID, userID).
			Updates(map[string]any{
				"participant_decision":     "",
				"author_decision":          "",
				"final_status":             score.SettlementPending,
				"participant_note":         "",
				"participant_confirmed_at": 0,
				"settled_at":               0,
				"updated_at":               now,
			}).Error; err != nil {
			return post, err
		}
	} else if err := tx.Create(&model.PostParticipant{
		PostID:   postID,
		UserID:   userID,
		Status:   score.ParticipantStatusActive,
		JoinedAt: now,
	}).Error; err != nil {
		return post, err
	}

	// Conditional update rather than a read-modify-write: the SQLite driver
	// silently drops clause.Locking, so the row lock above is not enforced.
	result := tx.Model(&model.Post{}).
		Where("id = ? AND current_count < max_count", postID).
		Updates(map[string]any{
			"current_count": gorm.Expr("current_count + 1"),
			"updated_at":    now,
		})
	if result.Error != nil {
		return post, result.Error
	}
	if result.RowsAffected == 0 {
		return post, errJoinPostFull
	}

	post.CurrentCount++
	post.UpdatedAt = now
	return post, nil
}
