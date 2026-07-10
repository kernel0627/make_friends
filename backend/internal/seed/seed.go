package seed

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type Options struct {
	Reset           bool
	Users           int
	Posts           int
	MessagesPerPost int
}

type Result struct {
	Users            int
	BehaviorProfiles int
	Posts            int
	Participants     int
	Messages         int
	Reviews          int
	Exposures        int
	Clicks           int
}

func Run(db *gorm.DB, opt Options) (Result, error) {
	if opt.Users <= 0 {
		opt.Users = 20
	}
	if opt.Users < 3 {
		opt.Users = 3
	}
	if opt.Posts <= 0 {
		opt.Posts = 80
	}
	if opt.MessagesPerPost <= 0 {
		opt.MessagesPerPost = 5
	}

	if opt.Reset {
		if err := clearTables(db); err != nil {
			return Result{}, err
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return Result{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UnixMilli()
	users := buildSeedUsers(opt.Users, string(hash), now, false)
	if err := db.Create(&users).Error; err != nil {
		return Result{}, fmt.Errorf("create users: %w", err)
	}
	behaviorProfiles := buildSeedBehaviorProfiles(users, buildPersonaByUser(users), now)
	if len(behaviorProfiles) > 0 {
		if err := db.Create(&behaviorProfiles).Error; err != nil {
			return Result{}, fmt.Errorf("create user behavior profiles: %w", err)
		}
	}

	posts := make([]model.Post, 0, opt.Posts)
	participants := make([]model.PostParticipant, 0, opt.Posts*2)
	messages := make([]model.ChatMessage, 0, opt.Posts*opt.MessagesPerPost)
	reviews := make([]model.Review, 0, opt.Posts)
	closedPostIDs := make([]string, 0, opt.Posts/4)

	for i := 0; i < opt.Posts; i++ {
		author := users[i%len(users)]
		content := buildSeedActivity(i, author)
		postID := fmt.Sprintf("post_seed_%03d", i+1)
		createdAt := now + int64((i+1)*1000)

		post := model.Post{
			ID:           postID,
			AuthorID:     author.ID,
			Title:        content.Title,
			Description:  content.Description,
			Category:     content.Category,
			SubCategory:  content.SubCategory,
			TimeMode:     "range",
			TimeDays:     2 + (i % 14),
			FixedTime:    "",
			Address:      content.Address,
			Lat:          content.Lat,
			Lng:          content.Lng,
			MaxCount:     6 + (i % 4),
			CurrentCount: 3,
			Status:       "open",
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		}
		if i%3 == 0 {
			post.TimeMode = "fixed"
			post.TimeDays = 0
			post.FixedTime = futureFixedTime(i)
		}
		if i%10 == 0 {
			post.Status = "closed"
		}

		if err := db.Create(&post).Error; err != nil {
			return Result{}, fmt.Errorf("create post %s: %w", postID, err)
		}
		posts = append(posts, post)

		participantA := users[(i+1)%len(users)]
		participantB := users[(i+2)%len(users)]
		participants = append(participants,
			model.PostParticipant{PostID: postID, UserID: participantA.ID, JoinedAt: createdAt + 20},
			model.PostParticipant{PostID: postID, UserID: participantB.ID, JoinedAt: createdAt + 30},
		)

		messages = append(messages, buildSeedMessages(post, author, []model.User{participantA, participantB}, opt.MessagesPerPost, i)...)
		if post.Status == "closed" {
			reviews = append(reviews, buildSeedReview(postID, participantA.ID, author.ID, createdAt+40, i))
			closedPostIDs = append(closedPostIDs, postID)
		}
	}

	if len(participants) > 0 {
		if err := db.Create(&participants).Error; err != nil {
			return Result{}, fmt.Errorf("create participants: %w", err)
		}
	}
	if len(messages) > 0 {
		if err := db.Create(&messages).Error; err != nil {
			return Result{}, fmt.Errorf("create messages: %w", err)
		}
	}
	if len(reviews) > 0 {
		if err := db.Create(&reviews).Error; err != nil {
			return Result{}, fmt.Errorf("create reviews: %w", err)
		}
	}
	for _, postID := range closedPostIDs {
		if err := score.RecalculatePostActivityScores(db, postID, now); err != nil {
			return Result{}, fmt.Errorf("recalc activity scores for %s: %w", postID, err)
		}
	}

	return Result{
		Users:            len(users),
		BehaviorProfiles: len(behaviorProfiles),
		Posts:            len(posts),
		Participants:     len(participants),
		Messages:         len(messages),
		Reviews:          len(reviews),
	}, nil
}

func clearTables(db *gorm.DB) error {
	if err := db.Where("1 = 1").Delete(&model.UserEmbedding{}).Error; err != nil {
		return fmt.Errorf("clear user_embeddings: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.PostEmbedding{}).Error; err != nil {
		return fmt.Errorf("clear post_embeddings: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.UserTag{}).Error; err != nil {
		return fmt.Errorf("clear user_tags: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.UserBehaviorProfile{}).Error; err != nil {
		return fmt.Errorf("clear user_behavior_profiles: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.FeedClick{}).Error; err != nil {
		return fmt.Errorf("clear feed_clicks: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.FeedExposure{}).Error; err != nil {
		return fmt.Errorf("clear feed_exposures: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.RecommendationModel{}).Error; err != nil {
		return fmt.Errorf("clear recommendation_models: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.ActivityScore{}).Error; err != nil {
		return fmt.Errorf("clear activity_scores: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.Review{}).Error; err != nil {
		return fmt.Errorf("clear reviews: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.ChatMessage{}).Error; err != nil {
		return fmt.Errorf("clear chat_messages: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.PostParticipant{}).Error; err != nil {
		return fmt.Errorf("clear post_participants: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.Post{}).Error; err != nil {
		return fmt.Errorf("clear posts: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.User{}).Error; err != nil {
		return fmt.Errorf("clear users: %w", err)
	}
	return nil
}
