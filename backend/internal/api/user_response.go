package api

import (
	"strings"
	"time"

	"make_friends/backend/internal/model"
)

func normalizeUserModel(user *model.User) {
	if user == nil {
		return
	}
	user.Role = model.NormalizeUserRole(user.Role)
	if strings.TrimSpace(user.AvatarURL) == "" {
		user.AvatarURL = avatarURLFromSeed(user.ID)
	}
	if user.UpdatedAt == 0 {
		user.UpdatedAt = time.Now().UnixMilli()
	}
}
