package scenarios

import "time"

// SettlementNoShowClear — Participant joins, doesn't show up, author reports.
// Clear no-show: no messages from participant before the event, no cancellation.
func SettlementNoShowClear() *Scenario {
	return &Scenario{
		ID:         "settlement_noshow_clear_01",
		CaseType:   "settlement_dispute",
		Difficulty: "easy",
		Summary:    "Author disputes settlement claiming participant did not attend the activity",
		Truth: Truth{
			Outcome:          "upheld",
			ResponsibleParty: "participant_1",
			PolicyRefs:       []string{"settlement_no_show"},
			RequiredEvidence: []string{"no_cancel_record", "no_chat_day_of", "settlement_dispute"},
			ForbiddenClaims:  []string{"author_fault", "material_change"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "活动组织者", CreditScore: 100},
			{Ref: "participant_1", Nickname: "鸽子选手", CreditScore: 90},
			{Ref: "participant_2", Nickname: "准时达人", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周日下午桌游局",
					"description": "桌游店包间已定，4人局，玩阿瓦隆+卡坦岛。下午2点到6点。请确定能来再报名，放鸽子扣信用分。",
					"category":    "游戏",
					"subCategory": "桌游",
					"address":     "欢乐桌游吧（大学城店）",
					"maxCount":    4,
				},
			},
			{
				Offset:   2 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_1",
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   4 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "周日见！期待",
				},
			},
			// Day of event — author sends reminder
			{
				Offset:   46 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "今天下午2点桌游吧见，大家准时哈",
				},
			},
			{
				Offset:   46*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "收到！已经在路上了",
				},
			},
			// participant_1 silence — no response, no cancel
			// After event — settlement
			{
				Offset:   52 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "@鸽子选手 你今天没来啊？也没说一声",
				},
			},
			{
				Offset:   53 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			{
				Offset:   54 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_2",
				Data: map[string]any{
					"role":     "participant",
					"decision": "completed",
					"note":     "活动很愉快，就是少了一个人",
				},
			},
			{
				Offset:   54*time.Hour + 30*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":      "author",
					"decision":  "dispute",
					"note":      "鸽子选手全程没来也没取消，浪费了一个名额",
					"targetRef": "participant_1",
				},
			},
			{
				Offset:   55 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "author",
				Data: map[string]any{
					"caseType":    "settlement_dispute",
					"targetRef":   "participant_1",
					"description": "Participant joined but did not attend and did not cancel. No response to day-of messages.",
				},
			},
		},
	}
}
