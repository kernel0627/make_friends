package seed

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.PostParticipant{},
		&model.ChatMessage{},
		&model.Review{},
		&model.ActivityScore{},
		&model.PostParticipantSettlement{},
		&model.CreditLedger{},
		&model.AdminCase{},
		&model.UserTag{},
		&model.UserBehaviorProfile{},
		&model.FeedExposure{},
		&model.FeedClick{},
		&model.PostEmbedding{},
		&model.UserEmbedding{},
		&model.RecommendationModel{},
	); err != nil {
		t.Fatalf("migrate test db failed: %v", err)
	}
	return db
}

func TestRunSeed(t *testing.T) {
	db := openTestDB(t)

	result, err := Run(db, Options{
		Reset:           true,
		Users:           4,
		Posts:           3,
		MessagesPerPost: 2,
	})
	if err != nil {
		t.Fatalf("run seed failed: %v", err)
	}

	if result.Users != 4 {
		t.Fatalf("expect users=4, got=%d", result.Users)
	}
	if result.Posts != 3 {
		t.Fatalf("expect posts=3, got=%d", result.Posts)
	}
	if result.Messages != 6 {
		t.Fatalf("expect messages=6, got=%d", result.Messages)
	}

	var userCount int64
	var postCount int64
	var msgCount int64
	var profileCount int64
	if err := db.Model(&model.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count user failed: %v", err)
	}
	if err := db.Model(&model.Post{}).Count(&postCount).Error; err != nil {
		t.Fatalf("count post failed: %v", err)
	}
	if err := db.Model(&model.ChatMessage{}).Count(&msgCount).Error; err != nil {
		t.Fatalf("count message failed: %v", err)
	}
	if err := db.Model(&model.UserBehaviorProfile{}).Count(&profileCount).Error; err != nil {
		t.Fatalf("count behavior profiles failed: %v", err)
	}

	if userCount != 4 || postCount != 3 || msgCount != 6 {
		t.Fatalf("unexpected counts users=%d posts=%d messages=%d", userCount, postCount, msgCount)
	}
	if result.BehaviorProfiles != 4 || profileCount != 4 {
		t.Fatalf("expected one behavior profile per seed user, result=%d table=%d", result.BehaviorProfiles, profileCount)
	}
	backfilled, err := BackfillBehaviorProfiles(db, 1234567890)
	if err != nil {
		t.Fatalf("backfill behavior profiles failed: %v", err)
	}
	if backfilled != 0 {
		t.Fatalf("all test users are deleted, expected no active backfill profiles, got=%d", backfilled)
	}

	var firstPost model.Post
	if err := db.First(&firstPost, "id = ?", "post_seed_001").Error; err != nil {
		t.Fatalf("first seeded post should exist: %v", err)
	}
	if strings.Contains(firstPost.Title, "Seed") || strings.Contains(firstPost.Description, "Generated activity data") {
		t.Fatalf("seed content should be concrete, got title=%q description=%q", firstPost.Title, firstPost.Description)
	}
}
