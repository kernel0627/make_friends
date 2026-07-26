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

// TestHotFeedBoundsCandidateSet checks the ranker scores a bounded set rather
// than every active post.
func TestHotFeedBoundsCandidateSet(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("HOT_FEED_CANDIDATES", "40")
	ensureTestUser(t, db, "user_hot_author")
	seedActivePosts(t, db, 120)
	router := NewRouter(db)

	// Page 2 is inside the 40-post budget and must be full.
	if got := len(listHotPosts(t, router, 2)); got != 20 {
		t.Fatalf("page 2 should hold 20 posts, got %d", got)
	}
	// Page 3 starts at offset 40, past the candidate budget.
	if got := len(listHotPosts(t, router, 3)); got != 0 {
		t.Fatalf("page beyond the candidate budget should be empty, got %d", got)
	}
}

// TestHotFeedPagesDoNotOverlap guards the in-memory slicing around the cap.
func TestHotFeedPagesDoNotOverlap(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("HOT_FEED_CANDIDATES", "100")
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

// TestHotFeedKeepsMostRecentCandidates confirms truncation drops the oldest
// posts rather than an arbitrary slice.
func TestHotFeedKeepsMostRecentCandidates(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("HOT_FEED_CANDIDATES", "10")
	ensureTestUser(t, db, "user_hot_author")
	seedActivePosts(t, db, 50)
	router := NewRouter(db)

	seen := map[string]bool{}
	for page := 1; page <= 2; page++ {
		for _, item := range listHotPosts(t, router, page) {
			seen[item.ID] = true
		}
	}
	if len(seen) != 10 {
		t.Fatalf("expected exactly the 10 newest candidates, got %d", len(seen))
	}
	// post_hot_0000 is the newest, post_hot_0049 the oldest.
	if !seen["post_hot_0000"] {
		t.Fatalf("the newest post must survive truncation")
	}
	if seen["post_hot_0049"] {
		t.Fatalf("the oldest post must be dropped by truncation")
	}
}
