package scenarios

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"make_friends/backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Generator converts a Scenario into concrete database records.
type Generator struct {
	db       *gorm.DB
	scenario *Scenario

	// Generated entity IDs, keyed by role ref
	userIDs map[string]string
	postID  string
	caseID  string
}

// NewGenerator creates a generator for the given scenario and DB.
func NewGenerator(db *gorm.DB, scenario *Scenario) *Generator {
	if scenario.BaseTime.IsZero() {
		scenario.BaseTime = time.Now().Add(-24 * time.Hour)
	}
	return &Generator{
		db:       db,
		scenario: scenario,
		userIDs:  make(map[string]string),
	}
}

// Generate executes the full scenario, writing all records.
// Returns the generated case ID for agent consumption.
func (g *Generator) Generate() (string, error) {
	return g.caseID, g.db.Transaction(func(tx *gorm.DB) error {
		if err := g.createUsers(tx); err != nil {
			return fmt.Errorf("create users: %w", err)
		}
		for i, evt := range g.scenario.Timeline {
			if err := g.applyEvent(tx, i, evt); err != nil {
				return fmt.Errorf("event %d (%s): %w", i, evt.Action, err)
			}
		}
		return nil
	})
}

func (g *Generator) createUsers(tx *gorm.DB) error {
	now := g.scenario.BaseTime.UnixMilli()
	for _, role := range g.scenario.Roles {
		id := uuid.NewString()
		g.userIDs[role.Ref] = id
		credit := role.CreditScore
		if credit == 0 {
			credit = 100
		}
		user := model.User{
			ID:          id,
			Platform:    "wechat",
			OpenID:      "test_" + id[:8],
			Nickname:    role.Nickname,
			Role:        "user",
			CreditScore: credit,
			RatingScore: 5.0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) applyEvent(tx *gorm.DB, idx int, evt Event) error {
	ts := g.scenario.BaseTime.Add(evt.Offset).UnixMilli()
	actorID := g.userIDs[evt.ActorRef]

	switch evt.Action {
	case ActionCreatePost:
		return g.createPost(tx, actorID, ts, evt.Data)
	case ActionUpdatePost:
		return g.updatePost(tx, actorID, ts, evt.Data)
	case ActionClosePost:
		return g.closePost(tx, ts)
	case ActionCancelPost:
		return g.cancelPost(tx, ts)
	case ActionJoinPost:
		return g.joinPost(tx, actorID, ts)
	case ActionCancelJoin:
		return g.cancelJoin(tx, actorID, ts)
	case ActionSendMessage:
		return g.sendMessage(tx, actorID, ts, evt.Data)
	case ActionSendNotification:
		return g.sendNotification(tx, actorID, ts, evt.Data)
	case ActionSubmitSettlement:
		return g.submitSettlement(tx, actorID, ts, evt.Data)
	case ActionCreditPenalty:
		return g.creditPenalty(tx, actorID, ts, evt.Data)
	case ActionCreateCase:
		return g.createCase(tx, actorID, ts, evt.Data)
	case ActionCreateReport:
		return g.createReport(tx, actorID, ts, evt.Data)
	case ActionModerationReject:
		return g.moderationReject(tx, ts, evt.Data)
	case ActionModerationAppeal:
		return g.moderationAppeal(tx, actorID, ts, evt.Data)
	default:
		return fmt.Errorf("unknown action: %s", evt.Action)
	}
}

// --- Action handlers ---

func (g *Generator) createPost(tx *gorm.DB, authorID string, ts int64, data map[string]any) error {
	g.postID = uuid.NewString()
	title := strVal(data, "title")
	desc := strVal(data, "description")
	category := strVal(data, "category")
	subCategory := strVal(data, "subCategory")
	address := strVal(data, "address")
	maxCount := intVal(data, "maxCount", 4)

	hash := contentHash(title + desc)
	post := model.Post{
		ID:               g.postID,
		AuthorID:         authorID,
		Title:            title,
		Description:      desc,
		Category:         category,
		SubCategory:      subCategory,
		TimeMode:         "flexible",
		Address:          address,
		MaxCount:         maxCount,
		CurrentCount:     0,
		Status:           "open",
		ModerationStatus: "approved",
		ContentHash:      hash,
		CreatedAt:        ts,
		UpdatedAt:        ts,
	}
	if err := tx.Create(&post).Error; err != nil {
		return err
	}
	// Emit domain event
	return g.emitEvent(tx, "post.created", "post", g.postID, authorID, ts, map[string]any{
		"title": title, "category": category,
	})
}

func (g *Generator) updatePost(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	updates := make(map[string]any)
	payload := map[string]any{"changed_fields": []string{}}
	changed := []string{}

	for _, field := range []string{"title", "description", "address", "category"} {
		if v, ok := data[field]; ok {
			updates[field] = v
			changed = append(changed, field)
		}
	}
	updates["updated_at"] = ts
	payload["changed_fields"] = changed

	if err := tx.Model(&model.Post{}).Where("id = ?", g.postID).Updates(updates).Error; err != nil {
		return err
	}
	// Create snapshot before update (for evidence)
	var post model.Post
	tx.First(&post, "id = ?", g.postID)
	snapID := uuid.NewString()
	snap := model.ContentSnapshot{
		ID:          snapID,
		PostID:      g.postID,
		Title:       post.Title,
		Description: post.Description,
		Address:     post.Address,
		Category:    post.Category,
		SubCategory: post.SubCategory,
		MaxCount:    post.MaxCount,
		ContentHash: post.ContentHash,
		SnapshotAt:  ts,
		CreatedAt:   ts,
	}
	if err := tx.Create(&snap).Error; err != nil {
		return err
	}
	return g.emitEvent(tx, "post.updated", "post", g.postID, actorID, ts, payload)
}

func (g *Generator) closePost(tx *gorm.DB, ts int64) error {
	return tx.Model(&model.Post{}).Where("id = ?", g.postID).Updates(map[string]any{
		"status": "closed", "closed_at": ts, "updated_at": ts,
	}).Error
}

func (g *Generator) cancelPost(tx *gorm.DB, ts int64) error {
	return tx.Model(&model.Post{}).Where("id = ?", g.postID).Updates(map[string]any{
		"status": "cancelled", "cancelled_at": ts, "updated_at": ts,
	}).Error
}

func (g *Generator) joinPost(tx *gorm.DB, userID string, ts int64) error {
	pp := model.PostParticipant{
		PostID:   g.postID,
		UserID:   userID,
		Status:   "active",
		JoinedAt: ts,
	}
	if err := tx.Create(&pp).Error; err != nil {
		return err
	}
	tx.Model(&model.Post{}).Where("id = ?", g.postID).
		UpdateColumn("current_count", gorm.Expr("current_count + 1"))
	return g.emitEvent(tx, "participant.joined", "post", g.postID, userID, ts, nil)
}

func (g *Generator) cancelJoin(tx *gorm.DB, userID string, ts int64) error {
	tx.Model(&model.PostParticipant{}).
		Where("post_id = ? AND user_id = ?", g.postID, userID).
		Updates(map[string]any{"status": "cancelled", "cancelled_at": ts})
	tx.Model(&model.Post{}).Where("id = ?", g.postID).
		UpdateColumn("current_count", gorm.Expr("current_count - 1"))
	return g.emitEvent(tx, "participant.cancelled", "post", g.postID, userID, ts, nil)
}

func (g *Generator) sendMessage(tx *gorm.DB, senderID string, ts int64, data map[string]any) error {
	id := uuid.NewString()
	msg := model.ChatMessage{
		ID:          id,
		PostID:      g.postID,
		SenderID:    senderID,
		Content:     strVal(data, "content"),
		ClientMsgID: id, // unique per message
		CreatedAt:   ts,
	}
	return tx.Create(&msg).Error
}

func (g *Generator) sendNotification(tx *gorm.DB, userID string, ts int64, data map[string]any) error {
	notif := model.Notification{
		ID:        uuid.NewString(),
		UserID:    userID,
		PostID:    g.postID,
		Type:      strVal(data, "type"),
		Channel:   "in_app",
		Status:    strVal(data, "status"),
		SentAt:    ts,
		CreatedAt: ts,
	}
	if notif.Status == "" {
		notif.Status = "sent"
	}
	return tx.Create(&notif).Error
}

func (g *Generator) submitSettlement(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	role := strVal(data, "role") // "participant" or "author"
	decision := strVal(data, "decision")
	note := strVal(data, "note")
	targetRef := strVal(data, "targetRef") // role ref of the other party

	targetID := actorID
	if targetRef != "" {
		targetID = g.userIDs[targetRef]
	}

	// Check if settlement record exists
	var existing model.PostParticipantSettlement
	err := tx.Where("post_id = ? AND user_id = ?", g.postID, targetID).First(&existing).Error
	if err != nil {
		// Create new settlement
		s := model.PostParticipantSettlement{
			PostID:    g.postID,
			UserID:    targetID,
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if role == "participant" {
			s.ParticipantDecision = decision
			s.ParticipantNote = note
			s.ParticipantConfirmedAt = ts
		} else {
			s.AuthorDecision = decision
			s.AuthorNote = note
			s.AuthorConfirmedAt = ts
		}
		return tx.Create(&s).Error
	}
	// Update existing
	updates := map[string]any{"updated_at": ts}
	if role == "participant" {
		updates["participant_decision"] = decision
		updates["participant_note"] = note
		updates["participant_confirmed_at"] = ts
	} else {
		updates["author_decision"] = decision
		updates["author_note"] = note
		updates["author_confirmed_at"] = ts
	}
	return tx.Model(&existing).Updates(updates).Error
}

func (g *Generator) creditPenalty(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	targetRef := strVal(data, "targetRef")
	targetID := g.userIDs[targetRef]
	delta := intVal(data, "delta", -5)
	sourceType := strVal(data, "sourceType")
	if sourceType == "" {
		sourceType = "settlement_penalty"
	}

	ledger := model.CreditLedger{
		UserID:     targetID,
		PostID:     g.postID,
		SourceType: sourceType,
		Delta:      delta,
		Status:     "settled",
		Note:       strVal(data, "note"),
		CreatedAt:  ts,
		UpdatedAt:  ts,
	}
	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}
	// Also update user credit score
	tx.Model(&model.User{}).Where("id = ?", targetID).
		UpdateColumn("credit_score", gorm.Expr("credit_score + ?", delta))
	return g.emitEvent(tx, "credit.changed", "user", targetID, actorID, ts, map[string]any{
		"delta": delta, "source_type": sourceType,
	})
}

func (g *Generator) createCase(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	g.caseID = uuid.NewString()
	caseType := strVal(data, "caseType")
	if caseType == "" {
		caseType = g.scenario.CaseType
	}
	targetRef := strVal(data, "targetRef")
	targetID := g.userIDs[targetRef]

	adminCase := model.AdminCase{
		ID:             g.caseID,
		CaseType:       caseType,
		PostID:         g.postID,
		TargetUserID:   targetID,
		ReporterUserID: actorID,
		Status:         "open",
		SourceRef:      "scenario:" + g.scenario.ID,
		Summary:        g.scenario.Summary,
		SourceType:     caseType,
		ReporterID:     actorID,
		Description:    strVal(data, "description"),
		CreatedAt:      ts,
		UpdatedAt:      ts,
	}
	if err := tx.Create(&adminCase).Error; err != nil {
		return err
	}
	return g.emitEvent(tx, "case.created", "case", g.caseID, actorID, ts, map[string]any{
		"case_type": caseType, "target_user_id": targetID,
	})
}

func (g *Generator) createReport(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	report := model.Report{
		ID:         uuid.NewString(),
		CaseID:     g.caseID,
		ReporterID: actorID,
		TargetType: strVal(data, "targetType"),
		TargetID:   strVal(data, "targetId"),
		Reason:     strVal(data, "reason"),
		Status:     "pending",
		CreatedAt:  ts,
	}
	if report.TargetType == "" {
		report.TargetType = "post"
	}
	if report.TargetID == "" {
		report.TargetID = g.postID
	}
	return tx.Create(&report).Error
}

func (g *Generator) moderationReject(tx *gorm.DB, ts int64, data map[string]any) error {
	// Reject moderation — take snapshot first
	var post model.Post
	tx.First(&post, "id = ?", g.postID)

	snapID := uuid.NewString()
	snap := model.ContentSnapshot{
		ID:          snapID,
		PostID:      g.postID,
		Title:       post.Title,
		Description: post.Description,
		Address:     post.Address,
		Category:    post.Category,
		SubCategory: post.SubCategory,
		MaxCount:    post.MaxCount,
		ContentHash: post.ContentHash,
		SnapshotAt:  ts,
		CreatedAt:   ts,
	}
	tx.Create(&snap)

	modRec := model.ModerationRecord{
		ID:              uuid.NewString(),
		PostID:          g.postID,
		SnapshotID:      snapID,
		ContentHash:     contentHash(post.ContentHash + snapID),
		Status:          "rejected",
		MatchedPolicies: strVal(data, "matchedPolicies"),
		DecisionReason:  strVal(data, "reason"),
		Confidence:      0.9,
		Model:           "rules",
		IdempotencyKey:  uuid.NewString(),
		CreatedAt:       ts,
		FinishedAt:      ts,
	}
	if modRec.MatchedPolicies == "" {
		modRec.MatchedPolicies = "[]"
	}
	if err := tx.Create(&modRec).Error; err != nil {
		return err
	}
	// Update post moderation status
	tx.Model(&model.Post{}).Where("id = ?", g.postID).Updates(map[string]any{
		"moderation_status":      "rejected",
		"current_moderation_id":  modRec.ID,
		"moderation_updated_at":  ts,
		"updated_at":             ts,
	})
	return nil
}

func (g *Generator) moderationAppeal(tx *gorm.DB, actorID string, ts int64, data map[string]any) error {
	// User appeals moderation decision — creates a case
	return g.createCase(tx, actorID, ts, map[string]any{
		"caseType":    model.CaseTypeModerationAppeal,
		"targetRef":   data["targetRef"],
		"description": strVal(data, "reason"),
	})
}

// --- Helpers ---

func (g *Generator) emitEvent(tx *gorm.DB, eventType, aggType, aggID, actorID string, ts int64, payload map[string]any) error {
	payloadJSON := "{}"
	if payload != nil {
		b, _ := json.Marshal(payload)
		payloadJSON = string(b)
	}
	evt := model.DomainEvent{
		EventType:     eventType,
		AggregateType: aggType,
		AggregateID:   aggID,
		ActorID:       actorID,
		Payload:       payloadJSON,
		CreatedAt:     ts,
	}
	return tx.Create(&evt).Error
}

func strVal(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func intVal(data map[string]any, key string, defaultVal int) int {
	if data == nil {
		return defaultVal
	}
	v, ok := data[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return defaultVal
	}
}

func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:16])
}
