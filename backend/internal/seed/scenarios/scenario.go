// Package scenarios defines the framework for scenario-driven test data generation.
//
// A Scenario describes a complete investigation case from ground truth through
// timeline to expected outcome. The Generator takes scenarios and writes all
// necessary records into the database.
package scenarios

import "time"

// Scenario is a self-contained case scenario for the agent benchmark.
type Scenario struct {
	ID         string     `json:"id"`
	CaseType   string     `json:"caseType"`
	Difficulty string     `json:"difficulty"` // "easy", "medium", "hard"
	Summary    string     `json:"summary"`    // Neutral, non-leaking description for the case record
	Truth      Truth      `json:"truth"`      // Hidden from agent, used by evaluator
	Roles      []Role     `json:"roles"`      // Actor definitions
	Timeline   []Event    `json:"timeline"`   // Ordered events to generate
	BaseTime   time.Time  `json:"-"`          // Anchor time for the scenario
}

// Truth holds the ground-truth labels hidden from the agent.
type Truth struct {
	Outcome          string   `json:"outcome"`          // "upheld", "rejected", "insufficient_evidence"
	ResponsibleParty string   `json:"responsibleParty"` // role ref of who's at fault
	PolicyRefs       []string `json:"policyRefs"`       // policy IDs that apply
	RequiredEvidence []string `json:"requiredEvidence"` // evidence IDs the agent must find
	ForbiddenClaims  []string `json:"forbiddenClaims"`  // things agent must NOT conclude
}

// Role defines an actor in the scenario.
type Role struct {
	Ref         string `json:"ref"`         // "author", "participant_1", "reporter"
	Nickname    string `json:"nickname"`    // Generated user nickname
	CreditScore int    `json:"creditScore"` // Starting credit score (default 100)
}

// Event is a single action in the scenario timeline.
type Event struct {
	Offset   time.Duration  `json:"offset"`   // Relative to scenario BaseTime
	Action   string         `json:"action"`   // See action constants below
	ActorRef string         `json:"actorRef"` // References a Role.Ref
	Data     map[string]any `json:"data"`     // Action-specific data
}

// Action constants
const (
	ActionCreatePost       = "create_post"
	ActionUpdatePost       = "update_post"
	ActionClosePost        = "close_post"
	ActionCancelPost       = "cancel_post"
	ActionJoinPost         = "join_post"
	ActionCancelJoin       = "cancel_join"
	ActionSendMessage      = "send_message"
	ActionSendNotification = "send_notification"
	ActionSubmitSettlement = "submit_settlement"
	ActionCreditPenalty    = "credit_penalty"
	ActionCreateCase       = "create_case"
	ActionCreateReport     = "create_report"
	ActionModerationReject = "moderation_reject"
	ActionModerationAppeal = "moderation_appeal"
)
