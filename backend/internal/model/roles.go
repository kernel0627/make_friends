package model

import "strings"

const (
	UserRoleUser  = "user"
	UserRoleAdmin = "admin"
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
