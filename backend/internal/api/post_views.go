package api

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

const (
	dicebearAvatarTemplate = "https://api.dicebear.com/7.x/avataaars/svg?seed=%s"
	recentWindowHours      = 48.0
	recentMaxBoost         = 5.0
)

type userBrief struct {
	ID          string  `json:"id"`
	Nickname    string  `json:"nickName"`
	AvatarURL   string  `json:"avatarUrl"`
	CreditScore int     `json:"creditScore"`
	RatingScore float64 `json:"ratingScore"`
}

type postView struct {
	model.Post
	Author         userBrief          `json:"author"`
	ViewerIsAuthor bool               `json:"viewerIsAuthor"`
	ViewerJoined   bool               `json:"viewerJoined"`
	Recommendation recommendationView `json:"recommendation"`
}

type reviewStateView struct {
	CanReview     bool    `json:"canReview"`
	PendingCount  int     `json:"pendingCount"`
	ReviewedCount int     `json:"reviewedCount"`
	MyStars       float64 `json:"myStars"`
	AverageStars  float64 `json:"averageStars"`
	StatusText    string  `json:"statusText"`
}

type activityScoreView struct {
	CreditScore int     `json:"creditScore"`
	RatingScore float64 `json:"ratingScore"`
	RatingCount int     `json:"ratingCount"`
}

type chatPreviewView struct {
	LatestMessage       string     `json:"latestMessage"`
	LatestMessageAt     int64      `json:"latestMessageAt"`
	LatestMessageSender *userBrief `json:"latestMessageSender,omitempty"`
}

type homePostView struct {
	postView
	ReviewState     reviewStateView     `json:"reviewState"`
	ActivityScore   activityScoreView   `json:"activityScore"`
	ChatPreview     chatPreviewView     `json:"chatPreview"`
	SettlementState settlementStateView `json:"settlementState"`
}

type chatMessageView struct {
	ID          string    `json:"id"`
	PostID      string    `json:"postId"`
	SenderID    string    `json:"senderId"`
	Sender      userBrief `json:"sender"`
	Content     string    `json:"content"`
	ClientMsgID string    `json:"clientMsgId"`
	CreatedAt   int64     `json:"createdAt"`
}

func avatarURLFromSeed(seed string) string {
	rawSeed := strings.TrimSpace(seed)
	if rawSeed == "" {
		rawSeed = "default"
	}
	return fmt.Sprintf(dicebearAvatarTemplate, url.QueryEscape(rawSeed))
}

func toUserBrief(u model.User) userBrief {
	return userBrief{
		ID:          u.ID,
		Nickname:    u.Nickname,
		AvatarURL:   strings.TrimSpace(u.AvatarURL),
		CreditScore: u.CreditScore,
		RatingScore: u.RatingScore,
	}
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, item := range input {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (s *Server) usersByIDs(ids []string) (map[string]model.User, error) {
	return s.usersByIDsInternal(ids, false)
}

func (s *Server) usersByIDsIncludingDeleted(ids []string) (map[string]model.User, error) {
	return s.usersByIDsInternal(ids, true)
}

func (s *Server) usersByIDsInternal(ids []string, includeDeleted bool) (map[string]model.User, error) {
	uniqueIDs := uniqueStrings(ids)
	if len(uniqueIDs) == 0 {
		return map[string]model.User{}, nil
	}

	query := s.DB
	if !includeDeleted {
		query = activeUsersQuery(query)
	}

	var users []model.User
	if err := query.Find(&users, "id IN ?", uniqueIDs).Error; err != nil {
		return nil, err
	}

	for idx := range users {
		normalizeUserModel(&users[idx])
	}
	return indexByStringKey(users, func(user model.User) string {
		return user.ID
	}), nil
}

func (s *Server) buildPostViews(posts []model.Post) ([]postView, error) {
	return s.buildPostViewsForViewer(posts, "")
}

func (s *Server) buildPostViewsForViewer(posts []model.Post, viewerID string) ([]postView, error) {
	if len(posts) == 0 {
		return []postView{}, nil
	}
	authorIDs := make([]string, 0, len(posts))
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		authorIDs = append(authorIDs, post.AuthorID)
		postIDs = append(postIDs, post.ID)
	}
	userMap, err := s.usersByIDs(authorIDs)
	if err != nil {
		return nil, err
	}

	joinedByPost := make(map[string]bool)
	if strings.TrimSpace(viewerID) != "" {
		var relations []model.PostParticipant
		if err := s.DB.Where("user_id = ? AND post_id IN ? AND status = ?", viewerID, uniqueStrings(postIDs), score.ParticipantStatusActive).Find(&relations).Error; err != nil {
			return nil, err
		}
		for _, relation := range relations {
			joinedByPost[relation.PostID] = true
		}
	}

	out := make([]postView, 0, len(posts))
	for _, post := range posts {
		author, ok := userMap[post.AuthorID]
		if !ok {
			author = model.User{
				ID:          post.AuthorID,
				Nickname:    "\u53d1\u8d77\u4eba",
				AvatarURL:   avatarURLFromSeed(post.AuthorID),
				CreditScore: 100,
				RatingScore: 5,
			}
		}
		out = append(out, postView{
			Post:           post,
			Author:         toUserBrief(author),
			ViewerIsAuthor: strings.TrimSpace(viewerID) != "" && viewerID == post.AuthorID,
			ViewerJoined:   joinedByPost[post.ID],
		})
	}
	return out, nil
}

func postHotScore(post model.Post, nowMS int64) float64 {
	ageMS := nowMS - post.CreatedAt
	if ageMS < 0 {
		ageMS = 0
	}

	recencyBoost := 0.0
	windowMS := int64(recentWindowHours * float64(time.Hour/time.Millisecond))
	if ageMS < windowMS {
		ageHours := float64(ageMS) / float64(time.Hour/time.Millisecond)
		recencyBoost = ((recentWindowHours - ageHours) / recentWindowHours) * recentMaxBoost
		if recencyBoost < 0 {
			recencyBoost = 0
		}
	}

	return float64(post.CurrentCount)*2 + recencyBoost
}

func sortPostsByHotScore(posts []model.Post, nowMS int64) {
	sort.SliceStable(posts, func(i, j int) bool {
		left := postHotScore(posts[i], nowMS)
		right := postHotScore(posts[j], nowMS)
		if left == right {
			return posts[i].CreatedAt > posts[j].CreatedAt
		}
		return left > right
	})
}

var fixedTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
}

func parseFixedTimeToMS(raw string) (int64, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, errors.New("fixedTime is empty")
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		// 10-digit seconds timestamp compatibility
		if n > 0 && n < 1_000_000_000_000 {
			return n * 1000, nil
		}
		return n, nil
	}

	for _, layout := range fixedTimeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, errors.New("invalid fixedTime format")
}

func validateTimeInfo(info timeInfoReq, nowMS int64) error {
	mode := strings.ToLower(strings.TrimSpace(info.Mode))
	switch mode {
	case "fixed":
		ts, err := parseFixedTimeToMS(info.FixedTime)
		if err != nil {
			return errors.New("fixedTime format invalid")
		}
		if ts <= nowMS {
			return errors.New("fixedTime must be greater than current timestamp")
		}
		return nil
	case "range":
		if info.Days < 1 || info.Days > 30 {
			return errors.New("time range days must be between 1 and 30")
		}
		return nil
	default:
		if mode == "" {
			return errors.New("time mode required")
		}
		// Keep backward compatibility for historical modes such as "weekend".
		return nil
	}
}

func (s *Server) buildChatMessageViews(messages []model.ChatMessage) ([]chatMessageView, error) {
	if len(messages) == 0 {
		return []chatMessageView{}, nil
	}
	senderIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		senderIDs = append(senderIDs, msg.SenderID)
	}
	senderMap, err := s.usersByIDs(senderIDs)
	if err != nil {
		return nil, err
	}

	views := make([]chatMessageView, 0, len(messages))
	for _, msg := range messages {
		sender, ok := senderMap[msg.SenderID]
		if !ok {
			sender = model.User{
				ID:          msg.SenderID,
				Nickname:    "\u53d1\u8d77\u4eba",
				AvatarURL:   avatarURLFromSeed(msg.SenderID),
				CreditScore: 100,
				RatingScore: 5,
			}
		}
		views = append(views, chatMessageView{
			ID:          msg.ID,
			PostID:      msg.PostID,
			SenderID:    msg.SenderID,
			Sender:      toUserBrief(sender),
			Content:     msg.Content,
			ClientMsgID: msg.ClientMsgID,
			CreatedAt:   msg.CreatedAt,
		})
	}
	return views, nil
}
