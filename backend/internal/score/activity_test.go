package score

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func openScoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.PostParticipant{},
		&model.Review{},
		&model.ActivityScore{},
		&model.PostParticipantSettlement{},
		&model.CreditLedger{},
		&model.AdminCase{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

// seedClosedPost creates an author, one participant, and a closed post.
func seedClosedPost(t *testing.T, db *gorm.DB, postID string, closedAt int64) (authorID, participantID string) {
	t.Helper()
	authorID = postID + "_author"
	participantID = postID + "_participant"
	for _, id := range []string{authorID, participantID} {
		user := model.User{
			ID: id, Platform: "test", OpenID: "test_" + id, Nickname: id,
			Role: model.UserRoleUser, CreditScore: defaultCreditScore, RatingScore: 5,
			CreatedAt: closedAt, UpdatedAt: closedAt,
		}
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user %s failed: %v", id, err)
		}
	}
	post := model.Post{
		ID: postID, AuthorID: authorID, Title: "t", Category: "running", Address: "x",
		MaxCount: 4, CurrentCount: 2, Status: "closed", TimeMode: "weekend",
		ClosedAt: closedAt, CreatedAt: closedAt, UpdatedAt: closedAt,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post failed: %v", err)
	}
	relation := model.PostParticipant{
		PostID: postID, UserID: participantID, Status: ParticipantStatusActive, JoinedAt: closedAt,
	}
	if err := db.Create(&relation).Error; err != nil {
		t.Fatalf("create participant failed: %v", err)
	}
	return authorID, participantID
}

func settlementRow(t *testing.T, db *gorm.DB, postID, userID string) model.PostParticipantSettlement {
	t.Helper()
	var row model.PostParticipantSettlement
	if err := db.First(&row, "post_id = ? AND user_id = ?", postID, userID).Error; err != nil {
		t.Fatalf("load settlement failed: %v", err)
	}
	return row
}

func TestResolveFinalStatusMatrix(t *testing.T) {
	openPost := model.Post{ID: "p", Status: "closed"}
	cancelledPost := model.Post{ID: "p", Status: "closed", CancelledAt: 1}
	active := participantRelation{UserID: "u", Status: ParticipantStatusActive}
	cancelledRelation := participantRelation{UserID: "u", Status: ParticipantStatusCancelled}

	cases := []struct {
		name        string
		post        model.Post
		relation    participantRelation
		participant string
		author      string
		adminRuling string
		want        string
	}{
		{"both silent", openPost, active, "", "", "", SettlementPending},
		{"both completed", openPost, active, DecisionCompleted, DecisionCompleted, "", SettlementCompleted},
		{"author completed only", openPost, active, "", DecisionCompleted, "", SettlementCompleted},
		{"participant completed only", openPost, active, DecisionCompleted, "", "", SettlementCompleted},
		{"both no_show", openPost, active, DecisionNoShow, DecisionNoShow, "", SettlementNoShow},
		{"conflicting decisions", openPost, active, DecisionCompleted, DecisionNoShow, "", SettlementDisputed},
		{"participant disputes", openPost, active, DecisionDisputed, DecisionCompleted, "", SettlementDisputed},
		{"cancelled post wins", cancelledPost, active, DecisionCompleted, DecisionCompleted, "", SettlementCancelled},
		{"cancelled relation wins", openPost, cancelledRelation, DecisionCompleted, DecisionCompleted, "", SettlementCancelled},
		{"admin no_show beats dispute", openPost, active, DecisionDisputed, DecisionNoShow, SettlementNoShow, SettlementNoShow},
		{"admin completed beats dispute", openPost, active, DecisionDisputed, DecisionNoShow, SettlementCompleted, SettlementCompleted},
		{"cancelled post beats admin ruling", cancelledPost, active, DecisionDisputed, "", SettlementCompleted, SettlementCancelled},
		{"cancelled relation beats admin ruling", openPost, cancelledRelation, DecisionDisputed, "", SettlementNoShow, SettlementCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := model.PostParticipantSettlement{
				ParticipantDecision: tc.participant,
				AuthorDecision:      tc.author,
				AdminResolution:     tc.adminRuling,
			}
			got := resolveFinalStatus(tc.post, tc.relation, row, 1000)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCancellationOverridesAdminRulingAndRemovesOutcomeCredits(t *testing.T) {
	cases := []struct {
		name       string
		cancel     func(*testing.T, *gorm.DB, string, string, int64)
		wantLedger string
		wantUser   func(string, string) string
	}{
		{
			name: "project cancellation",
			cancel: func(t *testing.T, db *gorm.DB, postID, _ string, now int64) {
				t.Helper()
				if err := db.Model(&model.Post{}).Where("id = ?", postID).
					Update("cancelled_at", now).Error; err != nil {
					t.Fatalf("cancel post failed: %v", err)
				}
			},
			wantLedger: LedgerOrganizerCancelled,
			wantUser: func(authorID, _ string) string {
				return authorID
			},
		},
		{
			name: "participant cancellation",
			cancel: func(t *testing.T, db *gorm.DB, postID, participantID string, now int64) {
				t.Helper()
				if err := db.Model(&model.PostParticipant{}).
					Where("post_id = ? AND user_id = ?", postID, participantID).
					Updates(map[string]any{
						"status":       ParticipantStatusCancelled,
						"cancelled_at": now,
					}).Error; err != nil {
					t.Fatalf("cancel relation failed: %v", err)
				}
			},
			wantLedger: LedgerParticipantCancelled,
			wantUser: func(_, participantID string) string {
				return participantID
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openScoreTestDB(t)
			now := time.Now().UnixMilli()
			postID := "post_cancel_after_ruling"
			authorID, participantID := seedClosedPost(t, db, postID, now)

			if err := RecalculatePostActivityScores(db, postID, now); err != nil {
				t.Fatalf("initial recalculate failed: %v", err)
			}
			if err := db.Model(&model.PostParticipantSettlement{}).
				Where("post_id = ? AND user_id = ?", postID, participantID).
				Updates(map[string]any{
					"participant_decision": DecisionDisputed,
					"author_decision":      DecisionNoShow,
					"admin_resolution":     SettlementNoShow,
					"final_status":         SettlementNoShow,
					"settled_at":           now,
				}).Error; err != nil {
				t.Fatalf("seed admin ruling failed: %v", err)
			}
			if err := RecalculatePostActivityScores(db, postID, now+1000); err != nil {
				t.Fatalf("recalculate ruling failed: %v", err)
			}

			tc.cancel(t, db, postID, participantID, now+2000)
			if err := RecalculatePostActivityScores(db, postID, now+3000); err != nil {
				t.Fatalf("recalculate cancellation failed: %v", err)
			}
			if err := RecalculatePostActivityScores(db, postID, now+4000); err != nil {
				t.Fatalf("repeat recalculation failed: %v", err)
			}

			if got := settlementRow(t, db, postID, participantID).FinalStatus; got != SettlementCancelled {
				t.Fatalf("cancellation must override admin ruling, got %q", got)
			}
			var outcomeRows int64
			if err := db.Model(&model.CreditLedger{}).
				Where("post_id = ? AND source_type IN ?", postID, []string{
					LedgerParticipantCompleted,
					LedgerOrganizerCompleted,
					LedgerParticipantNoShow,
				}).
				Count(&outcomeRows).Error; err != nil {
				t.Fatalf("count stale outcome ledgers failed: %v", err)
			}
			if outcomeRows != 0 {
				t.Fatalf("completed/no-show ledgers must be removed after cancellation, got %d", outcomeRows)
			}
			var cancellationRows int64
			if err := db.Model(&model.CreditLedger{}).
				Where("post_id = ? AND user_id = ? AND source_type = ?",
					postID, tc.wantUser(authorID, participantID), tc.wantLedger).
				Count(&cancellationRows).Error; err != nil {
				t.Fatalf("count cancellation ledger failed: %v", err)
			}
			if cancellationRows != 1 {
				t.Fatalf("expected one %s ledger after repeated recalculation, got %d", tc.wantLedger, cancellationRows)
			}
		})
	}
}

// TestAdminNoShowRulingSurvivesRecalculation is the regression test for the bug
// where an admin ruling of no_show was immediately re-derived back to disputed
// and the resolved case was reopened.
func TestAdminNoShowRulingSurvivesRecalculation(t *testing.T) {
	db := openScoreTestDB(t)
	now := time.Now().UnixMilli()
	postID := "post_admin_ruling"
	_, participantID := seedClosedPost(t, db, postID, now)

	// The two sides disagree: participant disputes, author says no_show.
	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("initial recalculate failed: %v", err)
	}
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"participant_decision": DecisionDisputed,
			"author_decision":      DecisionNoShow,
		}).Error; err != nil {
		t.Fatalf("seed conflicting decisions failed: %v", err)
	}
	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("recalculate after conflict failed: %v", err)
	}
	if got := settlementRow(t, db, postID, participantID).FinalStatus; got != SettlementDisputed {
		t.Fatalf("expected disputed before ruling, got %q", got)
	}

	// Admin rules no_show, exactly as ResolveAdminCase does.
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"final_status":     SettlementNoShow,
			"admin_resolution": SettlementNoShow,
			"settled_at":       now,
		}).Error; err != nil {
		t.Fatalf("apply admin ruling failed: %v", err)
	}
	if err := db.Model(&model.AdminCase{}).
		Where("source_ref = ?", settlementCaseSource(postID, participantID)).
		Updates(map[string]any{"status": "resolved", "resolution": SettlementNoShow}).Error; err != nil {
		t.Fatalf("resolve case failed: %v", err)
	}

	// Any later recalculation (e.g. someone opening the settlement page).
	if err := RecalculatePostActivityScores(db, postID, now+1000); err != nil {
		t.Fatalf("recalculate after ruling failed: %v", err)
	}

	if got := settlementRow(t, db, postID, participantID).FinalStatus; got != SettlementNoShow {
		t.Fatalf("admin ruling must survive recalculation, got final_status=%q", got)
	}
	var reopened int64
	if err := db.Model(&model.AdminCase{}).
		Where("source_ref = ? AND status = ?", settlementCaseSource(postID, participantID), "open").
		Count(&reopened).Error; err != nil {
		t.Fatalf("count reopened cases failed: %v", err)
	}
	if reopened != 0 {
		t.Fatalf("resolved case must not be reopened, got %d open", reopened)
	}

	var ledger model.CreditLedger
	if err := db.First(&ledger, "post_id = ? AND user_id = ? AND source_type = ?",
		postID, participantID, LedgerParticipantNoShow).Error; err != nil {
		t.Fatalf("no_show credit penalty should be recorded: %v", err)
	}
	if ledger.Delta != creditNoShow {
		t.Fatalf("expected delta %d, got %d", creditNoShow, ledger.Delta)
	}
}

// TestCreditLedgerCreatedAtIsStable is the regression test for recalculation
// wiping the ledger's audit timestamps.
func TestCreditLedgerCreatedAtIsStable(t *testing.T) {
	db := openScoreTestDB(t)
	now := time.Now().UnixMilli()
	postID := "post_ledger_stable"
	_, participantID := seedClosedPost(t, db, postID, now)

	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("initial recalculate failed: %v", err)
	}
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"participant_decision": DecisionCompleted,
			"author_decision":      DecisionCompleted,
		}).Error; err != nil {
		t.Fatalf("seed decisions failed: %v", err)
	}
	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("recalculate failed: %v", err)
	}

	var before []model.CreditLedger
	if err := db.Order("user_id, source_type").Find(&before, "post_id = ?", postID).Error; err != nil {
		t.Fatalf("load ledgers failed: %v", err)
	}
	if len(before) == 0 {
		t.Fatalf("expected credit ledger rows after settlement")
	}

	// Simulate someone opening the settlement page much later.
	later := now + int64(72*time.Hour/time.Millisecond)
	if err := RecalculatePostActivityScores(db, postID, later); err != nil {
		t.Fatalf("later recalculate failed: %v", err)
	}

	var after []model.CreditLedger
	if err := db.Order("user_id, source_type").Find(&after, "post_id = ?", postID).Error; err != nil {
		t.Fatalf("reload ledgers failed: %v", err)
	}
	// Crossing the review deadline legitimately adds review_missed rows, so
	// only the entries that existed before are compared — none of them may
	// have had its timestamps or amount rewritten.
	afterByKey := make(map[string]model.CreditLedger, len(after))
	for _, row := range after {
		afterByKey[row.UserID+"/"+row.SourceType] = row
	}
	for _, row := range before {
		key := row.UserID + "/" + row.SourceType
		got, ok := afterByKey[key]
		if !ok {
			t.Fatalf("ledger %s disappeared after recalculation", key)
		}
		if got.CreatedAt != row.CreatedAt {
			t.Fatalf("createdAt for %s was rewritten: %d -> %d", key, row.CreatedAt, got.CreatedAt)
		}
		if got.Delta != row.Delta {
			t.Fatalf("delta for %s changed: %d -> %d", key, row.Delta, got.Delta)
		}
	}
}

// TestCreditLedgerDropsStaleRows checks the upsert still removes entries that
// no longer apply after the settlement outcome changes.
func TestCreditLedgerDropsStaleRows(t *testing.T) {
	db := openScoreTestDB(t)
	now := time.Now().UnixMilli()
	postID := "post_ledger_stale"
	_, participantID := seedClosedPost(t, db, postID, now)

	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("initial recalculate failed: %v", err)
	}
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"participant_decision": DecisionCompleted,
			"author_decision":      DecisionCompleted,
		}).Error; err != nil {
		t.Fatalf("seed completed decisions failed: %v", err)
	}
	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("recalculate completed failed: %v", err)
	}
	var completedCount int64
	if err := db.Model(&model.CreditLedger{}).
		Where("post_id = ? AND source_type = ?", postID, LedgerParticipantCompleted).
		Count(&completedCount).Error; err != nil {
		t.Fatalf("count completed ledger failed: %v", err)
	}
	if completedCount != 1 {
		t.Fatalf("expected 1 participant_completed row, got %d", completedCount)
	}

	// Outcome flips to no_show: the completed entry must disappear.
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"participant_decision": DecisionNoShow,
			"author_decision":      DecisionNoShow,
		}).Error; err != nil {
		t.Fatalf("flip to no_show failed: %v", err)
	}
	if err := RecalculatePostActivityScores(db, postID, now+1000); err != nil {
		t.Fatalf("recalculate no_show failed: %v", err)
	}

	if err := db.Model(&model.CreditLedger{}).
		Where("post_id = ? AND source_type = ?", postID, LedgerParticipantCompleted).
		Count(&completedCount).Error; err != nil {
		t.Fatalf("recount completed ledger failed: %v", err)
	}
	if completedCount != 0 {
		t.Fatalf("stale participant_completed row must be removed, got %d", completedCount)
	}
	var noShowCount int64
	if err := db.Model(&model.CreditLedger{}).
		Where("post_id = ? AND source_type = ?", postID, LedgerParticipantNoShow).
		Count(&noShowCount).Error; err != nil {
		t.Fatalf("count no_show ledger failed: %v", err)
	}
	if noShowCount != 1 {
		t.Fatalf("expected 1 participant_no_show row, got %d", noShowCount)
	}
}

// TestRecalculationIsIdempotent guards the whole pipeline: running it twice
// must not change any derived row.
func TestRecalculationIsIdempotent(t *testing.T) {
	db := openScoreTestDB(t)
	now := time.Now().UnixMilli()
	postID := "post_idempotent"
	authorID, participantID := seedClosedPost(t, db, postID, now)

	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("initial recalculate failed: %v", err)
	}
	if err := db.Model(&model.PostParticipantSettlement{}).
		Where("post_id = ? AND user_id = ?", postID, participantID).
		Updates(map[string]any{
			"participant_decision": DecisionCompleted,
			"author_decision":      DecisionCompleted,
		}).Error; err != nil {
		t.Fatalf("seed decisions failed: %v", err)
	}
	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("second recalculate failed: %v", err)
	}

	var firstScores []model.ActivityScore
	if err := db.Order("user_id").Find(&firstScores, "post_id = ?", postID).Error; err != nil {
		t.Fatalf("load activity scores failed: %v", err)
	}
	var firstUsers []model.User
	if err := db.Order("id").Find(&firstUsers, "id IN ?", []string{authorID, participantID}).Error; err != nil {
		t.Fatalf("load users failed: %v", err)
	}

	if err := RecalculatePostActivityScores(db, postID, now); err != nil {
		t.Fatalf("third recalculate failed: %v", err)
	}

	var secondScores []model.ActivityScore
	if err := db.Order("user_id").Find(&secondScores, "post_id = ?", postID).Error; err != nil {
		t.Fatalf("reload activity scores failed: %v", err)
	}
	if len(firstScores) != len(secondScores) {
		t.Fatalf("activity score count changed: %d -> %d", len(firstScores), len(secondScores))
	}
	for i := range firstScores {
		if firstScores[i].CreditScore != secondScores[i].CreditScore {
			t.Fatalf("credit score for %s changed: %d -> %d",
				firstScores[i].UserID, firstScores[i].CreditScore, secondScores[i].CreditScore)
		}
		if firstScores[i].FulfillmentStatus != secondScores[i].FulfillmentStatus {
			t.Fatalf("fulfillment status for %s changed: %q -> %q",
				firstScores[i].UserID, firstScores[i].FulfillmentStatus, secondScores[i].FulfillmentStatus)
		}
	}

	var secondUsers []model.User
	if err := db.Order("id").Find(&secondUsers, "id IN ?", []string{authorID, participantID}).Error; err != nil {
		t.Fatalf("reload users failed: %v", err)
	}
	for i := range firstUsers {
		if firstUsers[i].CreditScore != secondUsers[i].CreditScore {
			t.Fatalf("credit score for %s changed: %d -> %d",
				firstUsers[i].ID, firstUsers[i].CreditScore, secondUsers[i].CreditScore)
		}
	}
}

func TestCreditScoreStaysWithinBounds(t *testing.T) {
	db := openScoreTestDB(t)
	now := time.Now().UnixMilli()

	// Many no-shows must not drive the credit score below the floor.
	for i := 0; i < 12; i++ {
		postID := fmt.Sprintf("post_bounds_%02d", i)
		authorID := "post_bounds_00_author"
		participantID := "post_bounds_00_participant"
		if i == 0 {
			authorID, participantID = seedClosedPost(t, db, postID, now)
			_ = authorID
		} else {
			post := model.Post{
				ID: postID, AuthorID: authorID, Title: "t", Category: "running", Address: "x",
				MaxCount: 4, CurrentCount: 2, Status: "closed", TimeMode: "weekend",
				ClosedAt: now, CreatedAt: now, UpdatedAt: now,
			}
			if err := db.Create(&post).Error; err != nil {
				t.Fatalf("create post failed: %v", err)
			}
			relation := model.PostParticipant{
				PostID: postID, UserID: participantID, Status: ParticipantStatusActive, JoinedAt: now,
			}
			if err := db.Create(&relation).Error; err != nil {
				t.Fatalf("create participant failed: %v", err)
			}
		}
		if err := RecalculatePostActivityScores(db, postID, now); err != nil {
			t.Fatalf("recalculate %s failed: %v", postID, err)
		}
		if err := db.Model(&model.PostParticipantSettlement{}).
			Where("post_id = ? AND user_id = ?", postID, participantID).
			Updates(map[string]any{
				"participant_decision": DecisionNoShow,
				"author_decision":      DecisionNoShow,
			}).Error; err != nil {
			t.Fatalf("seed no_show failed: %v", err)
		}
		if err := RecalculatePostActivityScores(db, postID, now); err != nil {
			t.Fatalf("recalculate no_show %s failed: %v", postID, err)
		}
	}

	var participant model.User
	if err := db.First(&participant, "id = ?", "post_bounds_00_participant").Error; err != nil {
		t.Fatalf("load participant failed: %v", err)
	}
	if participant.CreditScore < minCreditScore || participant.CreditScore > maxCreditScore {
		t.Fatalf("credit score %d out of bounds [%d, %d]",
			participant.CreditScore, minCreditScore, maxCreditScore)
	}
}
