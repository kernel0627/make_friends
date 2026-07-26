package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"make_friends/backend/internal/model"
)

func ensureRedisForTest(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	if err := client.FlushAll(t.Context()).Err(); err != nil {
		t.Fatalf("redis flush failed: %v", err)
	}
	return client
}

func TestPostsCacheHitAndInvalidateWithRedis(t *testing.T) {
	client := ensureRedisForTest(t)
	defer client.Close()

	t.Setenv("USE_REDIS", "true")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("WS_ENABLED", "true")

	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	if err := db.Create(&[]model.Post{
		{ID: "post_r_1", AuthorID: "u1", Title: "a", Category: "跑步", Address: "x", MaxCount: 5, CurrentCount: 1, Status: "open", TimeMode: "weekend", CreatedAt: now, UpdatedAt: now},
		{ID: "post_r_2", AuthorID: "u2", Title: "b", Category: "跑步", Address: "x", MaxCount: 5, CurrentCount: 3, Status: "open", TimeMode: "weekend", CreatedAt: now + 1, UpdatedAt: now + 1},
	}).Error; err != nil {
		t.Fatalf("seed posts failed: %v", err)
	}

	router := NewRouter(db)

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/posts?sortBy=hot&page=1", nil)
	resp1 := httptest.NewRecorder()
	router.ServeHTTP(resp1, req1)
	if resp1.Code != http.StatusOK {
		t.Fatalf("first list failed: %d %s", resp1.Code, resp1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/posts?sortBy=hot&page=1", nil)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("second list failed: %d %s", resp2.Code, resp2.Body.String())
	}

	createBody := map[string]any{
		"title":       "new_post",
		"description": "cache_invalidate",
		"category":    "跑步",
		"subCategory": "",
		"timeInfo":    map[string]any{"mode": "weekend", "days": 1, "fixedTime": ""},
		"address":     "x",
		"maxCount":    5,
	}
	raw, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(raw))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", bearerFor(t, db, "u1"))
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create post failed: %d %s", createResp.Code, createResp.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/posts?sortBy=hot&page=1", nil)
	resp3 := httptest.NewRecorder()
	router.ServeHTTP(resp3, req3)
	if resp3.Code != http.StatusOK {
		t.Fatalf("third list failed: %d %s", resp3.Code, resp3.Body.String())
	}

	var payload struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(resp3.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode third list failed: %v", err)
	}
	if len(payload.Posts) != 3 {
		t.Fatalf("expected 3 posts after create, got=%d body=%s", len(payload.Posts), resp3.Body.String())
	}
}

func TestWSBroadcastWithRedisPubSub(t *testing.T) {
	client := ensureRedisForTest(t)
	defer client.Close()

	t.Setenv("USE_REDIS", "true")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("WS_ENABLED", "true")

	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	if err := db.Create(&model.Post{ID: "post_ws_1", AuthorID: "u_author", Title: "ws", Category: "跑步", Address: "x", MaxCount: 5, CurrentCount: 2, Status: "open", TimeMode: "weekend", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&[]model.PostParticipant{{PostID: "post_ws_1", UserID: "u_member_1", JoinedAt: now}, {PostID: "post_ws_1", UserID: "u_member_2", JoinedAt: now}}).Error; err != nil {
		t.Fatalf("create participants failed: %v", err)
	}

	router := NewRouter(db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws/chat?postId=post_ws_1"
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}

	h1 := http.Header{}
	h1.Set("Authorization", bearerFor(t, db, "u_member_1"))
	c1, _, err := dialer.Dial(wsURL, h1)
	if err != nil {
		t.Fatalf("dial ws client1 failed: %v", err)
	}
	defer c1.Close()

	h2 := http.Header{}
	h2.Set("Authorization", bearerFor(t, db, "u_member_2"))
	c2, _, err := dialer.Dial(wsURL, h2)
	if err != nil {
		t.Fatalf("dial ws client2 failed: %v", err)
	}
	defer c2.Close()
	time.Sleep(200 * time.Millisecond)

	sendBody := []byte(`{"content":"hello_ws"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/chats/post_ws_1/messages", bytes.NewReader(sendBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "u_member_1"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send message status=%d", resp.StatusCode)
	}

	_ = c1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = c2.SetReadDeadline(time.Now().Add(3 * time.Second))

	_, m1, err := c1.ReadMessage()
	if err != nil {
		t.Fatalf("read ws client1 failed: %v", err)
	}
	_, m2, err := c2.ReadMessage()
	if err != nil {
		t.Fatalf("read ws client2 failed: %v", err)
	}

	if !strings.Contains(string(m1), "hello_ws") {
		t.Fatalf("client1 not receive expected payload: %s", string(m1))
	}
	if !strings.Contains(string(m2), "hello_ws") {
		t.Fatalf("client2 not receive expected payload: %s", string(m2))
	}

	// The pushed event must carry the same enriched shape the REST endpoints
	// return, so a client can render it without a follow-up sender lookup.
	var event struct {
		Type    string `json:"type"`
		Message struct {
			ID        string `json:"id"`
			PostID    string `json:"postId"`
			SenderID  string `json:"senderId"`
			Content   string `json:"content"`
			CreatedAt int64  `json:"createdAt"`
			Sender    struct {
				ID        string `json:"id"`
				NickName  string `json:"nickName"`
				AvatarURL string `json:"avatarUrl"`
			} `json:"sender"`
		} `json:"message"`
	}
	if err := json.Unmarshal(m1, &event); err != nil {
		t.Fatalf("decode ws event failed: %v payload=%s", err, string(m1))
	}
	if event.Type != "chat_message" {
		t.Fatalf("unexpected event type %q", event.Type)
	}
	if event.Message.ID == "" || event.Message.CreatedAt == 0 {
		t.Fatalf("event message missing id/createdAt: %s", string(m1))
	}
	if event.Message.PostID != "post_ws_1" {
		t.Fatalf("unexpected postId %q", event.Message.PostID)
	}
	if event.Message.SenderID != "u_member_1" {
		t.Fatalf("unexpected senderId %q", event.Message.SenderID)
	}
	if event.Message.Sender.ID != "u_member_1" || event.Message.Sender.NickName == "" {
		t.Fatalf("event message must embed the sender profile: %s", string(m1))
	}
}
