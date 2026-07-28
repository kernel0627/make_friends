package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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

// Redis queue key for agent investigation tasks.
const agentTaskQueue = "agent:tasks"

// AgentTask is the JSON payload pushed to the Redis queue.
type AgentTask struct {
	CaseID string `json:"caseId"`
	RunID  string `json:"runId"`
}

// InvestigateCase triggers an async agent investigation for the given case.
// POST /api/v1/admin/cases/:id/investigate
//
// Dispatch modes:
//   - Redis enabled (USE_REDIS=true): LPUSH task to agent:tasks queue
//   - Redis disabled: falls back to goroutine log warning (no execution)
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

	// Dispatch to Redis queue
	task := AgentTask{CaseID: caseID, RunID: runID}
	if err := s.enqueueAgentTask(task); err != nil {
		log.Printf("[agent] failed to enqueue task: %v (case=%s run=%s)", err, caseID, runID)
		// Mark run as failed since we can't dispatch
		s.DB.Model(&model.AgentRun{}).Where("id = ?", runID).Updates(map[string]any{
			"status":       model.AgentRunFailed,
			"error_msg":    "failed to enqueue: " + err.Error(),
			"completed_at": time.Now().UnixMilli(),
		})
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "agent queue unavailable",
			"runId": runID,
		})
		return
	}

	log.Printf("[agent] enqueued investigation case=%s run=%s", caseID, runID)
	c.JSON(http.StatusAccepted, gin.H{
		"runId":  runID,
		"caseId": caseID,
		"status": "pending",
	})
}

// enqueueAgentTask pushes a task to the Redis agent queue.
func (s *Server) enqueueAgentTask(task AgentTask) error {
	if !s.UseRedis || s.RedisClient == nil {
		return errRedisNotAvailable
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.RedisClient.LPush(ctx, agentTaskQueue, payload).Err()
}

var errRedisNotAvailable = &agentQueueError{msg: "Redis not available; enable USE_REDIS=true to use agent queue"}

type agentQueueError struct{ msg string }

func (e *agentQueueError) Error() string { return e.msg }

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
