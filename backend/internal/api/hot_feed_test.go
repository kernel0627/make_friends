package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func seedActivePosts(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	now := time.Now().UnixMilli()
	posts := make([]model.Post, 0, count)
	for i := 0; i < count; i++ {
		posts = append(posts, model.Post{
			ID:       fmt.Sprintf("post_hot_%04d", i),
			AuthorID: "user_hot_author",
			Title:    fmt.Sprintf("hot %d", i),
			Category: "running",
			Address:  "x",
			MaxCount: 6,
			// Distinct creation times so "most recent N" is well defined.
			CurrentCount: 1,
			Status:       "open",
			TimeMode:     "weekend",
			CreatedAt:    now - int64(i)*1000,
			UpdatedAt:    now - int64(i)*1000,
		})
	}
	if err := db.CreateInBatches(&posts, 100).Error; err != nil {
		t.Fatalf("seed posts failed: %v", err)
	}
}

func listHotPosts(t *testing.T, router http.Handler, page int) []postView {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/posts?sortBy=hot&page=%d&pageSize=20", page), nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("hot feed page %d failed: %d %s", page, resp.Code, resp.Body.String())
	}
	var payload struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode hot feed failed: %v", err)
	}
	return payload.Posts
}

// TestHotFeedIncludesAllActivePosts guards the public feed semantics: every
// active post remains rankable and reachable through pagination, including
// posts beyond the previously introduced 500-candidate cutoff.
func TestHotFeedIncludesAllActivePosts(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_hot_author")
	seedActivePosts(t, db, 520)
	router := NewRouter(db)

	seen := map[string]bool{}
	for page := 1; page <= 26; page++ {
		for _, item := range listHotPosts(t, router, page) {
			seen[item.ID] = true
		}
	}
	if len(seen) != 520 {
		t.Fatalf("expected all 520 active posts across hot-feed pages, got %d", len(seen))
	}
	if !seen["post_hot_0519"] {
		t.Fatalf("the oldest post beyond the former 500-candidate cutoff must remain reachable")
	}
	if got := len(listHotPosts(t, router, 27)); got != 0 {
		t.Fatalf("page after all active posts should be empty, got %d", got)
	}
}

// TestHotFeedPagesDoNotOverlap guards the in-memory slicing around the cap.
func TestHotFeedPagesDoNotOverlap(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_hot_author")
	seedActivePosts(t, db, 60)
	router := NewRouter(db)

	seen := map[string]int{}
	for page := 1; page <= 3; page++ {
		for _, item := range listHotPosts(t, router, page) {
			seen[item.ID]++
			if seen[item.ID] > 1 {
				t.Fatalf("post %s appeared on more than one page", item.ID)
			}
		}
	}
	if len(seen) != 60 {
		t.Fatalf("expected all 60 posts across 3 pages, got %d", len(seen))
	}
}
