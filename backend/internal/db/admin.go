package db

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

const DefaultAdminNickname = "admin"

// EnsureDefaultAdmin backfills empty roles and bootstraps a root admin when
// the database has none. It never modifies existing accounts: promoting a
// user, resurrecting a soft-deleted admin, or resetting a password must be an
// explicit operator action, not a side effect of process startup.
func EnsureDefaultAdmin(database *gorm.DB) error {
	if err := database.Model(&model.User{}).
		Where("role = '' OR role IS NULL").
		Update("role", model.UserRoleUser).Error; err != nil {
		return err
	}

	var adminCount int64
	if err := database.Model(&model.User{}).
		Where("role = ? AND deleted_at = 0", model.UserRoleAdmin).
		Count(&adminCount).Error; err != nil {
		return err
	}
	if adminCount > 0 {
		return nil
	}

	var occupied model.User
	err := database.First(&occupied, "nickname = ?", DefaultAdminNickname).Error
	switch {
	case err == nil:
		log.Printf("no active admin exists but nickname %q is taken by user %s; create an admin manually (cmd/seed-admin or SQL)", DefaultAdminNickname, occupied.ID)
		return nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}

	password := strings.TrimSpace(os.Getenv("ADMIN_INIT_PASSWORD"))
	generated := false
	if password == "" {
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return err
		}
		password = hex.EncodeToString(random)
		generated = true
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	admin := model.User{
		ID:           "user_admin_" + uuid.NewString()[:8],
		Platform:     "password",
		OpenID:       "pwd_" + DefaultAdminNickname,
		Nickname:     DefaultAdminNickname,
		PasswordHash: string(hashed),
		AvatarURL:    "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(DefaultAdminNickname),
		Role:         model.UserRoleAdmin,
		RootAdmin:    true,
		CreditScore:  100,
		RatingScore:  5,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := database.Create(&admin).Error; err != nil {
		return err
	}
	if generated {
		log.Printf("created root admin %q with generated password: %s — log in and change it immediately (or set ADMIN_INIT_PASSWORD before first start)", DefaultAdminNickname, password)
	} else {
		log.Printf("created root admin %q with password from ADMIN_INIT_PASSWORD", DefaultAdminNickname)
	}
	return nil
}
