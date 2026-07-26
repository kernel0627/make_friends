package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"make_friends/backend/internal/model"
)

func getChatMessages(t *testing.T, router http.Handler, bearer, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", bearer)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeChatMessages(t *testing.T, resp *httptest.ResponseRecorder) []chatMessageView {
	t.Helper()
	var payload struct {
		Messages []chatMessageView `json:"messages"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode messages failed: %v body=%s", err, resp.Body.String())
	}
	return payload.Messages
}

func TestListChatMessagesIncrementalCursorAndLimit(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	userID := "user_chat_cursor"
	postID := "post_chat_cursor"
	bearer := bearerFor(t, db, userID)
	if err := db.Create(&model.Post{
		ID: postID, AuthorID: userID, Title: "chat", Category: "running",
		Address: "x", MaxCount: 4, CurrentCount: 1, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	messages := make([]model.ChatMessage, 0, 503)
	messages = append(messages,
		model.ChatMessage{ID: "msg_early", PostID: postID, SenderID: userID, Content: "early", ClientMsgID: "client_early", CreatedAt: now},
		model.ChatMessage{ID: "msg_boundary_a", PostID: postID, SenderID: userID, Content: "a", ClientMsgID: "client_boundary_a", CreatedAt: now + 1000},
		model.ChatMessage{ID: "msg_boundary_b", PostID: postID, SenderID: userID, Content: "b", ClientMsgID: "client_boundary_b", CreatedAt: now + 1000},
	)
	for i := 0; i < 500; i++ {
		messages = append(messages, model.ChatMessage{
			ID:          fmt.Sprintf("msg_later_%03d", i),
			PostID:      postID,
			SenderID:    userID,
			Content:     "later",
			ClientMsgID: fmt.Sprintf("client_later_%03d", i),
			CreatedAt:   now + 2000 + int64(i),
		})
	}
	if err := db.CreateInBatches(&messages, 100).Error; err != nil {
		t.Fatalf("seed messages failed: %v", err)
	}
	router := NewRouter(db)

	full := getChatMessages(t, router, bearer,
		"/api/v1/chats/"+postID+"/messages?limit=1")
	if full.Code != http.StatusOK {
		t.Fatalf("full history failed: %d %s", full.Code, full.Body.String())
	}
	if got := len(decodeChatMessages(t, full)); got != len(messages) {
		t.Fatalf("no cursor must retain complete history, got %d want %d", got, len(messages))
	}

	boundary := getChatMessages(t, router, bearer, fmt.Sprintf(
		"/api/v1/chats/%s/messages?sinceCreatedAt=%d&limit=2", postID, now+1000))
	if boundary.Code != http.StatusOK {
		t.Fatalf("boundary query failed: %d %s", boundary.Code, boundary.Body.String())
	}
	boundaryMessages := decodeChatMessages(t, boundary)
	if len(boundaryMessages) != 2 ||
		boundaryMessages[0].ID != "msg_boundary_a" ||
		boundaryMessages[1].ID != "msg_boundary_b" {
		t.Fatalf("cursor must include and deterministically order its boundary: %+v", boundaryMessages)
	}

	capped := getChatMessages(t, router, bearer, fmt.Sprintf(
		"/api/v1/chats/%s/messages?sinceCreatedAt=%d&limit=9999", postID, now+1000))
	if capped.Code != http.StatusOK {
		t.Fatalf("capped query failed: %d %s", capped.Code, capped.Body.String())
	}
	if got := len(decodeChatMessages(t, capped)); got != 500 {
		t.Fatalf("incremental limit must cap at 500, got %d", got)
	}
}

func TestListChatMessagesRejectsInvalidIncrementalQuery(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	userID := "user_chat_query"
	postID := "post_chat_query"
	bearer := bearerFor(t, db, userID)
	if err := db.Create(&model.Post{
		ID: postID, AuthorID: userID, Title: "chat", Category: "running",
		Address: "x", MaxCount: 4, CurrentCount: 1, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	router := NewRouter(db)

	for _, path := range []string{
		"/api/v1/chats/" + postID + "/messages?sinceCreatedAt=not-a-time",
		"/api/v1/chats/" + postID + "/messages?sinceCreatedAt=1&limit=0",
	} {
		resp := getChatMessages(t, router, bearer, path)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %q should return 400, got %d %s", path, resp.Code, resp.Body.String())
		}
	}
}
