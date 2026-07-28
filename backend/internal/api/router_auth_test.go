package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/auth"
	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type loginResp struct {
	Token        string `json:"token"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type errResp struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

const testJWTSecret = "router_test_secret"

func openRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("JWT_SECRET", testJWTSecret)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.PostParticipant{},
		&model.ChatMessage{},
		&model.PostInvitation{},
		&model.Review{},
		&model.ActivityScore{},
		&model.PostParticipantSettlement{},
		&model.CreditLedger{},
		&model.AdminCase{},
		&model.ModerationRecord{},
		&model.CaseEvent{},
		&model.OutboxEvent{},
		&model.DomainEvent{},
		&model.AgentRun{},
		&model.AgentStep{},
		&model.ContentSnapshot{},
		&model.Notification{},
		&model.Report{},
		&model.CaseEvidence{},
		&model.CaseDecision{},
		&model.UserTag{},
		&model.UserBehaviorProfile{},
		&model.FeedExposure{},
		&model.FeedClick{},
		&model.PostEmbedding{},
		&model.UserEmbedding{},
		&model.RecommendationModel{},
		&model.RefreshToken{},
		&model.RevokedAccessToken{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

// ensureTestUser creates a minimal user row when the test has not seeded one
// itself; RequireAuth rejects tokens whose user does not exist in the DB.
func ensureTestUser(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	var count int64
	if err := db.Model(&model.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("lookup test user failed: %v", err)
	}
	if count > 0 {
		return
	}
	now := time.Now().UnixMilli()
	user := model.User{
		ID:          userID,
		Platform:    "test",
		OpenID:      "test_" + userID,
		Nickname:    userID,
		Role:        model.UserRoleUser,
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create test user failed: %v", err)
	}
}

// bearerFor returns an Authorization header value carrying a real JWT for
// userID, creating the user row if needed.
func bearerFor(t *testing.T, db *gorm.DB, userID string) string {
	t.Helper()
	ensureTestUser(t, db, userID)
	token, err := auth.SignToken(userID, testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign test token failed: %v", err)
	}
	return "Bearer " + token
}

func TestCreatePostRequiresAuth(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)

	body := map[string]any{
		"title":    "test post",
		"category": "running",
		"address":  "test address",
		"maxCount": 4,
		"timeInfo": map[string]any{
			"mode":      "range",
			"days":      1,
			"fixedTime": "",
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestCreatePostWithJWTAndFutureFixedTime(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_test_001")
	token, err := auth.SignToken("user_test_001", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}

	router := NewRouter(db)
	body := map[string]any{
		"title":    "jwt post",
		"category": "badminton",
		"address":  "test address",
		"maxCount": 3,
		"timeInfo": map[string]any{
			"mode":      "fixed",
			"days":      0,
			"fixedTime": time.Now().Add(2 * time.Hour).Format(time.RFC3339),
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Post model.Post `json:"post"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create post resp failed: %v", err)
	}
	if payload.Post.CurrentCount != 1 {
		t.Fatalf("create post currentCount should start from 1, got=%d", payload.Post.CurrentCount)
	}
}

func TestPostInvitationFlow(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	users := []model.User{
		{
			ID:          "user_inviter",
			Platform:    "mock",
			OpenID:      "openid_inviter",
			Nickname:    "inviter",
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "user_invitee",
			Platform:    "mock",
			OpenID:      "openid_invitee",
			Nickname:    "invitee",
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}

	router := NewRouter(db)
	body := map[string]any{
		"title":         "invited post",
		"category":      "badminton",
		"address":       "test court",
		"maxCount":      3,
		"inviteeIds":    []string{"user_invitee"},
		"inviteMessage": "一起打球吗？",
		"timeInfo": map[string]any{
			"mode": "range",
			"days": 2,
		},
	}
	raw, _ := json.Marshal(body)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(raw))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", bearerFor(t, db, "user_inviter"))
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create invited post failed, code=%d body=%s", createResp.Code, createResp.Body.String())
	}

	var createPayload struct {
		Post model.Post `json:"post"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create post resp failed: %v", err)
	}
	// Approve the post so invitation acceptance isn't blocked by moderation hold
	db.Model(&model.Post{}).Where("id = ?", createPayload.Post.ID).Update("moderation_status", "approved")

	var invitation model.PostInvitation
	if err := db.First(&invitation, "post_id = ? AND invitee_id = ?", createPayload.Post.ID, "user_invitee").Error; err != nil {
		t.Fatalf("invitation should be created: %v", err)
	}
	if invitation.Status != model.InvitationStatusPending || invitation.Message != "一起打球吗？" {
		t.Fatalf("unexpected invitation=%+v", invitation)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/invitations", nil)
	listReq.Header.Set("Authorization", bearerFor(t, db, "user_invitee"))
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list invitations failed, code=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Invitations []postInvitationView `json:"invitations"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode invitation list failed: %v", err)
	}
	if len(listPayload.Invitations) != 1 || listPayload.Invitations[0].ID != invitation.ID {
		t.Fatalf("expect one invitation, got=%+v", listPayload.Invitations)
	}
	if listPayload.Invitations[0].Inviter.ID != "user_inviter" || listPayload.Invitations[0].Invitee.ID != "user_invitee" {
		t.Fatalf("invitation participants not hydrated, got=%+v", listPayload.Invitations[0])
	}

	sentReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/sent-invitations", nil)
	sentReq.Header.Set("Authorization", bearerFor(t, db, "user_inviter"))
	sentResp := httptest.NewRecorder()
	router.ServeHTTP(sentResp, sentReq)
	if sentResp.Code != http.StatusOK {
		t.Fatalf("list sent invitations failed, code=%d body=%s", sentResp.Code, sentResp.Body.String())
	}
	var sentPayload struct {
		Invitations []postInvitationView `json:"invitations"`
	}
	if err := json.Unmarshal(sentResp.Body.Bytes(), &sentPayload); err != nil {
		t.Fatalf("decode sent invitation list failed: %v", err)
	}
	if len(sentPayload.Invitations) != 1 || sentPayload.Invitations[0].Invitee.ID != "user_invitee" {
		t.Fatalf("expect one sent invitation with invitee, got=%+v", sentPayload.Invitations)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", bearerFor(t, db, "user_invitee"))
	acceptResp := httptest.NewRecorder()
	router.ServeHTTP(acceptResp, acceptReq)
	if acceptResp.Code != http.StatusOK {
		t.Fatalf("accept invitation failed, code=%d body=%s", acceptResp.Code, acceptResp.Body.String())
	}

	var participant model.PostParticipant
	if err := db.First(&participant, "post_id = ? AND user_id = ?", createPayload.Post.ID, "user_invitee").Error; err != nil {
		t.Fatalf("accept should create participant: %v", err)
	}
	if err := db.First(&invitation, "id = ?", invitation.ID).Error; err != nil {
		t.Fatalf("reload invitation failed: %v", err)
	}
	if invitation.Status != model.InvitationStatusAccepted || invitation.RespondedAt == 0 {
		t.Fatalf("invitation should be accepted, got=%+v", invitation)
	}
}

func TestCancelInvitationFlow(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	users := []model.User{
		{ID: "user_cancel_inviter", Platform: "mock", OpenID: "openid_cancel_inviter", Nickname: "cancel_inviter", Role: model.UserRoleUser, CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_cancel_invitee", Platform: "mock", OpenID: "openid_cancel_invitee", Nickname: "cancel_invitee", Role: model.UserRoleUser, CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_cancel_other", Platform: "mock", OpenID: "openid_cancel_other", Nickname: "cancel_other", Role: model.UserRoleUser, CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	post := model.Post{
		ID:           "post_cancel_invite",
		AuthorID:     "user_cancel_inviter",
		Title:        "cancel invite post",
		Category:     "运动",
		SubCategory:  "羽毛球",
		TimeMode:     "range",
		TimeDays:     2,
		Address:      "test court",
		MaxCount:     4,
		CurrentCount: 1,
		Status:       "open",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	invitation := model.PostInvitation{
		ID:        "invite_cancel_test",
		PostID:    post.ID,
		InviterID: "user_cancel_inviter",
		InviteeID: "user_cancel_invitee",
		Message:   "一起打球吗？",
		Status:    model.InvitationStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&invitation).Error; err != nil {
		t.Fatalf("create invitation failed: %v", err)
	}

	router := NewRouter(db)
	otherReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.ID+"/cancel", nil)
	otherReq.Header.Set("Authorization", bearerFor(t, db, "user_cancel_other"))
	otherResp := httptest.NewRecorder()
	router.ServeHTTP(otherResp, otherReq)
	if otherResp.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden cancel by non-inviter, got=%d body=%s", otherResp.Code, otherResp.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.ID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", bearerFor(t, db, "user_cancel_inviter"))
	cancelResp := httptest.NewRecorder()
	router.ServeHTTP(cancelResp, cancelReq)
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("cancel invitation failed, code=%d body=%s", cancelResp.Code, cancelResp.Body.String())
	}
	if err := db.First(&invitation, "id = ?", invitation.ID).Error; err != nil {
		t.Fatalf("reload invitation failed: %v", err)
	}
	if invitation.Status != model.InvitationStatusCancelled || invitation.RespondedAt == 0 {
		t.Fatalf("invitation should be cancelled, got=%+v", invitation)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/"+invitation.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", bearerFor(t, db, "user_cancel_invitee"))
	acceptResp := httptest.NewRecorder()
	router.ServeHTTP(acceptResp, acceptReq)
	if acceptResp.Code != http.StatusConflict {
		t.Fatalf("cancelled invitation should not be accepted, got=%d body=%s", acceptResp.Code, acceptResp.Body.String())
	}
}

func TestInviteCalendarHeatmapAndCandidates(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	targetDate := dateOnly(time.Now()).AddDate(0, 0, 1)
	weekday := int(targetDate.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	users := []model.User{
		{ID: "user_invite_calendar_author", Platform: "mock", OpenID: "openid_invite_calendar_author", Nickname: "author", Role: model.UserRoleUser, CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_invite_calendar_a", Platform: "mock", OpenID: "openid_invite_calendar_a", Nickname: "alpha", Role: model.UserRoleUser, CreditScore: 98, RatingScore: 4.9, CreatedAt: now, UpdatedAt: now + 30},
		{ID: "user_invite_calendar_b", Platform: "mock", OpenID: "openid_invite_calendar_b", Nickname: "beta", Role: model.UserRoleUser, CreditScore: 92, RatingScore: 4.6, CreatedAt: now, UpdatedAt: now + 20},
		{ID: "user_invite_calendar_weak", Platform: "mock", OpenID: "openid_invite_calendar_weak", Nickname: "weak", Role: model.UserRoleUser, CreditScore: 70, RatingScore: 3.8, CreatedAt: now, UpdatedAt: now + 10},
		{ID: "user_invite_calendar_deleted", Platform: "mock", OpenID: "openid_invite_calendar_deleted", Nickname: "deleted", Role: model.UserRoleUser, CreditScore: 100, RatingScore: 5, DeletedAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: "user_invite_calendar_admin", Platform: "mock", OpenID: "openid_invite_calendar_admin", Nickname: "admin", Role: model.UserRoleAdmin, CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	profiles := []model.UserBehaviorProfile{
		{
			UserID:                   "user_invite_calendar_a",
			ActiveWeekdaysJSON:       fmt.Sprintf(`[{"weekday":%d,"weight":0.95}]`, weekday),
			JoinWeekdaysJSON:         fmt.Sprintf(`[{"weekday":%d,"weight":0.98}]`, weekday),
			ActivePeriodsJSON:        `[{"period":"evening","weight":0.96}]`,
			CategoryWeightsJSON:      `{"运动":0.96}`,
			SubCategoryWeightsJSON:   `{"运动/羽毛球":0.98}`,
			PreferredLocationsJSON:   `[{"name":"北邮","address":"北京邮电大学沙河校区","lat":40.1574,"lng":116.2878,"weight":0.96}]`,
			InviteAcceptRate:         0.82,
			ReliabilityScore:         0.92,
			ExplorationScore:         0.24,
			WeeklyActiveScore:        0.86,
			WeeklyParticipationScore: 0.88,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		{
			UserID:                   "user_invite_calendar_b",
			ActiveWeekdaysJSON:       fmt.Sprintf(`[{"weekday":%d,"weight":0.82}]`, weekday),
			JoinWeekdaysJSON:         fmt.Sprintf(`[{"weekday":%d,"weight":0.84}]`, weekday),
			ActivePeriodsJSON:        `[{"period":"evening","weight":0.78}]`,
			CategoryWeightsJSON:      `{"运动":0.78}`,
			SubCategoryWeightsJSON:   `{"运动/羽毛球":0.74}`,
			PreferredLocationsJSON:   `[{"name":"北邮体育馆","address":"北京邮电大学沙河校区","lat":40.158,"lng":116.288,"weight":0.76}]`,
			InviteAcceptRate:         0.66,
			ReliabilityScore:         0.82,
			ExplorationScore:         0.2,
			WeeklyActiveScore:        0.74,
			WeeklyParticipationScore: 0.72,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		{
			UserID:                   "user_invite_calendar_weak",
			ActiveWeekdaysJSON:       fmt.Sprintf(`[{"weekday":%d,"weight":0.94}]`, weekday),
			JoinWeekdaysJSON:         fmt.Sprintf(`[{"weekday":%d,"weight":0.92}]`, weekday),
			ActivePeriodsJSON:        `[{"period":"evening","weight":0.78}]`,
			CategoryWeightsJSON:      `{"娱乐":0.86}`,
			SubCategoryWeightsJSON:   `{"娱乐/桌游":0.88}`,
			PreferredLocationsJSON:   `[{"name":"远处咖啡馆","address":"上海市静安区","lat":31.23,"lng":121.47,"weight":0.7}]`,
			InviteAcceptRate:         0.22,
			ReliabilityScore:         0.42,
			ExplorationScore:         0.1,
			WeeklyActiveScore:        0.22,
			WeeklyParticipationScore: 0.2,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("create behavior profiles failed: %v", err)
	}
	tags := []model.UserTag{
		{UserID: "user_invite_calendar_a", TagType: "sub_category", TagValue: "羽毛球", Weight: 6, EvidenceCount: 6, LastEventAt: now, CreatedAt: now, UpdatedAt: now},
		{UserID: "user_invite_calendar_a", TagType: "category", TagValue: "运动", Weight: 4, EvidenceCount: 4, LastEventAt: now, CreatedAt: now, UpdatedAt: now},
		{UserID: "user_invite_calendar_b", TagType: "sub_category", TagValue: "羽毛球", Weight: 4, EvidenceCount: 3, LastEventAt: now, CreatedAt: now, UpdatedAt: now},
		{UserID: "user_invite_calendar_weak", TagType: "sub_category", TagValue: "桌游", Weight: 5, EvidenceCount: 3, LastEventAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&tags).Error; err != nil {
		t.Fatalf("create user tags failed: %v", err)
	}

	router := NewRouter(db)
	startDate := dateOnly(time.Now()).Format("2006-01-02")
	heatReq := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/invite-heatmap?startDate="+startDate+"&days=30&category=运动&subCategory=羽毛球&period=evening&lat=40.1574&lng=116.2878&maxCount=6", nil)
	heatReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	heatResp := httptest.NewRecorder()
	router.ServeHTTP(heatResp, heatReq)
	if heatResp.Code != http.StatusOK {
		t.Fatalf("invite heatmap failed, code=%d body=%s", heatResp.Code, heatResp.Body.String())
	}
	var heatPayload struct {
		Days []inviteCalendarDayView `json:"days"`
	}
	if err := json.Unmarshal(heatResp.Body.Bytes(), &heatPayload); err != nil {
		t.Fatalf("decode invite heatmap failed: %v", err)
	}
	if len(heatPayload.Days) != 30 {
		t.Fatalf("expected 30 heatmap days, got=%d", len(heatPayload.Days))
	}
	if heatPayload.Days[1].CandidateCount < 2 || heatPayload.Days[1].FireLevel == 0 {
		t.Fatalf("expected target day candidates, got=%+v", heatPayload.Days[1])
	}
	counts := map[int]bool{}
	for i := 0; i < 7 && i < len(heatPayload.Days); i++ {
		counts[heatPayload.Days[i].CandidateCount] = true
	}
	if len(counts) < 2 {
		t.Fatalf("expected invite heatmap candidate counts to vary by date, got=%+v", heatPayload.Days[:7])
	}

	candidateURL := "/api/v1/calendar/invite-candidates?date=" + targetDate.Format("2006-01-02") + "&category=运动&subCategory=羽毛球&period=evening&lat=40.1574&lng=116.2878&selectedIds=user_invite_calendar_b"
	candidateReq := httptest.NewRequest(http.MethodGet, candidateURL, nil)
	candidateReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	candidateResp := httptest.NewRecorder()
	router.ServeHTTP(candidateResp, candidateReq)
	if candidateResp.Code != http.StatusOK {
		t.Fatalf("invite candidates failed, code=%d body=%s", candidateResp.Code, candidateResp.Body.String())
	}
	var candidatePayload struct {
		Candidates []inviteCalendarCandidateView `json:"candidates"`
	}
	if err := json.Unmarshal(candidateResp.Body.Bytes(), &candidatePayload); err != nil {
		t.Fatalf("decode invite candidates failed: %v", err)
	}
	if len(candidatePayload.Candidates) < 2 {
		t.Fatalf("expected at least two candidates, got=%+v", candidatePayload.Candidates)
	}
	if candidatePayload.Candidates[0].User.ID != "user_invite_calendar_a" {
		t.Fatalf("expected best candidate first, got=%+v", candidatePayload.Candidates)
	}
	foundSelected := false
	for _, candidate := range candidatePayload.Candidates {
		if candidate.User.ID == "user_invite_calendar_author" || candidate.User.ID == "user_invite_calendar_deleted" || candidate.User.ID == "user_invite_calendar_admin" {
			t.Fatalf("candidate list should exclude author/deleted/admin, got=%+v", candidate)
		}
		if candidate.User.ID == "user_invite_calendar_b" && candidate.Selected {
			foundSelected = true
		}
	}
	if !foundSelected {
		t.Fatalf("selectedIds should mark user_invite_calendar_b, got=%+v", candidatePayload.Candidates)
	}

	nonPreferredReq := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/invite-candidates?date="+startDate+"&category=运动&subCategory=羽毛球&period=evening&lat=40.1574&lng=116.2878", nil)
	nonPreferredReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	nonPreferredResp := httptest.NewRecorder()
	router.ServeHTTP(nonPreferredResp, nonPreferredReq)
	if nonPreferredResp.Code != http.StatusOK {
		t.Fatalf("non-preferred invite candidates failed, code=%d body=%s", nonPreferredResp.Code, nonPreferredResp.Body.String())
	}
	var nonPreferredPayload struct {
		Candidates []inviteCalendarCandidateView `json:"candidates"`
	}
	if err := json.Unmarshal(nonPreferredResp.Body.Bytes(), &nonPreferredPayload); err != nil {
		t.Fatalf("decode non-preferred invite candidates failed: %v", err)
	}
	if len(nonPreferredPayload.Candidates) >= len(candidatePayload.Candidates) {
		t.Fatalf("expected fewer candidates on non-preferred date, preferred=%d nonPreferred=%d", len(candidatePayload.Candidates), len(nonPreferredPayload.Candidates))
	}

	overrideURL := "/api/v1/calendar/invite-candidates?date=" + targetDate.Format("2006-01-02") +
		"&query=" + url.QueryEscape("桌游") +
		"&category=" + url.QueryEscape("运动") +
		"&subCategory=" + url.QueryEscape("羽毛球") +
		"&period=evening&lat=40.1574&lng=116.2878&limit=30"
	overrideReq := httptest.NewRequest(http.MethodGet, overrideURL, nil)
	overrideReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	overrideResp := httptest.NewRecorder()
	router.ServeHTTP(overrideResp, overrideReq)
	if overrideResp.Code != http.StatusOK {
		t.Fatalf("override invite candidates failed, code=%d body=%s", overrideResp.Code, overrideResp.Body.String())
	}
	var overridePayload struct {
		Candidates []inviteCalendarCandidateView `json:"candidates"`
	}
	if err := json.Unmarshal(overrideResp.Body.Bytes(), &overridePayload); err != nil {
		t.Fatalf("decode override invite candidates failed: %v", err)
	}
	foundTabletop := false
	for _, candidate := range overridePayload.Candidates {
		if candidate.User.ID == "user_invite_calendar_a" || candidate.User.ID == "user_invite_calendar_b" {
			t.Fatalf("query activity should override form activity, got sports candidate=%+v", candidate)
		}
		if candidate.User.ID == "user_invite_calendar_weak" {
			foundTabletop = true
		}
	}
	if !foundTabletop {
		t.Fatalf("expected tabletop candidate when query overrides form activity, got=%+v", overridePayload.Candidates)
	}

	preserveURL := "/api/v1/calendar/invite-candidates?date=" + targetDate.Format("2006-01-02") +
		"&query=" + url.QueryEscape("北邮 晚上") +
		"&category=" + url.QueryEscape("运动") +
		"&subCategory=" + url.QueryEscape("羽毛球") +
		"&lat=40.1574&lng=116.2878&limit=30"
	preserveReq := httptest.NewRequest(http.MethodGet, preserveURL, nil)
	preserveReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	preserveResp := httptest.NewRecorder()
	router.ServeHTTP(preserveResp, preserveReq)
	if preserveResp.Code != http.StatusOK {
		t.Fatalf("preserve invite candidates failed, code=%d body=%s", preserveResp.Code, preserveResp.Body.String())
	}
	var preservePayload struct {
		Candidates []inviteCalendarCandidateView `json:"candidates"`
	}
	if err := json.Unmarshal(preserveResp.Body.Bytes(), &preservePayload); err != nil {
		t.Fatalf("decode preserve invite candidates failed: %v", err)
	}
	if len(preservePayload.Candidates) < 2 {
		t.Fatalf("expected form activity to be preserved for location/time-only query, got=%+v", preservePayload.Candidates)
	}
	for _, candidate := range preservePayload.Candidates {
		if candidate.User.ID == "user_invite_calendar_weak" {
			t.Fatalf("location/time-only query should keep form activity, got tabletop candidate=%+v", candidate)
		}
	}

	emptyReq := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/invite-candidates?date="+targetDate.Format("2006-01-02")+"&category=学习&subCategory=编程&period=morning&address=火星基地", nil)
	emptyReq.Header.Set("Authorization", bearerFor(t, db, "user_invite_calendar_author"))
	emptyResp := httptest.NewRecorder()
	router.ServeHTTP(emptyResp, emptyReq)
	if emptyResp.Code != http.StatusOK {
		t.Fatalf("empty invite candidates failed, code=%d body=%s", emptyResp.Code, emptyResp.Body.String())
	}
	var emptyPayload struct {
		Candidates []inviteCalendarCandidateView `json:"candidates"`
	}
	if err := json.Unmarshal(emptyResp.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("decode empty invite candidates failed: %v", err)
	}
	if len(emptyPayload.Candidates) != 0 {
		t.Fatalf("expected no candidates for unrelated activity, got=%+v", emptyPayload.Candidates)
	}
}

func TestCreatePostRejectNowOrPastFixedTime(t *testing.T) {
	db := openRouterTestDB(t)
	ensureTestUser(t, db, "user_test_001")
	token, err := auth.SignToken("user_test_001", testJWTSecret, 1)
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	router := NewRouter(db)

	cases := []string{
		time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
	}
	for _, fixedTime := range cases {
		body := map[string]any{
			"title":    "invalid fixed time post",
			"category": "running",
			"address":  "test address",
			"maxCount": 4,
			"timeInfo": map[string]any{
				"mode":      "fixed",
				"days":      0,
				"fixedTime": fixedTime,
			},
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for fixedTime=%s, got=%d body=%s", fixedTime, resp.Code, resp.Body.String())
		}
	}
}

func TestSendMessageRequiresParticipant(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	if err := db.Create(&model.Post{
		ID:           "post_a1",
		AuthorID:     "user_author",
		Title:        "p",
		Description:  "d",
		Category:     "running",
		SubCategory:  "",
		TimeMode:     "range",
		TimeDays:     1,
		Address:      "addr",
		MaxCount:     5,
		CurrentCount: 1,
		Status:       "open",
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	router := NewRouter(db)
	body := []byte(`{"content":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/post_a1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_outsider"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestReviewOnlyAfterClosed(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	if err := db.Create(&model.Post{
		ID:           "post_b1",
		AuthorID:     "user_author",
		Title:        "p",
		Description:  "d",
		Category:     "running",
		SubCategory:  "",
		TimeMode:     "range",
		TimeDays:     1,
		Address:      "addr",
		MaxCount:     5,
		CurrentCount: 2,
		Status:       "open",
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   "post_b1",
		UserID:   "user_member",
		JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}

	router := NewRouter(db)
	body := []byte(`{"items":[{"toUserId":"user_author","rating":5,"comment":"ok"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/post_b1/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_member"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for open post, got %d, body=%s", resp.Code, resp.Body.String())
	}

	if err := db.Model(&model.Post{}).Where("id = ?", "post_b1").Update("status", "closed").Error; err != nil {
		t.Fatalf("close post failed: %v", err)
	}
	resp2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/posts/post_b1/reviews", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", bearerFor(t, db, "user_member"))
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("expected 200 for closed post, got %d, body=%s", resp2.Code, resp2.Body.String())
	}
}

func TestReviewRecalcRatingScore(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	targetUser := model.User{
		ID:          "user_target",
		Platform:    "password",
		OpenID:      "oid_target",
		Nickname:    "target",
		AvatarURL:   "",
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	reviewer := model.User{
		ID:          "user_reviewer",
		Platform:    "password",
		OpenID:      "oid_reviewer",
		Nickname:    "reviewer",
		AvatarURL:   "",
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(&[]model.User{targetUser, reviewer}).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	if err := db.Create(&model.Post{
		ID:           "post_rating_1",
		AuthorID:     targetUser.ID,
		Title:        "p",
		Description:  "d",
		Category:     "running",
		TimeMode:     "range",
		TimeDays:     1,
		Address:      "addr",
		MaxCount:     5,
		CurrentCount: 1,
		Status:       "closed",
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   "post_rating_1",
		UserID:   reviewer.ID,
		JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}

	router := NewRouter(db)
	body := []byte(`{"items":[{"toUserId":"user_target","rating":4,"comment":"ok"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/post_rating_1/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, reviewer.ID))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("review request failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var updated model.User
	if err := db.First(&updated, "id = ?", targetUser.ID).Error; err != nil {
		t.Fatalf("query target user failed: %v", err)
	}
	if updated.RatingScore < 3.9 || updated.RatingScore > 4.1 {
		t.Fatalf("expect rating score around 4.0, got=%f", updated.RatingScore)
	}
}

func TestAuthMe(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	if err := db.Create(&model.User{
		ID:          "user_me_1",
		Platform:    "mock",
		OpenID:      "openid_me_1",
		Nickname:    "me",
		AvatarURL:   "",
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", bearerFor(t, db, "user_me_1"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestRefreshAndLogout(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("ENABLE_MOCK_LOGIN", "true")
	router := NewRouter(db)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-login", bytes.NewReader([]byte(`{"nickname":"u1"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRespRec := httptest.NewRecorder()
	router.ServeHTTP(loginRespRec, loginReq)
	if loginRespRec.Code != http.StatusOK {
		t.Fatalf("login failed, code=%d body=%s", loginRespRec.Code, loginRespRec.Body.String())
	}

	var loginData loginResp
	if err := json.Unmarshal(loginRespRec.Body.Bytes(), &loginData); err != nil {
		t.Fatalf("decode login resp failed: %v", err)
	}
	if loginData.RefreshToken == "" || loginData.AccessToken == "" {
		t.Fatalf("tokens should not be empty")
	}

	refreshBody := fmt.Sprintf(`{"refreshToken":"%s"}`, loginData.RefreshToken)
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte(refreshBody)))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshResp := httptest.NewRecorder()
	router.ServeHTTP(refreshResp, refreshReq)
	if refreshResp.Code != http.StatusOK {
		t.Fatalf("refresh failed, code=%d body=%s", refreshResp.Code, refreshResp.Body.String())
	}

	var refreshData loginResp
	if err := json.Unmarshal(refreshResp.Body.Bytes(), &refreshData); err != nil {
		t.Fatalf("decode refresh resp failed: %v", err)
	}
	if refreshData.RefreshToken == "" || refreshData.RefreshToken == loginData.RefreshToken {
		t.Fatalf("refresh token should rotate")
	}

	logoutBody := fmt.Sprintf(`{"refreshToken":"%s"}`, refreshData.RefreshToken)
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader([]byte(logoutBody)))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutResp := httptest.NewRecorder()
	router.ServeHTTP(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("logout failed, code=%d body=%s", logoutResp.Code, logoutResp.Body.String())
	}

	refreshAgainReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte(logoutBody)))
	refreshAgainReq.Header.Set("Content-Type", "application/json")
	refreshAgainResp := httptest.NewRecorder()
	router.ServeHTTP(refreshAgainResp, refreshAgainReq)
	if refreshAgainResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got=%d body=%s", refreshAgainResp.Code, refreshAgainResp.Body.String())
	}
}

func TestLogoutRevokesAccessToken(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("ENABLE_MOCK_LOGIN", "true")
	router := NewRouter(db)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-login", bytes.NewReader([]byte(`{"nickname":"u2"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRespRec := httptest.NewRecorder()
	router.ServeHTTP(loginRespRec, loginReq)
	if loginRespRec.Code != http.StatusOK {
		t.Fatalf("login failed, code=%d body=%s", loginRespRec.Code, loginRespRec.Body.String())
	}

	var loginData loginResp
	if err := json.Unmarshal(loginRespRec.Body.Bytes(), &loginData); err != nil {
		t.Fatalf("decode login resp failed: %v", err)
	}
	if loginData.AccessToken == "" {
		t.Fatalf("access token should not be empty")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginData.AccessToken)
	meResp := httptest.NewRecorder()
	router.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusOK {
		t.Fatalf("expect /auth/me 200 before logout, got=%d body=%s", meResp.Code, meResp.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader([]byte(`{}`)))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.Header.Set("Authorization", "Bearer "+loginData.AccessToken)
	logoutResp := httptest.NewRecorder()
	router.ServeHTTP(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("logout failed, code=%d body=%s", logoutResp.Code, logoutResp.Body.String())
	}

	meReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq2.Header.Set("Authorization", "Bearer "+loginData.AccessToken)
	meResp2 := httptest.NewRecorder()
	router.ServeHTTP(meResp2, meReq2)
	if meResp2.Code != http.StatusUnauthorized {
		t.Fatalf("expect /auth/me 401 after logout, got=%d body=%s", meResp2.Code, meResp2.Body.String())
	}
}

func TestRegisterAndPasswordLogin(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)

	registerReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/register",
		bytes.NewReader([]byte(`{"nickname":"alice","password":"123456"}`)),
	)
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	router.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register failed, code=%d body=%s", registerResp.Code, registerResp.Body.String())
	}

	var user model.User
	if err := db.First(&user, "nickname = ?", "alice").Error; err != nil {
		t.Fatalf("query user failed: %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatalf("password hash should not be empty")
	}
	if !strings.Contains(user.AvatarURL, "dicebear.com") {
		t.Fatalf("register user avatar should come from dicebear, got=%s", user.AvatarURL)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("123456")); err != nil {
		t.Fatalf("password hash verify failed: %v", err)
	}

	registerDupReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/register",
		bytes.NewReader([]byte(`{"nickname":"alice","password":"123456"}`)),
	)
	registerDupReq.Header.Set("Content-Type", "application/json")
	registerDupResp := httptest.NewRecorder()
	router.ServeHTTP(registerDupResp, registerDupReq)
	if registerDupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate register should be 409, got=%d body=%s", registerDupResp.Code, registerDupResp.Body.String())
	}
	var dupErr errResp
	_ = json.Unmarshal(registerDupResp.Body.Bytes(), &dupErr)
	if dupErr.Code != "NICKNAME_ALREADY_EXISTS" {
		t.Fatalf("expect NICKNAME_ALREADY_EXISTS, got=%s", dupErr.Code)
	}

	loginReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password-login",
		bytes.NewReader([]byte(`{"nickname":"alice","password":"123456"}`)),
	)
	loginReq.Header.Set("Content-Type", "application/json")
	loginRespRec := httptest.NewRecorder()
	router.ServeHTTP(loginRespRec, loginReq)
	if loginRespRec.Code != http.StatusOK {
		t.Fatalf("password login failed, code=%d body=%s", loginRespRec.Code, loginRespRec.Body.String())
	}
	var okLogin loginResp
	if err := json.Unmarshal(loginRespRec.Body.Bytes(), &okLogin); err != nil {
		t.Fatalf("decode login resp failed: %v", err)
	}
	if okLogin.AccessToken == "" || okLogin.RefreshToken == "" {
		t.Fatalf("tokens should not be empty")
	}

	wrongPassReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password-login",
		bytes.NewReader([]byte(`{"nickname":"alice","password":"654321"}`)),
	)
	wrongPassReq.Header.Set("Content-Type", "application/json")
	wrongPassResp := httptest.NewRecorder()
	router.ServeHTTP(wrongPassResp, wrongPassReq)
	if wrongPassResp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password should be 401, got=%d body=%s", wrongPassResp.Code, wrongPassResp.Body.String())
	}
	var wrongErr errResp
	_ = json.Unmarshal(wrongPassResp.Body.Bytes(), &wrongErr)
	if wrongErr.Code != "LOGIN_FAILED" {
		t.Fatalf("expect LOGIN_FAILED, got=%s", wrongErr.Code)
	}
}

func TestRandomAvatarEndpoint(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("ENABLE_MOCK_LOGIN", "true")
	router := NewRouter(db)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-login", bytes.NewReader([]byte(`{"nickname":"avatar_u"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRespRec := httptest.NewRecorder()
	router.ServeHTTP(loginRespRec, loginReq)
	if loginRespRec.Code != http.StatusOK {
		t.Fatalf("login failed, code=%d body=%s", loginRespRec.Code, loginRespRec.Body.String())
	}
	var loginData struct {
		AccessToken string     `json:"accessToken"`
		User        model.User `json:"user"`
	}
	if err := json.Unmarshal(loginRespRec.Body.Bytes(), &loginData); err != nil {
		t.Fatalf("decode login resp failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/avatar/random", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+loginData.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("random avatar failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		User model.User `json:"user"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode random avatar resp failed: %v", err)
	}
	if payload.User.AvatarURL == "" {
		t.Fatalf("avatar url should not be empty")
	}
	if payload.User.AvatarURL == loginData.User.AvatarURL {
		t.Fatalf("avatar url should be changed")
	}
}

func TestGetUserHome(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()
	user := model.User{
		ID:          "user_home_1",
		Platform:    "password",
		OpenID:      "oid_home_1",
		Nickname:    "home_user",
		AvatarURL:   "",
		CreditScore: 99,
		RatingScore: 4.7,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	other := model.User{
		ID:          "user_home_2",
		Platform:    "password",
		OpenID:      "oid_home_2",
		Nickname:    "other_user",
		AvatarURL:   "",
		CreditScore: 100,
		RatingScore: 4.8,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(&[]model.User{user, other}).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	if err := db.Create(&[]model.Post{
		{
			ID:           "post_home_initiated",
			AuthorID:     user.ID,
			Title:        "initiated",
			Category:     "running",
			TimeMode:     "range",
			TimeDays:     1,
			Address:      "a",
			MaxCount:     5,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "post_home_joined",
			AuthorID:     other.ID,
			Title:        "joined",
			Category:     "running",
			TimeMode:     "range",
			TimeDays:     1,
			Address:      "b",
			MaxCount:     5,
			CurrentCount: 2,
			Status:       "open",
			CreatedAt:    now + 1,
			UpdatedAt:    now + 1,
		},
	}).Error; err != nil {
		t.Fatalf("create posts failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   "post_home_joined",
		UserID:   user.ID,
		JoinedAt: now + 2,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	if err := db.Create(&[]model.UserTag{
		{UserID: user.ID, TagType: "sub_category", TagValue: "badminton", Weight: 8.6, EvidenceCount: 3, LastEventAt: now + 3, CreatedAt: now + 3, UpdatedAt: now + 3},
		{UserID: user.ID, TagType: "city", TagValue: "shanghai", Weight: 6.2, EvidenceCount: 2, LastEventAt: now + 4, CreatedAt: now + 4, UpdatedAt: now + 4},
	}).Error; err != nil {
		t.Fatalf("create user tags failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+user.ID+"/home", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("get user home failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		User           model.User        `json:"user"`
		InitiatedPosts []postView        `json:"initiatedPosts"`
		JoinedPosts    []postView        `json:"joinedPosts"`
		InterestTags   []interestTagView `json:"interestTags"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if payload.User.ID != user.ID {
		t.Fatalf("unexpected user id: %s", payload.User.ID)
	}
	if len(payload.InitiatedPosts) != 1 || payload.InitiatedPosts[0].ID != "post_home_initiated" {
		t.Fatalf("unexpected initiated posts: %+v", payload.InitiatedPosts)
	}
	if len(payload.JoinedPosts) != 1 || payload.JoinedPosts[0].ID != "post_home_joined" {
		t.Fatalf("unexpected joined posts: %+v", payload.JoinedPosts)
	}
	if len(payload.InterestTags) != 2 || payload.InterestTags[0].Value != "badminton" {
		t.Fatalf("unexpected interest tags: %+v", payload.InterestTags)
	}
}

func TestRecommendationFeedAndLogEndpoints(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_rec_author", Platform: "password", OpenID: "oid_rec_author", Nickname: "rec_author", AvatarURL: avatarURLFromSeed("rec_author"), CreditScore: 98, RatingScore: 4.8, CreatedAt: now, UpdatedAt: now},
		{ID: "user_rec_viewer", Platform: "password", OpenID: "oid_rec_viewer", Nickname: "rec_viewer", AvatarURL: avatarURLFromSeed("rec_viewer"), CreditScore: 101, RatingScore: 4.9, CreatedAt: now + 1, UpdatedAt: now + 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	if err := db.Create(&model.Post{
		ID:           "post_rec_001",
		AuthorID:     "user_rec_author",
		Title:        "viewer aligned event",
		Description:  "viewer matches the event interests and city",
		Category:     "sports",
		SubCategory:  "badminton",
		TimeMode:     "range",
		TimeDays:     2,
		Address:      "Shanghai sports center",
		MaxCount:     6,
		CurrentCount: 1,
		Status:       "open",
		CreatedAt:    now + 20,
		UpdatedAt:    now + 20,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&[]model.UserTag{
		{UserID: "user_rec_viewer", TagType: "sub_category", TagValue: "badminton", Weight: 9.0, EvidenceCount: 4, LastEventAt: now + 30, CreatedAt: now + 30, UpdatedAt: now + 30},
		{UserID: "user_rec_viewer", TagType: "city", TagValue: "shanghai", Weight: 7.0, EvidenceCount: 2, LastEventAt: now + 30, CreatedAt: now + 30, UpdatedAt: now + 30},
	}).Error; err != nil {
		t.Fatalf("create user tags failed: %v", err)
	}

	router := NewRouter(db)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts?sortBy=hot&page=1", nil)
	listReq.Header.Set("Authorization", bearerFor(t, db, "user_rec_viewer"))
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list posts failed, code=%d body=%s", listResp.Code, listResp.Body.String())
	}

	var listPayload struct {
		FeedRequestID string     `json:"feedRequestId"`
		Posts         []postView `json:"posts"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if listPayload.FeedRequestID == "" {
		t.Fatalf("feedRequestId should not be empty: %s", listResp.Body.String())
	}
	if len(listPayload.Posts) != 1 || listPayload.Posts[0].Recommendation.Strategy == "" {
		t.Fatalf("recommendation payload missing: %+v", listPayload.Posts)
	}

	exposureBody := fmt.Sprintf(`{"feedRequestId":"%s","sessionId":"session_test","items":[{"postId":"post_rec_001","position":1,"strategy":"personalized","score":0.88}]}`, listPayload.FeedRequestID)
	exposureReq := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/exposures", bytes.NewReader([]byte(exposureBody)))
	exposureReq.Header.Set("Content-Type", "application/json")
	exposureReq.Header.Set("Authorization", bearerFor(t, db, "user_rec_viewer"))
	exposureResp := httptest.NewRecorder()
	router.ServeHTTP(exposureResp, exposureReq)
	if exposureResp.Code != http.StatusOK {
		t.Fatalf("report exposures failed, code=%d body=%s", exposureResp.Code, exposureResp.Body.String())
	}

	clickBody := fmt.Sprintf(`{"feedRequestId":"%s","sessionId":"session_test","postId":"post_rec_001","position":1,"strategy":"personalized","score":0.88}`, listPayload.FeedRequestID)
	clickReq := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/click", bytes.NewReader([]byte(clickBody)))
	clickReq.Header.Set("Content-Type", "application/json")
	clickReq.Header.Set("Authorization", bearerFor(t, db, "user_rec_viewer"))
	clickResp := httptest.NewRecorder()
	router.ServeHTTP(clickResp, clickReq)
	if clickResp.Code != http.StatusOK {
		t.Fatalf("report click failed, code=%d body=%s", clickResp.Code, clickResp.Body.String())
	}

	var exposureCount int64
	if err := db.Model(&model.FeedExposure{}).Count(&exposureCount).Error; err != nil {
		t.Fatalf("count exposures failed: %v", err)
	}
	if exposureCount != 1 {
		t.Fatalf("expected 1 exposure row, got=%d", exposureCount)
	}
	var clickCount int64
	if err := db.Model(&model.FeedClick{}).Count(&clickCount).Error; err != nil {
		t.Fatalf("count clicks failed: %v", err)
	}
	if clickCount != 1 {
		t.Fatalf("expected 1 click row, got=%d", clickCount)
	}
}

func TestListPostsIncludesViewerFlags(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_author_flag", Platform: "password", OpenID: "oid_author_flag", Nickname: "author_flag", AvatarURL: avatarURLFromSeed("author_flag"), CreditScore: 100, RatingScore: 4.8, CreatedAt: now, UpdatedAt: now},
		{ID: "user_joiner_flag", Platform: "password", OpenID: "oid_joiner_flag", Nickname: "joiner_flag", AvatarURL: avatarURLFromSeed("joiner_flag"), CreditScore: 101, RatingScore: 4.7, CreatedAt: now + 1, UpdatedAt: now + 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	post := model.Post{
		ID:           "post_flag_001",
		AuthorID:     "user_author_flag",
		Title:        "viewer flag post",
		Category:     "running",
		TimeMode:     "range",
		TimeDays:     2,
		Address:      "test address",
		MaxCount:     6,
		CurrentCount: 1,
		Status:       "open",
		CreatedAt:    now + 10,
		UpdatedAt:    now + 10,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   post.ID,
		UserID:   "user_joiner_flag",
		JoinedAt: now + 20,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}

	router := NewRouter(db)

	authorReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts?sortBy=latest&page=1", nil)
	authorReq.Header.Set("Authorization", bearerFor(t, db, "user_author_flag"))
	authorResp := httptest.NewRecorder()
	router.ServeHTTP(authorResp, authorReq)
	if authorResp.Code != http.StatusOK {
		t.Fatalf("list posts failed, code=%d body=%s", authorResp.Code, authorResp.Body.String())
	}

	var authorPayload struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(authorResp.Body.Bytes(), &authorPayload); err != nil {
		t.Fatalf("decode author list failed: %v", err)
	}
	if len(authorPayload.Posts) != 1 || !authorPayload.Posts[0].ViewerIsAuthor {
		t.Fatalf("author viewer flag missing: %+v", authorPayload.Posts)
	}

	joinerReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+post.ID, nil)
	joinerReq.Header.Set("Authorization", bearerFor(t, db, "user_joiner_flag"))
	joinerResp := httptest.NewRecorder()
	router.ServeHTTP(joinerResp, joinerReq)
	if joinerResp.Code != http.StatusOK {
		t.Fatalf("get post failed, code=%d body=%s", joinerResp.Code, joinerResp.Body.String())
	}

	var joinerPayload struct {
		Post postView `json:"post"`
	}
	if err := json.Unmarshal(joinerResp.Body.Bytes(), &joinerPayload); err != nil {
		t.Fatalf("decode joiner detail failed: %v", err)
	}
	if !joinerPayload.Post.ViewerJoined {
		t.Fatalf("joined viewer flag missing: %+v", joinerPayload.Post)
	}
}

func TestClosePostCreatesActivityScores(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_close_author", Platform: "password", OpenID: "oid_close_author", Nickname: "close_author", AvatarURL: avatarURLFromSeed("close_author"), CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_close_member", Platform: "password", OpenID: "oid_close_member", Nickname: "close_member", AvatarURL: avatarURLFromSeed("close_member"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 1, UpdatedAt: now + 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	if err := db.Create(&model.Post{
		ID:           "post_close_001",
		AuthorID:     "user_close_author",
		Title:        "close test",
		Category:     "running",
		TimeMode:     "range",
		TimeDays:     2,
		Address:      "test",
		MaxCount:     4,
		CurrentCount: 2,
		Status:       "open",
		CreatedAt:    now + 10,
		UpdatedAt:    now + 10,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   "post_close_001",
		UserID:   "user_close_member",
		JoinedAt: now + 20,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/post_close_001/close", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_close_author"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("close post failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var rows []model.ActivityScore
	if err := db.Order("user_id ASC").Find(&rows, "post_id = ?", "post_close_001").Error; err != nil {
		t.Fatalf("query activity scores failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 activity score rows, got=%d", len(rows))
	}
	if rows[0].UserID != "user_close_author" || rows[0].CreditScore != 100 || rows[0].FulfillmentStatus != score.SettlementPending {
		t.Fatalf("unexpected author activity score: %+v", rows[0])
	}
	if rows[1].UserID != "user_close_member" || rows[1].CreditScore != 100 || rows[1].FulfillmentStatus != score.SettlementPending {
		t.Fatalf("unexpected participant activity score: %+v", rows[1])
	}
}

func TestGetUserHomeIncludesReviewStateSortingAndChatPreview(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_home_subject", Platform: "password", OpenID: "oid_home_subject", Nickname: "subject", AvatarURL: avatarURLFromSeed("subject"), CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_home_author_a", Platform: "password", OpenID: "oid_home_author_a", Nickname: "author_a", AvatarURL: avatarURLFromSeed("author_a"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 1, UpdatedAt: now + 1},
		{ID: "user_home_author_b", Platform: "password", OpenID: "oid_home_author_b", Nickname: "author_b", AvatarURL: avatarURLFromSeed("author_b"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 2, UpdatedAt: now + 2},
		{ID: "user_home_author_c", Platform: "password", OpenID: "oid_home_author_c", Nickname: "author_c", AvatarURL: avatarURLFromSeed("author_c"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 3, UpdatedAt: now + 3},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}

	posts := []model.Post{
		{ID: "post_home_pending", AuthorID: "user_home_author_a", Title: "pending", Category: "running", TimeMode: "range", TimeDays: 2, Address: "a", MaxCount: 4, CurrentCount: 2, Status: "closed", CreatedAt: now + 10, UpdatedAt: now + 300},
		{ID: "post_home_open", AuthorID: "user_home_author_b", Title: "open", Category: "running", TimeMode: "fixed", FixedTime: time.Now().Add(3 * time.Hour).Format(time.RFC3339), Address: "b", MaxCount: 4, CurrentCount: 2, Status: "open", CreatedAt: now + 20, UpdatedAt: now + 200},
		{ID: "post_home_done", AuthorID: "user_home_author_c", Title: "done", Category: "running", TimeMode: "range", TimeDays: 4, Address: "c", MaxCount: 4, CurrentCount: 2, Status: "closed", CreatedAt: now + 30, UpdatedAt: now + 100},
	}
	if err := db.Create(&posts).Error; err != nil {
		t.Fatalf("create posts failed: %v", err)
	}

	relations := []model.PostParticipant{
		{PostID: "post_home_pending", UserID: "user_home_subject", JoinedAt: now + 11},
		{PostID: "post_home_open", UserID: "user_home_subject", JoinedAt: now + 21},
		{PostID: "post_home_done", UserID: "user_home_subject", JoinedAt: now + 31},
	}
	if err := db.Create(&relations).Error; err != nil {
		t.Fatalf("create participants failed: %v", err)
	}

	reviews := []model.Review{
		{PostID: "post_home_done", FromUserID: "user_home_subject", ToUserID: "user_home_author_c", Rating: 5, Comment: "great", CreatedAt: now + 40, UpdatedAt: now + 40},
	}
	if err := db.Create(&reviews).Error; err != nil {
		t.Fatalf("create reviews failed: %v", err)
	}

	messages := []model.ChatMessage{
		{ID: "msg_home_pending", PostID: "post_home_pending", SenderID: "user_home_author_a", Content: "pending latest", ClientMsgID: "client_pending", CreatedAt: now + 50},
		{ID: "msg_home_done", PostID: "post_home_done", SenderID: "user_home_author_c", Content: "done latest", ClientMsgID: "client_done", CreatedAt: now + 60},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages failed: %v", err)
	}

	for _, postID := range []string{"post_home_pending", "post_home_done"} {
		if err := score.RecalculatePostActivityScores(db, postID, now+1000); err != nil {
			t.Fatalf("recalc activity scores failed for %s: %v", postID, err)
		}
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user_home_subject/home", nil)
	req.Header.Set("Authorization", bearerFor(t, db, "user_home_subject"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("get user home failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		JoinedPosts []homePostView `json:"joinedPosts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode user home payload failed: %v", err)
	}
	if len(payload.JoinedPosts) != 3 {
		t.Fatalf("expected 3 joined posts, got=%d", len(payload.JoinedPosts))
	}
	if payload.JoinedPosts[0].ID != "post_home_done" || payload.JoinedPosts[1].ID != "post_home_pending" || payload.JoinedPosts[2].ID != "post_home_open" {
		t.Fatalf("unexpected joined post order: %+v", payload.JoinedPosts)
	}
	if !payload.JoinedPosts[0].SettlementState.CanParticipantConfirm {
		t.Fatalf("closed reviewed post should still require settlement confirmation: %+v", payload.JoinedPosts[0].SettlementState)
	}
	if payload.JoinedPosts[1].ReviewState.StatusText == "" {
		t.Fatalf("unexpected empty pending review state: %+v", payload.JoinedPosts[1].ReviewState)
	}
	if payload.JoinedPosts[1].ChatPreview.LatestMessage != "pending latest" {
		t.Fatalf("unexpected chat preview for pending post: %+v", payload.JoinedPosts[1].ChatPreview)
	}
	if payload.JoinedPosts[0].ReviewState.MyStars != 0 {
		t.Fatalf("review should wait until settlement finishes: %+v", payload.JoinedPosts[0].ReviewState)
	}
	if payload.JoinedPosts[0].ActivityScore.CreditScore != 100 {
		t.Fatalf("unexpected activity score for reviewed post: %+v", payload.JoinedPosts[0].ActivityScore)
	}
}

func TestListPostsSupportsAddressKeyword(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	posts := []model.Post{
		{
			ID:           "post_addr_001",
			AuthorID:     "user_addr_author",
			Title:        "city walk",
			Description:  "desc",
			Category:     "walking",
			TimeMode:     "range",
			TimeDays:     2,
			Address:      "上海图书馆",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "post_addr_002",
			AuthorID:     "user_addr_author",
			Title:        "night run",
			Description:  "desc",
			Category:     "running",
			TimeMode:     "range",
			TimeDays:     2,
			Address:      "北京奥森",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 1,
			UpdatedAt:    now + 1,
		},
	}
	if err := db.Create(&model.User{
		ID:          "user_addr_author",
		Platform:    "password",
		OpenID:      "oid_addr_author",
		Nickname:    "addr_author",
		AvatarURL:   avatarURLFromSeed("addr_author"),
		CreditScore: 100,
		RatingScore: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&posts).Error; err != nil {
		t.Fatalf("create posts failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?addressKeyword=图书馆&page=1&pageSize=10&sortBy=latest", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list posts failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(payload.Posts) != 1 || payload.Posts[0].ID != "post_addr_001" {
		t.Fatalf("unexpected filtered posts: %+v", payload.Posts)
	}
}

func TestListPostsKeywordSearchIncludesDescriptionAddressAndAuthorNickname(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{
			ID:          "user_search_author_1",
			Platform:    "password",
			OpenID:      "oid_search_author_1",
			Nickname:    "mike",
			AvatarURL:   avatarURLFromSeed("mike"),
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "user_search_author_2",
			Platform:    "password",
			OpenID:      "oid_search_author_2",
			Nickname:    "luna",
			AvatarURL:   avatarURLFromSeed("luna"),
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now + 1,
			UpdatedAt:   now + 1,
		},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}

	posts := []model.Post{
		{
			ID:           "post_search_title",
			AuthorID:     "user_search_author_1",
			Title:        "movie night",
			Description:  "watch together",
			Category:     "movie",
			TimeMode:     "range",
			TimeDays:     1,
			Address:      "xuhui cinema",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 10,
			UpdatedAt:    now + 10,
		},
		{
			ID:           "post_search_description",
			AuthorID:     "user_search_author_2",
			Title:        "weekend meetup",
			Description:  "board game evening",
			Category:     "game",
			TimeMode:     "range",
			TimeDays:     2,
			Address:      "jingan loft",
			MaxCount:     5,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 11,
			UpdatedAt:    now + 11,
		},
	}
	if err := db.Create(&posts).Error; err != nil {
		t.Fatalf("create posts failed: %v", err)
	}

	router := NewRouter(db)
	cases := []struct {
		name       string
		keyword    string
		expectedID string
	}{
		{name: "title", keyword: "movie", expectedID: "post_search_title"},
		{name: "description", keyword: "board", expectedID: "post_search_description"},
		{name: "address", keyword: "xuhui", expectedID: "post_search_title"},
		{name: "author nickname", keyword: "mike", expectedID: "post_search_title"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?keyword="+tc.keyword+"&page=1&pageSize=10&sortBy=latest", nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("list posts failed, code=%d body=%s", resp.Code, resp.Body.String())
			}

			var payload struct {
				Posts []postView `json:"posts"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}
			if len(payload.Posts) != 1 || payload.Posts[0].ID != tc.expectedID {
				t.Fatalf("unexpected posts for %s: %+v", tc.keyword, payload.Posts)
			}
		})
	}
}

func TestListPostsKeywordSearchSupportsChineseNicknameSubstringAndAddressFilter(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{
			ID:          "user_search_cn_1",
			Platform:    "password",
			OpenID:      "oid_search_cn_1",
			Nickname:    "云桃",
			AvatarURL:   avatarURLFromSeed("云桃"),
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "user_search_cn_2",
			Platform:    "password",
			OpenID:      "oid_search_cn_2",
			Nickname:    "桃子汽水",
			AvatarURL:   avatarURLFromSeed("桃子汽水"),
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now + 1,
			UpdatedAt:   now + 1,
		},
		{
			ID:          "user_search_cn_3",
			Platform:    "password",
			OpenID:      "oid_search_cn_3",
			Nickname:    "小夏",
			AvatarURL:   avatarURLFromSeed("小夏"),
			CreditScore: 100,
			RatingScore: 5,
			CreatedAt:   now + 2,
			UpdatedAt:   now + 2,
		},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}

	posts := []model.Post{
		{
			ID:           "post_search_cn_1",
			AuthorID:     "user_search_cn_1",
			Title:        "羽毛球夜场",
			Description:  "大学城集合",
			Category:     "运动",
			TimeMode:     "range",
			TimeDays:     2,
			Address:      "松江大学城",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 10,
			UpdatedAt:    now + 10,
		},
		{
			ID:           "post_search_cn_2",
			AuthorID:     "user_search_cn_2",
			Title:        "电影搭子",
			Description:  "周末一起看电影",
			Category:     "娱乐",
			TimeMode:     "range",
			TimeDays:     3,
			Address:      "徐汇滨江",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 11,
			UpdatedAt:    now + 11,
		},
		{
			ID:           "post_search_cn_3",
			AuthorID:     "user_search_cn_3",
			Title:        "自习室拼桌",
			Description:  "安静复习",
			Category:     "学习",
			TimeMode:     "range",
			TimeDays:     1,
			Address:      "松江大学城",
			MaxCount:     4,
			CurrentCount: 1,
			Status:       "open",
			CreatedAt:    now + 12,
			UpdatedAt:    now + 12,
		},
	}
	if err := db.Create(&posts).Error; err != nil {
		t.Fatalf("create posts failed: %v", err)
	}

	router := NewRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?keyword=桃&page=1&pageSize=10&sortBy=latest", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list posts by nickname substring failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode nickname search response failed: %v", err)
	}
	if len(payload.Posts) != 2 {
		t.Fatalf("expected 2 posts for nickname substring 桃, got=%d payload=%+v", len(payload.Posts), payload.Posts)
	}
	if payload.Posts[0].ID != "post_search_cn_2" || payload.Posts[1].ID != "post_search_cn_1" {
		t.Fatalf("unexpected nickname substring search order: %+v", payload.Posts)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/posts?keyword=桃&addressKeyword=大学城&page=1&pageSize=10&sortBy=latest", nil)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("list posts by nickname substring + address failed, code=%d body=%s", resp2.Code, resp2.Body.String())
	}

	var payload2 struct {
		Posts []postView `json:"posts"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &payload2); err != nil {
		t.Fatalf("decode combined search response failed: %v", err)
	}
	if len(payload2.Posts) != 1 || payload2.Posts[0].ID != "post_search_cn_1" {
		t.Fatalf("unexpected combined keyword/address search result: %+v", payload2.Posts)
	}
}

func TestGetSettlementRepairsStaleSettlementState(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_settle_author", Platform: "password", OpenID: "oid_settle_author", Nickname: "author", AvatarURL: avatarURLFromSeed("settle_author"), CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_settle_member", Platform: "password", OpenID: "oid_settle_member", Nickname: "member", AvatarURL: avatarURLFromSeed("settle_member"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 1, UpdatedAt: now + 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	post := model.Post{
		ID:           "post_settle_repair",
		AuthorID:     "user_settle_author",
		Title:        "repair settlement",
		Category:     "running",
		TimeMode:     "range",
		TimeDays:     2,
		Address:      "test address",
		MaxCount:     4,
		CurrentCount: 2,
		Status:       "closed",
		ClosedAt:     now + 10,
		CreatedAt:    now + 10,
		UpdatedAt:    now + 10,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   post.ID,
		UserID:   "user_settle_member",
		JoinedAt: now + 20,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	if err := db.Create(&model.PostParticipantSettlement{
		PostID:                 post.ID,
		UserID:                 "user_settle_member",
		ParticipantDecision:    score.DecisionCompleted,
		ParticipantConfirmedAt: now + 30,
		FinalStatus:            score.SettlementPending,
		CreatedAt:              now + 30,
		UpdatedAt:              now + 30,
	}).Error; err != nil {
		t.Fatalf("create stale settlement failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+post.ID+"/settlement", nil)
	req.Header.Set("Authorization", bearerFor(t, db, "user_settle_author"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("get settlement failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Stage              string                       `json:"stage"`
		PendingMemberCount int                          `json:"pendingMemberCount"`
		ReviewTargets      []settlementReviewTargetView `json:"reviewTargets"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode settlement payload failed: %v", err)
	}
	if payload.Stage != "review" {
		t.Fatalf("expected repaired settlement stage review, got=%s body=%s", payload.Stage, resp.Body.String())
	}
	if payload.PendingMemberCount != 0 {
		t.Fatalf("expected no pending members after repair, got=%d", payload.PendingMemberCount)
	}
	if len(payload.ReviewTargets) != 1 || payload.ReviewTargets[0].User.ID != "user_settle_member" {
		t.Fatalf("unexpected review targets after repair: %+v", payload.ReviewTargets)
	}

	var row model.PostParticipantSettlement
	if err := db.First(&row, "post_id = ? AND user_id = ?", post.ID, "user_settle_member").Error; err != nil {
		t.Fatalf("query repaired settlement failed: %v", err)
	}
	if row.FinalStatus != score.SettlementCompleted {
		t.Fatalf("expected final_status completed after repair, got=%s", row.FinalStatus)
	}
}

func TestGetUserHomeUsesRepairedSettlementWorkflow(t *testing.T) {
	db := openRouterTestDB(t)
	now := time.Now().UnixMilli()

	users := []model.User{
		{ID: "user_home_repair_author", Platform: "password", OpenID: "oid_home_repair_author", Nickname: "author", AvatarURL: avatarURLFromSeed("home_repair_author"), CreditScore: 100, RatingScore: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "user_home_repair_member", Platform: "password", OpenID: "oid_home_repair_member", Nickname: "member", AvatarURL: avatarURLFromSeed("home_repair_member"), CreditScore: 100, RatingScore: 5, CreatedAt: now + 1, UpdatedAt: now + 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users failed: %v", err)
	}
	post := model.Post{
		ID:           "post_home_repair",
		AuthorID:     "user_home_repair_author",
		Title:        "repair home workflow",
		Category:     "running",
		TimeMode:     "range",
		TimeDays:     3,
		Address:      "home address",
		MaxCount:     4,
		CurrentCount: 2,
		Status:       "closed",
		ClosedAt:     now + 10,
		CreatedAt:    now + 10,
		UpdatedAt:    now + 10,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID:   post.ID,
		UserID:   "user_home_repair_member",
		JoinedAt: now + 20,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	if err := db.Create(&model.PostParticipantSettlement{
		PostID:                 post.ID,
		UserID:                 "user_home_repair_member",
		ParticipantDecision:    score.DecisionCompleted,
		ParticipantConfirmedAt: now + 30,
		FinalStatus:            score.SettlementPending,
		CreatedAt:              now + 30,
		UpdatedAt:              now + 30,
	}).Error; err != nil {
		t.Fatalf("create stale settlement failed: %v", err)
	}

	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user_home_repair_author/home", nil)
	req.Header.Set("Authorization", bearerFor(t, db, "user_home_repair_author"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("get user home failed, code=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		InitiatedPosts []homePostView `json:"initiatedPosts"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode user home payload failed: %v", err)
	}
	if len(payload.InitiatedPosts) != 1 {
		t.Fatalf("expected 1 initiated post, got=%d", len(payload.InitiatedPosts))
	}

	postView := payload.InitiatedPosts[0]
	if postView.SettlementState.CanAuthorConfirm {
		t.Fatalf("author settlement button should be closed after repair: %+v", postView.SettlementState)
	}
	if !postView.ReviewState.CanReview {
		t.Fatalf("author should enter review stage after repair: %+v", postView.ReviewState)
	}
}
