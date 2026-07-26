package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

func promoteToAdmin(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	ensureTestUser(t, db, userID)
	if err := db.Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"role": model.UserRoleAdmin, "root_admin": true}).Error; err != nil {
		t.Fatalf("promote admin failed: %v", err)
	}
}

// TestCancelledParticipantCanRejoin covers the bug where a cancelled
// participation permanently blocked rejoining the same post.
func TestCancelledParticipantCanRejoin(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	now := time.Now().UnixMilli()

	if err := db.Create(&model.Post{
		ID: "post_rejoin", AuthorID: "user_rejoin_author", Title: "t", Category: "running",
		Address: "x", MaxCount: 4, CurrentCount: 1, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	ensureTestUser(t, db, "user_rejoin_author")
	joiner := bearerFor(t, db, "user_rejoin_joiner")

	doPost := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", joiner)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	if resp := doPost("/api/v1/posts/post_rejoin/join"); resp.Code != http.StatusOK {
		t.Fatalf("first join failed: %d %s", resp.Code, resp.Body.String())
	}
	if resp := doPost("/api/v1/posts/post_rejoin/participation/cancel"); resp.Code != http.StatusOK {
		t.Fatalf("cancel failed: %d %s", resp.Code, resp.Body.String())
	}
	if resp := doPost("/api/v1/posts/post_rejoin/join"); resp.Code != http.StatusOK {
		t.Fatalf("rejoin after cancel must succeed, got %d %s", resp.Code, resp.Body.String())
	}

	var relation model.PostParticipant
	if err := db.First(&relation, "post_id = ? AND user_id = ?", "post_rejoin", "user_rejoin_joiner").Error; err != nil {
		t.Fatalf("load relation failed: %v", err)
	}
	if score.NormalizedParticipantStatus(relation.Status) != score.ParticipantStatusActive {
		t.Fatalf("relation should be active after rejoin, got %q", relation.Status)
	}
	if relation.CancelledAt != 0 {
		t.Fatalf("cancelledAt should be cleared after rejoin, got %d", relation.CancelledAt)
	}

	var settlement model.PostParticipantSettlement
	if err := db.First(&settlement, "post_id = ? AND user_id = ?", "post_rejoin", "user_rejoin_joiner").Error; err == nil {
		if settlement.FinalStatus != score.SettlementPending {
			t.Fatalf("settlement should be reset to pending after rejoin, got %q", settlement.FinalStatus)
		}
	}

	var post model.Post
	if err := db.First(&post, "id = ?", "post_rejoin").Error; err != nil {
		t.Fatalf("load post failed: %v", err)
	}
	if post.CurrentCount != 2 {
		t.Fatalf("currentCount should be 2 (author + rejoined participant), got %d", post.CurrentCount)
	}
}

// TestJoinIsRejectedWhenFull checks the conditional count update rejects a join
// that would exceed maxCount.
func TestJoinIsRejectedWhenFull(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	now := time.Now().UnixMilli()

	if err := db.Create(&model.Post{
		ID: "post_full", AuthorID: "user_full_author", Title: "t", Category: "running",
		Address: "x", MaxCount: 2, CurrentCount: 2, Status: "open", TimeMode: "weekend",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	ensureTestUser(t, db, "user_full_author")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/post_full/join", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_full_joiner"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code == http.StatusOK {
		t.Fatalf("join on a full post must fail, got 200 %s", resp.Body.String())
	}
	var post model.Post
	if err := db.First(&post, "id = ?", "post_full").Error; err != nil {
		t.Fatalf("load post failed: %v", err)
	}
	if post.CurrentCount != 2 {
		t.Fatalf("currentCount must not exceed maxCount, got %d", post.CurrentCount)
	}
}

// TestAdminNoShowResolutionPersists drives the admin case resolution over HTTP
// and then forces a recalculation, asserting the ruling and its credit penalty
// both survive.
func TestAdminNoShowResolutionPersists(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	now := time.Now().UnixMilli()

	authorID := "user_case_author"
	participantID := "user_case_participant"
	ensureTestUser(t, db, authorID)
	ensureTestUser(t, db, participantID)
	promoteToAdmin(t, db, "user_case_admin")

	if err := db.Create(&model.Post{
		ID: "post_case", AuthorID: authorID, Title: "t", Category: "running", Address: "x",
		MaxCount: 4, CurrentCount: 2, Status: "closed", TimeMode: "weekend",
		ClosedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	if err := db.Create(&model.PostParticipant{
		PostID: "post_case", UserID: participantID, Status: score.ParticipantStatusActive, JoinedAt: now,
	}).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	if err := db.Create(&model.PostParticipantSettlement{
		PostID: "post_case", UserID: participantID,
		ParticipantDecision: score.DecisionDisputed, AuthorDecision: score.DecisionNoShow,
		FinalStatus: score.SettlementDisputed, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create settlement failed: %v", err)
	}
	caseID := "case_post_case"
	if err := db.Create(&model.AdminCase{
		ID: caseID, CaseType: score.AdminCaseSettlementDispute, PostID: "post_case",
		TargetUserID: participantID, ReporterUserID: authorID, Status: "open",
		SourceRef: "settlement:post_case:" + participantID, Summary: "dispute",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create admin case failed: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"resolution": score.SettlementNoShow, "note": "证据显示未到场"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/cases/"+caseID+"/resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_case_admin"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("resolve case failed: %d %s", resp.Code, resp.Body.String())
	}

	// Force another recalculation, as any settlement page view would.
	if err := score.RecalculatePostActivityScores(db, "post_case", now+5000); err != nil {
		t.Fatalf("recalculate failed: %v", err)
	}

	var settlement model.PostParticipantSettlement
	if err := db.First(&settlement, "post_id = ? AND user_id = ?", "post_case", participantID).Error; err != nil {
		t.Fatalf("load settlement failed: %v", err)
	}
	if settlement.FinalStatus != score.SettlementNoShow {
		t.Fatalf("admin no_show ruling must persist, got %q", settlement.FinalStatus)
	}

	var openCases int64
	if err := db.Model(&model.AdminCase{}).Where("id = ? AND status = ?", caseID, "open").
		Count(&openCases).Error; err != nil {
		t.Fatalf("count open cases failed: %v", err)
	}
	if openCases != 0 {
		t.Fatalf("resolved case must stay resolved, got %d open", openCases)
	}
}

// TestRefreshTokenCannotBeReplayed asserts a rotated refresh token is rejected
// on reuse instead of minting a second set of tokens.
func TestRefreshTokenCannotBeReplayed(t *testing.T) {
	db := openRouterTestDB(t)
	t.Setenv("ENABLE_MOCK_LOGIN", "true")
	router := NewRouter(db)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-login",
		bytes.NewReader([]byte(`{"nickname":"replay_user"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	var login loginResp2
	if err := json.Unmarshal(loginResp.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login failed: %v", err)
	}
	if login.RefreshToken == "" {
		t.Fatalf("expected a refresh token from login")
	}

	refresh := func() *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]string{"refreshToken": login.RefreshToken})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	if first := refresh(); first.Code != http.StatusOK {
		t.Fatalf("first refresh should succeed: %d %s", first.Code, first.Body.String())
	}
	second := refresh()
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replayed refresh token must be rejected, got %d %s", second.Code, second.Body.String())
	}
}

type loginResp2 struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}
