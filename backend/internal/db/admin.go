package db

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

const (
	DefaultAdminNickname = "admin"
	DefaultAdminPassword = "123456"
)

var DefaultAdminNicknames = []string{"admin", "admin1", "admin2"}

func EnsureDefaultAdmin(database *gorm.DB) error {
	if err := database.Model(&model.User{}).
		Where("role = '' OR role IS NULL").
		Update("role", model.UserRoleUser).Error; err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(DefaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	for _, nickname := range DefaultAdminNicknames {
		isRootAdmin := nickname == DefaultAdminNickname
		var user model.User
		err := database.First(&user, "nickname = ?", nickname).Error
		switch {
		case err == nil:
			updates := map[string]any{
				"role":        model.UserRoleAdmin,
				"root_admin":  isRootAdmin,
				"deleted_at":  0,
				"deleted_by":  "",
				"updated_at":  time.Now().UnixMilli(),
			}
			if strings.TrimSpace(user.PasswordHash) == "" {
				updates["password_hash"] = string(hashed)
			}
			if strings.TrimSpace(user.AvatarURL) == "" {
				updates["avatar_url"] = "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(nickname)
			}
			if err := database.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			now := time.Now().UnixMilli()
			admin := model.User{
				ID:           "user_admin_" + uuid.NewString()[:8],
				Platform:     "password",
				OpenID:       "pwd_" + nickname,
				Nickname:     nickname,
				PasswordHash: string(hashed),
				AvatarURL:    "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(nickname),
				Role:         model.UserRoleAdmin,
				RootAdmin:    isRootAdmin,
				CreditScore:  100,
				RatingScore:  5,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := database.Create(&admin).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}
