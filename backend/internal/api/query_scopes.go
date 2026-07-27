package api

import (
	"strings"

	"gorm.io/gorm"
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
		return query.Where(column + " = 0")
	case "deleted":
		return query.Where(column + " > 0")
	case "all":
		return query
	default:
		return query.Where(column + " = 0")
	}
}
