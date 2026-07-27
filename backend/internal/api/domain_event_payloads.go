package api

// Typed payloads for domain events.
// These structs define what goes into DomainEvent.Payload for each event type.

// PostCreatedPayload — emitted when a post is created.
type PostCreatedPayload struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Address  string `json:"address,omitempty"`
}

// PostUpdatedPayload — emitted when a post is updated.
// Records which fields changed and their old/new values.
type PostUpdatedPayload struct {
	ChangedFields []string       `json:"changedFields"`
	Old           map[string]any `json:"old,omitempty"`
	New           map[string]any `json:"new,omitempty"`
	Context       map[string]any `json:"context,omitempty"` // e.g. {"minutesBeforeStart": 35}
}

// PostClosedPayload — emitted when a post is closed.
type PostClosedPayload struct {
	ParticipantCount int `json:"participantCount"`
}

// ParticipantJoinedPayload — emitted when a user joins a post.
type ParticipantJoinedPayload struct {
	CurrentCount int `json:"currentCount"`
}

// ParticipantCancelledPayload — emitted when a participant cancels.
type ParticipantCancelledPayload struct {
	MinutesBeforeStart int `json:"minutesBeforeStart,omitempty"` // -1 if unknown
}

// PostCancelledAllPayload — emitted when organizer cancels the entire activity.
type PostCancelledAllPayload struct {
	Reason string `json:"reason,omitempty"`
}

// CaseCreatedPayload — emitted when a case is opened.
type CaseCreatedPayload struct {
	CaseType     string `json:"caseType"`
	PostID       string `json:"postId,omitempty"`
	TargetUserID string `json:"targetUserId"`
}

// CaseDecidedPayload — emitted when a case is decided.
type CaseDecidedPayload struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}
