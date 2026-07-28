package api

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

// ---------- Action Execution Helpers ----------
// These functions execute remediation actions. They are called by the admin
// approval endpoint (admin_approve.go) after an admin approves a proposed decision.
// The agent itself cannot execute actions directly (safety contract).

// agentExecuteActionReq is the internal parameter struct for action execution helpers.
type agentExecuteActionReq struct {
	Action   string // credit_deduct, credit_restore, post_restore, post_takedown
	TargetID string // user_id for credit actions, post_id for post actions
	Amount   int    // for credit actions
	Reason   string
	RunID    string
}

func (s *Server) executeCredtDeduct(adminCase model.AdminCase, req agentExecuteActionReq, operatorID string, now int64) error {
	targetID := req.TargetID
	if targetID == "" {
		targetID = adminCase.TargetUserID
	}
	if targetID == "" {
		return fmt.Errorf("no target user for credit deduction")
	}
	amount := req.Amount
	if amount == 0 {
		amount = -5 // default penalty
	}
	if amount > 0 {
		amount = -amount // ensure it's a deduction
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, "id = ?", targetID).Error; err != nil {
			return err
		}
		row := model.CreditLedger{
			UserID:         targetID,
			PostID:         adminCase.PostID,
			SourceType:     "agent_penalty",
			Delta:          amount,
			Status:         "settled",
			Note:           strings.TrimSpace(req.Reason),
			OperatorUserID: operatorID,
			CaseID:         adminCase.ID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return score.RecalculateUsersFromActivityScoresTx(tx, []string{targetID}, now)
	})
}

func (s *Server) executeCreditRestore(adminCase model.AdminCase, req agentExecuteActionReq, operatorID string, now int64) error {
	targetID := req.TargetID
	if targetID == "" {
		targetID = adminCase.TargetUserID
	}
	if targetID == "" {
		return fmt.Errorf("no target user for credit restore")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		// Find the original penalty ledger entry for this case
		var originalPenalty model.CreditLedger
		q := tx.Where("user_id = ? AND delta < 0", targetID)
		if adminCase.PostID != "" {
			q = q.Where("post_id = ?", adminCase.PostID)
		}
		if err := q.Order("created_at DESC").First(&originalPenalty).Error; err != nil {
			// No penalty found to reverse — just add positive credit
			row := model.CreditLedger{
				UserID:         targetID,
				PostID:         adminCase.PostID,
				SourceType:     score.LedgerCreditAppealReversal,
				Delta:          abs(req.Amount),
				Status:         "settled",
				Note:           strings.TrimSpace(req.Reason),
				OperatorUserID: operatorID,
				CaseID:         adminCase.ID,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if row.Delta == 0 {
				row.Delta = 5 // default restore
			}
			if err2 := tx.Create(&row).Error; err2 != nil {
				return err2
			}
			return score.RecalculateUsersFromActivityScoresTx(tx, []string{targetID}, now)
		}

		// Create a reversal entry
		restoreAmount := -originalPenalty.Delta // Reverse the exact amount
		if req.Amount > 0 {
			restoreAmount = req.Amount
		}
		row := model.CreditLedger{
			UserID:         targetID,
			PostID:         adminCase.PostID,
			SourceType:     score.LedgerCreditAppealReversal,
			Delta:          restoreAmount,
			Status:         "settled",
			Note:           strings.TrimSpace(req.Reason),
			OperatorUserID: operatorID,
			ReversalOfID:   originalPenalty.ID,
			CaseID:         adminCase.ID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return score.RecalculateUsersFromActivityScoresTx(tx, []string{targetID}, now)
	})
}

func (s *Server) executePostRestore(adminCase model.AdminCase, req agentExecuteActionReq, operatorID string, now int64) error {
	postID := req.TargetID
	if postID == "" {
		postID = adminCase.PostID
	}
	if postID == "" {
		return fmt.Errorf("no post to restore")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		// Set moderation_status back to approved
		result := tx.Model(&model.Post{}).Where("id = ?", postID).Updates(map[string]any{
			"moderation_status": model.ModerationApproved,
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("post not found: %s", postID)
		}
		// Also update the moderation record if one exists
		tx.Model(&model.ModerationRecord{}).
			Where("post_id = ? AND decision = ?", postID, model.ModerationRejected).
			Order("created_at DESC").Limit(1).
			Updates(map[string]any{
				"decision":   model.ModerationApproved,
				"updated_at": now,
			})
		return nil
	})
}

func (s *Server) executePostTakedown(adminCase model.AdminCase, req agentExecuteActionReq, operatorID string, now int64) error {
	postID := req.TargetID
	if postID == "" {
		postID = adminCase.PostID
	}
	if postID == "" {
		return fmt.Errorf("no post to take down")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Post{}).Where("id = ?", postID).Updates(map[string]any{
			"moderation_status": model.ModerationRejected,
			"status":            "closed",
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("post not found: %s", postID)
		}
		return nil
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
