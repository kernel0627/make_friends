package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

type createCaseReq struct {
	Description string `json:"description"`
	Evidence    any    `json:"evidence"`
}

type caseContext struct {
	Case          model.AdminCase                   `json:"case"`
	Post          *model.Post                       `json:"post,omitempty"`
	Messages      []model.ChatMessage               `json:"messages"`
	Participants  []model.PostParticipant           `json:"participants"`
	Settlements   []model.PostParticipantSettlement `json:"settlements"`
	CreditLedgers []model.CreditLedger              `json:"creditLedgers"`
	Moderations   []model.ModerationRecord          `json:"moderations"`
	Events        []model.CaseEvent                 `json:"events"`
}

type caseDecisionReq struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Note     string `json:"note"`
}

func caseEvidenceJSON(value any) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func createCaseTx(tx *gorm.DB, caseType, sourceType, sourceID, postID, targetID, reporterID, description, evidence, idem string, now int64) (model.AdminCase, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	if idem == "" {
		idem = sourceType + ":" + sourceID
	}
	var existing model.AdminCase
	if err := tx.Where("source_ref = ?", idem).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}
	row := model.AdminCase{
		ID: "case_" + uuid.NewString()[:12], CaseType: caseType, PostID: postID,
		TargetUserID: targetID, ReporterUserID: reporterID, ReporterID: reporterID,
		Status: "open", SourceRef: idem, SourceType: sourceType, SourceID: sourceID,
		Summary: description, Description: description, EvidenceSnapshot: evidence,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return existing, tx.Where("source_ref = ?", idem).First(&existing).Error
		}
		return row, err
	}
	event := model.CaseEvent{CaseID: row.ID, EventType: "case_created", ActorID: reporterID, Payload: evidence, CreatedAt: now}
	if err := tx.Create(&event).Error; err != nil {
		return row, err
	}
	emitDomainEvent(tx, "case.created", "case", row.ID, reporterID, map[string]string{"caseType": caseType, "postId": postID, "targetUserId": targetID})
	return row, nil
}

func (s *Server) createUserCase(c *gin.Context, caseType, sourceType, sourceID, postID, targetID string) {
	reporterID := mustUserID(c)
	var req createCaseReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	now := time.Now().UnixMilli()
	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		idem = sourceType + ":" + sourceID + ":" + reporterID
	}
	var row model.AdminCase
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = createCaseTx(tx, caseType, sourceType, sourceID, postID, targetID, reporterID,
			strings.TrimSpace(req.Description), caseEvidenceJSON(req.Evidence), idem, now)
		return err
	})
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"case": row})
}

func (s *Server) ReportPost(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	s.createUserCase(c, model.CaseTypeContentReport, "post", postID, postID, post.AuthorID)
}

func (s *Server) ReportMessage(c *gin.Context) {
	messageID := strings.TrimSpace(c.Param("id"))
	var message model.ChatMessage
	if err := s.DB.First(&message, "id = ?", messageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	s.createUserCase(c, model.CaseTypeContentReport, "message", messageID, message.PostID, message.SenderID)
}

func (s *Server) AppealModeration(c *gin.Context) {
	recordID := strings.TrimSpace(c.Param("id"))
	var record model.ModerationRecord
	if err := s.DB.First(&record, "id = ?", recordID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "moderation record not found"})
		return
	}
	var post model.Post
	if err := s.DB.First(&post, "id = ?", record.PostID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	if mustUserID(c) != post.AuthorID && mustUserRole(c) != model.UserRoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission"})
		return
	}
	s.createUserCase(c, model.CaseTypeModerationAppeal, "moderation", record.ID, post.ID, post.AuthorID)
}

func (s *Server) AppealCreditLedger(c *gin.Context) {
	ledgerID := strings.TrimSpace(c.Param("id"))
	var ledger model.CreditLedger
	if err := s.DB.First(&ledger, "id = ?", ledgerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "credit ledger not found"})
		return
	}
	if mustUserID(c) != ledger.UserID && mustUserRole(c) != model.UserRoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission"})
		return
	}
	s.createUserCase(c, model.CaseTypeCreditAppeal, "credit_ledger", ledgerID, ledger.PostID, ledger.UserID)
}

func applyCreditAppealDecisionTx(tx *gorm.DB, item model.AdminCase, decision, reason string, adminID string, now int64) error {
	if strings.TrimSpace(item.SourceID) == "" {
		return errors.New("missing credit ledger reference")
	}
	var original model.CreditLedger
	if err := tx.First(&original, "id = ?", strings.TrimSpace(item.SourceID)).Error; err != nil {
		return err
	}
	reversal := model.CreditLedger{
		UserID:         original.UserID,
		PostID:         original.PostID,
		SourceType:     score.LedgerCreditAppealReversal,
		Delta:          0,
		Status:         "voided",
		Note:           strings.TrimSpace(reason),
		OperatorUserID: adminID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if decision == "approve" || decision == "approved" || decision == "dismiss" || decision == "dismissed" {
		reversal.Delta = -original.Delta
		reversal.Status = "settled"
		if reversal.Note == "" {
			reversal.Note = "credit appeal upheld"
		}
	} else {
		if reversal.Note == "" {
			reversal.Note = "credit appeal rejected"
		}
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "post_id"}, {Name: "source_type"}},
		DoUpdates: clause.Assignments(map[string]any{
			"delta":            gorm.Expr("excluded.delta"),
			"status":           gorm.Expr("excluded.status"),
			"note":             gorm.Expr("excluded.note"),
			"operator_user_id": gorm.Expr("excluded.operator_user_id"),
			"updated_at":       gorm.Expr("excluded.updated_at"),
		}),
	}).Create(&reversal).Error; err != nil {
		return err
	}
	return score.RecalculateUsersFromActivityScoresTx(tx, []string{original.UserID}, now)
}

func (s *Server) BuildCaseContext(caseID string) (caseContext, error) {
	var ctx caseContext
	if err := s.DB.First(&ctx.Case, "id = ?", strings.TrimSpace(caseID)).Error; err != nil {
		return ctx, err
	}
	if ctx.Case.PostID != "" {
		var post model.Post
		if err := s.DB.First(&post, "id = ?", ctx.Case.PostID).Error; err == nil {
			ctx.Post = &post
		}
		_ = s.DB.Where("post_id = ?", ctx.Case.PostID).Order("created_at ASC").Find(&ctx.Messages).Error
		_ = s.DB.Where("post_id = ?", ctx.Case.PostID).Order("joined_at ASC").Find(&ctx.Participants).Error
		_ = s.DB.Where("post_id = ?", ctx.Case.PostID).Order("updated_at ASC").Find(&ctx.Settlements).Error
		_ = s.DB.Where("post_id = ?", ctx.Case.PostID).Order("created_at ASC").Find(&ctx.CreditLedgers).Error
		_ = s.DB.Where("post_id = ?", ctx.Case.PostID).Order("created_at ASC").Find(&ctx.Moderations).Error
	}
	_ = s.DB.Where("case_id = ?", ctx.Case.ID).Order("created_at ASC").Find(&ctx.Events).Error
	return ctx, nil
}

func (s *Server) GetAdminCaseContext(c *gin.Context) {
	ctx, err := s.BuildCaseContext(c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, ctx)
}

func (s *Server) DecideAdminCase(c *gin.Context) {
	caseID := strings.TrimSpace(c.Param("id"))
	adminID := mustUserID(c)
	var req caseDecisionReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	decision := strings.TrimSpace(req.Decision)
	if decision == "" {
		decision = strings.TrimSpace(req.Note)
	}
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var item model.AdminCase
		if err := tx.First(&item, "id = ?", caseID).Error; err != nil {
			return err
		}
		if item.Status == "resolved" {
			return nil // idempotent duplicate decision
		}
		if item.CaseType == model.CaseTypeSettlementDispute {
			if decision != score.SettlementCompleted && decision != score.SettlementCancelled && decision != score.SettlementNoShow {
				return errors.New("invalid settlement decision")
			}
			updates := map[string]any{"final_status": decision, "admin_resolution": decision, "settled_at": now, "updated_at": now}
			if err := tx.Model(&model.PostParticipantSettlement{}).Where("post_id = ? AND user_id = ?", item.PostID, item.TargetUserID).Updates(updates).Error; err != nil {
				return err
			}
			var post model.Post
			if err := tx.First(&post, "id = ?", item.PostID).Error; err != nil {
				return err
			}
			if err := score.RecalculatePostActivityScoresTx(tx, post, now); err != nil {
				return err
			}
		} else if item.CaseType == model.CaseTypeModerationAppeal || item.CaseType == model.CaseTypeContentReport {
			var status string
			switch decision {
			case "approve", "approved", "dismiss", "dismissed":
				status = model.ModerationApproved
			case "reject", "rejected", "remove":
				status = model.ModerationRejected
			default:
				return errors.New("invalid moderation decision")
			}
			if item.PostID != "" {
				if err := tx.Model(&model.Post{}).Where("id = ?", item.PostID).Updates(map[string]any{
					"moderation_status": status, "moderation_updated_at": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
		} else if item.CaseType == model.CaseTypeCreditAppeal {
			if decision != "approve" && decision != "approved" && decision != "reject" && decision != "rejected" && decision != "dismiss" && decision != "dismissed" {
				return errors.New("invalid credit appeal decision")
			}
			if err := applyCreditAppealDecisionTx(tx, item, decision, req.Reason, adminID, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&model.AdminCase{}).Where("id = ? AND status <> ?", caseID, "resolved").
			Updates(map[string]any{
				"status": "resolved", "resolver_user_id": adminID, "resolution": decision,
				"resolution_note": strings.TrimSpace(req.Reason + " " + req.Note), "decision": decision,
				"decision_reason": strings.TrimSpace(req.Reason), "resolved_at": now, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		payload, _ := json.Marshal(req)
		if err := tx.Create(&model.CaseEvent{CaseID: caseID, EventType: "decision_made", ActorID: adminID, Payload: string(payload), CreatedAt: now}).Error; err != nil {
			return err
		}
		emitDomainEvent(tx, "case.decided", "case", caseID, adminID, map[string]string{"decision": decision})
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		fail(c, http.StatusBadRequest, "DECIDE_CASE_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "caseId": caseID})
}

func (s *Server) ReopenAdminCase(c *gin.Context) {
	caseID := strings.TrimSpace(c.Param("id"))
	adminID := mustUserID(c)
	now := time.Now().UnixMilli()
	result := s.DB.Model(&model.AdminCase{}).Where("id = ? AND status = ?", caseID, "resolved").
		Updates(map[string]any{"status": "open", "resolver_user_id": "", "resolved_at": 0, "updated_at": now})
	if result.Error != nil {
		serverError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		var count int64
		_ = s.DB.Model(&model.AdminCase{}).Where("id = ?", caseID).Count(&count).Error
		if count == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
	}
	_ = s.DB.Create(&model.CaseEvent{CaseID: caseID, EventType: "case_reopened", ActorID: adminID, Payload: "{}", CreatedAt: now}).Error
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Keep the generic context endpoint independently usable by future Agent
// integrations without exposing unrestricted database access.
func (s *Server) CaseContextJSON(caseID string) ([]byte, error) {
	ctx, err := s.BuildCaseContext(caseID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(ctx)
}
