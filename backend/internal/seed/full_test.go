package seed

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"make_friends/backend/internal/model"
)

func TestRunFullSeed(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(
		&model.UserTag{},
		&model.UserBehaviorProfile{},
		&model.FeedExposure{},
		&model.FeedClick{},
		&model.PostEmbedding{},
		&model.UserEmbedding{},
		&model.RecommendationModel{},
		&model.PostParticipantSettlement{},
		&model.CreditLedger{},
		&model.AdminCase{},
		&model.RefreshToken{},
		&model.RevokedAccessToken{},
	); err != nil {
		t.Fatalf("migrate token tables failed: %v", err)
	}

	result, err := RunFull(db, FullOptions{Reset: true})
	if err != nil {
		t.Fatalf("run full seed failed: %v", err)
	}
	if result.Users < 80 || result.Posts < 500 || result.Exposures < 12000 || result.Clicks < 1500 {
		t.Fatalf("unexpected full seed result: %+v", result)
	}

	for _, nickname := range []string{"admin", "admin1", "admin2"} {
		var admin model.User
		if err := db.First(&admin, "nickname = ?", nickname).Error; err != nil {
			t.Fatalf("%s should exist: %v", nickname, err)
		}
		if admin.PasswordHash == "" {
			t.Fatalf("%s password hash should not be empty", nickname)
		}
		if admin.Role != model.UserRoleAdmin {
			t.Fatalf("%s role should be admin, got=%s", nickname, admin.Role)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("123456")); err != nil {
			t.Fatalf("%s hash verify failed: %v", nickname, err)
		}
	}

	var badCount int64
	if err := db.Model(&model.User{}).Where("credit_score <= 0 OR rating_score <= 0").Count(&badCount).Error; err != nil {
		t.Fatalf("count user score failed: %v", err)
	}
	if badCount > 0 {
		t.Fatalf("all users should have non-zero credit/rating, bad=%d", badCount)
	}
	var activeUserCount int64
	if err := db.Model(&model.User{}).Where("role = ? AND deleted_at = 0", model.UserRoleUser).Count(&activeUserCount).Error; err != nil {
		t.Fatalf("count active users failed: %v", err)
	}
	var profileCount int64
	if err := db.Model(&model.UserBehaviorProfile{}).Count(&profileCount).Error; err != nil {
		t.Fatalf("count behavior profiles failed: %v", err)
	}
	if profileCount != activeUserCount || int(profileCount) != result.BehaviorProfiles {
		t.Fatalf("behavior profile count mismatch active=%d table=%d result=%d", activeUserCount, profileCount, result.BehaviorProfiles)
	}

	var samplePost model.Post
	if err := db.First(&samplePost, "id = ?", "post_seed_001").Error; err != nil {
		t.Fatalf("seed post should exist: %v", err)
	}
	if strings.Contains(samplePost.Title, "Seed") || strings.Contains(samplePost.Address, "Seed") {
		t.Fatalf("seed post should contain concrete content, got title=%q address=%q", samplePost.Title, samplePost.Address)
	}

	var activityScoreCount int64
	if err := db.Model(&model.ActivityScore{}).Count(&activityScoreCount).Error; err != nil {
		t.Fatalf("count activity scores failed: %v", err)
	}
	if activityScoreCount == 0 {
		t.Fatalf("full seed should create activity score rows")
	}

	var exposureCount int64
	if err := db.Model(&model.FeedExposure{}).Count(&exposureCount).Error; err != nil {
		t.Fatalf("count exposures failed: %v", err)
	}
	if exposureCount < 12000 {
		t.Fatalf("full seed should create enough exposures, got=%d", exposureCount)
	}

	var clickCount int64
	if err := db.Model(&model.FeedClick{}).Count(&clickCount).Error; err != nil {
		t.Fatalf("count clicks failed: %v", err)
	}
	if clickCount < 1500 {
		t.Fatalf("full seed should create enough clicks, got=%d", clickCount)
	}
}
