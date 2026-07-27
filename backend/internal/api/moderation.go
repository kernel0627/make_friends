package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
)

const (
	moderationStream        = "zgbe:moderation:jobs"
	moderationEventType     = "moderation.submit"
	outboxPending           = "pending"
	outboxPublished         = "published"
	outboxFailed            = "failed"
	moderationConsumerGroup = "moderation-workers"
	outboxMaxRetry          = 5
)

type moderationJob struct {
	RecordID    string `json:"recordId"`
	PostID      string `json:"postId"`
	ContentHash string `json:"contentHash"`
}

type moderationDecision struct {
	Status     string
	Policies   []string
	Evidence   []string
	Reason     string
	Confidence float64
}

type moderationAdminDecisionReq struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func postContentHash(post model.Post) string {
	raw, _ := json.Marshal([]any{
		strings.TrimSpace(post.Title), strings.TrimSpace(post.Description),
		strings.TrimSpace(post.Category), strings.TrimSpace(post.SubCategory),
		strings.TrimSpace(post.TimeMode), post.TimeDays, strings.TrimSpace(post.FixedTime),
		strings.TrimSpace(post.Address), post.Lat, post.Lng, post.MaxCount,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func moderationRecordID(postID, hash string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(postID) + ":" + strings.TrimSpace(hash)))
	return hex.EncodeToString(sum[:])
}

// enqueuePostModerationTx creates the durable moderation record and outbox
// event in the same transaction as the post write.
func enqueuePostModerationTx(tx *gorm.DB, post *model.Post, idempotencyKey string, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	hash := postContentHash(*post)
	post.ContentHash = hash
	post.ModerationStatus = model.ModerationPending
	post.ModerationUpdatedAt = now
	recordID := moderationRecordID(post.ID, hash)
	if idempotencyKey == "" {
		idempotencyKey = "post:" + post.ID + ":" + hash
	}
	record := model.ModerationRecord{
		ID:             recordID,
		PostID:         post.ID,
		ContentHash:    hash,
		Status:         model.ModerationPending,
		Model:          "rules",
		PolicyVersion:  "v1",
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "post_id"}, {Name: "content_hash"}},
		DoUpdates: clause.Assignments(map[string]any{
			"status":          model.ModerationPending,
			"attempt_count":   0,
			"error_message":   "",
			"finished_at":     0,
			"idempotency_key": idempotencyKey,
		}),
	}).Create(&record).Error; err != nil {
		return err
	}
	var current model.ModerationRecord
	if err := tx.Where("post_id = ? AND content_hash = ?", post.ID, hash).First(&current).Error; err != nil {
		return err
	}
	post.CurrentModerationID = current.ID
	payload, _ := json.Marshal(moderationJob{RecordID: current.ID, PostID: post.ID, ContentHash: hash})
	outbox := model.OutboxEvent{
		EventType:      moderationEventType,
		AggregateID:    post.ID,
		IdempotencyKey: "moderation:" + current.ID,
		Payload:        string(payload),
		Status:         outboxPending,
		CreatedAt:      now,
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&outbox).Error
}

func (s *Server) DispatchOutboxOnce(ctx context.Context) (int, error) {
	if !s.UseRedis || s.RedisClient == nil {
		return 0, nil
	}
	var rows []model.OutboxEvent
	if err := s.DB.Where("status = ?", outboxPending).Order("id ASC").Limit(100).Find(&rows).Error; err != nil {
		return 0, err
	}
	published := 0
	for _, row := range rows {
		_, err := s.RedisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: moderationStream,
			Values: map[string]any{
				"eventId": row.ID, "eventType": row.EventType,
				"aggregateId": row.AggregateID, "payload": row.Payload,
			},
		}).Result()
		if err != nil {
			updates := map[string]any{
				"retry_count":   gorm.Expr("retry_count + 1"),
				"error_message": err.Error(),
			}
			if row.RetryCount+1 >= outboxMaxRetry {
				updates["status"] = outboxFailed
			}
			_ = s.DB.Model(&model.OutboxEvent{}).Where("id = ?", row.ID).Updates(updates).Error
			continue
		}
		now := time.Now().UnixMilli()
		if err := s.DB.Model(&model.OutboxEvent{}).Where("id = ? AND status = ?", row.ID, outboxPending).
			Updates(map[string]any{"status": outboxPublished, "published_at": now}).Error; err == nil {
			published++
		}
	}
	return published, nil
}

func (s *Server) StartModerationWorkers(ctx context.Context) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	consumer := "server-" + strconv.Itoa(os.Getpid())
	go s.runOutboxDispatcher(ctx)
	go s.runModerationConsumer(ctx, consumer)
}

func (s *Server) runOutboxDispatcher(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := s.DispatchOutboxOnce(ctx); err != nil {
			log.Printf("outbox dispatch failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) runModerationConsumer(ctx context.Context, consumer string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := s.ConsumeModerationOnce(ctx, consumer); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("moderation consume failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func moderationDecisionForPost(post model.Post) moderationDecision {
	text := strings.ToLower(strings.Join([]string{post.Title, post.Description, post.Category, post.Address}, " "))
	policies := make([]string, 0, 2)
	evidence := make([]string, 0, 2)
	for _, keyword := range []string{"诈骗", "色情", "毒品", "自杀", "暴力", "赌博", "scam", "porn"} {
		if strings.Contains(text, keyword) {
			policies = append(policies, "high_risk_"+keyword)
			evidence = append(evidence, "matched keyword: "+keyword)
		}
	}
	if len(policies) > 0 {
		return moderationDecision{
			Status: model.ModerationManualReview, Policies: policies, Evidence: evidence,
			Reason: "high-risk policy match requires manual review", Confidence: 0.99,
		}
	}
	return moderationDecision{
		Status: model.ModerationApproved, Policies: []string{}, Evidence: []string{},
		Reason: "passed automated policy checks", Confidence: 0.98,
	}
}

func (s *Server) ProcessModerationRecord(recordID string) error {
	var record model.ModerationRecord
	if err := s.DB.First(&record, "id = ?", strings.TrimSpace(recordID)).Error; err != nil {
		return err
	}
	if record.Status != model.ModerationPending && record.FinishedAt > 0 {
		return nil
	}
	var post model.Post
	if err := s.DB.First(&post, "id = ?", record.PostID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(post.ContentHash) != strings.TrimSpace(record.ContentHash) || strings.TrimSpace(post.CurrentModerationID) != strings.TrimSpace(record.ID) {
		return nil
	}
	decision := moderationDecisionForPost(post)
	policies, _ := json.Marshal(decision.Policies)
	evidence, _ := json.Marshal(decision.Evidence)
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ModerationRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status": decision.Status, "matched_policies": string(policies), "evidence": string(evidence),
			"decision_reason": decision.Reason, "confidence": decision.Confidence,
			"attempt_count": gorm.Expr("attempt_count + 1"), "finished_at": now, "error_message": "",
		}).Error; err != nil {
			return err
		}
		// The content hash guard makes late results harmless after an edit.
		return tx.Model(&model.Post{}).Where("id = ? AND content_hash = ? AND moderation_status = ? AND current_moderation_id = ?",
			post.ID, record.ContentHash, model.ModerationPending, record.ID).
			Updates(map[string]any{"moderation_status": decision.Status, "moderation_updated_at": now, "updated_at": gorm.Expr("updated_at")}).Error
	})
	return err
}

// ConsumeModerationOnce handles one Redis Streams delivery. It is deliberately
// a small primitive so production can run multiple consumers and tests can
// drive the whole flow without a long-lived goroutine.
func (s *Server) ConsumeModerationOnce(ctx context.Context, consumer string) error {
	if !s.UseRedis || s.RedisClient == nil {
		return errors.New("redis is disabled")
	}
	_ = s.RedisClient.XGroupCreateMkStream(ctx, moderationStream, moderationConsumerGroup, "0").Err()
	items, err := s.RedisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: moderationConsumerGroup, Consumer: consumer, Streams: []string{moderationStream, ">"}, Count: 1, Block: time.Millisecond,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}
	for _, stream := range items {
		for _, message := range stream.Messages {
			raw, _ := message.Values["payload"].(string)
			var job moderationJob
			if err := json.Unmarshal([]byte(raw), &job); err != nil {
				_ = s.RedisClient.XAck(ctx, moderationStream, moderationConsumerGroup, message.ID).Err()
				continue
			}
			if err := s.ProcessModerationRecord(job.RecordID); err != nil {
				return err
			}
			if err := s.RedisClient.XAck(ctx, moderationStream, moderationConsumerGroup, message.ID).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) moderationDetail(postID string) (model.Post, *model.ModerationRecord, error) {
	var post model.Post
	if err := s.DB.First(&post, "id = ?", postID).Error; err != nil {
		return post, nil, err
	}
	var record model.ModerationRecord
	if post.CurrentModerationID == "" {
		return post, nil, nil
	}
	if err := s.DB.First(&record, "id = ?", post.CurrentModerationID).Error; err != nil {
		return post, nil, err
	}
	return post, &record, nil
}

func (s *Server) GetPostModeration(c *gin.Context) {
	post, record, err := s.moderationDetail(c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		serverError(c, err)
		return
	}
	if mustUserID(c) != post.AuthorID && mustUserRole(c) != model.UserRoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"post": post, "moderation": record})
}

func (s *Server) ListAdminModerations(c *gin.Context) {
	page, pageSize := parsePageParams(c)
	query := s.DB.Model(&model.ModerationRecord{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	query = applyTimeRange(query, "created_at", c)
	var rows []model.ModerationRecord
	total, err := paginate(query.Order("created_at ASC"), page, pageSize, &rows)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "page": page, "pageSize": pageSize, "total": total})
}

func (s *Server) GetAdminModeration(c *gin.Context) {
	var row model.ModerationRecord
	if err := s.DB.First(&row, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "moderation record not found"})
			return
		}
		serverError(c, err)
		return
	}
	var post model.Post
	_ = s.DB.First(&post, "id = ?", row.PostID).Error
	c.JSON(http.StatusOK, gin.H{"moderation": row, "post": post})
}

func (s *Server) DecideAdminModeration(c *gin.Context) {
	var req moderationAdminDecisionReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	switch status {
	case model.ModerationApproved, model.ModerationNeedsRevision, model.ModerationManualReview, model.ModerationRejected:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid moderation status"})
		return
	}
	now := time.Now().UnixMilli()
	adminID := mustUserID(c)
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var record model.ModerationRecord
		if err := tx.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
			return err
		}
		if record.Status == status && record.FinishedAt > 0 {
			return nil
		}
		if err := tx.Model(&model.ModerationRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status": status, "decision_reason": strings.TrimSpace(req.Reason),
			"model": "admin", "finished_at": now, "attempt_count": gorm.Expr("attempt_count + 1"),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("id = ? AND content_hash = ?", record.PostID, record.ContentHash).
			Updates(map[string]any{"moderation_status": status, "moderation_updated_at": now}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "moderation record not found"})
			return
		}
		serverError(c, err)
		return
	}
	_ = adminID
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": status})
}

func moderationIdempotencyKey(c interface{ GetHeader(string) string }, postID, hash string) string {
	if key := strings.TrimSpace(c.GetHeader("Idempotency-Key")); key != "" {
		return key
	}
	return fmt.Sprintf("post:%s:%s", postID, hash)
}
