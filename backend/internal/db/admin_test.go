package db

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func openAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := database.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return database
}

func TestEnsureDefaultAdminIdempotent(t *testing.T) {
	database := openAdminTestDB(t)

	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("first EnsureDefaultAdmin failed: %v", err)
	}
	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("second EnsureDefaultAdmin failed: %v", err)
	}

	var users []model.User
	if err := database.Find(&users, "nickname IN ?", DefaultAdminNicknames).Error; err != nil {
		t.Fatalf("query admins failed: %v", err)
	}
	if len(users) != len(DefaultAdminNicknames) {
		t.Fatalf("expect %d admins, got=%d", len(DefaultAdminNicknames), len(users))
	}
	for _, admin := range users {
		if admin.PasswordHash == "" {
			t.Fatalf("admin password hash should not be empty")
		}
		if admin.Role != model.UserRoleAdmin {
			t.Fatalf("admin role should be admin, got=%s", admin.Role)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(DefaultAdminPassword)); err != nil {
			t.Fatalf("admin password hash mismatch: %v", err)
		}
	}
}
