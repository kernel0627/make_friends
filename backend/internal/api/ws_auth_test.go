package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"make_friends/backend/internal/auth"
	"make_friends/backend/internal/model"
)

// dialChatWS attempts a websocket handshake and returns the HTTP status the
// server answered with (101 on success).
func dialChatWS(t *testing.T, serverURL, postID, bearer string) int {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws/chat?postId=" + postID
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	header := http.Header{}
	if bearer != "" {
		header.Set("Authorization", bearer)
	}
	conn, resp, err := dialer.Dial(wsURL, header)
	if conn != nil {
		defer conn.Close()
	}
	if err == nil {
		return http.StatusSwitchingProtocols
	}
	if resp != nil {
		return resp.StatusCode
	}
	t.Fatalf("dial failed without a response: %v", err)
	return 0
}

func setupChatWSServer(t *testing.T) (*httptest.Server, *gorm.DB) {
	t.Helper()
	client := ensureRedisForTest(t)
	t.Cleanup(func() { _ = client.Close() })

	t.Setenv("USE_REDIS", "true")
	t.Setenv("REDIS_ADDR", strings.TrimSpace(os.Getenv("REDIS_TEST_ADDR")))
	t.Setenv("WS_ENABLED", "true")

	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	if err := db.Create(&model.Post{
		ID: "post_ws_auth", AuthorID: "u_ws_author", Title: "ws", Category: "跑步",
		Address: "x", MaxCount: 5, CurrentCount: 2, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID: "post_ws_auth", UserID: "u_ws_member", Status: "active", JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	ensureTestUser(t, db, "u_ws_member")

	ts := httptest.NewServer(NewRouter(db))
	t.Cleanup(ts.Close)
	return ts, db
}

// TestWSRejectsRevokedToken covers logging out: the access token is revoked,
// but the websocket handler skipped that check entirely, so a logged-out
// client kept receiving chat until the token expired days later.
func TestWSRejectsRevokedToken(t *testing.T) {
	ts, db := setupChatWSServer(t)

	token, err := auth.SignToken("u_ws_member", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	if code := dialChatWS(t, ts.URL, "post_ws_auth", "Bearer "+token); code != http.StatusSwitchingProtocols {
		t.Fatalf("valid member should connect, got %d", code)
	}

	claims, err := auth.ParseClaims(token, testJWTSecret)
	if err != nil {
		t.Fatalf("parse claims failed: %v", err)
	}
	if err := db.Create(&model.RevokedAccessToken{
		JTI:       claims.ID,
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("revoke token failed: %v", err)
	}

	if code := dialChatWS(t, ts.URL, "post_ws_auth", "Bearer "+token); code != http.StatusUnauthorized {
		t.Fatalf("revoked token must be rejected, got %d", code)
	}
}

// TestWSRejectsDisabledUser covers banning: a soft-deleted user could still
// open chat sockets.
func TestWSRejectsDisabledUser(t *testing.T) {
	ts, db := setupChatWSServer(t)

	token, err := auth.SignToken("u_ws_member", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", "u_ws_member").
		Update("deleted_at", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("disable user failed: %v", err)
	}

	if code := dialChatWS(t, ts.URL, "post_ws_auth", "Bearer "+token); code != http.StatusUnauthorized {
		t.Fatalf("disabled user must be rejected, got %d", code)
	}
}

func TestWSRejectsMissingToken(t *testing.T) {
	ts, _ := setupChatWSServer(t)
	if code := dialChatWS(t, ts.URL, "post_ws_auth", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous websocket must be rejected, got %d", code)
	}
}
