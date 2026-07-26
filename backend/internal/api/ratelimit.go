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

// reset clears a key's window, used when a successful login proves the earlier
// failures were not an attack.
func (r *rateLimiter) reset(key string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.counters, key)
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
	authIP          *rateLimiter
	accountFailures *rateLimiter
	sessionIP       *rateLimiter
	smartDraft      *rateLimiter
	recommendation  *rateLimiter
}

// noteFailedLogin records a wrong-password attempt against an account and
// reports whether that account has now exceeded its failure budget.
//
// Only *failures* are counted, and this is called after the password has been
// checked, so a caller presenting the correct password is never refused. An
// earlier version throttled before checking, which let anyone lock a known
// account — "admin", say — out of its own login by spraying wrong passwords
// from any address.
func (s *Server) noteFailedLogin(nickname string) (bool, int) {
	nickname = strings.TrimSpace(strings.ToLower(nickname))
	if nickname == "" || s.AccountFailureLimiter == nil {
		return true, 0
	}
	return s.AccountFailureLimiter.allow("account_fail:"+nickname, time.Now())
}

// clearLoginFailures drops the failure budget after a successful login so a
// legitimate user who mistyped a few times starts clean.
func (s *Server) clearLoginFailures(nickname string) {
	nickname = strings.TrimSpace(strings.ToLower(nickname))
	if nickname == "" || s.AccountFailureLimiter == nil {
		return
	}
	s.AccountFailureLimiter.reset("account_fail:" + nickname)
}

// failLoginThrottled writes the 429 used when an account's failure budget is
// spent. The caller has already established the credentials were wrong.
func failLoginThrottled(c *gin.Context, retryAfter int) {
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	fail(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many failed attempts, please retry later")
}

// newRateLimits builds the limiter set from env overrides. A limit of 0
// disables that bucket, which is how the tests opt out.
func newRateLimits() rateLimits {
	return rateLimits{
		// Deliberate, human-initiated credential entry: rare even for a large
		// shared egress address.
		authIP: newRateLimiter(
			envInt("RATE_LIMIT_AUTH_PER_MIN", 60),
			time.Minute,
		),
		// Wrong passwords are counted separately by normalized account name.
		// Correct credentials are checked before this bucket and therefore
		// cannot be locked out by another caller's failures.
		accountFailures: newRateLimiter(
			envInt("RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN", 10),
			time.Minute,
		),
		// Automatic session traffic (token refresh, silent WeChat login). These
		// fire without user action, and mobile carriers put very many real
		// users behind one address, so this budget must be far looser than the
		// credential one. Refresh tokens are 256-bit random, so guessing them
		// is not the threat this defends against.
		sessionIP: newRateLimiter(
			envInt("RATE_LIMIT_SESSION_PER_MIN", 600),
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
