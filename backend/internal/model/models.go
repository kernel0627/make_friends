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
	ID           string  `gorm:"primaryKey;size:64" json:"id"`
	AuthorID     string  `gorm:"size:64;not null;index" json:"authorId"`
	Title        string  `gorm:"size:255;not null" json:"title"`
	Description  string  `gorm:"type:text;not null;default:''" json:"description"`
	Category     string  `gorm:"size:64;not null;index:idx_posts_category" json:"category"`
	SubCategory  string  `gorm:"size:64;not null;default:'';index:idx_posts_category" json:"subCategory"`
	TimeMode     string  `gorm:"size:16;not null" json:"timeMode"`
	TimeDays     int     `json:"timeDays"`
	FixedTime    string  `gorm:"size:64" json:"fixedTime"`
	Address      string  `gorm:"size:255;not null" json:"address"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	MaxCount     int     `gorm:"not null" json:"maxCount"`
	CurrentCount int     `gorm:"not null;default:0" json:"currentCount"`
	Status       string  `gorm:"size:16;not null;default:open" json:"status"`
	CancelledAt  int64   `gorm:"not null;default:0;index" json:"cancelledAt"`
	DeletedAt    int64   `gorm:"not null;default:0;index" json:"deletedAt"`
	DeletedBy    string  `gorm:"size:64;not null;default:''" json:"deletedBy"`
	ClosedAt     int64   `gorm:"not null;default:0;index" json:"closedAt"`
	CreatedAt    int64   `gorm:"not null;index:idx_posts_created_at,sort:desc" json:"createdAt"`
	UpdatedAt    int64   `gorm:"not null" json:"updatedAt"`
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
	CreatedAt      int64  `gorm:"not null;index" json:"createdAt"`
	UpdatedAt      int64  `gorm:"not null" json:"updatedAt"`
}

type AdminCase struct {
	ID             string `gorm:"primaryKey;size:64" json:"id"`
	CaseType       string `gorm:"size:32;not null;index" json:"caseType"`
	PostID         string `gorm:"size:64;not null;index" json:"postId"`
	TargetUserID   string `gorm:"size:64;not null;index" json:"targetUserId"`
	ReporterUserID string `gorm:"size:64;not null;index" json:"reporterUserId"`
	ResolverUserID string `gorm:"size:64;not null;default:'';index" json:"resolverUserId"`
	Status         string `gorm:"size:16;not null;default:open;index" json:"status"`
	Resolution     string `gorm:"size:32;not null;default:''" json:"resolution"`
	ResolutionNote string `gorm:"type:text;not null;default:''" json:"resolutionNote"`
	ResolvedAt     int64  `gorm:"not null;default:0;index" json:"resolvedAt"`
	SourceRef      string `gorm:"size:128;not null;uniqueIndex" json:"sourceRef"`
	Summary        string `gorm:"type:text;not null;default:''" json:"summary"`
	CreatedAt      int64  `gorm:"not null;index" json:"createdAt"`
	UpdatedAt      int64  `gorm:"not null" json:"updatedAt"`
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
