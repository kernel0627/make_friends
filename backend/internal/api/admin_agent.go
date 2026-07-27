package api

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

// ---------- Admin Agent Endpoints ----------
// These endpoints let the admin frontend trigger agent investigations
// and view investigation history.

// InvestigateCase triggers an async agent investigation for the given case.
// POST /api/v1/admin/cases/:id/investigate
func (s *Server) InvestigateCase(c *gin.Context) {
	caseID := c.Param("id")

	// Validate case exists
	var adminCase model.AdminCase
	if err := s.DB.First(&adminCase, "id = ?", caseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}

	// Check if there's already a running investigation
	var runningCount int64
	s.DB.Model(&model.AgentRun{}).
		Where("case_id = ? AND status IN ?", caseID, []string{model.AgentRunPending, model.AgentRunRunning}).
		Count(&runningCount)
	if runningCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "investigation already in progress"})
		return
	}

	// Create a pending run record so we can return the run_id immediately
	runID := "run_" + uuid.NewString()[:8]
	now := time.Now().UnixMilli()
	run := model.AgentRun{
		ID:        runID,
		CaseID:    caseID,
		Status:    model.AgentRunPending,
		StartedAt: now,
		CreatedAt: now,
	}
	if err := s.DB.Create(&run).Error; err != nil {
		serverError(c, err)
		return
	}

	// Update the case to reference this run
	s.DB.Model(&model.AdminCase{}).Where("id = ?", caseID).
		Update("agent_run_id", runID)

	// Launch Python agent as a subprocess (async)
	go s.execAgentInvestigation(caseID, runID)

	c.JSON(http.StatusAccepted, gin.H{
		"runId":  runID,
		"caseId": caseID,
		"status": "pending",
	})
}

// execAgentInvestigation runs the Python agent CLI in a subprocess.
// It captures output and updates the run record on failure.
func (s *Server) execAgentInvestigation(caseID, runID string) {
	// The Python agent will create its own run record and manage status,
	// but we already created a placeholder. The agent's runner.py calls
	// create_run which will get a UNIQUE conflict — so we pass the run_id
	// as env so it can reuse ours.
	secret := strings.TrimSpace(envStr("AGENT_API_SECRET", ""))
	addr := strings.TrimSpace(envStr("BACKEND_ADDR", ":8080"))

	// Determine the backend URL the agent should call back to
	backendURL := fmt.Sprintf("http://localhost%s", addr)
	if !strings.Contains(addr, ":") {
		backendURL = fmt.Sprintf("http://localhost:%s", addr)
	}

	cmd := exec.Command(
		"conda", "run", "-n", "agent", "--no-banner",
		"python", "-m", "agent.src.cli", "investigate",
		"--case-id", caseID,
	)
	cmd.Env = append(cmd.Environ(),
		"AGENT_BACKEND_URL="+backendURL,
		"AGENT_API_SECRET="+secret,
		"AGENT_RUN_ID="+runID,
	)
	// Set working directory to project root (parent of backend/)
	cmd.Dir = projectRoot()

	// Capture stderr for logging
	stderr, _ := cmd.StderrPipe()

	log.Printf("[agent] starting investigation case=%s run=%s", caseID, runID)
	if err := cmd.Start(); err != nil {
		log.Printf("[agent] failed to start: %v", err)
		s.DB.Model(&model.AgentRun{}).Where("id = ?", runID).Updates(map[string]any{
			"status":       model.AgentRunFailed,
			"error_msg":    fmt.Sprintf("failed to start agent process: %v", err),
			"completed_at": time.Now().UnixMilli(),
		})
		return
	}

	// Log stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[agent:%s] %s", runID[:12], scanner.Text())
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("[agent] process exited with error: %v (run=%s)", err, runID)
		// Only mark as failed if it's still pending (agent may have updated it)
		s.DB.Model(&model.AgentRun{}).
			Where("id = ? AND status IN ?", runID, []string{model.AgentRunPending}).
			Updates(map[string]any{
				"status":       model.AgentRunFailed,
				"error_msg":    fmt.Sprintf("agent process exited: %v", err),
				"completed_at": time.Now().UnixMilli(),
			})
		return
	}
	log.Printf("[agent] investigation completed case=%s run=%s", caseID, runID)
}

// projectRoot returns the project root directory (parent of backend/).
func projectRoot() string {
	// This binary runs from backend/, so go up one level
	return envStr("PROJECT_ROOT", "..")
}

// ListAgentRuns returns paginated agent runs, optionally filtered by case_id.
// GET /api/v1/admin/agent-runs
func (s *Server) ListAgentRuns(c *gin.Context) {
	page := queryIntOrDefault(c.Query("page"), 1)
	pageSize := queryIntOrDefault(c.Query("pageSize"), 20)
	if pageSize > 50 {
		pageSize = 50
	}

	query := s.DB.Model(&model.AgentRun{})

	if caseID := strings.TrimSpace(c.Query("caseId")); caseID != "" {
		query = query.Where("case_id = ?", caseID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var runs []model.AgentRun
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&runs).Error; err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"runs":     runs,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// GetAgentRun returns a single agent run with all its steps.
// GET /api/v1/admin/agent-runs/:id
func (s *Server) GetAgentRun(c *gin.Context) {
	runID := c.Param("id")

	var run model.AgentRun
	if err := s.DB.First(&run, "id = ?", runID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		serverError(c, err)
		return
	}

	var steps []model.AgentStep
	if err := s.DB.Where("run_id = ?", runID).Order("step_index ASC").Find(&steps).Error; err != nil {
		serverError(c, err)
		return
	}

	// Also fetch the decision if one exists
	var decision *model.CaseDecision
	var dec model.CaseDecision
	deciderPattern := "agent:" + runID
	if err := s.DB.First(&dec, "decider_id = ?", deciderPattern).Error; err == nil {
		decision = &dec
	}

	c.JSON(http.StatusOK, gin.H{
		"run":      run,
		"steps":    steps,
		"decision": decision,
	})
}
