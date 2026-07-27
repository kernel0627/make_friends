package scenarios

import "time"

// CreditAppealUnfounded — User appeals a credit penalty but the evidence clearly shows
// they were at fault. Agent should reject the appeal.
func CreditAppealUnfounded() *Scenario {
	return &Scenario{
		ID:         "credit_appeal_unfounded_01",
		CaseType:   "credit_appeal",
		Difficulty: "easy",
		Summary:    "User appeals credit penalty claiming system error; investigation shows clear no-show with no prior notice",
		Truth: Truth{
			Outcome:          "rejected",
			ResponsibleParty: "appellant",
			PolicyRefs:       []string{"settlement_no_show", "credit_reversal"},
			RequiredEvidence: []string{"credit_ledger_entry", "no_advance_notice", "settlement_record"},
			ForbiddenClaims:  []string{"system_error", "advance_notice_given"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "烧烤大师", CreditScore: 100},
			{Ref: "appellant", Nickname: "嘴硬选手", CreditScore: 85},
			{Ref: "participant_2", Nickname: "正常参与者", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周末烧烤趴",
					"description": "公园烧烤区，我负责炭火和工具。食材AA，预计每人50左右。下午4点开始。",
					"category":    "美食",
					"subCategory": "烧烤",
					"address":     "翠湖公园烧烤区",
					"maxCount":    6,
				},
			},
			{
				Offset:   2 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "appellant",
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   5 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "好耶！我带啤酒",
				},
			},
			{
				Offset:   6 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "太好了！那我多买点肉",
				},
			},
			// Day of — author confirms
			{
				Offset:   22 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "今天下午4点老地方见！食材我已经买好了",
				},
			},
			{
				Offset:   22*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "好的，路上了",
				},
			},
			// Appellant: total silence day of, no message, no cancel
			// Post event
			{
				Offset:   28 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			{
				Offset:   29 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":      "author",
					"decision":  "no_show",
					"note":      "没来也没说，多买的食材浪费了",
					"targetRef": "appellant",
				},
			},
			{
				Offset:   29*time.Hour + 30*time.Minute,
				Action:   ActionCreditPenalty,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef":  "appellant",
					"delta":      -5,
					"sourceType": "no_show_penalty",
					"note":       "Activity no-show: auto penalty",
				},
			},
			// Appellant appeals claiming "system glitch"
			{
				Offset:   30 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "appellant",
				Data: map[string]any{
					"caseType":    "credit_appeal",
					"targetRef":   "appellant",
					"description": "我明明取消了参与的，系统可能出了bug。不应该扣我分。",
				},
			},
		},
	}
}
