package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimiter is a fixed-window counter keyed by an arbitrary string.
//
// In-memory on purpose: the limits below exist to stop password guessing,
// paid-API abuse and training-data flooding from a single source, and an
// in-process counter does that without making Redis a hard dependency. If the
// backend is ever run as more than one replica these budgets become per-replica
// and should move to Redis.
type rateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateWindow
	limit    int
	window   time.Duration
	lastGC   time.Time
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		counters: make(map[string]*rateWindow),
		limit:    limit,
		window:   window,
		lastGC:   time.Now(),
	}
}

// allow records a hit for key and reports whether it stays within budget,
// along with the seconds until the current window resets.
func (r *rateLimiter) allow(key string, now time.Time) (bool, int) {
	if r == nil || r.limit <= 0 {
		return true, 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gcLocked(now)

	entry, ok := r.counters[key]
	if !ok || now.After(entry.resetAt) {
		r.counters[key] = &rateWindow{count: 1, resetAt: now.Add(r.window)}
		return true, 0
	}
	entry.count++
	if entry.count > r.limit {
		retry := int(entry.resetAt.Sub(now).Seconds())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	return true, 0
}

// gcLocked drops expired windows so the map cannot grow without bound from
// one-off keys (a rotating source IP would otherwise leak an entry each time).
func (r *rateLimiter) gcLocked(now time.Time) {
	if now.Sub(r.lastGC) < r.window {
		return
	}
	r.lastGC = now
	for key, entry := range r.counters {
		if now.After(entry.resetAt) {
			delete(r.counters, key)
		}
	}
}

// limitBy rejects requests once keyFn's value exceeds the budget. Returning an
// empty key skips the check.
func (r *rateLimiter) limitBy(code, message string, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			c.Next()
			return
		}
		ok, retryAfter := r.allow(key, time.Now())
		if !ok {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			fail(c, http.StatusTooManyRequests, code, message)
			c.Abort()
			return
		}
		c.Next()
	}
}

func clientIPKey(c *gin.Context) string {
	ip := strings.TrimSpace(c.ClientIP())
	if ip == "" {
		return "unknown"
	}
	return ip
}

// authAttemptKey buckets credential attempts by source address. Guessing
// spread across many IPs against one account is bucketed separately, by
// account name, inside the login handler (see Server.allowAccountAttempt).
func authAttemptKey(c *gin.Context) string {
	return "ip:" + clientIPKey(c)
}

func userOrIPKey(c *gin.Context) string {
	if userID := strings.TrimSpace(mustUserID(c)); userID != "" {
		return "user:" + userID
	}
	return "ip:" + clientIPKey(c)
}

type rateLimits struct {
	auth           *rateLimiter
	smartDraft     *rateLimiter
	recommendation *rateLimiter
}

// allowAccountAttempt throttles login attempts against a single account name,
// independent of where they come from. The per-IP bucket alone would let a
// distributed guess walk one account's password at full speed.
func (s *Server) allowAccountAttempt(c *gin.Context, nickname string) bool {
	nickname = strings.TrimSpace(strings.ToLower(nickname))
	if nickname == "" || s.AuthLimiter == nil {
		return true
	}
	ok, retryAfter := s.AuthLimiter.allow("account:"+nickname, time.Now())
	if !ok {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		fail(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, please retry later")
	}
	return ok
}

// newRateLimits builds the limiter set from env overrides. A limit of 0
// disables that bucket, which is how the tests opt out.
func newRateLimits() rateLimits {
	return rateLimits{
		auth: newRateLimiter(
			envInt("RATE_LIMIT_AUTH_PER_MIN", 20),
			time.Minute,
		),
		smartDraft: newRateLimiter(
			envInt("RATE_LIMIT_SMART_DRAFT_PER_HOUR", 30),
			time.Hour,
		),
		recommendation: newRateLimiter(
			envInt("RATE_LIMIT_FEEDBACK_PER_MIN", 120),
			time.Minute,
		),
	}
}
