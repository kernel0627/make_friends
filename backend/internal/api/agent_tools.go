package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ---------- Agent Tool API ----------
// Read-only endpoints consumed by the Python investigation agent.
// Protected by a shared secret (AGENT_API_SECRET env var).

// RequireAgentSecret returns middleware that validates the Bearer token
// against the AGENT_API_SECRET environment variable.
func RequireAgentSecret() gin.HandlerFunc {
	secret := strings.TrimSpace(envStr("AGENT_API_SECRET", ""))
	return func(c *gin.Context) {
		if secret == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "agent API not configured"})
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != secret {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
		c.Next()
	}
}

// RegisterAgentRoutes adds /internal/agent/* endpoints to the router.
func RegisterAgentRoutes(r *gin.Engine, s *Server) {
	g := r.Group("/internal/agent")
	g.Use(RequireAgentSecret())
	{
		g.GET("/case/:id", s.agentGetCase)
		g.GET("/case/:id/context", s.agentGetCaseContext)
		g.GET("/case/:id/events", s.agentGetDomainEvents)
		g.GET("/case/:id/messages", s.agentGetChatMessages)
		g.GET("/user/:id/profile", s.agentGetUserProfile)
		g.GET("/user/:id/history", s.agentGetUserHistory)
		g.POST("/run", s.agentCreateRun)
		g.PATCH("/run/:id", s.agentUpdateRun)
		g.POST("/run/:id/step", s.agentCreateStep)
	}
}

// --- Read-only tools ---

func (s *Server) agentGetCase(c *gin.Context) {
	var item model.AdminCase
	if err := s.DB.First(&item, "id = ?", c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) agentGetCaseContext(c *gin.Context) {
	ctx, err := s.BuildCaseContext(c.Param("id"))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, ctx)
}

func (s *Server) agentGetDomainEvents(c *gin.Context) {
	caseID := c.Param("id")
	var item model.AdminCase
	if err := s.DB.First(&item, "id = ?", caseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}

	// Gather domain events related to this case's post and target user.
	var events []model.DomainEvent
	q := s.DB.Order("created_at ASC")
	if item.PostID != "" {
		q = q.Where("(aggregate_type = 'post' AND aggregate_id = ?) OR (aggregate_type = 'case' AND aggregate_id = ?)", item.PostID, caseID)
	} else {
		q = q.Where("aggregate_type = 'case' AND aggregate_id = ?", caseID)
	}
	if err := q.Find(&events).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func (s *Server) agentGetChatMessages(c *gin.Context) {
	caseID := c.Param("id")
	var item model.AdminCase
	if err := s.DB.First(&item, "id = ?", caseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}
	if item.PostID == "" {
		c.JSON(http.StatusOK, gin.H{"messages": []any{}})
		return
	}
	var messages []model.ChatMessage
	_ = s.DB.Where("post_id = ?", item.PostID).Order("created_at ASC").Find(&messages).Error
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (s *Server) agentGetUserProfile(c *gin.Context) {
	uid := c.Param("id")
	var user model.User
	if err := s.DB.First(&user, "id = ?", uid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		serverError(c, err)
		return
	}
	// Return a safe subset — no password hashes or tokens.
	c.JSON(http.StatusOK, gin.H{
		"id": user.ID, "nickname": user.Nickname, "avatarURL": user.AvatarURL,
		"role": user.Role, "creditScore": user.CreditScore,
		"ratingScore": user.RatingScore, "createdAt": user.CreatedAt,
	})
}

func (s *Server) agentGetUserHistory(c *gin.Context) {
	uid := c.Param("id")
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	// Recent posts authored
	var posts []model.Post
	_ = s.DB.Where("author_id = ?", uid).Order("created_at DESC").Limit(limit).Find(&posts).Error

	// Recent participations
	var participations []model.PostParticipant
	_ = s.DB.Where("user_id = ?", uid).Order("joined_at DESC").Limit(limit).Find(&participations).Error

	// Cases involving this user
	var cases []model.AdminCase
	_ = s.DB.Where("target_user_id = ? OR reporter_user_id = ?", uid, uid).
		Order("created_at DESC").Limit(limit).Find(&cases).Error

	c.JSON(http.StatusOK, gin.H{
		"posts":          posts,
		"participations": participations,
		"cases":          cases,
	})
}

// --- Write endpoints (agent run tracking) ---

type createRunReq struct {
	CaseID string `json:"caseId" binding:"required"`
	Model  string `json:"model"`
}

func (s *Server) agentCreateRun(c *gin.Context) {
	var req createRunReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UnixMilli()
	run := model.AgentRun{
		ID:        "run_" + uuid.NewString()[:12],
		CaseID:    req.CaseID,
		Status:    model.AgentRunPending,
		Model:     req.Model,
		CreatedAt: now,
	}
	if err := s.DB.Create(&run).Error; err != nil {
		serverError(c, err)
		return
	}
	// Link run to case
	_ = s.DB.Model(&model.AdminCase{}).Where("id = ?", req.CaseID).
		Update("agent_run_id", run.ID).Error
	c.JSON(http.StatusCreated, run)
}

type updateRunReq struct {
	Status      string `json:"status"`
	Report      string `json:"report"`
	ErrorMsg    string `json:"errorMsg"`
	TokensUsed  int    `json:"tokensUsed"`
	StepCount   int    `json:"stepCount"`
}

func (s *Server) agentUpdateRun(c *gin.Context) {
	runID := c.Param("id")
	var req updateRunReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UnixMilli()
	updates := map[string]any{}
	if req.Status != "" {
		updates["status"] = req.Status
		if req.Status == model.AgentRunRunning {
			updates["started_at"] = now
		}
		if req.Status == model.AgentRunCompleted || req.Status == model.AgentRunFailed {
			updates["completed_at"] = now
		}
	}
	if req.Report != "" {
		updates["report"] = req.Report
	}
	if req.ErrorMsg != "" {
		updates["error_msg"] = req.ErrorMsg
	}
	if req.TokensUsed > 0 {
		updates["tokens_used"] = req.TokensUsed
	}
	if req.StepCount > 0 {
		updates["step_count"] = req.StepCount
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}
	if err := s.DB.Model(&model.AgentRun{}).Where("id = ?", runID).Updates(updates).Error; err != nil {
		serverError(c, err)
		return
	}
	var run model.AgentRun
	_ = s.DB.First(&run, "id = ?", runID).Error
	c.JSON(http.StatusOK, run)
}

type createStepReq struct {
	StepIndex  int    `json:"stepIndex"`
	Action     string `json:"action" binding:"required"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	Reasoning  string `json:"reasoning"`
	LatencyMs  int    `json:"latencyMs"`
	TokensUsed int    `json:"tokensUsed"`
}

func (s *Server) agentCreateStep(c *gin.Context) {
	runID := c.Param("id")
	var req createStepReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UnixMilli()
	step := model.AgentStep{
		RunID:      runID,
		StepIndex:  req.StepIndex,
		Action:     req.Action,
		Input:      req.Input,
		Output:     req.Output,
		Reasoning:  req.Reasoning,
		LatencyMs:  req.LatencyMs,
		TokensUsed: req.TokensUsed,
		CreatedAt:  now,
	}
	if err := s.DB.Create(&step).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, step)
}
