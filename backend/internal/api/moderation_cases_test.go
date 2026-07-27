package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

func doJSONRequest(t *testing.T, router http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body failed: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestModerationLifecycleAndStaleResultIgnored(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	srv := &Server{DB: db}

	authorID := "user_mod_author"
	ensureTestUser(t, db, authorID)
	authorBearer := bearerFor(t, db, authorID)

	createResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/posts", authorBearer, map[string]any{
		"title":       "周末跑步",
		"description": "一起去公园跑步",
		"category":    "running",
		"address":     "city park",
		"maxCount":    4,
		"timeInfo": map[string]any{
			"mode":      "range",
			"days":      1,
			"fixedTime": "",
		},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create post failed: %d %s", createResp.Code, createResp.Body.String())
	}

	var createPayload struct {
		Post model.Post `json:"post"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	var post model.Post
	if err := db.First(&post, "id = ?", createPayload.Post.ID).Error; err != nil {
		t.Fatalf("load post failed: %v", err)
	}
	if post.ModerationStatus != model.ModerationPending {
		t.Fatalf("new post should be pending, got %q", post.ModerationStatus)
	}
	oldRecordID := post.CurrentModerationID

	if err := srv.ProcessModerationRecord(oldRecordID); err != nil {
		t.Fatalf("process moderation failed: %v", err)
	}
	if err := db.First(&post, "id = ?", post.ID).Error; err != nil {
		t.Fatalf("reload post failed: %v", err)
	}
	if post.ModerationStatus != model.ModerationApproved {
		t.Fatalf("expected approved moderation, got %q", post.ModerationStatus)
	}

	var record model.ModerationRecord
	if err := db.First(&record, "id = ?", oldRecordID).Error; err != nil {
		t.Fatalf("load moderation record failed: %v", err)
	}
	if record.FinishedAt == 0 || record.AttemptCount != 1 {
		t.Fatalf("moderation record should finish once, got finishedAt=%d attempts=%d", record.FinishedAt, record.AttemptCount)
	}

	if err := srv.ProcessModerationRecord(oldRecordID); err != nil {
		t.Fatalf("duplicate moderation process should be ignored: %v", err)
	}
	if err := db.First(&record, "id = ?", oldRecordID).Error; err != nil {
		t.Fatalf("reload moderation record failed: %v", err)
	}
	if record.AttemptCount != 1 {
		t.Fatalf("duplicate moderation run should not increase attempts, got %d", record.AttemptCount)
	}

	updateResp := doJSONRequest(t, router, http.MethodPut, "/api/v1/posts/"+post.ID, authorBearer, map[string]any{
		"title":       "诈骗团建",
		"description": "一起去公园跑步",
		"category":    "running",
		"address":     "city park",
		"maxCount":    4,
		"timeInfo": map[string]any{
			"mode":      "range",
			"days":      1,
			"fixedTime": "",
		},
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update post failed: %d %s", updateResp.Code, updateResp.Body.String())
	}

	if err := db.First(&post, "id = ?", post.ID).Error; err != nil {
		t.Fatalf("reload updated post failed: %v", err)
	}
	newRecordID := post.CurrentModerationID
	if newRecordID == oldRecordID {
		t.Fatalf("expected a new moderation record after edit")
	}
	if post.ModerationStatus != model.ModerationPending {
		t.Fatalf("edited post should go back to pending, got %q", post.ModerationStatus)
	}

	if err := srv.ProcessModerationRecord(oldRecordID); err != nil {
		t.Fatalf("stale moderation should be ignored: %v", err)
	}
	if err := db.First(&post, "id = ?", post.ID).Error; err != nil {
		t.Fatalf("reload after stale process failed: %v", err)
	}
	if post.CurrentModerationID != newRecordID || post.ModerationStatus != model.ModerationPending {
		t.Fatalf("stale moderation changed current state: %+v", post)
	}

	if err := srv.ProcessModerationRecord(newRecordID); err != nil {
		t.Fatalf("process new moderation failed: %v", err)
	}
	if err := db.First(&post, "id = ?", post.ID).Error; err != nil {
		t.Fatalf("reload final post failed: %v", err)
	}
	if post.ModerationStatus != model.ModerationManualReview {
		t.Fatalf("high-risk edit should require manual review, got %q", post.ModerationStatus)
	}
}

func TestModerationPendingBlocksJoinAndInvitationAcceptance(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	now := time.Now().UnixMilli()

	authorID := "user_join_author"
	memberID := "user_join_member"
	ensureTestUser(t, db, authorID)
	ensureTestUser(t, db, memberID)

	createResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/posts", bearerFor(t, db, authorID), map[string]any{
		"title":       "审核中的活动",
		"description": "先别急着进来",
		"category":    "running",
		"address":     "court",
		"maxCount":    4,
		"timeInfo": map[string]any{
			"mode":      "range",
			"days":      1,
			"fixedTime": "",
		},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create post failed: %d %s", createResp.Code, createResp.Body.String())
	}
	var payload struct {
		Post model.Post `json:"post"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	invitation := model.PostInvitation{
		ID:        "invite_mod_pending",
		PostID:    payload.Post.ID,
		InviterID: authorID,
		InviteeID: memberID,
		Message:   "来一起吧",
		Status:    model.InvitationStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&invitation).Error; err != nil {
		t.Fatalf("create invitation failed: %v", err)
	}

	joinResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/posts/"+payload.Post.ID+"/join", bearerFor(t, db, memberID), map[string]any{})
	if joinResp.Code != http.StatusBadRequest || !strings.Contains(joinResp.Body.String(), "post is under moderation") {
		t.Fatalf("join should be blocked by moderation, got %d %s", joinResp.Code, joinResp.Body.String())
	}

	acceptResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/invitations/"+invitation.ID+"/accept", bearerFor(t, db, memberID), map[string]any{})
	if acceptResp.Code != http.StatusBadRequest || !strings.Contains(acceptResp.Body.String(), "post is under moderation") {
		t.Fatalf("accept should be blocked by moderation, got %d %s", acceptResp.Code, acceptResp.Body.String())
	}
}

func TestCreditAppealCanReverseAndRevokeCredit(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)
	now := time.Now().UnixMilli()

	userID := "user_credit_target"
	adminID := "user_credit_admin"
	ensureTestUser(t, db, userID)
	promoteToAdmin(t, db, adminID)

	ledger := model.CreditLedger{
		UserID:     userID,
		PostID:     "post_credit_case",
		SourceType: score.LedgerManualCreditAdjust,
		Delta:      -10,
		Status:     "settled",
		Note:       "manual penalty",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(&ledger).Error; err != nil {
		t.Fatalf("create credit ledger failed: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return score.RecalculateUsersFromActivityScoresTx(tx, []string{userID}, now)
	}); err != nil {
		t.Fatalf("recalculate credit failed: %v", err)
	}

	var user model.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatalf("load user failed: %v", err)
	}
	if user.CreditScore != 90 {
		t.Fatalf("expected credit score 90 after penalty, got %d", user.CreditScore)
	}

	caseResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/credit-ledgers/"+strconv.FormatUint(ledger.ID, 10)+"/appeals", bearerFor(t, db, userID), map[string]any{
		"description": "处罚有误",
		"evidence": map[string]any{
			"note": "请复核",
		},
	})
	if caseResp.Code != http.StatusOK {
		t.Fatalf("create appeal failed: %d %s", caseResp.Code, caseResp.Body.String())
	}
	var appealPayload struct {
		Case model.AdminCase `json:"case"`
	}
	if err := json.Unmarshal(caseResp.Body.Bytes(), &appealPayload); err != nil {
		t.Fatalf("decode appeal response failed: %v", err)
	}
	if appealPayload.Case.CaseType != model.CaseTypeCreditAppeal {
		t.Fatalf("unexpected case type %q", appealPayload.Case.CaseType)
	}

	decisionResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/cases/"+appealPayload.Case.ID+"/decision", bearerFor(t, db, adminID), map[string]any{
		"decision": "approve",
		"reason":   "appeal upheld",
	})
	if decisionResp.Code != http.StatusOK {
		t.Fatalf("approve appeal failed: %d %s", decisionResp.Code, decisionResp.Body.String())
	}

	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatalf("reload user failed: %v", err)
	}
	if user.CreditScore != 100 {
		t.Fatalf("expected credit score restored to 100, got %d", user.CreditScore)
	}

	var reversal model.CreditLedger
	if err := db.First(&reversal, "user_id = ? AND post_id = ? AND source_type = ?", userID, ledger.PostID, score.LedgerCreditAppealReversal).Error; err != nil {
		t.Fatalf("load reversal ledger failed: %v", err)
	}
	if reversal.Status != "settled" || reversal.Delta != 10 {
		t.Fatalf("unexpected reversal ledger: %+v", reversal)
	}

	reopenResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/cases/"+appealPayload.Case.ID+"/reopen", bearerFor(t, db, adminID), map[string]any{})
	if reopenResp.Code != http.StatusOK {
		t.Fatalf("reopen case failed: %d %s", reopenResp.Code, reopenResp.Body.String())
	}

	rejectResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/cases/"+appealPayload.Case.ID+"/decision", bearerFor(t, db, adminID), map[string]any{
		"decision": "reject",
		"reason":   "appeal denied",
	})
	if rejectResp.Code != http.StatusOK {
		t.Fatalf("reject reopened appeal failed: %d %s", rejectResp.Code, rejectResp.Body.String())
	}

	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatalf("reload user after reject failed: %v", err)
	}
	if user.CreditScore != 90 {
		t.Fatalf("expected credit score to return to 90, got %d", user.CreditScore)
	}
	if err := db.First(&reversal, "user_id = ? AND post_id = ? AND source_type = ?", userID, ledger.PostID, score.LedgerCreditAppealReversal).Error; err != nil {
		t.Fatalf("reload reversal ledger failed: %v", err)
	}
	if reversal.Status != "voided" || reversal.Delta != 0 {
		t.Fatalf("reopened rejection should void reversal, got %+v", reversal)
	}
}

func TestReportPostBuildsCaseContext(t *testing.T) {
	db := openRouterTestDB(t)
	router := NewRouter(db)

	authorID := "user_report_author"
	reporterID := "user_report_reporter"
	adminID := "user_report_admin"
	ensureTestUser(t, db, authorID)
	ensureTestUser(t, db, reporterID)
	promoteToAdmin(t, db, adminID)

	createResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/posts", bearerFor(t, db, authorID), map[string]any{
		"title":       "待举报活动",
		"description": "需要生成案件上下文",
		"category":    "running",
		"address":     "court",
		"maxCount":    4,
		"timeInfo": map[string]any{
			"mode":      "range",
			"days":      1,
			"fixedTime": "",
		},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create post failed: %d %s", createResp.Code, createResp.Body.String())
	}
	var postPayload struct {
		Post model.Post `json:"post"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &postPayload); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	reportResp := doJSONRequest(t, router, http.MethodPost, "/api/v1/posts/"+postPayload.Post.ID+"/reports", bearerFor(t, db, reporterID), map[string]any{
		"description": "内容有问题",
		"evidence": map[string]any{
			"screenshot": "attached",
		},
	})
	if reportResp.Code != http.StatusOK {
		t.Fatalf("report post failed: %d %s", reportResp.Code, reportResp.Body.String())
	}
	var reportPayload struct {
		Case model.AdminCase `json:"case"`
	}
	if err := json.Unmarshal(reportResp.Body.Bytes(), &reportPayload); err != nil {
		t.Fatalf("decode report response failed: %v", err)
	}

	contextResp := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/cases/"+reportPayload.Case.ID+"/context", bearerFor(t, db, adminID), nil)
	if contextResp.Code != http.StatusOK {
		t.Fatalf("load case context failed: %d %s", contextResp.Code, contextResp.Body.String())
	}
	var ctx caseContext
	if err := json.Unmarshal(contextResp.Body.Bytes(), &ctx); err != nil {
		t.Fatalf("decode context failed: %v", err)
	}
	if ctx.Case.ID != reportPayload.Case.ID {
		t.Fatalf("case context returned wrong case: %+v", ctx.Case)
	}
	if ctx.Post == nil || ctx.Post.ID != postPayload.Post.ID {
		t.Fatalf("context should include the reported post")
	}
	if len(ctx.Events) != 1 || ctx.Events[0].EventType != "case_created" {
		t.Fatalf("context should include creation event, got %+v", ctx.Events)
	}
}
