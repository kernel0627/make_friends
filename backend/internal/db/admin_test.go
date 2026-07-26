package db

import (
	"fmt"
	"testing"
	"time"

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

func TestEnsureDefaultAdminBootstrapsOnce(t *testing.T) {
	database := openAdminTestDB(t)
	t.Setenv("ADMIN_INIT_PASSWORD", "bootstrap_pw_1")

	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("first EnsureDefaultAdmin failed: %v", err)
	}
	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("second EnsureDefaultAdmin failed: %v", err)
	}

	var admins []model.User
	if err := database.Find(&admins, "role = ?", model.UserRoleAdmin).Error; err != nil {
		t.Fatalf("query admins failed: %v", err)
	}
	if len(admins) != 1 {
		t.Fatalf("expect exactly 1 admin, got=%d", len(admins))
	}
	admin := admins[0]
	if admin.Nickname != DefaultAdminNickname {
		t.Fatalf("expect nickname %q, got=%q", DefaultAdminNickname, admin.Nickname)
	}
	if !admin.RootAdmin {
		t.Fatalf("bootstrap admin should be root admin")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("bootstrap_pw_1")); err != nil {
		t.Fatalf("admin password hash mismatch: %v", err)
	}
}

func TestEnsureDefaultAdminNeverTouchesExistingAccounts(t *testing.T) {
	database := openAdminTestDB(t)
	now := time.Now().UnixMilli()

	deletedAdmin := model.User{
		ID: "user_deleted_admin", Platform: "password", OpenID: "pwd_deleted",
		Nickname: "old_admin", Role: model.UserRoleAdmin, RootAdmin: true,
		DeletedAt: now, DeletedBy: "user_x", CreatedAt: now, UpdatedAt: now,
	}
	squatter := model.User{
		ID: "user_squatter", Platform: "password", OpenID: "pwd_squatter",
		Nickname: DefaultAdminNickname, Role: model.UserRoleUser,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&deletedAdmin).Error; err != nil {
		t.Fatalf("seed deleted admin failed: %v", err)
	}
	if err := database.Create(&squatter).Error; err != nil {
		t.Fatalf("seed squatter failed: %v", err)
	}

	err := EnsureDefaultAdmin(database)
	if err == nil {
		t.Fatalf("nickname conflict with no active admin must fail loudly")
	}

	var gotDeleted model.User
	if err := database.First(&gotDeleted, "id = ?", deletedAdmin.ID).Error; err != nil {
		t.Fatalf("reload deleted admin failed: %v", err)
	}
	if gotDeleted.DeletedAt == 0 {
		t.Fatalf("soft-deleted admin must not be resurrected")
	}
	var gotSquatter model.User
	if err := database.First(&gotSquatter, "id = ?", squatter.ID).Error; err != nil {
		t.Fatalf("reload squatter failed: %v", err)
	}
	if gotSquatter.Role != model.UserRoleUser {
		t.Fatalf("user holding the admin nickname must not be promoted, got role=%q", gotSquatter.Role)
	}
	var adminCount int64
	if err := database.Model(&model.User{}).Where("role = ? AND deleted_at = 0", model.UserRoleAdmin).Count(&adminCount).Error; err != nil {
		t.Fatalf("count admins failed: %v", err)
	}
	if adminCount != 0 {
		t.Fatalf("no admin should be created while nickname is occupied, got=%d", adminCount)
	}
}

func TestEnsureDefaultAdminRecoversWithConfiguredNickname(t *testing.T) {
	database := openAdminTestDB(t)
	now := time.Now().UnixMilli()
	t.Setenv("ADMIN_INIT_NICKNAME", "ops_admin")
	t.Setenv("ADMIN_INIT_PASSWORD", "bootstrap_pw_2")

	squatter := model.User{
		ID: "user_squatter", Platform: "password", OpenID: "pwd_squatter",
		Nickname: DefaultAdminNickname, Role: model.UserRoleUser,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&squatter).Error; err != nil {
		t.Fatalf("seed squatter failed: %v", err)
	}

	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("configured nickname should recover bootstrap: %v", err)
	}
	if err := EnsureDefaultAdmin(database); err != nil {
		t.Fatalf("second bootstrap should remain idempotent: %v", err)
	}

	var admin model.User
	if err := database.First(&admin, "role = ? AND deleted_at = 0", model.UserRoleAdmin).Error; err != nil {
		t.Fatalf("load configured admin failed: %v", err)
	}
	if admin.Nickname != "ops_admin" {
		t.Fatalf("expected configured nickname, got %q", admin.Nickname)
	}
	if !admin.RootAdmin {
		t.Fatalf("configured bootstrap account should be root admin")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("bootstrap_pw_2")); err != nil {
		t.Fatalf("configured admin password hash mismatch: %v", err)
	}

	var gotSquatter model.User
	if err := database.First(&gotSquatter, "id = ?", squatter.ID).Error; err != nil {
		t.Fatalf("reload squatter failed: %v", err)
	}
	if gotSquatter.Role != model.UserRoleUser {
		t.Fatalf("existing account must not be promoted, got role %q", gotSquatter.Role)
	}
}
