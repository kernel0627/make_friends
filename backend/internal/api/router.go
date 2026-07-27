package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/auth"
	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type Server struct {
	DB                    *gorm.DB
	JWTSecret             string
	WechatAppID           string
	WechatSecret          string
	HTTPClient            *http.Client
	TokenExpireHr         int
	RefreshExpire         int
	RedisClient           *redis.Client
	UseRedis              bool
	WSEnabled             bool
	AccountFailureLimiter *rateLimiter
}

const contextUserIDKey = "userID"
const contextTokenJTIKey = "tokenJTI"

// errRefreshTokenReplayed signals that a refresh token was already rotated by
// a concurrent or replayed request.
var errRefreshTokenReplayed = errors.New("refresh token already rotated")

func NewRouter(db *gorm.DB) *gin.Engine {
	return NewRouterWithServer(NewServer(db))
}

func NewRouterWithServer(s *Server) *gin.Engine {
	limits := newRateLimits()
	authLimit := limits.authIP.limitBy("RATE_LIMITED", "too many attempts, please retry later", authAttemptKey)
	sessionLimit := limits.sessionIP.limitBy("RATE_LIMITED", "too many requests", authAttemptKey)
	smartDraftLimit := limits.smartDraft.limitBy("RATE_LIMITED", "smart draft quota exceeded, please retry later", userOrIPKey)
	feedbackLimit := limits.recommendation.limitBy("RATE_LIMITED", "too many requests", userOrIPKey)

	r := gin.Default()
	// Gin trusts every proxy by default, which makes c.ClientIP() return the
	// caller-supplied X-Forwarded-For. Any per-IP limit would then be defeated
	// by sending a different value each request. Trust nothing unless the
	// operator names the real proxies.
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}
	r.Use(adminWebCORS())
	// Cap request bodies: every endpoint here takes small JSON, and without a
	// limit a single request can stream unbounded data into memory.
	r.Use(limitRequestBody(maxRequestBodyBytes))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "time": time.Now().UnixMilli()})
	})

	// Internal agent API (service-to-service, shared-secret auth)
	RegisterAgentRoutes(r, s)

	v1 := r.Group("/api/v1")
	{
		if envBool("ENABLE_MOCK_LOGIN", false) {
			log.Printf("WARNING: /auth/mock-login is enabled; never enable this in production")
			v1.POST("/auth/mock-login", s.MockLogin)
		}
		// Password endpoints are throttled tightly: bcrypt makes each attempt
		// expensive enough to be both a guessing oracle and a CPU sink.
		v1.POST("/auth/register", authLimit, s.Register)
		v1.POST("/auth/password-login", authLimit, s.PasswordLogin)
		// Automatic session traffic gets the looser budget — see rateLimits.
		v1.POST("/auth/wechat-login", sessionLimit, s.WechatLogin)
		v1.POST("/auth/refresh", sessionLimit, s.RefreshToken)
		v1.POST("/auth/logout", s.Logout)

		v1.GET("/posts", s.ListPosts)
		v1.GET("/posts/:id", s.GetPost)
		v1.GET("/posts/:id/moderation", s.GetPostModeration)
		v1.GET("/posts/:id/settlement", s.GetSettlement)
		v1.GET("/calendar/activity-heatmap", s.CalendarActivityHeatmap)
		v1.GET("/calendar/activity-posts", s.CalendarActivityPosts)
		v1.GET("/users/:id/home", s.GetUserHome)
		v1.GET("/users/:id/credit-ledger", s.GetCreditLedger)
		// Anonymous write endpoints that feed the ranking model's training
		// data; unthrottled they are a free way to poison recommendations.
		v1.POST("/recommendations/exposures", feedbackLimit, s.ReportRecommendationExposures)
		v1.POST("/recommendations/click", feedbackLimit, s.ReportRecommendationClick)
		v1.GET("/ws/chat", s.WSChat)

		authed := v1.Group("")
		authed.Use(s.RequireAuth())
		{
			authed.GET("/auth/me", s.Me)
			authed.POST("/auth/avatar/random", s.RandomAvatar)
			authed.GET("/users/search", s.SearchUsers)
			authed.GET("/calendar/invite-heatmap", s.InviteCalendarHeatmap)
			authed.GET("/calendar/invite-candidates", s.InviteCalendarCandidates)
			authed.GET("/messages/invitations", s.ListInvitations)
			authed.GET("/messages/sent-invitations", s.ListSentInvitations)
			authed.POST("/invitations/:id/accept", s.AcceptInvitation)
			authed.POST("/invitations/:id/reject", s.RejectInvitation)
			authed.POST("/invitations/:id/cancel", s.CancelInvitation)
			// Each call spends real money at DeepSeek.
			authed.POST("/posts/smart-draft", smartDraftLimit, s.SmartPostDraft)
			authed.POST("/posts", s.CreatePost)
			authed.PUT("/posts/:id", s.UpdatePost)
			authed.POST("/messages/:id/reports", s.ReportMessage)
			authed.POST("/posts/:id/reports", s.ReportPost)
			authed.POST("/moderations/:id/appeals", s.AppealModeration)
			authed.POST("/credit-ledgers/:id/appeals", s.AppealCreditLedger)
			authed.POST("/posts/:id/join", s.JoinPost)
			authed.POST("/posts/:id/participation/cancel", s.CancelParticipation)
			authed.POST("/posts/:id/close", s.ClosePost)
			authed.POST("/posts/:id/settlement/participant", s.UpsertParticipantSettlement)
			authed.POST("/posts/:id/settlement/author", s.UpsertAuthorSettlement)
			authed.POST("/posts/:id/settlement/cancel-all", s.CancelAllSettlement)
			authed.GET("/chats/:postId/messages", s.ListChatMessages)
			authed.POST("/chats/:postId/messages", s.SendChatMessage)
			authed.POST("/posts/:id/reviews", s.UpsertReviews)
			admin := authed.Group("/admin")
			admin.Use(s.RequireAdmin())
			{
				admin.POST("/recommendations/rebuild", s.TriggerRecommendationRebuild)
				admin.GET("/dashboard/summary", s.AdminDashboardSummary)
				admin.GET("/dashboard/analytics", s.AdminDashboardAnalytics)
				admin.GET("/cases", s.ListAdminCases)
				admin.GET("/cases/:id", s.GetAdminCase)
				admin.POST("/cases/:id/resolve", s.ResolveAdminCase)
				admin.POST("/cases/:id/decision", s.DecideAdminCase)
				admin.POST("/cases/:id/reopen", s.ReopenAdminCase)
				admin.GET("/cases/:id/context", s.GetAdminCaseContext)
				admin.GET("/moderations", s.ListAdminModerations)
				admin.GET("/moderations/:id", s.GetAdminModeration)
				admin.POST("/moderations/:id/decision", s.DecideAdminModeration)
				admin.GET("/users", s.ListAdminUsers)
				admin.POST("/users", s.CreateAdminUser)
				admin.GET("/users/:id", s.GetAdminUser)
				admin.PATCH("/users/:id", s.UpdateAdminUser)
				admin.DELETE("/users/:id", s.DeleteAdminUser)
				admin.POST("/users/:id/restore", s.RestoreAdminUser)
				admin.POST("/users/:id/reset-password", s.ResetAdminUserPassword)
				admin.GET("/users/:id/credit-ledger", s.GetAdminUserCreditLedger)
				admin.POST("/users/:id/credit-adjust", s.AdminAdjustUserCredit)
				admin.GET("/posts", s.ListAdminPosts)
				admin.POST("/posts", s.CreateAdminPost)
				admin.GET("/posts/:id", s.GetAdminPost)
				admin.PATCH("/posts/:id", s.UpdateAdminPost)
				admin.DELETE("/posts/:id", s.DeleteAdminPost)
				admin.POST("/posts/:id/restore", s.RestoreAdminPost)
				admin.GET("/posts/:id/settlement", s.GetSettlement)
				admin.GET("/reviews", s.ListAdminReviews)
				admin.GET("/admin-users", s.ListAdminAccounts)
				rootAdmin := admin.Group("")
				rootAdmin.Use(s.RequireRootAdmin())
				{
					rootAdmin.POST("/admin-users", s.CreateAdminAccount)
					rootAdmin.PATCH("/admin-users/:id", s.UpdateAdminAccount)
					rootAdmin.DELETE("/admin-users/:id", s.DeleteAdminAccount)
					rootAdmin.POST("/admin-users/:id/restore", s.RestoreAdminAccount)
					rootAdmin.POST("/admin-users/:id/reset-password", s.ResetAdminAccountPassword)
				}
			}
		}
	}

	return r
}

func NewServer(db *gorm.DB) *Server {
	// Keep test-created databases and older local databases forward compatible
	// with the moderation/case tables introduced after the initial schema.
	if err := db.AutoMigrate(&model.ModerationRecord{}, &model.CaseEvent{}, &model.OutboxEvent{},
		&model.DomainEvent{}, &model.AgentRun{}, &model.AgentStep{}); err != nil {
		log.Printf("WARNING: moderation schema migration failed: %v", err)
	}
	useRedis := envBool("USE_REDIS", false)
	redisClient := buildRedisClient(useRedis)
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwtSecret == "" || jwtSecret == "dev_secret_change_me" {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatal("JWT_SECRET must be set to a non-default value in release mode")
		}
		if jwtSecret == "" {
			jwtSecret = "dev_secret_change_me"
		}
		log.Printf("WARNING: using insecure development JWT secret; set JWT_SECRET before deploying")
	}
	limits := newRateLimits()
	return &Server{
		DB:                    db,
		JWTSecret:             jwtSecret,
		WechatAppID:           os.Getenv("WECHAT_APP_ID"),
		WechatSecret:          os.Getenv("WECHAT_APP_SECRET"),
		HTTPClient:            &http.Client{Timeout: 8 * time.Second},
		TokenExpireHr:         24 * 7,
		RefreshExpire:         24 * 30,
		RedisClient:           redisClient,
		UseRedis:              useRedis && redisClient != nil,
		WSEnabled:             envBool("WS_ENABLED", true),
		AccountFailureLimiter: limits.accountFailures,
	}
}

func envOrDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

// trustedProxies returns the proxy CIDRs whose X-Forwarded-For may be believed.
// Empty (the default) means the peer address is used verbatim. Set
// TRUSTED_PROXIES to your load balancer's range when running behind one,
// otherwise every client can forge its own source address.
func trustedProxies() []string {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, item := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(item); value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		log.Printf("invalid %s=%q, using %d", key, raw, fallback)
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func buildRedisClient(enabled bool) *redis.Client {
	if !enabled {
		return nil
	}
	addr := envOrDefault("REDIS_ADDR", "127.0.0.1:6379")
	password := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	dbIndex := 0
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbIndex,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("redis disabled: ping failed addr=%s err=%v", addr, err)
		_ = client.Close()
		return nil
	}
	log.Printf("redis enabled addr=%s", addr)
	return client
}

type mockLoginReq struct {
	Nickname string `json:"nickname"`
}

func (s *Server) MockLogin(c *gin.Context) {
	var req mockLoginReq
	_ = c.ShouldBindJSON(&req)
	nick := strings.TrimSpace(req.Nickname)
	if nick == "" {
		nick = "mock_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:6]), "-", "")
	}
	var existed model.User
	if err := s.DB.First(&existed, "platform = ? AND nickname = ?", "mock", nick).Error; err == nil {
		if strings.TrimSpace(existed.AvatarURL) == "" {
			existed.AvatarURL = avatarURLFromSeed(existed.ID)
			existed.UpdatedAt = time.Now().UnixMilli()
			_ = s.DB.Save(&existed).Error
		}
		normalizeUserModel(&existed)
		authResp, signErr := s.issueAuth(existed.ID)
		if signErr != nil {
			fail(c, http.StatusInternalServerError, "TOKEN_SIGN_FAILED", "sign token failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"token":        authResp.AccessToken,
			"accessToken":  authResp.AccessToken,
			"refreshToken": authResp.RefreshToken,
			"user":         existed,
		})
		return
	}
	now := time.Now().UnixMilli()
	userID := "user_mock_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:8]), "-", "")
	user := model.User{
		ID:          userID,
		Platform:    "mock",
		OpenID:      "mock_" + userID,
		Nickname:    nick,
		AvatarURL:   "https://api.dicebear.com/7.x/avataaars/svg?seed=" + userID,
		Role:        model.UserRoleUser,
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.DB.Create(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "CREATE_USER_FAILED", "create user failed")
		return
	}
	normalizeUserModel(&user)
	authResp, err := s.issueAuth(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "TOKEN_SIGN_FAILED", "sign token failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":        authResp.AccessToken,
		"accessToken":  authResp.AccessToken,
		"refreshToken": authResp.RefreshToken,
		"user":         user,
	})
}

type wechatLoginReq struct {
	Code string `json:"code"`
}

type wechatSessionResp struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (s *Server) WechatLogin(c *gin.Context) {
	if strings.TrimSpace(s.WechatAppID) == "" || strings.TrimSpace(s.WechatSecret) == "" {
		fail(c, http.StatusBadRequest, "WECHAT_CONFIG_MISSING", "wechat appid/secret not configured")
		return
	}

	var req wechatLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		fail(c, http.StatusBadRequest, "WECHAT_CODE_REQUIRED", "code required")
		return
	}

	wxResp, err := s.fetchWechatSession(code)
	if err != nil {
		log.Printf("wechat login request failed: %v", err)
		fail(c, http.StatusBadGateway, "WECHAT_REQUEST_FAILED", err.Error())
		return
	}
	if wxResp.ErrCode != 0 {
		log.Printf("wechat login failed with errcode=%d errmsg=%s", wxResp.ErrCode, wxResp.ErrMsg)
		fail(c, http.StatusBadGateway, "WECHAT_LOGIN_FAILED", fmt.Sprintf("wechat login failed: %d %s", wxResp.ErrCode, wxResp.ErrMsg))
		return
	}
	if strings.TrimSpace(wxResp.OpenID) == "" {
		fail(c, http.StatusBadGateway, "WECHAT_OPENID_EMPTY", "wechat openid empty")
		return
	}

	now := time.Now().UnixMilli()
	var user model.User
	err = s.DB.First(&user, "openid = ?", wxResp.OpenID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		nick, nickErr := s.ensureUniqueNickname("wx_" + strings.ToLower(openIDTail(wxResp.OpenID, 6)))
		if nickErr != nil {
			fail(c, http.StatusInternalServerError, "NICKNAME_GENERATE_FAILED", "generate nickname failed")
			return
		}
		user = model.User{
			ID:          "user_wx_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:8]), "-", ""),
			Platform:    "wechat",
			OpenID:      wxResp.OpenID,
			Nickname:    nick,
			AvatarURL:   avatarURLFromSeed(wxResp.OpenID),
			Role:        model.UserRoleUser,
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.DB.Create(&user).Error; err != nil {
			fail(c, http.StatusInternalServerError, "CREATE_USER_FAILED", "create user failed")
			return
		}
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	} else {
		if strings.TrimSpace(user.AvatarURL) == "" {
			user.AvatarURL = avatarURLFromSeed(user.ID)
		}
		user.Role = model.NormalizeUserRole(user.Role)
		user.UpdatedAt = now
		if err := s.DB.Save(&user).Error; err != nil {
			fail(c, http.StatusInternalServerError, "UPDATE_USER_FAILED", "update user failed")
			return
		}
	}
	log.Printf("wechat login success openid=%s uid=%s", maskOpenID(wxResp.OpenID), user.ID)

	normalizeUserModel(&user)
	authResp, err := s.issueAuth(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "TOKEN_SIGN_FAILED", "sign token failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":        authResp.AccessToken,
		"accessToken":  authResp.AccessToken,
		"refreshToken": authResp.RefreshToken,
		"user":         user,
	})
}

type registerReq struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

func (s *Server) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	password := strings.TrimSpace(req.Password)
	if nickname == "" {
		fail(c, http.StatusBadRequest, "NICKNAME_REQUIRED", "nickname required")
		return
	}
	if len(password) < 6 {
		fail(c, http.StatusBadRequest, "PASSWORD_TOO_SHORT", "password must be at least 6 chars")
		return
	}
	var existed model.User
	if err := s.DB.First(&existed, "nickname = ?", nickname).Error; err == nil {
		fail(c, http.StatusConflict, "NICKNAME_ALREADY_EXISTS", "nickname already exists")
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "password hash failed")
		return
	}
	now := time.Now().UnixMilli()
	user := model.User{
		ID:           "user_pwd_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:8]), "-", ""),
		Platform:     "password",
		OpenID:       "pwd_" + nickname,
		Nickname:     nickname,
		PasswordHash: string(hashed),
		AvatarURL:    avatarURLFromSeed(nickname),
		Role:         model.UserRoleUser,
		CreditScore:  100,
		RatingScore:  5,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.DB.Create(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "CREATE_USER_FAILED", "create user failed")
		return
	}
	normalizeUserModel(&user)
	authResp, err := s.issueAuth(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "TOKEN_SIGN_FAILED", "sign token failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":        authResp.AccessToken,
		"accessToken":  authResp.AccessToken,
		"refreshToken": authResp.RefreshToken,
		"user":         user,
	})
}

func (s *Server) PasswordLogin(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	password := strings.TrimSpace(req.Password)
	if nickname == "" || password == "" {
		fail(c, http.StatusBadRequest, "NICKNAME_PASSWORD_REQUIRED", "nickname and password required")
		return
	}
	// Credentials are checked before any account-level throttling, so a caller
	// with the right password always gets in. Only failures are counted.
	rejectAttempt := func() {
		withinBudget, retryAfter := s.noteFailedLogin(nickname)
		if !withinBudget {
			failLoginThrottled(c, retryAfter)
			return
		}
		fail(c, http.StatusUnauthorized, "LOGIN_FAILED", "invalid nickname or password")
	}

	var user model.User
	if err := s.DB.First(&user, "nickname = ?", nickname).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rejectAttempt()
			return
		}
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	}
	if user.DeletedAt > 0 {
		fail(c, http.StatusUnauthorized, "LOGIN_FAILED", "account has been disabled")
		return
	}
	if user.PasswordHash == "" {
		rejectAttempt()
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		rejectAttempt()
		return
	}
	s.clearLoginFailures(nickname)
	if strings.TrimSpace(user.AvatarURL) == "" {
		user.AvatarURL = avatarURLFromSeed(user.ID)
		user.UpdatedAt = time.Now().UnixMilli()
		_ = s.DB.Save(&user).Error
	}
	normalizeUserModel(&user)
	authResp, err := s.issueAuth(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "TOKEN_SIGN_FAILED", "sign token failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":        authResp.AccessToken,
		"accessToken":  authResp.AccessToken,
		"refreshToken": authResp.RefreshToken,
		"user":         user,
	})
}

type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

func (s *Server) RefreshToken(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	rt := strings.TrimSpace(req.RefreshToken)
	if rt == "" {
		fail(c, http.StatusBadRequest, "REFRESH_TOKEN_REQUIRED", "refreshToken required")
		return
	}

	var record model.RefreshToken
	if err := s.DB.First(&record, "token = ?", rt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("refresh failed: token not found")
			fail(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "invalid refresh token")
			return
		}
		fail(c, http.StatusInternalServerError, "QUERY_REFRESH_FAILED", "query refresh token failed")
		return
	}
	now := time.Now().UnixMilli()
	if record.RevokedAt > 0 || record.ExpiresAt <= now {
		log.Printf("refresh failed: token expired or revoked uid=%s", record.UserID)
		fail(c, http.StatusUnauthorized, "REFRESH_TOKEN_EXPIRED", "refresh token expired")
		return
	}
	var refreshUser model.User
	if err := s.DB.Select("id", "deleted_at").First(&refreshUser, "id = ?", record.UserID).Error; err != nil {
		fail(c, http.StatusUnauthorized, "REFRESH_FAILED", "user not found")
		return
	}
	if refreshUser.DeletedAt > 0 {
		fail(c, http.StatusUnauthorized, "REFRESH_FAILED", "account has been disabled")
		return
	}

	authResp, err := s.rotateRefresh(record)
	if err != nil {
		if errors.Is(err, errRefreshTokenReplayed) {
			log.Printf("refresh rejected: token already rotated uid=%s", record.UserID)
			fail(c, http.StatusUnauthorized, "REFRESH_TOKEN_EXPIRED", "refresh token expired")
			return
		}
		log.Printf("refresh failed: rotate error uid=%s err=%v", record.UserID, err)
		fail(c, http.StatusInternalServerError, "REFRESH_FAILED", "refresh failed")
		return
	}
	log.Printf("refresh success uid=%s", record.UserID)
	var user model.User
	if err := s.DB.First(&user, "id = ?", record.UserID).Error; err == nil {
		normalizeUserModel(&user)
		c.JSON(http.StatusOK, gin.H{
			"token":        authResp.AccessToken,
			"accessToken":  authResp.AccessToken,
			"refreshToken": authResp.RefreshToken,
			"user":         user,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":        authResp.AccessToken,
		"accessToken":  authResp.AccessToken,
		"refreshToken": authResp.RefreshToken,
	})
}

func (s *Server) Logout(c *gin.Context) {
	now := time.Now().UnixMilli()

	var req refreshReq
	_ = c.ShouldBindJSON(&req)
	rt := strings.TrimSpace(req.RefreshToken)
	if rt != "" {
		if err := s.DB.Model(&model.RefreshToken{}).
			Where("token = ? AND revoked_at = 0", rt).
			Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
			fail(c, http.StatusInternalServerError, "LOGOUT_FAILED", "logout failed")
			return
		}
		log.Printf("logout revoked refresh token")
	}

	token := bearerTokenFromHeader(c)
	if token != "" {
		claims, err := auth.ParseClaims(token, s.JWTSecret)
		if err == nil && claims.ID != "" {
			if err := s.revokeAccessToken(claims.ID, claims.ExpiresAt.Time.UnixMilli()); err != nil {
				fail(c, http.StatusInternalServerError, "LOGOUT_FAILED", "revoke access token failed")
				return
			}
			log.Printf("logout revoked access token jti=%s", claims.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) Me(c *gin.Context) {
	userID := mustUserID(c)
	var user model.User
	if err := s.DB.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
			return
		}
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	}
	if user.DeletedAt > 0 {
		fail(c, http.StatusUnauthorized, "USER_DISABLED", "user disabled")
		return
	}
	if strings.TrimSpace(user.AvatarURL) == "" {
		user.AvatarURL = avatarURLFromSeed(user.ID)
		user.UpdatedAt = time.Now().UnixMilli()
		_ = s.DB.Save(&user).Error
	}
	normalizeUserModel(&user)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

type timeInfoReq struct {
	Mode      string `json:"mode"`
	Days      int    `json:"days"`
	FixedTime string `json:"fixedTime"`
}

type coordsReq struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type postUpsertReq struct {
	Title         string                    `json:"title"`
	Description   string                    `json:"description"`
	Category      string                    `json:"category"`
	SubCategory   string                    `json:"subCategory"`
	TimeInfo      timeInfoReq               `json:"timeInfo"`
	Address       string                    `json:"address"`
	Coords        *coordsReq                `json:"coords"`
	MaxCount      int                       `json:"maxCount"`
	Invitations   []postInvitationCreateReq `json:"invitations"`
	InviteeIDs    []string                  `json:"inviteeIds"`
	InviteMessage string                    `json:"inviteMessage"`
}

func (req postUpsertReq) validate(nowMS int64, minMaxCount int, minMaxCountErr string) error {
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Category) == "" || strings.TrimSpace(req.Address) == "" {
		return errors.New("title/category/address required")
	}
	if req.MaxCount < minMaxCount {
		return errors.New(minMaxCountErr)
	}
	return validateTimeInfo(req.TimeInfo, nowMS)
}

func (req postUpsertReq) applyToPost(post *model.Post, nowMS int64) {
	post.Title = strings.TrimSpace(req.Title)
	post.Description = strings.TrimSpace(req.Description)
	post.Category = strings.TrimSpace(req.Category)
	post.SubCategory = strings.TrimSpace(req.SubCategory)
	post.TimeMode = strings.TrimSpace(req.TimeInfo.Mode)
	post.TimeDays = req.TimeInfo.Days
	post.FixedTime = strings.TrimSpace(req.TimeInfo.FixedTime)
	post.Address = strings.TrimSpace(req.Address)
	post.MaxCount = req.MaxCount
	post.UpdatedAt = nowMS
	if req.Coords != nil {
		post.Lat = req.Coords.Latitude
		post.Lng = req.Coords.Longitude
	}
}

func (s *Server) ListPosts(c *gin.Context) {
	sortBy := c.DefaultQuery("sortBy", "latest")
	page := queryIntOrDefault(c.Query("page"), 1)
	pageSize := queryIntOrDefault(c.Query("pageSize"), 10)
	if pageSize > 20 {
		pageSize = 20
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	viewerLocation := queryGeoPoint(c)
	var posts []model.Post
	query := activePostsQuery(s.DB.Model(&model.Post{}))
	if category := strings.TrimSpace(c.Query("category")); category != "" {
		query = query.Where("category = ?", category)
	}
	if subCategory := strings.TrimSpace(c.Query("subCategory")); subCategory != "" {
		query = query.Where("sub_category = ?", subCategory)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		query = query.
			Joins("LEFT JOIN users ON users.id = posts.author_id").
			Select("posts.*").
			Where(
				"posts.title LIKE ? OR posts.description LIKE ? OR posts.address LIKE ? OR users.nickname LIKE ?",
				like,
				like,
				like,
				like,
			)
	}
	if addressKeyword := strings.TrimSpace(c.Query("addressKeyword")); addressKeyword != "" {
		query = query.Where("address LIKE ?", "%"+addressKeyword+"%")
	}

	nowMS := time.Now().UnixMilli()
	recommendationMap := map[string]recommendationView{}
	feedRequestID := ""
	hasMore := false
	nextPage := 0
	if sortBy == "hot" {
		if err := query.Find(&posts).Error; err != nil {
			serverError(c, err)
			return
		}
		var err error
		posts, recommendationMap, feedRequestID, err = s.buildRecommendedFeed(posts, viewerID, viewerLocation, nowMS)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "build recommendation feed failed"})
			return
		}
		start := (page - 1) * pageSize
		if start < 0 {
			start = 0
		}
		if start < len(posts) {
			end := start + pageSize
			if end > len(posts) {
				end = len(posts)
			}
			hasMore = end < len(posts)
			if hasMore {
				nextPage = page + 1
			}
			posts = posts[start:end]
		} else {
			posts = []model.Post{}
		}
	} else {
		offset := (page - 1) * pageSize
		if offset < 0 {
			offset = 0
		}
		if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize + 1).Find(&posts).Error; err != nil {
			serverError(c, err)
			return
		}
		if len(posts) > pageSize {
			hasMore = true
			nextPage = page + 1
			posts = posts[:pageSize]
		}
	}

	views, err := s.buildPostViewsForViewer(posts, viewerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post author failed"})
		return
	}
	for i := range views {
		if rec, ok := recommendationMap[views[i].ID]; ok {
			views[i].Recommendation = rec
		}
	}
	resp := gin.H{
		"posts":    views,
		"page":     page,
		"pageSize": pageSize,
		"hasMore":  hasMore,
		"nextPage": nextPage,
	}
	if feedRequestID != "" {
		resp["feedRequestId"] = feedRequestID
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) GetPost(c *gin.Context) {
	postID := c.Param("id")
	viewerID := optionalUserIDFromRequest(c, s.JWTSecret)
	cached := false
	var post model.Post
	var participants []model.PostParticipant

	if cachedPost, cachedParticipants, ok := s.getCachedPostDetail(c.Request.Context(), postID); ok {
		post = cachedPost
		participants = cachedParticipants
		cached = true
	}
	if post.ID == "" {
		if err := activePostsQuery(s.DB).First(&post, "id = ?", postID).Error; err != nil {
			writeRecordError(c, err, "post not found", "")
			return
		}
		if err := s.DB.Find(&participants, "post_id = ? AND status = ?", post.ID, score.ParticipantStatusActive).Error; err != nil {
			serverError(c, err)
			return
		}
		s.setCachedPostDetail(c.Request.Context(), post, participants)
	}

	views, err := s.buildPostViewsForViewer([]model.Post{post}, viewerID)
	if err != nil || len(views) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post author failed"})
		return
	}
	participantIDs := make([]string, 0, len(participants))
	for _, item := range participants {
		participantIDs = append(participantIDs, item.UserID)
	}
	participantUserMap, err := s.usersByIDs(participantIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query participants failed"})
		return
	}
	participantViews := make([]userBrief, 0, len(participants))
	for _, item := range participants {
		user, ok := participantUserMap[item.UserID]
		if !ok {
			user = model.User{
				ID:          item.UserID,
				Nickname:    "未知用户",
				AvatarURL:   avatarURLFromSeed(item.UserID),
				CreditScore: 100,
				RatingScore: 5,
			}
		}
		participantViews = append(participantViews, toUserBrief(user))
	}

	resp := gin.H{
		"post":         views[0],
		"participants": participantViews,
	}
	setPostETag(c, post)
	if cached {
		resp["cached"] = true
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) CreatePost(c *gin.Context) {
	userID := mustUserID(c)

	var req postUpsertReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}

	now := time.Now().UnixMilli()
	if err := req.validate(now, 2, "maxCount must be >= 2"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	post := model.Post{
		ID:           "post_" + uuid.NewString()[:8],
		AuthorID:     userID,
		CurrentCount: 1,
		Status:       "open",
		CreatedAt:    now,
	}
	req.applyToPost(&post, now)

	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&post).Error; err != nil {
			return err
		}
		if err := enqueuePostModerationTx(tx, &post, moderationIdempotencyKey(c, post.ID, postContentHash(post)), now); err != nil {
			return err
		}
		if err := tx.Model(&model.Post{}).Where("id = ?", post.ID).Updates(map[string]any{
			"moderation_status": post.ModerationStatus, "current_moderation_id": post.CurrentModerationID,
			"content_hash": post.ContentHash, "moderation_updated_at": post.ModerationUpdatedAt,
		}).Error; err != nil {
			return err
		}
		emitDomainEvent(tx, "post.created", "post", post.ID, userID, map[string]string{"title": post.Title, "category": post.Category})
		return s.createPostInvitationsTx(tx, post.ID, userID, req.invitationInputs(), now)
	}); err != nil {
		serverError(c, err)
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	s.rebuildUserTagsForUsers([]string{userID})
	s.pushRecommendationEvent(c.Request.Context(), "post_created", map[string]any{
		"postId":       post.ID,
		"authorId":     post.AuthorID,
		"category":     post.Category,
		"subCategory":  post.SubCategory,
		"address":      post.Address,
		"currentCount": post.CurrentCount,
		"maxCount":     post.MaxCount,
		"createdAt":    post.CreatedAt,
	})
	views, err := s.buildPostViews([]model.Post{post})
	if err != nil || len(views) == 0 {
		setPostETag(c, post)
		c.JSON(http.StatusOK, gin.H{"post": post})
		return
	}
	setPostETag(c, post)
	c.JSON(http.StatusOK, gin.H{"post": views[0]})
}

func (s *Server) UpdatePost(c *gin.Context) {
	userID := mustUserID(c)

	var post model.Post
	if err := s.DB.First(&post, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	if post.AuthorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission"})
		return
	}

	var req postUpsertReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	now := time.Now().UnixMilli()
	if err := req.validate(now, post.CurrentCount, "maxCount cannot be less than currentCount"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expectedUpdatedAt := strings.TrimSpace(c.GetHeader("If-Match"))
	if expectedUpdatedAt != "" && strings.Trim(expectedUpdatedAt, `"`) != strconv.FormatInt(post.UpdatedAt, 10) {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "post was modified; refresh and retry"})
		return
	}
	req.applyToPost(&post, now)

	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		var current model.Post
		if err := tx.First(&current, "id = ?", post.ID).Error; err != nil {
			return err
		}
		if err := enqueuePostModerationTx(tx, &post, moderationIdempotencyKey(c, post.ID, postContentHash(post)), now); err != nil {
			return err
		}
		result := tx.Model(&model.Post{}).Where("id = ? AND updated_at = ?", post.ID, current.UpdatedAt).
			Updates(map[string]any{
				"title": post.Title, "description": post.Description, "category": post.Category,
				"sub_category": post.SubCategory, "time_mode": post.TimeMode, "time_days": post.TimeDays,
				"fixed_time": post.FixedTime, "address": post.Address, "lat": post.Lat, "lng": post.Lng,
				"max_count": post.MaxCount, "updated_at": post.UpdatedAt,
				"moderation_status": post.ModerationStatus, "current_moderation_id": post.CurrentModerationID,
				"content_hash": post.ContentHash, "moderation_updated_at": post.ModerationUpdatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrInvalidData
		}
		emitDomainEvent(tx, "post.updated", "post", post.ID, userID, map[string]string{"title": post.Title, "category": post.Category})
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrInvalidData) {
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "post was modified; refresh and retry"})
			return
		}
		serverError(c, err)
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	s.rebuildUserTagsForUsers([]string{userID})
	s.pushRecommendationEvent(c.Request.Context(), "post_updated", map[string]any{
		"postId":       post.ID,
		"authorId":     post.AuthorID,
		"category":     post.Category,
		"subCategory":  post.SubCategory,
		"address":      post.Address,
		"currentCount": post.CurrentCount,
		"maxCount":     post.MaxCount,
		"updatedAt":    post.UpdatedAt,
	})
	views, err := s.buildPostViews([]model.Post{post})
	if err != nil || len(views) == 0 {
		setPostETag(c, post)
		c.JSON(http.StatusOK, gin.H{"post": post})
		return
	}
	setPostETag(c, post)
	c.JSON(http.StatusOK, gin.H{"post": views[0]})
}

func setPostETag(c *gin.Context, post model.Post) {
	if post.UpdatedAt > 0 {
		c.Header("ETag", strconv.Quote(strconv.FormatInt(post.UpdatedAt, 10)))
	}
}

func (s *Server) JoinPost(c *gin.Context) {
	userID := mustUserID(c)
	postID := c.Param("id")
	now := time.Now().UnixMilli()

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.ModerationStatus != "" && post.ModerationStatus != model.ModerationApproved {
			return errJoinPostPending
		}
		_, err := s.joinPostTx(tx, postID, userID, now)
		if err == nil {
			emitDomainEvent(tx, "participant.joined", "post", postID, userID, nil)
		}
		return err
	})

	if err != nil {
		switch {
		case errors.Is(err, errJoinAuthorOwnPost), errors.Is(err, errJoinPostClosed), errors.Is(err, errJoinPostPending):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, errJoinPostFull):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, errJoinAlreadyJoined):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
				return
			}
			serverError(c, err)
		}
		return
	}

	var post model.Post
	_ = s.DB.First(&post, "id = ?", postID).Error
	s.invalidatePostsCache(c.Request.Context())
	s.rebuildUserTagsForUsers([]string{userID})
	s.pushRecommendationEvent(c.Request.Context(), "post_joined", map[string]any{
		"postId":       postID,
		"userId":       userID,
		"authorId":     post.AuthorID,
		"currentCount": post.CurrentCount,
		"updatedAt":    post.UpdatedAt,
	})
	c.JSON(http.StatusOK, gin.H{
		"joined":       true,
		"currentCount": post.CurrentCount,
	})
}

func (s *Server) ClosePost(c *gin.Context) {
	userID := mustUserID(c)
	postID := c.Param("id")

	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		writeRecordError(c, err, "post not found", "")
		return
	}
	if post.AuthorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission"})
		return
	}
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if post.Status != "closed" {
			post.Status = "closed"
			post.ClosedAt = now
			post.UpdatedAt = now
			if err := tx.Save(&post).Error; err != nil {
				return err
			}
			emitDomainEvent(tx, "post.closed", "post", post.ID, userID, nil)
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		serverError(c, err)
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	s.pushRecommendationEvent(c.Request.Context(), "post_closed", map[string]any{
		"postId":       post.ID,
		"authorId":     post.AuthorID,
		"currentCount": post.CurrentCount,
		"updatedAt":    post.UpdatedAt,
	})
	c.JSON(http.StatusOK, gin.H{"post": post})
}

func (s *Server) ListChatMessages(c *gin.Context) {
	userID := mustUserID(c)
	postID := c.Param("postId")
	isMember, err := s.isPostMember(postID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post member failed"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "only participants can read messages"})
		return
	}
	var list []model.ChatMessage
	query := s.DB.Where("post_id = ?", postID)
	if rawSince := strings.TrimSpace(c.Query("sinceCreatedAt")); rawSince != "" {
		sinceCreatedAt, err := strconv.ParseInt(rawSince, 10, 64)
		if err != nil || sinceCreatedAt < 0 {
			fail(c, http.StatusBadRequest, "INVALID_SINCE_CREATED_AT", "sinceCreatedAt must be a non-negative millisecond timestamp")
			return
		}
		limit := 200
		if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
			value, err := strconv.Atoi(rawLimit)
			if err != nil || value <= 0 {
				fail(c, http.StatusBadRequest, "INVALID_LIMIT", "limit must be a positive integer")
				return
			}
			limit = value
		}
		if limit > 500 {
			limit = 500
		}
		// The timestamp boundary is inclusive. Multiple messages may share one
		// millisecond, so clients merge by message ID before advancing.
		query = query.Where("created_at >= ?", sinceCreatedAt).Limit(limit)
	}
	// Without sinceCreatedAt this deliberately retains the original complete
	// history response, even if a caller happens to provide limit.
	if err := query.Order("created_at ASC, id ASC").Find(&list).Error; err != nil {
		serverError(c, err)
		return
	}
	views, err := s.buildChatMessageViews(list)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query chat sender failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": views})
}

type sendMsgReq struct {
	Content     string `json:"content"`
	ClientMsgID string `json:"clientMsgId"`
}

func (s *Server) SendChatMessage(c *gin.Context) {
	userID := mustUserID(c)
	postID := c.Param("postId")
	isMember, err := s.isPostMember(postID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query post member failed"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "only participants can send message"})
		return
	}

	var req sendMsgReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content required"})
		return
	}

	if req.ClientMsgID != "" {
		var existed model.ChatMessage
		if err := s.DB.First(&existed, "post_id = ? AND client_msg_id = ?", postID, req.ClientMsgID).Error; err == nil {
			c.JSON(http.StatusOK, gin.H{"message": existed})
			return
		}
	}
	clientMsgID := strings.TrimSpace(req.ClientMsgID)
	if clientMsgID == "" {
		clientMsgID = "client_" + uuid.NewString()[:12]
	}

	msg := model.ChatMessage{
		ID:          "msg_" + uuid.NewString()[:10],
		PostID:      postID,
		SenderID:    userID,
		Content:     content,
		ClientMsgID: clientMsgID,
		CreatedAt:   time.Now().UnixMilli(),
	}
	if err := s.DB.Create(&msg).Error; err != nil {
		serverError(c, err)
		return
	}
	var senderMessageCount int64
	if err := s.DB.Model(&model.ChatMessage{}).
		Where("post_id = ? AND sender_id = ?", postID, userID).
		Count(&senderMessageCount).Error; err == nil && senderMessageCount == 1 {
		s.rebuildUserTagsForUsers([]string{userID})
		s.pushRecommendationEvent(c.Request.Context(), "chat_first_message", map[string]any{
			"postId":    postID,
			"userId":    userID,
			"createdAt": msg.CreatedAt,
		})
	}
	s.publishChatMessage(c.Request.Context(), msg)
	views, err := s.buildChatMessageViews([]model.ChatMessage{msg})
	if err != nil || len(views) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": views[0]})
}

type reviewItemReq struct {
	ToUserID string `json:"toUserId"`
	Rating   int    `json:"rating"`
	Comment  string `json:"comment"`
}

type reviewUpsertReq struct {
	Items []reviewItemReq `json:"items"`
}

func (s *Server) UpsertReviews(c *gin.Context) {
	fromUserID := mustUserID(c)
	postID := c.Param("id")

	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		writeRecordError(c, err, "post not found", "")
		return
	}
	if post.Status != "closed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "review is allowed only after post is closed"})
		return
	}
	fromIsMember, err := s.isPostMember(postID, fromUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query reviewer relation failed"})
		return
	}
	if !fromIsMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "only participants can review"})
		return
	}

	var req reviewUpsertReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty review items"})
		return
	}

	now := time.Now().UnixMilli()
	allowedTargets, err := s.allowedReviewTargets(post, fromUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query review target relation failed"})
		return
	}
	if len(allowedTargets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no review targets available"})
		return
	}

	seenTargets := make(map[string]struct{}, len(req.Items))
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			targetID := strings.TrimSpace(item.ToUserID)
			if targetID == "" || item.Rating < 1 || item.Rating > 5 {
				return errors.New("invalid review item")
			}
			if _, ok := allowedTargets[targetID]; !ok {
				return errors.New("review target not allowed")
			}
			if _, duplicated := seenTargets[targetID]; duplicated {
				return errors.New("duplicate review target")
			}
			seenTargets[targetID] = struct{}{}

			review := model.Review{
				PostID:     postID,
				FromUserID: fromUserID,
				ToUserID:   targetID,
				Rating:     item.Rating,
				Comment:    strings.TrimSpace(item.Comment),
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "post_id"}, {Name: "from_user_id"}, {Name: "to_user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"rating", "comment", "updated_at"}),
			}).Create(&review).Error; err != nil {
				return err
			}
		}
		return score.RecalculatePostActivityScoresTx(tx, post, now)
	})
	if err != nil {
		switch err.Error() {
		case "invalid review item", "review target not allowed", "duplicate review target":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			serverError(c, err)
		}
		return
	}
	targets := make([]string, 0, len(seenTargets))
	for targetID := range seenTargets {
		targets = append(targets, targetID)
	}
	sort.Strings(targets)
	s.pushRecommendationEvent(c.Request.Context(), "review_written", map[string]any{
		"postId":     postID,
		"fromUserId": fromUserID,
		"targets":    strings.Join(targets, ","),
		"createdAt":  now,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func userIDFromRequest(c *gin.Context, jwtSecret string) (string, string, string, bool) {
	token := bearerTokenFromHeader(c)
	if token != "" {
		claims, err := auth.ParseClaims(token, jwtSecret)
		if err == nil && claims.UserID != "" {
			return claims.UserID, model.NormalizeUserRole(claims.Role), claims.ID, true
		}
	}
	return "", "", "", false
}

func optionalUserIDFromRequest(c *gin.Context, jwtSecret string) string {
	userID, _, _, ok := userIDFromRequest(c, jwtSecret)
	if !ok {
		return ""
	}
	return strings.TrimSpace(userID)
}

func (s *Server) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, jti, ok := userIDFromRequest(c, s.JWTSecret)
		if !ok {
			fail(c, http.StatusUnauthorized, "AUTH_REQUIRED", "missing user identity")
			c.Abort()
			return
		}
		if jti != "" {
			revoked, err := s.isAccessTokenRevoked(jti)
			if err != nil {
				fail(c, http.StatusInternalServerError, "AUTH_CHECK_FAILED", "auth check failed")
				c.Abort()
				return
			}
			if revoked {
				fail(c, http.StatusUnauthorized, "ACCESS_TOKEN_REVOKED", "access token revoked")
				c.Abort()
				return
			}
			c.Set(contextTokenJTIKey, jti)
		}
		resolvedRole, rootAdmin, deleted, found := s.resolveUserAccess(userID)
		if !found {
			fail(c, http.StatusUnauthorized, "USER_NOT_FOUND", "user no longer exists")
			c.Abort()
			return
		}
		if deleted {
			fail(c, http.StatusUnauthorized, "USER_DISABLED", "account has been disabled")
			c.Abort()
			return
		}
		c.Set(contextUserIDKey, userID)
		c.Set(contextUserRoleKey, resolvedRole)
		c.Set(contextUserRootAdminKey, rootAdmin)
		c.Next()
	}
}

func mustUserID(c *gin.Context) string {
	raw, ok := c.Get(contextUserIDKey)
	if !ok {
		return ""
	}
	userID, _ := raw.(string)
	return userID
}

func (s *Server) isPostMember(postID, userID string) (bool, error) {
	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if post.AuthorID == userID {
		return true, nil
	}

	var count int64
	if err := s.DB.Model(&model.PostParticipant{}).
		Where("post_id = ? AND user_id = ? AND status = ?", postID, userID, score.ParticipantStatusActive).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Server) allowedReviewTargets(post model.Post, fromUserID string) (map[string]struct{}, error) {
	targets := make(map[string]struct{})
	authorID := strings.TrimSpace(post.AuthorID)
	reviewerID := strings.TrimSpace(fromUserID)
	if reviewerID == "" {
		return targets, nil
	}

	var relations []model.PostParticipant
	if err := s.DB.Where("post_id = ? AND status = ?", post.ID, score.ParticipantStatusActive).Find(&relations).Error; err != nil {
		return nil, err
	}
	participantSet := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		userID := strings.TrimSpace(relation.UserID)
		if userID == "" {
			continue
		}
		participantSet[userID] = struct{}{}
	}

	if reviewerID == authorID {
		for userID := range participantSet {
			if userID == reviewerID {
				continue
			}
			targets[userID] = struct{}{}
		}
		return targets, nil
	}

	if _, ok := participantSet[reviewerID]; ok && authorID != "" && authorID != reviewerID {
		targets[authorID] = struct{}{}
	}
	return targets, nil
}

func bearerTokenFromHeader(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

func (s *Server) isAccessTokenRevoked(jti string) (bool, error) {
	var count int64
	err := s.DB.Model(&model.RevokedAccessToken{}).
		Where("jti = ? AND expires_at > ?", jti, time.Now().UnixMilli()).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Server) revokeAccessToken(jti string, expiresAt int64) error {
	if jti == "" || expiresAt <= 0 {
		return nil
	}
	item := model.RevokedAccessToken{
		JTI:       jti,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UnixMilli(),
	}
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
}

type authTokens struct {
	AccessToken  string
	RefreshToken string
}

func (s *Server) issueAuth(userID string) (authTokens, error) {
	role := s.resolveUserRole(userID)
	accessToken, err := auth.SignToken(userID, s.JWTSecret, s.TokenExpireHr, role)
	if err != nil {
		return authTokens{}, err
	}
	refreshToken, err := auth.NewRefreshToken()
	if err != nil {
		return authTokens{}, err
	}
	now := time.Now().UnixMilli()
	refresh := model.RefreshToken{
		Token:     refreshToken,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Duration(s.RefreshExpire) * time.Hour).UnixMilli(),
		RevokedAt: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.DB.Create(&refresh).Error; err != nil {
		return authTokens{}, err
	}
	return authTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *Server) rotateRefresh(record model.RefreshToken) (authTokens, error) {
	returnTokens := authTokens{}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		// Revoking is the claim on this token: if no row changed, another
		// request already rotated it and this one is a replay.
		result := tx.Model(&model.RefreshToken{}).
			Where("id = ? AND revoked_at = 0", record.ID).
			Updates(map[string]any{"revoked_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errRefreshTokenReplayed
		}

		role := s.resolveUserRole(record.UserID)
		accessToken, err := auth.SignToken(record.UserID, s.JWTSecret, s.TokenExpireHr, role)
		if err != nil {
			return err
		}
		newRefresh, err := auth.NewRefreshToken()
		if err != nil {
			return err
		}
		next := model.RefreshToken{
			Token:     newRefresh,
			UserID:    record.UserID,
			ExpiresAt: time.Now().Add(time.Duration(s.RefreshExpire) * time.Hour).UnixMilli(),
			RevokedAt: 0,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		returnTokens.AccessToken = accessToken
		returnTokens.RefreshToken = newRefresh
		return nil
	})
	if err != nil {
		return authTokens{}, err
	}
	return returnTokens, nil
}

func (s *Server) fetchWechatSession(code string) (*wechatSessionResp, error) {
	wxURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		url.QueryEscape(s.WechatAppID),
		url.QueryEscape(s.WechatSecret),
		url.QueryEscape(code),
	)

	var lastErr error
	for i := 0; i < 2; i++ {
		resp, err := s.HTTPClient.Get(wxURL)
		if err != nil {
			lastErr = err
			if isRetryableNetErr(err) && i == 0 {
				continue
			}
			return nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if i == 0 {
				continue
			}
			return nil, readErr
		}

		var wxResp wechatSessionResp
		if err := json.Unmarshal(body, &wxResp); err != nil {
			return nil, errors.New("invalid wechat response")
		}
		return &wxResp, nil
	}

	if lastErr == nil {
		lastErr = errors.New("wechat request failed")
	}
	return nil, lastErr
}

func isRetryableNetErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": message,
		"code":  code,
	})
}

func (s *Server) ensureUniqueNickname(base string) (string, error) {
	nickname := strings.TrimSpace(base)
	if nickname == "" {
		nickname = "user_" + strings.ToLower(uuid.NewString()[:6])
	}
	candidate := nickname
	for i := 0; i < 20; i++ {
		var count int64
		if err := s.DB.Model(&model.User{}).Where("nickname = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s_%d", nickname, i+1)
	}
	return "", errors.New("generate unique nickname failed")
}
func maskOpenID(openid string) string {
	if len(openid) <= 6 {
		return "***"
	}
	return openid[:3] + "***" + openid[len(openid)-3:]
}

func openIDTail(openid string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(openid) <= maxLen {
		return openid
	}
	return openid[len(openid)-maxLen:]
}
