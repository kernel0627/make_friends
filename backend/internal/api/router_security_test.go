package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"make_friends/backend/internal/auth"
	"make_friends/backend/internal/model"
)

// TestHeaderIdentityIsRejected asserts the old X-User-ID / X-User-Role bypass
// is gone: header-only identity must not authenticate a request.
func TestHeaderIdentityIsRejected(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_victim")
	router := NewRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("X-User-ID", "user_victim")
	req.Header.Set("X-User-Role", "admin")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("header identity must be rejected, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestForgedAdminRoleClaimIsIgnored asserts admin authorization comes from the
// database, not from the role embedded in the JWT: a normal user carrying a
// token minted with role=admin must still be forbidden.
func TestForgedAdminRoleClaimIsIgnored(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_normal")
	token, err := auth.SignToken("user_normal", testJWTSecret, 1, model.UserRoleAdmin)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	router := NewRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("forged admin role claim must be forbidden, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestTokenForMissingUserIsRejected asserts a validly-signed token whose user
// row does not exist is rejected (resolveUserAccess no longer falls back to
// claimed identity).
func TestTokenForMissingUserIsRejected(t *testing.T) {
	db := openRouterTestDB(t)
	token, err := auth.SignToken("user_ghost", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	router := NewRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("token for missing user must be rejected, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestRealAdminReachesAdminRoutes is the positive counterpart: a user whose DB
// row has role=admin passes RequireAdmin.
func TestRealAdminReachesAdminRoutes(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_real_admin")
	if err := db.Model(&model.User{}).Where("id = ?", "user_real_admin").
		Updates(map[string]any{"role": model.UserRoleAdmin}).Error; err != nil {
		t.Fatalf("promote admin failed: %v", err)
	}
	token, err := auth.SignToken("user_real_admin", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	router := NewRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("real admin should reach admin routes, got %d body=%s", resp.Code, resp.Body.String())
	}
}

// TestChatMessagesRequireMembership asserts the chat history endpoint is now
// authenticated and membership-gated.
func TestChatMessagesRequireMembership(t *testing.T) {
	db := openRouterTestDB(t)
	now := int64(1_700_000_000_000)
	if err := db.Create(&model.Post{
		ID: "post_chat_sec", AuthorID: "user_author_sec", Title: "p", Category: "running",
		Address: "x", MaxCount: 4, CurrentCount: 1, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	router := NewRouter(db)

	// No token at all -> 401.
	anon := httptest.NewRequest(http.MethodGet, "/api/v1/chats/post_chat_sec/messages", nil)
	anonResp := httptest.NewRecorder()
	router.ServeHTTP(anonResp, anon)
	if anonResp.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous chat read must be 401, got %d", anonResp.Code)
	}

	// Authenticated non-member -> 403.
	outsider := httptest.NewRequest(http.MethodGet, "/api/v1/chats/post_chat_sec/messages", nil)
	outsider.Header.Set("Authorization", bearerFor(t, db, "user_outsider_sec"))
	outResp := httptest.NewRecorder()
	router.ServeHTTP(outResp, outsider)
	if outResp.Code != http.StatusForbidden {
		t.Fatalf("non-member chat read must be 403, got %d body=%s", outResp.Code, outResp.Body.String())
	}
}
