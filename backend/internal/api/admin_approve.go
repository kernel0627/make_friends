package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

// ---------- Admin Decision Approval ----------
// These endpoints let admins review agent-proposed decisions and approve/reject them.
// Approved decisions trigger action execution; rejected decisions leave the case open.

type approveDecisionReq struct {
	DecisionID uint64 `json:"decisionId" binding:"required"`
	Approve    bool   `json:"approve"`                // true=approve, false=reject
	AdminID    string `json:"adminId" binding:"required"`
	Comment    string `json:"comment"`                // optional admin comment
}

// POST /admin/cases/:id/review-decision
func (s *Server) adminReviewDecision(c *gin.Context) {
	caseID := c.Param("id")
	var req approveDecisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UnixMilli()

	// Load the decision
	var decision model.CaseDecision
	if err := s.DB.First(&decision, "id = ? AND case_id = ?", req.DecisionID, caseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "decision not found"})
			return
		}
		serverError(c, err)
		return
	}

	// Only proposed decisions can be reviewed
	if decision.Status != model.DecisionStatusProposed {
		c.JSON(http.StatusConflict, gin.H{
			"error":  fmt.Sprintf("decision already in status: %s", decision.Status),
			"status": decision.Status,
		})
		return
	}

	if req.Approve {
		// Approve → execute actions → mark executed → resolve case
		decision.Status = model.DecisionStatusApproved
		decision.ApprovedBy = req.AdminID
		decision.ApprovedAt = now
		s.DB.Save(&decision)

		// Execute each proposed action
		var actions []map[string]any
		_ = json.Unmarshal([]byte(decision.Actions), &actions)

		var execErrors []string
		for _, act := range actions {
			if err := s.executeProposedAction(caseID, act, req.AdminID, now); err != nil {
				execErrors = append(execErrors, fmt.Sprintf("%v: %v", act["action"], err))
			}
		}

		// Mark executed (even with partial errors — we record what happened)
		decision.Status = model.DecisionStatusExecuted
		s.DB.Save(&decision)

		// Resolve the case
		caseUpdates := map[string]any{
			"status":      "resolved",
			"decision":    decision.Outcome,
			"resolved_at": now,
			"updated_at":  now,
		}
		if decision.Reasoning != "" {
			caseUpdates["decision_reason"] = decision.Reasoning
		}
		s.DB.Model(&model.AdminCase{}).Where("id = ?", caseID).Updates(caseUpdates)

		emitDomainEvent(s.DB, "admin.decision_approved", "case", caseID, req.AdminID,
			map[string]string{"decisionId": fmt.Sprintf("%d", decision.ID), "comment": req.Comment})

		resp := gin.H{
			"ok":         true,
			"status":     decision.Status,
			"decisionId": decision.ID,
		}
		if len(execErrors) > 0 {
			resp["actionErrors"] = execErrors
		}
		c.JSON(http.StatusOK, resp)
	} else {
		// Reject → mark rejected → reopen case for manual handling
		decision.Status = model.DecisionStatusRejected
		decision.ApprovedBy = req.AdminID
		decision.ApprovedAt = now
		s.DB.Save(&decision)

		s.DB.Model(&model.AdminCase{}).Where("id = ?", caseID).Updates(map[string]any{
			"status":     "open",
			"updated_at": now,
		})

		emitDomainEvent(s.DB, "admin.decision_rejected", "case", caseID, req.AdminID,
			map[string]string{"decisionId": fmt.Sprintf("%d", decision.ID), "comment": req.Comment})

		c.JSON(http.StatusOK, gin.H{
			"ok":         true,
			"status":     decision.Status,
			"decisionId": decision.ID,
		})
	}
}

// executeProposedAction runs a single action from the decision's Actions JSON array.
func (s *Server) executeProposedAction(caseID string, act map[string]any, adminID string, now int64) error {
	action, _ := act["action"].(string)
	targetID, _ := act["targetId"].(string)
	reason, _ := act["reason"].(string)
	amount := 0
	if v, ok := act["amount"].(float64); ok {
		amount = int(v)
	}
	duration := 0
	if v, ok := act["duration"].(float64); ok {
		duration = int(v)
	}

	var adminCase model.AdminCase
	if err := s.DB.First(&adminCase, "id = ?", caseID).Error; err != nil {
		return err
	}

	operatorID := "admin:" + adminID
	req := agentExecuteActionReq{
		Action:   action,
		TargetID: targetID,
		Amount:   amount,
		Duration: duration,
		Reason:   reason,
	}

	switch action {
	case "credit_deduct":
		return s.executeCredtDeduct(adminCase, req, operatorID, now)
	case "credit_restore":
		return s.executeCreditRestore(adminCase, req, operatorID, now)
	case "post_restore":
		return s.executePostRestore(adminCase, req, operatorID, now)
	case "post_takedown":
		return s.executePostTakedown(adminCase, req, operatorID, now)
	case "suspend_user":
		return s.executeSuspendUser(adminCase, req, operatorID, now)
	case "ban_user":
		return s.executeBanUser(adminCase, req, operatorID, now)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}