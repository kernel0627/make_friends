package score

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
)

const (
	RoleAuthor      = "author"
	RoleParticipant = "participant"

	ParticipantStatusActive    = "active"
	ParticipantStatusCancelled = "cancelled"

	SettlementPending   = "pending"
	SettlementCompleted = "completed"
	SettlementCancelled = "cancelled"
	SettlementNoShow    = "no_show"
	SettlementDisputed  = "disputed"

	DecisionCompleted = "completed"
	DecisionCancelled = "cancelled"
	DecisionNoShow    = "no_show"
	DecisionDisputed  = "disputed"

	LedgerParticipantCompleted = "participant_completed"
	LedgerOrganizerCompleted   = "organizer_completed"
	LedgerParticipantCancelled = "participant_cancelled"
	LedgerParticipantNoShow    = "participant_no_show"
	LedgerOrganizerCancelled   = "organizer_cancelled"
	LedgerReviewCompleted      = "review_completed"
	LedgerReviewMissed         = "review_missed"
	LedgerManualCreditAdjust   = "manual_credit_adjust"
	LedgerCreditAppealReversal = "credit_appeal_reversal"

	AdminCaseSettlementDispute = "settlement_dispute"
	AdminCaseManualCredit      = "manual_credit_review"
)

const (
	reviewWindowMS      = int64(48 * time.Hour / time.Millisecond)
	creditWindowMS      = int64(180 * 24 * time.Hour / time.Millisecond)
	defaultCreditScore  = 100
	minCreditScore      = 60
	maxCreditScore      = 100
	creditCompleted     = 3
	creditAuthorSuccess = 4
	creditCancelled     = -2
	creditNoShow        = -12
	creditAuthorCancel  = -4
	creditReviewInTime  = 2
	creditReviewMiss    = -3
)

type member struct {
	UserID string
	Role   string
}

type participantRelation struct {
	UserID      string
	Status      string
	JoinedAt    int64
	CancelledAt int64
}

type settlementInfo struct {
	model.PostParticipantSettlement
	Relation participantRelation
}

type ledgerSpec struct {
	UserID     string
	PostID     string
	SourceType string
	Delta      int
	Status     string
	Note       string
}

func RecalculatePostActivityScores(db *gorm.DB, postID string, nowMS int64) error {
	if nowMS <= 0 {
		nowMS = time.Now().UnixMilli()
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, "id = ?", strings.TrimSpace(postID)).Error; err != nil {
			return err
		}
		return RecalculatePostActivityScoresTx(tx, post, nowMS)
	})
}

func RecalculatePostActivityScoresTx(tx *gorm.DB, post model.Post, nowMS int64) error {
	if strings.TrimSpace(post.ID) == "" {
		return nil
	}
	if nowMS <= 0 {
		nowMS = time.Now().UnixMilli()
	}
	if post.Status == "closed" && post.ClosedAt == 0 {
		post.ClosedAt = nowMS
		if err := tx.Model(&model.Post{}).
			Where("id = ?", post.ID).
			Update("closed_at", post.ClosedAt).Error; err != nil {
			return err
		}
	}

	relations, err := loadParticipantRelations(tx, post.ID)
	if err != nil {
		return err
	}
	members := membersFromPost(post, relations)
	if len(members) == 0 {
		return nil
	}

	settlements, err := ensureSettlementsTx(tx, post, relations, nowMS)
	if err != nil {
		return err
	}
	if err := reconcileSettlementCases(tx, post, settlements, nowMS); err != nil {
		return err
	}

	var reviews []model.Review
	if err := tx.Where("post_id = ?", post.ID).Find(&reviews).Error; err != nil {
		return err
	}

	expectedByUser, completedByUser, timelyReviewByUser := buildReviewProgress(post, settlements, reviews)
	desiredLedgers := buildDesiredLedgers(post, settlements, expectedByUser, completedByUser, timelyReviewByUser, nowMS)
	if err := reconcileCreditLedgers(tx, post.ID, desiredLedgers, nowMS); err != nil {
		return err
	}

	ledgerTotals, err := loadLedgerTotalsByUser(tx, post.ID)
	if err != nil {
		return err
	}
	receivedTotal, receivedCount := buildReceivedRatings(reviews)
	settlementByUser := make(map[string]settlementInfo, len(settlements))
	for _, item := range settlements {
		settlementByUser[item.UserID] = item
	}

	for _, item := range members {
		ratingCount := receivedCount[item.UserID]
		ratingScore := 0.0
		if ratingCount > 0 {
			ratingScore = float64(receivedTotal[item.UserID]) / float64(ratingCount)
		}
		creditScore := clampCredit(defaultCreditScore + ledgerTotals[item.UserID])
		record := model.ActivityScore{
			PostID:               post.ID,
			UserID:               item.UserID,
			Role:                 item.Role,
			RatingScore:          ratingScore,
			RatingCount:          ratingCount,
			CreditScore:          creditScore,
			ExpectedReviewCount:  expectedByUser[item.UserID],
			CompletedReviewCount: completedByUser[item.UserID],
			FulfillmentStatus:    fulfillmentStatusForMember(post, item, settlementByUser, settlements),
			CreatedAt:            nowMS,
			UpdatedAt:            nowMS,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"role":                   record.Role,
				"rating_score":           record.RatingScore,
				"rating_count":           record.RatingCount,
				"credit_score":           record.CreditScore,
				"expected_review_count":  record.ExpectedReviewCount,
				"completed_review_count": record.CompletedReviewCount,
				"fulfillment_status":     record.FulfillmentStatus,
				"updated_at":             record.UpdatedAt,
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
	}

	memberIDs := make([]string, 0, len(members))
	for _, item := range members {
		memberIDs = append(memberIDs, item.UserID)
	}
	return RecalculateUsersFromActivityScoresTx(tx, memberIDs, nowMS)
}

func RecalculateUsersFromActivityScoresTx(tx *gorm.DB, userIDs []string, nowMS int64) error {
	uniqueIDs := uniqueStrings(userIDs)
	if len(uniqueIDs) == 0 {
		return nil
	}
	if nowMS <= 0 {
		nowMS = time.Now().UnixMilli()
	}

	var rows []model.ActivityScore
	if err := tx.Where("user_id IN ?", uniqueIDs).Find(&rows).Error; err != nil {
		return err
	}
	byUser := make(map[string][]model.ActivityScore, len(uniqueIDs))
	for _, row := range rows {
		byUser[row.UserID] = append(byUser[row.UserID], row)
	}

	var ledgers []model.CreditLedger
	cutoff := nowMS - creditWindowMS
	if err := tx.Where("user_id IN ? AND status = ? AND created_at >= ?", uniqueIDs, "settled", cutoff).Find(&ledgers).Error; err != nil {
		return err
	}
	creditDeltaByUser := make(map[string]int, len(uniqueIDs))
	for _, row := range ledgers {
		creditDeltaByUser[row.UserID] += row.Delta
	}

	for _, userID := range uniqueIDs {
		items := byUser[userID]
		ratingScore := 5.0
		totalRatingWeight := 0
		totalRatingValue := 0.0
		for _, item := range items {
			if item.RatingCount <= 0 {
				continue
			}
			totalRatingWeight += item.RatingCount
			totalRatingValue += item.RatingScore * float64(item.RatingCount)
		}
		if totalRatingWeight > 0 {
			ratingScore = totalRatingValue / float64(totalRatingWeight)
		}
		creditScore := clampCredit(defaultCreditScore + creditDeltaByUser[userID])
		if err := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"rating_score": ratingScore,
				"credit_score": creditScore,
				"updated_at":   nowMS,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func loadParticipantRelations(tx *gorm.DB, postID string) ([]participantRelation, error) {
	var rows []model.PostParticipant
	if err := tx.Where("post_id = ?", postID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]participantRelation, 0, len(rows))
	for _, row := range rows {
		out = append(out, participantRelation{
			UserID:      strings.TrimSpace(row.UserID),
			Status:      NormalizedParticipantStatus(row.Status),
			JoinedAt:    row.JoinedAt,
			CancelledAt: row.CancelledAt,
		})
	}
	return out, nil
}

func membersFromPost(post model.Post, relations []participantRelation) []member {
	seen := map[string]struct{}{}
	out := make([]member, 0, len(relations)+1)
	authorID := strings.TrimSpace(post.AuthorID)
	if authorID != "" {
		seen[authorID] = struct{}{}
		out = append(out, member{UserID: authorID, Role: RoleAuthor})
	}
	for _, relation := range relations {
		if relation.UserID == "" {
			continue
		}
		if _, ok := seen[relation.UserID]; ok {
			continue
		}
		seen[relation.UserID] = struct{}{}
		out = append(out, member{UserID: relation.UserID, Role: RoleParticipant})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role == RoleAuthor
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

func ensureSettlementsTx(tx *gorm.DB, post model.Post, relations []participantRelation, nowMS int64) ([]settlementInfo, error) {
	if len(relations) == 0 {
		return []settlementInfo{}, nil
	}

	var existing []model.PostParticipantSettlement
	if err := tx.Where("post_id = ?", post.ID).Find(&existing).Error; err != nil {
		return nil, err
	}
	existingByUser := make(map[string]model.PostParticipantSettlement, len(existing))
	for _, row := range existing {
		existingByUser[row.UserID] = row
	}

	result := make([]settlementInfo, 0, len(relations))
	for _, relation := range relations {
		row, ok := existingByUser[relation.UserID]
		if !ok {
			row = model.PostParticipantSettlement{
				PostID:      post.ID,
				UserID:      relation.UserID,
				FinalStatus: SettlementPending,
				CreatedAt:   nowMS,
				UpdatedAt:   nowMS,
			}
		}
		if relation.Status == ParticipantStatusCancelled && strings.TrimSpace(row.ParticipantDecision) == "" {
			row.ParticipantDecision = DecisionCancelled
			row.ParticipantConfirmedAt = maxInt64(row.ParticipantConfirmedAt, relation.CancelledAt)
		}
		finalStatus := resolveFinalStatus(post, relation, row, nowMS)
		row.FinalStatus = finalStatus
		if finalStatus == SettlementPending || finalStatus == SettlementDisputed {
			row.SettledAt = 0
		} else if row.SettledAt == 0 {
			row.SettledAt = nowMS
		}
		row.UpdatedAt = nowMS
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"participant_decision":     row.ParticipantDecision,
				"author_decision":          row.AuthorDecision,
				"final_status":             row.FinalStatus,
				"admin_resolution":         row.AdminResolution,
				"participant_note":         row.ParticipantNote,
				"author_note":              row.AuthorNote,
				"participant_confirmed_at": row.ParticipantConfirmedAt,
				"author_confirmed_at":      row.AuthorConfirmedAt,
				"settled_at":               row.SettledAt,
				"updated_at":               row.UpdatedAt,
			}),
		}).Create(&row).Error; err != nil {
			return nil, err
		}
		result = append(result, settlementInfo{
			PostParticipantSettlement: row,
			Relation:                  relation,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].UserID < result[j].UserID
	})
	return result, nil
}

func resolveFinalStatus(post model.Post, relation participantRelation, row model.PostParticipantSettlement, nowMS int64) string {
	// Cancellation ends the settlement relationship. It must take precedence
	// even when an administrator ruled on an earlier dispute.
	if post.CancelledAt > 0 {
		return SettlementCancelled
	}
	if relation.Status == ParticipantStatusCancelled {
		return SettlementCancelled
	}
	// For a relationship that still exists, an admin ruling is durable across
	// later recalculations so the parties' conflicting decisions do not reopen
	// a resolved case.
	if adminResolution := strings.TrimSpace(row.AdminResolution); adminResolution != "" {
		return adminResolution
	}

	participantDecision := strings.TrimSpace(row.ParticipantDecision)
	authorDecision := strings.TrimSpace(row.AuthorDecision)

	if participantDecision == DecisionDisputed {
		return SettlementDisputed
	}

	switch {
	case authorDecision == DecisionCompleted:
		if participantDecision == "" || participantDecision == DecisionCompleted {
			return SettlementCompleted
		}
		return SettlementDisputed
	case authorDecision == DecisionNoShow:
		if participantDecision == "" || participantDecision == DecisionNoShow {
			return SettlementNoShow
		}
		return SettlementDisputed
	case participantDecision == DecisionCompleted:
		return SettlementCompleted
	case participantDecision == DecisionNoShow:
		return SettlementNoShow
	case participantDecision == DecisionCancelled:
		return SettlementCancelled
	default:
		return SettlementPending
	}
}

func buildReviewProgress(post model.Post, settlements []settlementInfo, reviews []model.Review) (map[string]int, map[string]int, map[string]bool) {
	expectedByUser := map[string]int{}
	completedByUser := map[string]int{}
	timelyByUser := map[string]bool{}
	if post.CancelledAt > 0 {
		return expectedByUser, completedByUser, timelyByUser
	}

	authorID := strings.TrimSpace(post.AuthorID)
	requiredTargets := map[string]map[string]struct{}{}
	reviewLatestAt := map[string]int64{}
	activeParticipantIDs := make([]string, 0, len(settlements))

	for _, item := range settlements {
		status := item.FinalStatus
		if status == SettlementCancelled {
			continue
		}
		activeParticipantIDs = append(activeParticipantIDs, item.UserID)
		requiredTargets[item.UserID] = map[string]struct{}{authorID: {}}
	}
	if authorID != "" {
		targets := map[string]struct{}{}
		for _, participantID := range activeParticipantIDs {
			targets[participantID] = struct{}{}
		}
		requiredTargets[authorID] = targets
	}

	for userID, targets := range requiredTargets {
		expectedByUser[userID] = len(targets)
	}

	for _, review := range reviews {
		fromID := strings.TrimSpace(review.FromUserID)
		toID := strings.TrimSpace(review.ToUserID)
		targets, ok := requiredTargets[fromID]
		if !ok {
			continue
		}
		if _, ok := targets[toID]; !ok {
			continue
		}
		completedByUser[fromID]++
		reviewLatestAt[fromID] = maxInt64(reviewLatestAt[fromID], maxInt64(review.CreatedAt, review.UpdatedAt))
	}

	deadline := ReviewDeadlineAt(post)
	for userID, expected := range expectedByUser {
		if expected == 0 {
			continue
		}
		if completedByUser[userID] >= expected && deadline > 0 && reviewLatestAt[userID] > 0 && reviewLatestAt[userID] <= deadline {
			timelyByUser[userID] = true
		}
	}
	return expectedByUser, completedByUser, timelyByUser
}

func buildDesiredLedgers(
	post model.Post,
	settlements []settlementInfo,
	expectedByUser map[string]int,
	completedByUser map[string]int,
	timelyReviewByUser map[string]bool,
	nowMS int64,
) []ledgerSpec {
	if post.CancelledAt > 0 {
		return []ledgerSpec{{
			UserID:     post.AuthorID,
			PostID:     post.ID,
			SourceType: LedgerOrganizerCancelled,
			Delta:      creditAuthorCancel,
			Status:     "settled",
			Note:       "发起人取消了整个项目",
		}}
	}

	desired := make([]ledgerSpec, 0, len(settlements)*2+4)
	completedParticipants := 0

	for _, item := range settlements {
		switch item.FinalStatus {
		case SettlementCompleted:
			completedParticipants++
			desired = append(desired, ledgerSpec{
				UserID:     item.UserID,
				PostID:     post.ID,
				SourceType: LedgerParticipantCompleted,
				Delta:      creditCompleted,
				Status:     "settled",
				Note:       "参与者确认到场并完成活动",
			})
		case SettlementCancelled:
			desired = append(desired, ledgerSpec{
				UserID:     item.UserID,
				PostID:     post.ID,
				SourceType: LedgerParticipantCancelled,
				Delta:      creditCancelled,
				Status:     "settled",
				Note:       "参与者取消了这次活动",
			})
		case SettlementNoShow:
			desired = append(desired, ledgerSpec{
				UserID:     item.UserID,
				PostID:     post.ID,
				SourceType: LedgerParticipantNoShow,
				Delta:      creditNoShow,
				Status:     "settled",
				Note:       "参与者未到场",
			})
		}
	}

	if completedParticipants > 0 {
		desired = append(desired, ledgerSpec{
			UserID:     post.AuthorID,
			PostID:     post.ID,
			SourceType: LedgerOrganizerCompleted,
			Delta:      creditAuthorSuccess,
			Status:     "settled",
			Note:       "发起人顺利完成活动并有参与者到场",
		})
	}

	deadline := ReviewDeadlineAt(post)
	for userID, expected := range expectedByUser {
		if expected == 0 {
			continue
		}
		completed := completedByUser[userID]
		if timelyReviewByUser[userID] {
			desired = append(desired, ledgerSpec{
				UserID:     userID,
				PostID:     post.ID,
				SourceType: LedgerReviewCompleted,
				Delta:      creditReviewInTime,
				Status:     "settled",
				Note:       "在 48 小时内完成了应评评价",
			})
			continue
		}
		if deadline > 0 && nowMS > deadline && completed < expected {
			desired = append(desired, ledgerSpec{
				UserID:     userID,
				PostID:     post.ID,
				SourceType: LedgerReviewMissed,
				Delta:      creditReviewMiss,
				Status:     "settled",
				Note:       "超过 48 小时仍未完成应评评价",
			})
		}
	}
	return desired
}

func reconcileCreditLedgers(tx *gorm.DB, postID string, desired []ledgerSpec, nowMS int64) error {
	managedTypes := []string{
		LedgerParticipantCompleted,
		LedgerOrganizerCompleted,
		LedgerParticipantCancelled,
		LedgerParticipantNoShow,
		LedgerOrganizerCancelled,
		LedgerReviewCompleted,
		LedgerReviewMissed,
	}
	// Remove only the managed rows that are no longer desired, then upsert the
	// rest. Recreating every row on each recalculation would reset created_at,
	// which destroys the audit trail and keeps sliding the 180-day credit
	// window forward — and recalculation runs far more often than settlements
	// actually change.
	keep := make([]string, 0, len(desired))
	for _, item := range desired {
		keep = append(keep, item.UserID+"\x00"+item.SourceType)
	}
	stale := tx.Where("post_id = ? AND source_type IN ?", postID, managedTypes)
	if len(keep) > 0 {
		stale = stale.Where("(user_id || ? || source_type) NOT IN ?", "\x00", keep)
	}
	if err := stale.Delete(&model.CreditLedger{}).Error; err != nil {
		return err
	}
	if len(desired) == 0 {
		return nil
	}
	rows := make([]model.CreditLedger, 0, len(desired))
	for _, item := range desired {
		rows = append(rows, model.CreditLedger{
			UserID:     item.UserID,
			PostID:     item.PostID,
			SourceType: item.SourceType,
			Delta:      item.Delta,
			Status:     item.Status,
			Note:       item.Note,
			CreatedAt:  nowMS,
			UpdatedAt:  nowMS,
		})
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "post_id"}, {Name: "source_type"}},
		DoUpdates: clause.Assignments(map[string]any{
			"delta":      gorm.Expr("excluded.delta"),
			"status":     gorm.Expr("excluded.status"),
			"note":       gorm.Expr("excluded.note"),
			"updated_at": gorm.Expr("excluded.updated_at"),
		}),
	}).Create(&rows).Error
}

func loadLedgerTotalsByUser(tx *gorm.DB, postID string) (map[string]int, error) {
	var rows []model.CreditLedger
	if err := tx.Where("post_id = ? AND status = ?", postID, "settled").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.UserID] += row.Delta
	}
	return result, nil
}

func buildReceivedRatings(reviews []model.Review) (map[string]int, map[string]int) {
	total := map[string]int{}
	count := map[string]int{}
	for _, review := range reviews {
		toID := strings.TrimSpace(review.ToUserID)
		if toID == "" {
			continue
		}
		total[toID] += review.Rating
		count[toID]++
	}
	return total, count
}

func fulfillmentStatusForMember(post model.Post, item member, settlementByUser map[string]settlementInfo, settlements []settlementInfo) string {
	if post.CancelledAt > 0 {
		return SettlementCancelled
	}
	if item.Role == RoleParticipant {
		if settlement, ok := settlementByUser[item.UserID]; ok {
			return settlement.FinalStatus
		}
		return SettlementPending
	}

	if len(settlements) == 0 {
		if post.Status == "closed" {
			return SettlementCompleted
		}
		return SettlementPending
	}
	hasPending := false
	hasDispute := false
	hasCompleted := false
	hasNoShow := false
	allCancelled := true
	for _, item := range settlements {
		switch item.FinalStatus {
		case SettlementPending:
			hasPending = true
			allCancelled = false
		case SettlementDisputed:
			hasDispute = true
			allCancelled = false
		case SettlementCompleted:
			hasCompleted = true
			allCancelled = false
		case SettlementNoShow:
			hasNoShow = true
			allCancelled = false
		case SettlementCancelled:
		default:
			allCancelled = false
		}
	}
	switch {
	case hasDispute:
		return SettlementDisputed
	case hasCompleted:
		return SettlementCompleted
	case hasPending:
		return SettlementPending
	case hasNoShow:
		return SettlementNoShow
	case allCancelled:
		return SettlementCancelled
	default:
		return SettlementPending
	}
}

func reconcileSettlementCases(tx *gorm.DB, post model.Post, settlements []settlementInfo, nowMS int64) error {
	for _, item := range settlements {
		sourceRef := settlementCaseSource(post.ID, item.UserID)
		if item.FinalStatus == SettlementDisputed {
			summary := fmt.Sprintf("活动《%s》的履约结果出现冲突，等待管理员介入处理。", post.Title)
			row := model.AdminCase{
				ID:             settlementCaseID(post.ID, item.UserID),
				CaseType:       AdminCaseSettlementDispute,
				PostID:         post.ID,
				TargetUserID:   item.UserID,
				ReporterUserID: post.AuthorID,
				Status:         "open",
				SourceRef:      sourceRef,
				Summary:        summary,
				CreatedAt:      nowMS,
				UpdatedAt:      nowMS,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "source_ref"}},
				DoUpdates: clause.Assignments(map[string]any{
					"status":     row.Status,
					"summary":    row.Summary,
					"updated_at": row.UpdatedAt,
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Model(&model.AdminCase{}).
			Where("source_ref = ? AND status = ?", sourceRef, "open").
			Updates(map[string]any{
				"status":          "resolved",
				"resolution":      "auto_closed",
				"resolution_note": "结算重新计算后已不再存在争议",
				"resolved_at":     nowMS,
				"updated_at":      nowMS,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ReviewDeadlineAt(post model.Post) int64 {
	if post.ClosedAt <= 0 || post.CancelledAt > 0 {
		return 0
	}
	return post.ClosedAt + reviewWindowMS
}

func NormalizedParticipantStatus(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case ParticipantStatusCancelled:
		return ParticipantStatusCancelled
	default:
		return ParticipantStatusActive
	}
}

func IsParticipantDecision(raw string) bool {
	switch strings.TrimSpace(raw) {
	case DecisionCompleted, DecisionCancelled, DecisionNoShow, DecisionDisputed:
		return true
	default:
		return false
	}
}

func IsAuthorDecision(raw string) bool {
	switch strings.TrimSpace(raw) {
	case DecisionCompleted, DecisionNoShow:
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func clampCredit(value int) int {
	if value < minCreditScore {
		return minCreditScore
	}
	if value > maxCreditScore {
		return maxCreditScore
	}
	return value
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func settlementCaseID(postID, userID string) string {
	return fmt.Sprintf("case_%s_%s", sanitizeKey(postID), sanitizeKey(userID))
}

func settlementCaseSource(postID, userID string) string {
	return "settlement:" + sanitizeKey(postID) + ":" + sanitizeKey(userID)
}

func sanitizeKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
