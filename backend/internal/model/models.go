package model

type User struct {
	ID           string  `gorm:"primaryKey;size:64" json:"id"`
	Platform     string  `gorm:"size:32;not null;default:wechat" json:"platform"`
	OpenID       string  `gorm:"size:128;uniqueIndex" json:"openId"`
	Nickname     string  `gorm:"size:128;not null;uniqueIndex" json:"nickName"`
	PasswordHash string  `gorm:"size:255;not null;default:''" json:"-"`
	AvatarURL    string  `gorm:"size:512;not null;default:''" json:"avatarUrl"`
	Role         string  `gorm:"size:16;not null;default:user;index" json:"role"`
	RootAdmin    bool    `gorm:"not null;default:false;index" json:"rootAdmin"`
	CreditScore  int     `gorm:"not null;default:100" json:"creditScore"`
	RatingScore  float64 `gorm:"not null;default:5.0" json:"ratingScore"`
	DeletedAt    int64   `gorm:"not null;default:0;index" json:"deletedAt"`
	DeletedBy    string  `gorm:"size:64;not null;default:''" json:"deletedBy"`
	CreatedAt    int64   `gorm:"not null" json:"createdAt"`
	UpdatedAt    int64   `gorm:"not null" json:"updatedAt"`
}

type Post struct {
	ID                  string  `gorm:"primaryKey;size:64" json:"id"`
	AuthorID            string  `gorm:"size:64;not null;index" json:"authorId"`
	Title               string  `gorm:"size:255;not null" json:"title"`
	Description         string  `gorm:"type:text;not null;default:''" json:"description"`
	Category            string  `gorm:"size:64;not null;index:idx_posts_category" json:"category"`
	SubCategory         string  `gorm:"size:64;not null;default:'';index:idx_posts_category" json:"subCategory"`
	TimeMode            string  `gorm:"size:16;not null" json:"timeMode"`
	TimeDays            int     `json:"timeDays"`
	FixedTime           string  `gorm:"size:64" json:"fixedTime"`
	Address             string  `gorm:"size:255;not null" json:"address"`
	Lat                 float64 `json:"lat"`
	Lng                 float64 `json:"lng"`
	MaxCount            int     `gorm:"not null" json:"maxCount"`
	CurrentCount        int     `gorm:"not null;default:0" json:"currentCount"`
	Status              string  `gorm:"size:16;not null;default:open" json:"status"`
	ModerationStatus    string  `gorm:"size:24;not null;default:approved;index" json:"moderationStatus"`
	CurrentModerationID string  `gorm:"size:64;not null;default:'';index" json:"currentModerationId"`
	ContentHash         string  `gorm:"size:128;not null;default:'';index" json:"contentHash"`
	ModerationUpdatedAt int64   `gorm:"not null;default:0;index" json:"moderationUpdatedAt"`
	CancelledAt         int64   `gorm:"not null;default:0;index" json:"cancelledAt"`
	DeletedAt           int64   `gorm:"not null;default:0;index" json:"deletedAt"`
	DeletedBy           string  `gorm:"size:64;not null;default:''" json:"deletedBy"`
	ClosedAt            int64   `gorm:"not null;default:0;index" json:"closedAt"`
	CreatedAt           int64   `gorm:"not null;index:idx_posts_created_at,sort:desc" json:"createdAt"`
	UpdatedAt           int64   `gorm:"not null" json:"updatedAt"`
}

type PostParticipant struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID      string `gorm:"size:64;not null;index;uniqueIndex:uq_post_user" json:"postId"`
	UserID      string `gorm:"size:64;not null;index;uniqueIndex:uq_post_user" json:"userId"`
	Status      string `gorm:"size:16;not null;default:active;index" json:"status"`
	JoinedAt    int64  `gorm:"not null" json:"joinedAt"`
	CancelledAt int64  `gorm:"not null;default:0" json:"cancelledAt"`
}

type ChatMessage struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	PostID      string `gorm:"size:64;not null;index:idx_chat_post_created" json:"postId"`
	SenderID    string `gorm:"size:64;not null" json:"senderId"`
	Content     string `gorm:"type:text;not null" json:"content"`
	ClientMsgID string `gorm:"size:128;index:uq_post_client_msg,unique" json:"clientMsgId"`
	CreatedAt   int64  `gorm:"not null;index:idx_chat_post_created,sort:desc" json:"createdAt"`
}

type PostInvitation struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	PostID      string `gorm:"size:64;not null;index;uniqueIndex:uq_post_invitation" json:"postId"`
	InviterID   string `gorm:"size:64;not null;index" json:"inviterId"`
	InviteeID   string `gorm:"size:64;not null;index;uniqueIndex:uq_post_invitation" json:"inviteeId"`
	Message     string `gorm:"type:text;not null;default:''" json:"message"`
	Status      string `gorm:"size:16;not null;default:pending;index" json:"status"`
	RespondedAt int64  `gorm:"not null;default:0" json:"respondedAt"`
	CreatedAt   int64  `gorm:"not null;index" json:"createdAt"`
	UpdatedAt   int64  `gorm:"not null" json:"updatedAt"`
}

const (
	InvitationStatusPending   = "pending"
	InvitationStatusAccepted  = "accepted"
	InvitationStatusRejected  = "rejected"
	InvitationStatusCancelled = "cancelled"
	InvitationStatusExpired   = "expired"
)

type Review struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID     string `gorm:"size:64;not null;index;uniqueIndex:uq_review_key" json:"postId"`
	FromUserID string `gorm:"size:64;not null;index;uniqueIndex:uq_review_key" json:"fromUserId"`
	ToUserID   string `gorm:"size:64;not null;index;uniqueIndex:uq_review_key" json:"toUserId"`
	Rating     int    `gorm:"not null" json:"rating"`
	Comment    string `gorm:"type:text;not null;default:''" json:"comment"`
	CreatedAt  int64  `gorm:"not null" json:"createdAt"`
	UpdatedAt  int64  `gorm:"not null" json:"updatedAt"`
}

type ActivityScore struct {
	ID                   uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID               string  `gorm:"size:64;not null;index;uniqueIndex:uq_activity_score" json:"postId"`
	UserID               string  `gorm:"size:64;not null;index;uniqueIndex:uq_activity_score" json:"userId"`
	Role                 string  `gorm:"size:16;not null" json:"role"`
	RatingScore          float64 `gorm:"not null;default:0" json:"ratingScore"`
	RatingCount          int     `gorm:"not null;default:0" json:"ratingCount"`
	CreditScore          int     `gorm:"not null;default:0" json:"creditScore"`
	ExpectedReviewCount  int     `gorm:"not null;default:0" json:"expectedReviewCount"`
	CompletedReviewCount int     `gorm:"not null;default:0" json:"completedReviewCount"`
	FulfillmentStatus    string  `gorm:"size:16;not null;default:pending" json:"fulfillmentStatus"`
	CreatedAt            int64   `gorm:"not null" json:"createdAt"`
	UpdatedAt            int64   `gorm:"not null" json:"updatedAt"`
}

type PostParticipantSettlement struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID                 string `gorm:"size:64;not null;index;uniqueIndex:uq_post_settlement" json:"postId"`
	UserID                 string `gorm:"size:64;not null;index;uniqueIndex:uq_post_settlement" json:"userId"`
	ParticipantDecision    string `gorm:"size:16;not null;default:''" json:"participantDecision"`
	AuthorDecision         string `gorm:"size:16;not null;default:''" json:"authorDecision"`
	FinalStatus            string `gorm:"size:16;not null;default:pending;index" json:"finalStatus"`
	AdminResolution        string `gorm:"size:16;not null;default:''" json:"adminResolution"`
	ParticipantNote        string `gorm:"type:text;not null;default:''" json:"participantNote"`
	AuthorNote             string `gorm:"type:text;not null;default:''" json:"authorNote"`
	ParticipantConfirmedAt int64  `gorm:"not null;default:0" json:"participantConfirmedAt"`
	AuthorConfirmedAt      int64  `gorm:"not null;default:0" json:"authorConfirmedAt"`
	SettledAt              int64  `gorm:"not null;default:0" json:"settledAt"`
	CreatedAt              int64  `gorm:"not null" json:"createdAt"`
	UpdatedAt              int64  `gorm:"not null" json:"updatedAt"`
}

type CreditLedger struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         string `gorm:"size:64;not null;index;uniqueIndex:uq_credit_ledger" json:"userId"`
	PostID         string `gorm:"size:64;not null;index;uniqueIndex:uq_credit_ledger" json:"postId"`
	SourceType     string `gorm:"size:32;not null;index;uniqueIndex:uq_credit_ledger" json:"sourceType"`
	Delta          int    `gorm:"not null;default:0" json:"delta"`
	Status         string `gorm:"size:16;not null;default:settled" json:"status"`
	Note           string `gorm:"type:text;not null;default:''" json:"note"`
	OperatorUserID string `gorm:"size:64;not null;default:'';index" json:"operatorUserId"`
	ReversalOfID   uint64 `gorm:"not null;default:0;index" json:"reversalOfId"`
	CaseID         string `gorm:"size:64;not null;default:'';index" json:"caseId"`
	IdempotencyKey string `gorm:"size:128;not null;default:''" json:"idempotencyKey"`
	CreatedAt      int64  `gorm:"not null;index" json:"createdAt"`
	UpdatedAt      int64  `gorm:"not null" json:"updatedAt"`
}

type AdminCase struct {
	ID               string `gorm:"primaryKey;size:64" json:"id"`
	CaseType         string `gorm:"size:32;not null;index" json:"caseType"`
	PostID           string `gorm:"size:64;not null;index" json:"postId"`
	TargetUserID     string `gorm:"size:64;not null;index" json:"targetUserId"`
	ReporterUserID   string `gorm:"size:64;not null;index" json:"reporterUserId"`
	ResolverUserID   string `gorm:"size:64;not null;default:'';index" json:"resolverUserId"`
	AgentRunID       string `gorm:"size:64;not null;default:''" json:"agentRunId"`
	Status           string `gorm:"size:24;not null;default:open;index" json:"status"`
	Resolution       string `gorm:"size:32;not null;default:''" json:"resolution"`
	ResolutionNote   string `gorm:"type:text;not null;default:''" json:"resolutionNote"`
	ResolvedAt       int64  `gorm:"not null;default:0;index" json:"resolvedAt"`
	SourceRef        string `gorm:"size:128;not null;uniqueIndex" json:"sourceRef"`
	Summary          string `gorm:"type:text;not null;default:''" json:"summary"`
	SourceType       string `gorm:"size:32;not null;default:'';index" json:"sourceType"`
	SourceID         string `gorm:"size:128;not null;default:'';index" json:"sourceId"`
	ReporterID       string `gorm:"size:64;not null;default:'';index" json:"reporterId"`
	Description      string `gorm:"type:text;not null;default:''" json:"description"`
	EvidenceSnapshot string `gorm:"type:text;not null;default:'{}'" json:"evidenceSnapshot"`
	Decision         string `gorm:"size:32;not null;default:''" json:"decision"`
	DecisionReason   string `gorm:"type:text;not null;default:''" json:"decisionReason"`
	CreatedAt        int64  `gorm:"not null;index" json:"createdAt"`
	UpdatedAt        int64  `gorm:"not null" json:"updatedAt"`
}

const (
	ModerationPending       = "pending"
	ModerationApproved      = "approved"
	ModerationNeedsRevision = "needs_revision"
	ModerationManualReview  = "manual_review"
	ModerationRejected      = "rejected"

	CaseTypeContentReport     = "content_report"
	CaseTypeModerationAppeal  = "moderation_appeal"
	CaseTypeSettlementDispute = "settlement_dispute"
	CaseTypeCreditAppeal      = "credit_appeal"
)

type ModerationRecord struct {
	ID              string  `gorm:"primaryKey;size:64" json:"id"`
	PostID          string  `gorm:"size:64;not null;index;uniqueIndex:uq_moderation_post_hash" json:"postId"`
	SnapshotID      string  `gorm:"size:64;not null;default:''" json:"snapshotId"`
	ContentHash     string  `gorm:"size:128;not null;uniqueIndex:uq_moderation_post_hash" json:"contentHash"`
	Status          string  `gorm:"size:24;not null;default:pending;index" json:"status"`
	MatchedPolicies string  `gorm:"type:text;not null;default:'[]'" json:"matchedPolicies"`
	Evidence        string  `gorm:"type:text;not null;default:'[]'" json:"evidence"`
	DecisionReason  string  `gorm:"type:text;not null;default:''" json:"decisionReason"`
	Confidence      float64 `gorm:"not null;default:0" json:"confidence"`
	Model           string  `gorm:"size:64;not null;default:'rules'" json:"model"`
	PolicyVersion   string  `gorm:"size:64;not null;default:'v1'" json:"policyVersion"`
	AttemptCount    int     `gorm:"not null;default:0" json:"attemptCount"`
	ErrorMessage    string  `gorm:"type:text;not null;default:''" json:"errorMessage"`
	IdempotencyKey  string  `gorm:"size:128;not null;default:'';uniqueIndex" json:"-"`
	CreatedAt       int64   `gorm:"not null;index" json:"createdAt"`
	FinishedAt      int64   `gorm:"not null;default:0" json:"finishedAt"`
}

type CaseEvent struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	CaseID    string `gorm:"size:64;not null;index" json:"caseId"`
	EventType string `gorm:"size:32;not null;index" json:"eventType"`
	ActorID   string `gorm:"size:64;not null;default:'';index" json:"actorId"`
	Payload   string `gorm:"type:text;not null;default:'{}'" json:"payload"`
	CreatedAt int64  `gorm:"not null;index" json:"createdAt"`
}

type OutboxEvent struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType      string `gorm:"size:64;not null;index" json:"eventType"`
	AggregateID    string `gorm:"size:128;not null;index" json:"aggregateId"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex" json:"-"`
	Payload        string `gorm:"type:text;not null;default:'{}'" json:"payload"`
	Status         string `gorm:"size:16;not null;default:pending;index" json:"status"`
	RetryCount     int    `gorm:"not null;default:0" json:"retryCount"`
	ErrorMessage   string `gorm:"type:text;not null;default:''" json:"errorMessage"`
	CreatedAt      int64  `gorm:"not null;index" json:"createdAt"`
	PublishedAt    int64  `gorm:"not null;default:0" json:"publishedAt"`
}

type UserTag struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string  `gorm:"size:64;not null;index;uniqueIndex:uq_user_tag" json:"userId"`
	TagType       string  `gorm:"size:32;not null;uniqueIndex:uq_user_tag" json:"tagType"`
	TagValue      string  `gorm:"size:128;not null;uniqueIndex:uq_user_tag" json:"tagValue"`
	Weight        float64 `gorm:"not null;default:0" json:"weight"`
	EvidenceCount int     `gorm:"not null;default:0" json:"evidenceCount"`
	LastEventAt   int64   `gorm:"not null;default:0" json:"lastEventAt"`
	CreatedAt     int64   `gorm:"not null" json:"createdAt"`
	UpdatedAt     int64   `gorm:"not null" json:"updatedAt"`
}

type UserBehaviorProfile struct {
	ID                       uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID                   string  `gorm:"size:64;not null;index;uniqueIndex:uq_user_behavior_profile" json:"userId"`
	ActiveWeekdaysJSON       string  `gorm:"type:text;not null;default:'[]'" json:"activeWeekdaysJson"`
	JoinWeekdaysJSON         string  `gorm:"type:text;not null;default:'[]'" json:"joinWeekdaysJson"`
	ActivePeriodsJSON        string  `gorm:"type:text;not null;default:'[]'" json:"activePeriodsJson"`
	CategoryWeightsJSON      string  `gorm:"type:text;not null;default:'{}'" json:"categoryWeightsJson"`
	SubCategoryWeightsJSON   string  `gorm:"type:text;not null;default:'{}'" json:"subCategoryWeightsJson"`
	PreferredLocationsJSON   string  `gorm:"type:text;not null;default:'[]'" json:"preferredLocationsJson"`
	InviteAcceptRate         float64 `gorm:"not null;default:0" json:"inviteAcceptRate"`
	ReliabilityScore         float64 `gorm:"not null;default:0" json:"reliabilityScore"`
	ExplorationScore         float64 `gorm:"not null;default:0" json:"explorationScore"`
	WeeklyActiveScore        float64 `gorm:"not null;default:0" json:"weeklyActiveScore"`
	WeeklyParticipationScore float64 `gorm:"not null;default:0" json:"weeklyParticipationScore"`
	CreatedAt                int64   `gorm:"not null" json:"createdAt"`
	UpdatedAt                int64   `gorm:"not null" json:"updatedAt"`
}

type FeedExposure struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	RequestID string  `gorm:"size:64;not null;index;uniqueIndex:uq_feed_exposure" json:"requestId"`
	UserID    string  `gorm:"size:64;index" json:"userId"`
	PostID    string  `gorm:"size:64;not null;index;uniqueIndex:uq_feed_exposure" json:"postId"`
	Position  int     `gorm:"not null;default:0" json:"position"`
	Strategy  string  `gorm:"size:32;not null;default:''" json:"strategy"`
	Score     float64 `gorm:"not null;default:0" json:"score"`
	SessionID string  `gorm:"size:64;not null;default:'';index" json:"sessionId"`
	CreatedAt int64   `gorm:"not null;index" json:"createdAt"`
}

type FeedClick struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	RequestID string  `gorm:"size:64;not null;index;uniqueIndex:uq_feed_click" json:"requestId"`
	UserID    string  `gorm:"size:64;index" json:"userId"`
	PostID    string  `gorm:"size:64;not null;index;uniqueIndex:uq_feed_click" json:"postId"`
	Position  int     `gorm:"not null;default:0" json:"position"`
	Strategy  string  `gorm:"size:32;not null;default:''" json:"strategy"`
	Score     float64 `gorm:"not null;default:0" json:"score"`
	SessionID string  `gorm:"size:64;not null;default:'';index" json:"sessionId"`
	CreatedAt int64   `gorm:"not null;index" json:"createdAt"`
}

type PostEmbedding struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID        string `gorm:"size:64;not null;index;uniqueIndex:uq_post_embedding" json:"postId"`
	ModelName     string `gorm:"size:128;not null;uniqueIndex:uq_post_embedding" json:"modelName"`
	EmbeddingJSON string `gorm:"type:text;not null;default:'[]'" json:"embeddingJson"`
	ContentDigest string `gorm:"size:128;not null;default:''" json:"contentDigest"`
	UpdatedAt     int64  `gorm:"not null;index" json:"updatedAt"`
}

type UserEmbedding struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string `gorm:"size:64;not null;index;uniqueIndex:uq_user_embedding" json:"userId"`
	ModelName     string `gorm:"size:128;not null;uniqueIndex:uq_user_embedding" json:"modelName"`
	EmbeddingJSON string `gorm:"type:text;not null;default:'[]'" json:"embeddingJson"`
	ProfileDigest string `gorm:"size:128;not null;default:''" json:"profileDigest"`
	UpdatedAt     int64  `gorm:"not null;index" json:"updatedAt"`
}

type RecommendationModel struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ModelKey      string  `gorm:"size:64;not null;uniqueIndex" json:"modelKey"`
	Version       int64   `gorm:"not null;index" json:"version"`
	Intercept     float64 `gorm:"not null;default:0" json:"intercept"`
	FeatureJSON   string  `gorm:"type:text;not null;default:'{}'" json:"featureJson"`
	TrainingStats string  `gorm:"type:text;not null;default:'{}'" json:"trainingStats"`
	TrainedAt     int64   `gorm:"not null;index" json:"trainedAt"`
	CreatedAt     int64   `gorm:"not null" json:"createdAt"`
	UpdatedAt     int64   `gorm:"not null" json:"updatedAt"`
}

type RefreshToken struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Token     string `gorm:"size:128;not null;uniqueIndex" json:"token"`
	UserID    string `gorm:"size:64;not null;index" json:"userId"`
	ExpiresAt int64  `gorm:"not null;index" json:"expiresAt"`
	RevokedAt int64  `gorm:"not null;default:0" json:"revokedAt"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
	UpdatedAt int64  `gorm:"not null" json:"updatedAt"`
}

type RevokedAccessToken struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	JTI       string `gorm:"size:128;not null;uniqueIndex" json:"jti"`
	ExpiresAt int64  `gorm:"not null;index" json:"expiresAt"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
}

// --- Domain Events (audit trail for Agent investigation) ---

type DomainEvent struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType     string `gorm:"size:64;not null;index:idx_de_type" json:"eventType"`
	AggregateType string `gorm:"size:32;not null;index:idx_de_agg" json:"aggregateType"`
	AggregateID   string `gorm:"size:128;not null;index:idx_de_agg" json:"aggregateId"`
	ActorID       string `gorm:"size:64;not null;index" json:"actorId"`
	Payload       string `gorm:"type:text;not null;default:'{}'" json:"payload"`
	CreatedAt     int64  `gorm:"not null;index" json:"createdAt"`
}

// --- Agent Run / Step (investigation trajectory) ---

type AgentRun struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	CaseID      string `gorm:"size:64;not null;index" json:"caseId"`
	Status      string `gorm:"size:24;not null;default:pending;index" json:"status"`
	Model       string `gorm:"size:64;not null;default:''" json:"model"`
	StepCount   int    `gorm:"not null;default:0" json:"stepCount"`
	TokensUsed  int    `gorm:"not null;default:0" json:"tokensUsed"`
	Report      string `gorm:"type:text;not null;default:''" json:"report"`
	ErrorMsg    string `gorm:"type:text;not null;default:''" json:"errorMsg"`
	StartedAt   int64  `gorm:"not null;default:0" json:"startedAt"`
	CompletedAt int64  `gorm:"not null;default:0" json:"completedAt"`
	CreatedAt   int64  `gorm:"not null;index" json:"createdAt"`
}

const (
	AgentRunPending   = "pending"
	AgentRunRunning   = "running"
	AgentRunCompleted = "completed"
	AgentRunFailed    = "failed"
)

type AgentStep struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	RunID      string `gorm:"size:64;not null;index" json:"runId"`
	StepIndex  int    `gorm:"not null" json:"stepIndex"`
	Action     string `gorm:"size:64;not null" json:"action"`
	Input      string `gorm:"type:text;not null;default:'{}'" json:"input"`
	Output     string `gorm:"type:text;not null;default:'{}'" json:"output"`
	Reasoning  string `gorm:"type:text;not null;default:''" json:"reasoning"`
	LatencyMs  int    `gorm:"not null;default:0" json:"latencyMs"`
	TokensUsed int    `gorm:"not null;default:0" json:"tokensUsed"`
	CreatedAt  int64  `gorm:"not null" json:"createdAt"`
}

// --- Evidence Layer (investigation support) ---

// ContentSnapshot freezes post content at the time of moderation submission.
type ContentSnapshot struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	PostID      string `gorm:"size:64;not null;index" json:"postId"`
	Title       string `gorm:"size:255;not null" json:"title"`
	Description string `gorm:"type:text;not null" json:"description"`
	Address     string `gorm:"size:255;not null;default:''" json:"address"`
	Category    string `gorm:"size:64;not null;default:''" json:"category"`
	SubCategory string `gorm:"size:64;not null;default:''" json:"subCategory"`
	MaxCount    int    `gorm:"not null;default:0" json:"maxCount"`
	ContentHash string `gorm:"size:128;not null;index" json:"contentHash"`
	SnapshotAt  int64  `gorm:"not null;index" json:"snapshotAt"`
	CreatedAt   int64  `gorm:"not null" json:"createdAt"`
}

// Notification tracks whether users were notified of key changes.
type Notification struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	UserID      string `gorm:"size:64;not null;index" json:"userId"`
	PostID      string `gorm:"size:64;not null;index" json:"postId"`
	Type        string `gorm:"size:32;not null;index" json:"type"`    // "activity_changed", "reminder", "settlement_request"
	Channel     string `gorm:"size:16;not null" json:"channel"`       // "in_app", "push"
	Status      string `gorm:"size:16;not null;index" json:"status"`  // "sent", "delivered", "failed", "read"
	Payload     string `gorm:"type:text;not null;default:'{}'" json:"payload"`
	SentAt      int64  `gorm:"not null" json:"sentAt"`
	DeliveredAt int64  `gorm:"not null;default:0" json:"deliveredAt"`
	ReadAt      int64  `gorm:"not null;default:0" json:"readAt"`
	CreatedAt   int64  `gorm:"not null;index" json:"createdAt"`
}

// Report is a raw user-submitted report or appeal (multiple can merge into one case).
type Report struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	CaseID      string `gorm:"size:64;not null;default:'';index" json:"caseId"`
	ReporterID  string `gorm:"size:64;not null;index" json:"reporterId"`
	TargetType  string `gorm:"size:32;not null" json:"targetType"`       // "post", "message", "user"
	TargetID    string `gorm:"size:128;not null;index" json:"targetId"`
	Reason      string `gorm:"type:text;not null" json:"reason"`
	EvidenceIDs string `gorm:"type:text;not null;default:'[]'" json:"evidenceIds"` // JSON array
	Status      string `gorm:"size:16;not null;default:pending;index" json:"status"`
	CreatedAt   int64  `gorm:"not null;index" json:"createdAt"`
}

// CaseEvidence links a piece of evidence to a case with relevance context.
type CaseEvidence struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	CaseID       string `gorm:"size:64;not null;index" json:"caseId"`
	EvidenceType string `gorm:"size:32;not null" json:"evidenceType"` // "domain_event", "chat_message", "content_snapshot", "credit_ledger", "notification"
	EvidenceID   string `gorm:"size:128;not null" json:"evidenceId"`
	AddedBy      string `gorm:"size:64;not null" json:"addedBy"`                           // "agent:run_xxx", "admin:user_xxx", "system"
	Relevance    string `gorm:"size:16;not null;default:supporting" json:"relevance"`      // "key", "supporting", "context"
	Note         string `gorm:"type:text;not null;default:''" json:"note"`
	CreatedAt    int64  `gorm:"not null" json:"createdAt"`
}

// CaseDecision records each decision made on a case (supports initial, appeal, reopen).
type CaseDecision struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	CaseID       string `gorm:"size:64;not null;index" json:"caseId"`
	DeciderID    string `gorm:"size:64;not null;index" json:"deciderId"` // admin user ID or "agent:run_xxx"
	DecisionType string `gorm:"size:32;not null" json:"decisionType"`    // "initial", "appeal", "reopen"
	Outcome      string `gorm:"size:32;not null" json:"outcome"`         // "upheld", "rejected", "insufficient_evidence", "escalate"
	Reasoning    string `gorm:"type:text;not null;default:''" json:"reasoning"`
	EvidenceRefs string `gorm:"type:text;not null;default:'[]'" json:"evidenceRefs"` // JSON: evidence IDs cited
	Actions      string `gorm:"type:text;not null;default:'[]'" json:"actions"`      // JSON: backend actions taken
	CreatedAt    int64  `gorm:"not null;index" json:"createdAt"`
}
