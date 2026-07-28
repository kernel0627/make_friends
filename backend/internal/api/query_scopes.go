package api

import (
	"strings"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func activeUsersQuery(db *gorm.DB) *gorm.DB {
	return db.Where("users.deleted_at = 0")
}

func activePostsQuery(db *gorm.DB) *gorm.DB {
	// Empty moderation status is treated as approved for posts created before
	// moderation was introduced; newly written posts are explicitly pending.
	return db.Where("posts.deleted_at = 0 AND (posts.moderation_status = '' OR posts.moderation_status = ?)", "approved")
}

func applySoftDeleteStatus(query *gorm.DB, column string, raw string) *gorm.DB {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "active":
		return query.Where(column+" = 0 AND (status = '' OR status = ?)", model.UserStatusActive)
	case "deleted":
		return query.Where(column + " > 0")
	case "suspended":
		return query.Where(column+" = 0 AND status = ?", model.UserStatusSuspended)
	case "banned":
		return query.Where(column+" = 0 AND status = ?", model.UserStatusBanned)
	case "all":
		return query
	default:
		return query.Where(column + " = 0")
	}
}
