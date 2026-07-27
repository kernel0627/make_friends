package api

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

// emitDomainEvent records a business event in the domain_events audit table.
// It is designed to be called inside an existing transaction so the event
// is atomically committed with the business state change.
func emitDomainEvent(tx *gorm.DB, eventType, aggregateType, aggregateID, actorID string, payload any) {
	raw := "{}"
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = string(b)
		}
	}
	now := time.Now().UnixMilli()
	_ = tx.Create(&model.DomainEvent{
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		ActorID:       actorID,
		Payload:       raw,
		CreatedAt:     now,
	}).Error
}
