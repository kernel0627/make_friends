package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

func (s *Server) RandomAvatar(c *gin.Context) {
	userID := mustUserID(c)
	if strings.TrimSpace(userID) == "" {
		fail(c, http.StatusUnauthorized, "AUTH_REQUIRED", "missing user identity")
		return
	}

	var user model.User
	if err := activeUsersQuery(s.DB).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
			return
		}
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	}

	user.AvatarURL = avatarURLFromSeed(fmt.Sprintf("%s_%d", userID, time.Now().UnixNano()))
	user.UpdatedAt = time.Now().UnixMilli()
	user.Role = model.NormalizeUserRole(user.Role)
	if err := s.DB.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "UPDATE_USER_FAILED", "update user failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (s *Server) GetUserHome(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}

	var user model.User
	if err := activeUsersQuery(s.DB).First(&user, "id = ?", userID).Error; err != nil {
		writeRecordError(c, err, "user not found", "query user failed")
		return
	}
	if strings.TrimSpace(user.AvatarURL) == "" {
		user.AvatarURL = avatarURLFromSeed(user.ID)
	}
	user.Role = model.NormalizeUserRole(user.Role)

	var initiatedPosts []model.Post
	// Author sees all their posts regardless of moderation status
	if err := s.DB.Where("deleted_at = 0 AND author_id = ?", userID).Order("created_at DESC").Find(&initiatedPosts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query initiated posts failed"})
		return
	}
	initiatedViews, err := s.buildHomePostViews(initiatedPosts, viewerID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build initiated posts failed"})
		return
	}

	var relations []model.PostParticipant
	if err := s.DB.Where("user_id = ?", userID).Order("joined_at DESC").Find(&relations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query joined posts failed"})
		return
	}
	joinedViews := make([]homePostView, 0)
	if len(relations) > 0 {
		postIDs := make([]string, 0, len(relations))
		for _, item := range relations {
			postIDs = append(postIDs, item.PostID)
		}
		var joinedPosts []model.Post
		// Participant sees posts they joined even if later taken down
		if err := s.DB.Where("deleted_at = 0 AND id IN ?", uniqueStrings(postIDs)).Order("created_at DESC").Find(&joinedPosts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query joined post records failed"})
			return
		}
		joinedViews, err = s.buildHomePostViews(joinedPosts, viewerID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "build joined posts failed"})
			return
		}
	}

	interestTags, err := s.interestTagsForUser(userID, 3)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query interest tags failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":           user,
		"initiatedPosts": initiatedViews,
		"joinedPosts":    joinedViews,
		"interestTags":   interestTags,
	})
}

func (s *Server) buildHomePostViews(posts []model.Post, viewerID, subjectUserID string) ([]homePostView, error) {
	if len(posts) == 0 {
		return []homePostView{}, nil
	}
	if err := s.refreshClosedPostsDerivedState(posts); err != nil {
		return nil, err
	}

	baseViews, err := s.buildPostViewsForViewer(posts, viewerID)
	if err != nil {
		return nil, err
	}

	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}
	postIDs = uniqueStrings(postIDs)

	relationMap, err := s.participantsByPost(postIDs)
	if err != nil {
		return nil, err
	}
	reviewsByPost, err := s.reviewsByPost(postIDs)
	if err != nil {
		return nil, err
	}
	activityByPost, err := s.activityScoresByPost(postIDs, subjectUserID)
	if err != nil {
		return nil, err
	}
	chatPreviewByPost, err := s.latestChatPreviewByPost(postIDs)
	if err != nil {
		return nil, err
	}
	settlementMap, err := s.settlementsByPost(postIDs)
	if err != nil {
		return nil, err
	}

	result := make([]homePostView, 0, len(baseViews))
	for _, view := range baseViews {
		relations := relationMap[view.ID]
		settlements := settlementMap[view.ID]
		reviewState := buildReviewState(view.Post, subjectUserID, relations, settlements, reviewsByPost[view.ID])

		activityScore := activityScoreView{}
		if scoreRow, ok := activityByPost[view.ID]; ok {
			activityScore = activityScoreView{
				CreditScore: scoreRow.CreditScore,
				RatingScore: scoreRow.RatingScore,
				RatingCount: scoreRow.RatingCount,
			}
		}

		result = append(result, homePostView{
			postView:        view,
			ReviewState:     reviewState,
			ActivityScore:   activityScore,
			ChatPreview:     chatPreviewByPost[view.ID],
			SettlementState: buildHomeSettlementState(view.Post, subjectUserID, relations, settlements, reviewState),
		})
	}

	sortHomePostViews(result)
	return result, nil
}

func (s *Server) participantsByPost(postIDs []string) (map[string][]model.PostParticipant, error) {
	if len(postIDs) == 0 {
		return map[string][]model.PostParticipant{}, nil
	}
	var relations []model.PostParticipant
	if err := s.DB.Where("post_id IN ?", postIDs).Find(&relations).Error; err != nil {
		return nil, err
	}
	return groupByStringKey(relations, func(relation model.PostParticipant) string {
		return relation.PostID
	}), nil
}

func (s *Server) reviewsByPost(postIDs []string) (map[string][]model.Review, error) {
	if len(postIDs) == 0 {
		return map[string][]model.Review{}, nil
	}
	var reviews []model.Review
	if err := s.DB.Where("post_id IN ?", postIDs).Find(&reviews).Error; err != nil {
		return nil, err
	}
	return groupByStringKey(reviews, func(review model.Review) string {
		return review.PostID
	}), nil
}

func (s *Server) activityScoresByPost(postIDs []string, userID string) (map[string]model.ActivityScore, error) {
	targetUserID := strings.TrimSpace(userID)
	if len(postIDs) == 0 || targetUserID == "" {
		return map[string]model.ActivityScore{}, nil
	}
	var rows []model.ActivityScore
	if err := s.DB.Where("post_id IN ? AND user_id = ?", postIDs, targetUserID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return indexByStringKey(rows, func(row model.ActivityScore) string {
		return row.PostID
	}), nil
}

func (s *Server) settlementsByPost(postIDs []string) (map[string]map[string]model.PostParticipantSettlement, error) {
	if len(postIDs) == 0 {
		return map[string]map[string]model.PostParticipantSettlement{}, nil
	}
	var rows []model.PostParticipantSettlement
	if err := s.DB.Where("post_id IN ?", postIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	return nestedIndexByStringKey(rows, func(row model.PostParticipantSettlement) string {
		return row.PostID
	}, func(row model.PostParticipantSettlement) string {
		return row.UserID
	}), nil
}

func (s *Server) latestChatPreviewByPost(postIDs []string) (map[string]chatPreviewView, error) {
	if len(postIDs) == 0 {
		return map[string]chatPreviewView{}, nil
	}

	var messages []model.ChatMessage
	if err := s.DB.Where("post_id IN ?", postIDs).Order("created_at DESC").Find(&messages).Error; err != nil {
		return nil, err
	}

	latestByPost := firstByStringKey(messages, func(msg model.ChatMessage) string {
		return msg.PostID
	})
	senderIDs := make([]string, 0, len(latestByPost))
	for _, msg := range latestByPost {
		senderIDs = append(senderIDs, msg.SenderID)
	}

	senderMap, err := s.usersByIDs(senderIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[string]chatPreviewView, len(latestByPost))
	for postID, msg := range latestByPost {
		var sender *userBrief
		if user, ok := senderMap[msg.SenderID]; ok {
			brief := toUserBrief(user)
			sender = &brief
		}
		result[postID] = chatPreviewView{
			LatestMessage:       msg.Content,
			LatestMessageAt:     msg.CreatedAt,
			LatestMessageSender: sender,
		}
	}
	return result, nil
}

func buildReviewState(
	post model.Post,
	subjectUserID string,
	relations []model.PostParticipant,
	settlements map[string]model.PostParticipantSettlement,
	reviews []model.Review,
) reviewStateView {
	state := reviewStateView{StatusText: "进行中"}
	subjectID := strings.TrimSpace(subjectUserID)
	if subjectID == "" {
		return state
	}
	if post.CancelledAt > 0 {
		state.StatusText = "活动已取消"
		return state
	}
	if post.Status != "closed" {
		return state
	}

	authorID := strings.TrimSpace(post.AuthorID)
	if subjectID == authorID {
		blockingSettlement := hasAuthorSettlementWork(relations, settlements)
		eligibleParticipants := make(map[string]struct{})
		for _, relation := range relations {
			participantID := strings.TrimSpace(relation.UserID)
			if participantID == "" || participantID == subjectID {
				continue
			}
			if strings.TrimSpace(settlements[participantID].FinalStatus) != score.SettlementCompleted {
				continue
			}
			eligibleParticipants[participantID] = struct{}{}
		}

		totalStars := 0
		for _, review := range reviews {
			if strings.TrimSpace(review.FromUserID) != subjectID {
				continue
			}
			if _, ok := eligibleParticipants[strings.TrimSpace(review.ToUserID)]; !ok {
				continue
			}
			state.ReviewedCount++
			totalStars += review.Rating
		}
		state.PendingCount = len(eligibleParticipants) - state.ReviewedCount
		if state.PendingCount < 0 {
			state.PendingCount = 0
		}
		state.CanReview = !blockingSettlement && state.PendingCount > 0
		if state.ReviewedCount > 0 {
			state.AverageStars = float64(totalStars) / float64(state.ReviewedCount)
			state.MyStars = state.AverageStars
		}
		switch {
		case blockingSettlement:
			state.StatusText = fmt.Sprintf("待履约 %d 人", pendingSettlementCount(relations, settlements))
		case state.PendingCount > 0:
			state.StatusText = fmt.Sprintf("待评分 %d 人", state.PendingCount)
		case state.ReviewedCount > 0:
			state.StatusText = fmt.Sprintf("已评价 %d 人，平均 %.1f 星", state.ReviewedCount, state.AverageStars)
		default:
			state.StatusText = "已完成"
		}
		return state
	}

	row := settlements[subjectID]
	finalStatus := strings.TrimSpace(row.FinalStatus)
	switch finalStatus {
	case score.SettlementCancelled:
		state.StatusText = "活动已取消"
		return state
	case score.SettlementNoShow:
		state.StatusText = "未到场"
		return state
	case score.SettlementDisputed:
		state.StatusText = "活动异常待处理"
		return state
	case score.SettlementCompleted:
	default:
		state.StatusText = "待履约确认"
		return state
	}

	state.PendingCount = 1
	for _, review := range reviews {
		if strings.TrimSpace(review.FromUserID) != subjectID || strings.TrimSpace(review.ToUserID) != authorID {
			continue
		}
		state.ReviewedCount = 1
		state.PendingCount = 0
		state.MyStars = float64(review.Rating)
		state.AverageStars = float64(review.Rating)
		break
	}
	state.CanReview = state.PendingCount > 0
	if state.CanReview {
		state.StatusText = "待评分"
		return state
	}
	if state.MyStars > 0 {
		state.StatusText = fmt.Sprintf("我给了 %.0f 星", state.MyStars)
		return state
	}
	state.StatusText = "已完成"
	return state
}

func sortHomePostViews(posts []homePostView) {
	sort.SliceStable(posts, func(i, j int) bool {
		left := posts[i]
		right := posts[j]
		leftBucket := homeSortBucket(left)
		rightBucket := homeSortBucket(right)
		if leftBucket != rightBucket {
			return leftBucket < rightBucket
		}

		if leftBucket == 2 {
			leftFixed, leftOK := homeFixedTime(left.Post)
			rightFixed, rightOK := homeFixedTime(right.Post)
			if leftOK && rightOK && leftFixed != rightFixed {
				return leftFixed < rightFixed
			}
			if leftOK != rightOK {
				return leftOK
			}
		}

		leftChatAt := left.ChatPreview.LatestMessageAt
		rightChatAt := right.ChatPreview.LatestMessageAt
		if leftChatAt != rightChatAt {
			return leftChatAt > rightChatAt
		}
		if left.UpdatedAt != right.UpdatedAt {
			return left.UpdatedAt > right.UpdatedAt
		}
		return left.CreatedAt > right.CreatedAt
	})
}

func homeSortBucket(post homePostView) int {
	if post.SettlementState.ProjectCancelled {
		return 3
	}
	if post.SettlementState.HasDispute || post.SettlementState.CanAuthorConfirm || post.SettlementState.CanParticipantConfirm {
		return 0
	}
	if post.ReviewState.CanReview {
		return 0
	}
	if post.Status != "closed" {
		return 1
	}
	return 2
}

func homeFixedTime(post model.Post) (int64, bool) {
	if strings.TrimSpace(post.FixedTime) == "" {
		return 0, false
	}
	ts, err := parseFixedTimeToMS(post.FixedTime)
	if err != nil {
		return 0, false
	}
	return ts, true
}
