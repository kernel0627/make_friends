package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"make_friends/backend/internal/model"
)

const contextUserRoleKey = "userRole"
const contextUserRootAdminKey = "userRootAdmin"

func mustUserRole(c *gin.Context) string {
	raw, ok := c.Get(contextUserRoleKey)
	if !ok {
		return model.UserRoleUser
	}
	role, _ := raw.(string)
	return model.NormalizeUserRole(role)
}

func mustUserRootAdmin(c *gin.Context) bool {
	raw, ok := c.Get(contextUserRootAdminKey)
	if !ok {
		return false
	}
	rootAdmin, _ := raw.(bool)
	return rootAdmin
}

func optionalUserRoleFromRequest(c *gin.Context, jwtSecret string) string {
	_, role, _, ok := userIDFromRequest(c, jwtSecret)
	if !ok {
		return ""
	}
	return model.NormalizeUserRole(role)
}

func (s *Server) resolveUserRole(userID, claimedRole string) string {
	role, _, _ := s.resolveUserAccess(userID, claimedRole)
	return role
}

func (s *Server) resolveUserAccess(userID, claimedRole string) (string, bool, bool) {
	role := model.NormalizeUserRole(claimedRole)
	if strings.TrimSpace(userID) == "" {
		return role, false, false
	}
	var user model.User
	if err := s.DB.Select("role", "root_admin", "deleted_at").First(&user, "id = ?", userID).Error; err == nil {
		return model.NormalizeUserRole(user.Role), user.RootAdmin, user.DeletedAt > 0
	}
	return role, false, false
}

func (s *Server) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if mustUserRole(c) != model.UserRoleAdmin {
			fail(c, 403, "FORBIDDEN", "admin only")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) RequireRootAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if mustUserRole(c) != model.UserRoleAdmin || !mustUserRootAdmin(c) {
			fail(c, 403, "FORBIDDEN", "root admin only")
			c.Abort()
			return
		}
		c.Next()
	}
}
