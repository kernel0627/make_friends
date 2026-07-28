package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"make_friends/backend/internal/model"
)

// ---------- Auto-Approve ----------
// Periodically scans proposed decisions. If a decision meets the auto-approve
// criteria (high confidence + low-risk actions), it is approved and executed
// automatically without admin intervention.
//
// Config (env):
//   AUTO_APPROVE_ENABLED   — "true" to enable (default "false")
//   AUTO_APPROVE_INTERVAL  — poll interval in seconds (default 120)
//   AUTO_APPROVE_THRESHOLD — min confidence to auto-approve (default 0.85)
//   AUTO_APPROVE_MAX_DEDUCT — max credit deduction that can be auto-approved (default 10)
//   AUTO_APPROVE_DELAY     — seconds a decision must sit before auto-approve (default 300)

const (
	defaultAutoApproveInterval  = 120   // seconds
	defaultAutoApproveThreshold = 0.85
	defaultAutoApproveMaxDeduct = 10
	defaultAutoApproveDelay     = 300   // 5 minutes grace period
)

type autoApproveConfig struct {
	Enabled   bool
	Interval  time.Duration
	Threshold float64
	MaxDeduct int
	Delay     time.Duration
}

func loadAutoApproveConfig() autoApproveConfig {
	cfg := autoApproveConfig{
		Enabled:   os.Getenv("AUTO_APPROVE_ENABLED") == "true",
		Interval:  time.Duration(defaultAutoApproveInterval) * time.Second,
		Threshold: defaultAutoApproveThreshold,
		MaxDeduct: defaultAutoApproveMaxDeduct,
		Delay:     time.Duration(defaultAutoApproveDelay) * time.Second,
	}

	if v, err := strconv.Atoi(os.Getenv("AUTO_APPROVE_INTERVAL")); err == nil && v > 0 {
		cfg.Interval = time.Duration(v) * time.Second
	}
	if v, err := strconv.ParseFloat(os.Getenv("AUTO_APPROVE_THRESHOLD"), 64); err == nil && v > 0 {
		cfg.Threshold = v
	}
	if v, err := strconv.Atoi(os.Getenv("AUTO_APPROVE_MAX_DEDUCT")); err == nil && v >= 0 {
		cfg.MaxDeduct = v
	}
	if v, err := strconv.Atoi(os.Getenv("AUTO_APPROVE_DELAY")); err == nil && v >= 0 {
		cfg.Delay = time.Duration(v) * time.Second
	}

	return cfg
}

// StartAutoApproveLoop launches the background auto-approve goroutine.
// Call this from server startup. It's a no-op if AUTO_APPROVE_ENABLED != "true".
func (s *Server) StartAutoApproveLoop() {
	cfg := loadAutoApproveConfig()
	if !cfg.Enabled {
		log.Printf("[auto-approve] disabled (set AUTO_APPROVE_ENABLED=true to enable)")
		return
	}
	log.Printf("[auto-approve] enabled: interval=%v threshold=%.2f maxDeduct=%d delay=%v",
		cfg.Interval, cfg.Threshold, cfg.MaxDeduct, cfg.Delay)

	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for range ticker.C {
			s.runAutoApprove(cfg)
		}
	}()
}

func (s *Server) runAutoApprove(cfg autoApproveConfig) {
	now := time.Now().UnixMilli()
	cutoff := now - cfg.Delay.Milliseconds()

	// Find proposed decisions that are old enough
	var decisions []model.CaseDecision
	err := s.DB.Where("status = ? AND created_at <= ?", model.DecisionStatusProposed, cutoff).
		Find(&decisions).Error
	if err != nil {
		log.Printf("[auto-approve] query error: %v", err)
		return
	}

	for _, d := range decisions {
		if eligible, reason := isAutoApproveEligible(d, cfg); eligible {
			s.autoApproveDecision(d, now)
			log.Printf("[auto-approve] approved decision %d (case %s, confidence %.2f)",
				d.ID, d.CaseID, d.Confidence)
		} else {
			log.Printf("[auto-approve] skipped decision %d (case %s): %s",
				d.ID, d.CaseID, reason)
		}
	}
}

// isAutoApproveEligible checks whether a decision is safe to auto-approve.
func isAutoApproveEligible(d model.CaseDecision, cfg autoApproveConfig) (bool, string) {
	// 1. Confidence must meet threshold
	if d.Confidence < cfg.Threshold {
		return false, fmt.Sprintf("confidence %.2f < threshold %.2f", d.Confidence, cfg.Threshold)
	}

	// 2. Parse actions and check risk level
	var actions []map[string]any
	if err := json.Unmarshal([]byte(d.Actions), &actions); err != nil {
		return false, "cannot parse actions"
	}

	for _, act := range actions {
		action, _ := act["action"].(string)
		amount := 0.0
		if v, ok := act["amount"].(float64); ok {
			amount = v
		}

		// High-risk actions: never auto-approve
		switch action {
		case "credit_deduct":
			if int(math.Abs(amount)) > cfg.MaxDeduct {
				return false, fmt.Sprintf("credit_deduct amount %d > max %d", int(math.Abs(amount)), cfg.MaxDeduct)
			}
		case "post_takedown":
			// post takedown is moderate risk — allow only at very high confidence
			if d.Confidence < 0.90 {
				return false, "post_takedown requires confidence >= 0.90"
			}
		}
	}

	// 3. Outcome "escalate" is never auto-approved (needs human judgment)
	if d.Outcome == "escalate" {
		return false, "outcome is escalate"
	}

	return true, ""
}

func (s *Server) autoApproveDecision(d model.CaseDecision, now int64) {
	d.Status = model.DecisionStatusApproved
	d.ApprovedBy = "system:auto-approve"
	d.ApprovedAt = now
	s.DB.Save(&d)

	// Execute actions
	var actions []map[string]any
	_ = json.Unmarshal([]byte(d.Actions), &actions)

	for _, act := range actions {
		if err := s.executeProposedAction(d.CaseID, act, "system:auto-approve", now); err != nil {
			log.Printf("[auto-approve] action error (decision %d): %v", d.ID, err)
		}
	}

	// Mark executed
	d.Status = model.DecisionStatusExecuted
	s.DB.Save(&d)

	// Resolve case
	caseUpdates := map[string]any{
		"status":      "resolved",
		"decision":    d.Outcome,
		"resolved_at": now,
		"updated_at":  now,
	}
	if d.Reasoning != "" {
		caseUpdates["decision_reason"] = d.Reasoning
	}
	s.DB.Model(&model.AdminCase{}).Where("id = ?", d.CaseID).Updates(caseUpdates)

	emitDomainEvent(s.DB, "system.decision_auto_approved", "case", d.CaseID, "system:auto-approve",
		map[string]string{"decisionId": fmt.Sprintf("%d", d.ID)})
}
