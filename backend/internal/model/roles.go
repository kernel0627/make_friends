package model

import "strings"

const (
	UserRoleUser  = "user"
	UserRoleAdmin = "admin"

	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"
	UserStatusBanned    = "banned"
)

func IsAdminRole(role string) bool {
	return NormalizeUserRole(role) == UserRoleAdmin
}

func NormalizeUserRole(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case UserRoleAdmin:
		return UserRoleAdmin
	default:
		return UserRoleUser
	}
}

// IsUserActive returns true if the user status allows normal operation.
func IsUserActive(status string) bool {
	s := strings.TrimSpace(strings.ToLower(status))
	return s == "" || s == UserStatusActive
}
