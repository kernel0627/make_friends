package scenarios

import "time"

// ModerationAppealEscalation — Post rejected for "off_platform" content (alleged QQ number).
// First appeal was rejected. User appeals again with new evidence: a chat message clarifying
// that "QQ楼501" is a building room number, not a QQ account. Second appeal should be upheld.
func ModerationAppealEscalation() *Scenario {
	return &Scenario{
		ID:         "moderation_appeal_escalation_01",
		CaseType:   "moderation_appeal",
		Difficulty: "hard",
		Summary:    "Second appeal of moderation rejection; user provides new evidence that 'QQ楼501' is a room number not a QQ account",
		Truth: Truth{
			Outcome:          "upheld", // second appeal upheld — original rejection was wrong
			ResponsibleParty: "",       // system error
			PolicyRefs:       []string{"content_off_platform"},
			RequiredEvidence: []string{"moderation_record", "content_snapshot", "first_appeal_record", "clarifying_evidence"},
			ForbiddenClaims:  []string{"off_platform_contact", "qq_account_sharing"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "自习室常客", CreditScore: 90},
			{Ref: "participant_1", Nickname: "考研同路人", CreditScore: 100},
		},
		Timeline: []Event{
			// Original post creation
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "考研自习室拼座位 QQ楼501",
					"description": "QQ楼501教室长期有空位，我每天早上7点到晚上9点都在。想找几个研友一起占座互相监督。带好自己的书和水杯就行，教室有空调。",
					"category":    "学习",
					"subCategory": "考试",
					"address":     "QQ楼501教室",
					"maxCount":    4,
				},
			},
			// Post rejected — system flags "QQ" as off-platform contact
			{
				Offset:   20 * time.Minute,
				Action:   ActionModerationReject,
				ActorRef: "author",
				Data: map[string]any{
					"matchedPolicies": `["content_off_platform"]`,
					"reason":          "Detected QQ account reference in post title and description. Off-platform contact sharing violates community guidelines.",
				},
			},
			// First appeal — author explains but without strong evidence
			{
				Offset:   1 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "我的帖子为什么被拒了？QQ楼是我们学校的教学楼名字啊，不是QQ号。我要申诉",
				},
			},
			// First appeal rejected — moderator/system not convinced without evidence
			{
				Offset:   5 * time.Hour,
				Action:   ActionSendNotification,
				ActorRef: "author",
				Data: map[string]any{
					"type":   "appeal_rejected",
					"status": "delivered",
				},
			},
			{
				Offset:   5*time.Hour + 1*time.Minute,
				Action:   ActionCreditPenalty,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef":  "author",
					"delta":      -5,
					"sourceType": "moderation_violation",
					"note":       "First appeal rejected: insufficient evidence that QQ refers to building name",
				},
			},
			// Author gathers evidence — asks a participant to confirm in chat
			{
				Offset:   6 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "我的帖子被误判了，说我发QQ号…QQ楼是咱学校的楼啊，你能帮我证明一下吗",
				},
			},
			{
				Offset:   6*time.Hour + 20*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "哈哈哈什么鬼，QQ楼就是齐谦楼的简称啊，全校都这么叫。501教室我天天去自习的",
				},
			},
			{
				Offset:   6*time.Hour + 25*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "对！全名是齐谦楼，大家都叫QQ楼。我再去申诉一次",
				},
			},
			// Second appeal with new evidence — creates the case for agent investigation
			{
				Offset:   7 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "author",
				Data: map[string]any{
					"caseType":    "moderation_appeal",
					"targetRef":   "author",
					"description": "Second appeal after first rejection. Author claims QQ楼 is a campus building name (齐谦楼), not a QQ messenger reference. New chat evidence from another student corroborates. Original post was about study room seat-sharing.",
				},
			},
		},
	}
}
