package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func loginAttempt(t *testing.T, router http.Handler, nickname, password, remoteAddr, xff string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"nickname": nickname, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp.Code
}

func seedPasswordUser(t *testing.T, db *gorm.DB, nickname, password string) {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	user := model.User{
		ID: "user_" + nickname, Platform: "password", OpenID: "pwd_" + nickname,
		Nickname: nickname, PasswordHash: string(hashed), Role: model.UserRoleUser,
		CreditScore: 100, RatingScore: 5, CreatedAt: 1, UpdatedAt: 1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
}

// TestSpoofedForwardedForCannotBypassRateLimit is the regression test for the
// per-IP budget being defeated by a header. Gin trusts every proxy by default,
// so c.ClientIP() returned whatever the caller put in X-Forwarded-For.
func TestSpoofedForwardedForCannotBypassRateLimit(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "3")
	router := NewRouter(db)

	blocked := 0
	for i := 0; i < 15; i++ {
		// One real peer, a different forged forwarded-for each time.
		code := loginAttempt(t, router, fmt.Sprintf("acct%02d", i), "wrong",
			"203.0.113.9:5555", fmt.Sprintf("198.51.100.%d", i+1))
		if code == http.StatusTooManyRequests {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatalf("a forged X-Forwarded-For must not buy a fresh rate-limit budget")
	}
}

// TestCorrectPasswordIsNeverLockedOut is the regression test for the
// account-lockout denial of service: throttling *before* checking credentials
// let anyone lock a known account out of its own login.
func TestCorrectPasswordIsNeverLockedOut(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "100")
	t.Setenv("RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN", "5")
	seedPasswordUser(t, db, "victim", "correct-horse")
	router := NewRouter(db)

	// Attacker sprays wrong passwords at the account from many addresses.
	for i := 0; i < 40; i++ {
		loginAttempt(t, router, "victim", "wrong", fmt.Sprintf("198.51.100.%d:1", i%250+1), "")
	}

	// The real owner, from an unrelated address, must still get in.
	code := loginAttempt(t, router, "victim", "correct-horse", "8.8.8.8:9999", "")
	if code != http.StatusOK {
		t.Fatalf("the account owner must not be locked out by others' failures, got %d", code)
	}
}

// TestRepeatedWrongPasswordsAreStillThrottled confirms the above did not simply
// remove the protection: guessing is still slowed down.
func TestRepeatedWrongPasswordsAreStillThrottled(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "100")
	t.Setenv("RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN", "5")
	seedPasswordUser(t, db, "target", "correct-horse")
	router := NewRouter(db)

	throttled := false
	for i := 0; i < 30; i++ {
		// Vary the address so only the per-account failure budget can bite.
		code := loginAttempt(t, router, "target", "wrong", fmt.Sprintf("198.51.100.%d:1", i%250+1), "")
		if code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatalf("repeated wrong passwords against one account must still be throttled")
	}
}

// TestSuccessfulLoginClearsFailureBudget checks a user who mistypes and then
// succeeds is not left with a spent budget.
func TestSuccessfulLoginClearsFailureBudget(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "100")
	t.Setenv("RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN", "5")
	seedPasswordUser(t, db, "typo", "correct-horse")
	router := NewRouter(db)

	// Distinct addresses throughout, so only the per-account failure budget is
	// under test and the per-IP budget cannot interfere.
	for i := 0; i < 4; i++ {
		loginAttempt(t, router, "typo", "wrong", fmt.Sprintf("198.51.100.%d:1", i+1), "")
	}
	if code := loginAttempt(t, router, "typo", "correct-horse", "8.8.8.8:1", ""); code != http.StatusOK {
		t.Fatalf("login after a few typos should succeed, got %d", code)
	}
	// The successful login cleared the account's failure budget, so a fresh
	// run of failures is available rather than tripping immediately.
	for i := 0; i < 4; i++ {
		code := loginAttempt(t, router, "typo", "wrong", fmt.Sprintf("198.51.100.%d:1", i+100), "")
		if code == http.StatusTooManyRequests {
			t.Fatalf("failure budget should have been cleared by the successful login (tripped at %d)", i+1)
		}
	}
}

// TestSessionEndpointsUseLooserBudget guards the carrier-NAT case: refresh is
// automatic client traffic and must not share the tight credential budget.
func TestSessionEndpointsUseLooserBudget(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("RATE_LIMIT_AUTH_PER_MIN", "2")
	t.Setenv("RATE_LIMIT_SESSION_PER_MIN", "50")
	router := NewRouter(db)

	body, _ := json.Marshal(map[string]string{"refreshToken": "does-not-exist"})
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.7:1234"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code == http.StatusTooManyRequests {
			t.Fatalf("refresh hit the credential budget at attempt %d; shared-NAT clients would break", i+1)
		}
	}
}
