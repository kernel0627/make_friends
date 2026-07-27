package api

import (
	"encoding/json"
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
		// Case basics
		g.GET("/case/:id", s.agentGetCase)
		g.GET("/case/:id/context", s.agentGetCaseContext)
		g.GET("/case/:id/events", s.agentGetDomainEvents)
		g.GET("/case/:id/messages", s.agentGetChatMessages)
		// Evidence layer
		g.GET("/case/:id/reports", s.agentGetReports)
		g.GET("/case/:id/evidence", s.agentGetCaseEvidence)
		g.GET("/case/:id/decisions", s.agentGetCaseDecisions)
		g.GET("/case/:id/snapshots", s.agentGetContentSnapshots)
		g.GET("/case/:id/notifications", s.agentGetNotifications)
		g.GET("/case/:id/settlements", s.agentGetSettlements)
		g.GET("/case/:id/credit-ledger", s.agentGetCreditLedger)
		// Case lookup
		g.GET("/cases", s.agentListCases)
		// User
		g.GET("/user/:id/profile", s.agentGetUserProfile)
		g.GET("/user/:id/history", s.agentGetUserHistory)
		// Policy lookup
		g.GET("/policy/:id", s.agentGetPolicy)
		// Write (run tracking)
		g.POST("/run", s.agentCreateRun)
		g.PATCH("/run/:id", s.agentUpdateRun)
		g.POST("/run/:id/step", s.agentCreateStep)
		// Evidence write (agent can link evidence to case)
		g.POST("/case/:id/evidence", s.agentAddEvidence)
		// Decision write (agent records its verdict)
		g.POST("/case/:id/decision", s.agentCreateDecision)
		// Action execution (agent executes remediation)
		registerAgentActionRoutes(g, s)
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

func (s *Server) agentListCases(c *gin.Context) {
	// List open cases, optionally filtered by source_ref prefix.
	q := s.DB.Model(&model.AdminCase{}).Order("created_at DESC")
	if sourceRef := c.Query("source_ref"); sourceRef != "" {
		q = q.Where("source_ref = ?", sourceRef)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	var cases []model.AdminCase
	q.Limit(limit).Find(&cases)
	c.JSON(http.StatusOK, gin.H{"cases": cases})
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

// --- Evidence layer read endpoints ---

func (s *Server) agentGetReports(c *gin.Context) {
	caseID := c.Param("id")
	var reports []model.Report
	_ = s.DB.Where("case_id = ?", caseID).Order("created_at ASC").Find(&reports).Error
	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

func (s *Server) agentGetCaseEvidence(c *gin.Context) {
	caseID := c.Param("id")
	var evidence []model.CaseEvidence
	_ = s.DB.Where("case_id = ?", caseID).Order("created_at ASC").Find(&evidence).Error
	c.JSON(http.StatusOK, gin.H{"evidence": evidence})
}

func (s *Server) agentGetCaseDecisions(c *gin.Context) {
	caseID := c.Param("id")
	var decisions []model.CaseDecision
	_ = s.DB.Where("case_id = ?", caseID).Order("created_at ASC").Find(&decisions).Error
	c.JSON(http.StatusOK, gin.H{"decisions": decisions})
}

func (s *Server) agentGetContentSnapshots(c *gin.Context) {
	caseID := c.Param("id")
	// Get post ID from case
	var item model.AdminCase
	if err := s.DB.First(&item, "id = ?", caseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		serverError(c, err)
		return
	}
	var snapshots []model.ContentSnapshot
	if item.PostID != "" {
		_ = s.DB.Where("post_id = ?", item.PostID).Order("snapshot_at ASC").Find(&snapshots).Error
	}
	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}

func (s *Server) agentGetNotifications(c *gin.Context) {
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
	var notifications []model.Notification
	if item.PostID != "" {
		_ = s.DB.Where("post_id = ?", item.PostID).Order("created_at ASC").Find(&notifications).Error
	}
	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

func (s *Server) agentGetSettlements(c *gin.Context) {
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
	var settlements []model.PostParticipantSettlement
	if item.PostID != "" {
		_ = s.DB.Where("post_id = ?", item.PostID).Order("created_at ASC").Find(&settlements).Error
	}
	c.JSON(http.StatusOK, gin.H{"settlements": settlements})
}

func (s *Server) agentGetCreditLedger(c *gin.Context) {
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
	var ledgers []model.CreditLedger
	if item.PostID != "" {
		_ = s.DB.Where("post_id = ?", item.PostID).Order("created_at ASC").Find(&ledgers).Error
	}
	c.JSON(http.StatusOK, gin.H{"ledgers": ledgers})
}

func (s *Server) agentGetPolicy(c *gin.Context) {
	policyID := c.Param("id")
	// Policies are YAML files under agent/policies/<id>.yaml
	// We serve them as raw text for the agent to parse.
	policyDir := envStr("AGENT_POLICY_DIR", "")
	if policyDir == "" {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "AGENT_POLICY_DIR not configured"})
		return
	}
	path := policyDir + "/" + policyID + ".yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "policy not found", "id": policyID})
			return
		}
		serverError(c, err)
		return
	}
	c.Data(http.StatusOK, "text/yaml", data)
}

// --- Evidence write (agent links evidence to case) ---

type addEvidenceReq struct {
	EvidenceType string `json:"evidenceType" binding:"required"`
	EvidenceID   string `json:"evidenceId" binding:"required"`
	Relevance    string `json:"relevance"`
	Note         string `json:"note"`
	RunID        string `json:"runId"`
}

func (s *Server) agentAddEvidence(c *gin.Context) {
	caseID := c.Param("id")
	var req addEvidenceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	relevance := req.Relevance
	if relevance == "" {
		relevance = "supporting"
	}
	addedBy := "agent"
	if req.RunID != "" {
		addedBy = "agent:" + req.RunID
	}
	now := time.Now().UnixMilli()
	evidence := model.CaseEvidence{
		CaseID:       caseID,
		EvidenceType: req.EvidenceType,
		EvidenceID:   req.EvidenceID,
		AddedBy:      addedBy,
		Relevance:    relevance,
		Note:         req.Note,
		CreatedAt:    now,
	}
	if err := s.DB.Create(&evidence).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, evidence)
}

// --- Decision write (agent records verdict) ---

type createDecisionReq struct {
	Outcome      string   `json:"outcome" binding:"required"`
	Reasoning    string   `json:"reasoning"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Actions      []string `json:"actions"`
	RunID        string   `json:"runId"`
}

func (s *Server) agentCreateDecision(c *gin.Context) {
	caseID := c.Param("id")
	var req createDecisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	deciderID := "agent"
	if req.RunID != "" {
		deciderID = "agent:" + req.RunID
	}

	evidenceJSON, _ := json.Marshal(req.EvidenceRefs)
	actionsJSON, _ := json.Marshal(req.Actions)

	now := time.Now().UnixMilli()
	decision := model.CaseDecision{
		CaseID:       caseID,
		DeciderID:    deciderID,
		DecisionType: "initial",
		Outcome:      req.Outcome,
		Reasoning:    req.Reasoning,
		EvidenceRefs: string(evidenceJSON),
		Actions:      string(actionsJSON),
		CreatedAt:    now,
	}
	if err := s.DB.Create(&decision).Error; err != nil {
		serverError(c, err)
		return
	}

	// Update case status based on outcome
	caseUpdates := map[string]any{
		"status":     "resolved",
		"decision":   req.Outcome,
		"resolved_at": now,
		"updated_at": now,
	}
	if req.Reasoning != "" {
		caseUpdates["decision_reason"] = req.Reasoning
	}
	s.DB.Model(&model.AdminCase{}).Where("id = ?", caseID).Updates(caseUpdates)

	c.JSON(http.StatusCreated, decision)
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
	Status     string `json:"status"`
	Model      string `json:"model"`
	Report     string `json:"report"`
	ErrorMsg   string `json:"errorMsg"`
	TokensUsed int    `json:"tokensUsed"`
	StepCount  int    `json:"stepCount"`
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
	if req.Model != "" {
		updates["model"] = req.Model
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
