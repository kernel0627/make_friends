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

func (s *Server) resolveUserRole(userID string) string {
	role, _, _, found := s.resolveUserAccess(userID)
	if !found {
		return model.UserRoleUser
	}
	return role
}

// resolveUserAccess returns the role/root-admin/deleted flags stored in the
// database. The caller must treat found == false as "no such user" — role
// claims from tokens or requests are never trusted here.
func (s *Server) resolveUserAccess(userID string) (string, bool, bool, bool) {
	if strings.TrimSpace(userID) == "" {
		return model.UserRoleUser, false, false, false
	}
	var user model.User
	if err := s.DB.Select("role", "root_admin", "deleted_at").First(&user, "id = ?", userID).Error; err != nil {
		return model.UserRoleUser, false, false, false
	}
	return model.NormalizeUserRole(user.Role), user.RootAdmin, user.DeletedAt > 0, true
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
