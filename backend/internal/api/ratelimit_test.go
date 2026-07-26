package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	limiter := newRateLimiter(3, time.Minute)
	now := time.Now()

	for i := 1; i <= 3; i++ {
		if ok, _ := limiter.allow("k", now); !ok {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	ok, retryAfter := limiter.allow("k", now)
	if ok {
		t.Fatalf("request 4 should be blocked")
	}
	if retryAfter < 1 {
		t.Fatalf("retryAfter should be at least 1, got %d", retryAfter)
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	now := time.Now()

	if ok, _ := limiter.allow("a", now); !ok {
		t.Fatalf("first hit on a should pass")
	}
	if ok, _ := limiter.allow("b", now); !ok {
		t.Fatalf("one key exhausting its budget must not affect another")
	}
	if ok, _ := limiter.allow("a", now); ok {
		t.Fatalf("second hit on a should be blocked")
	}
}

func TestRateLimiterWindowResets(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	start := time.Now()

	limiter.allow("k", start)
	if ok, _ := limiter.allow("k", start); ok {
		t.Fatalf("should be blocked inside the window")
	}
	if ok, _ := limiter.allow("k", start.Add(61*time.Second)); !ok {
		t.Fatalf("should be allowed after the window elapses")
	}
}

func TestRateLimiterZeroLimitDisables(t *testing.T) {
	limiter := newRateLimiter(0, time.Minute)
	now := time.Now()
	for i := 0; i < 50; i++ {
		if ok, _ := limiter.allow("k", now); !ok {
			t.Fatalf("a zero limit must disable throttling")
		}
	}
}

func TestRateLimiterExpiresIdleKeys(t *testing.T) {
	limiter := newRateLimiter(5, time.Minute)
	start := time.Now()
	for i := 0; i < 100; i++ {
		limiter.allow(string(rune('a'+i%26))+"-key", start)
	}
	// A later hit triggers the sweep; stale windows must not accumulate.
	limiter.allow("trigger", start.Add(2*time.Minute))
	limiter.mu.Lock()
	remaining := len(limiter.counters)
	limiter.mu.Unlock()
	if remaining > 2 {
		t.Fatalf("expired windows should be reclaimed, %d entries left", remaining)
	}
}

// TestLoginIsThrottledPerAccount covers the distributed-guessing case: the
// per-IP bucket alone would not stop attempts spread across many sources.
func TestLoginIsThrottledPerAccount(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "100")
	t.Setenv("RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN", "4")
	router := NewRouter(db)
	ensureTestUser(t, db, "user_throttle")

	body, _ := json.Marshal(map[string]string{"nickname": "user_throttle", "password": "wrong"})
	var lastCode int
	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// Every attempt comes from a different address, so the per-IP bucket
		// never fills; only the per-account bucket can stop this.
		req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1234", i/250, i%250+1)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		lastCode = resp.Code
		if lastCode == http.StatusTooManyRequests {
			break
		}
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("repeated guesses at one account must be throttled, last status %d", lastCode)
	}
}

func TestOversizedRequestBodyIsRejected(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "0")
	router := NewRouter(db)

	huge := `{"nickname":"` + strings.Repeat("a", maxRequestBodyBytes+1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte(huge)))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body should be rejected, got %d", resp.Code)
	}
}

// TestInternalErrorsDoNotLeakDetail asserts serverError returns a generic
// payload rather than the underlying database error.
func TestInternalErrorsDoNotLeakDetail(t *testing.T) {
	dbErr := errors.New(`no such column: users.secret_column`)

	engine := gin.New()
	engine.GET("/boom", func(c *gin.Context) {
		serverError(c, dbErr)
	})
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	body := resp.Body.String()
	if strings.Contains(body, "secret_column") {
		t.Fatalf("response must not echo the underlying error: %s", body)
	}
	if !strings.Contains(body, "INTERNAL_ERROR") {
		t.Fatalf("expected a generic error code, got %s", body)
	}
}
