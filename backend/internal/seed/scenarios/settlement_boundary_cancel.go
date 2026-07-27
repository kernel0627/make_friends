package scenarios

import "time"

// SettlementBoundaryCancelTiming — Activity cancelled 23 hours before start time.
// Policy requires 24h notice for no-penalty cancellation. Author argues they told
// everyone in chat 25 hours before, but the system cancel action was only 23 hours before.
// System record is authoritative — penalty applies.
func SettlementBoundaryCancelTiming() *Scenario {
	return &Scenario{
		ID:         "settlement_boundary_cancel_01",
		CaseType:   "settlement_dispute",
		Difficulty: "hard",
		Summary:    "Author cancelled activity 23 hours before start; argues chat notice was sent 25 hours before; disputes penalty",
		Truth: Truth{
			Outcome:          "rejected", // dispute rejected — author's complaint is invalid, system record confirms <24h cancel
			ResponsibleParty: "author",
			PolicyRefs:       []string{"settlement_no_show"},
			RequiredEvidence: []string{"cancel_timestamp", "chat_notice_timestamp", "system_cancel_record"},
			ForbiddenClaims:  []string{"timely_cancellation", "24h_notice_met"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "桌游组织者", CreditScore: 95},
			{Ref: "participant_1", Nickname: "桌游新手", CreditScore: 100},
			{Ref: "participant_2", Nickname: "策略游戏迷", CreditScore: 100},
			{Ref: "participant_3", Nickname: "周末玩家", CreditScore: 100},
		},
		Timeline: []Event{
			// Activity created for Saturday 2pm (BaseTime + 72h for a Wednesday post)
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周六下午桌游局（卡坦岛+风声）",
					"description": "周六下午2点开始，地点在学活桌游室。带了卡坦岛和风声两个游戏，看人数决定玩什么。新手友好会教规则！预计玩到6点。",
					"category":    "娱乐",
					"subCategory": "桌游",
					"address":     "学生活动中心B203桌游室",
					"maxCount":    6,
					"startTime":   "Saturday 14:00", // conceptual — actual offset is BaseTime + 72h
				},
			},
			{
				Offset:   5 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_1",
			},
			{
				Offset:   8 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   12 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_3",
			},
			{
				Offset:   24 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "好期待周六！卡坦岛我一直想玩",
				},
			},
			{
				Offset:   24*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "到时候我教你！规则其实不复杂",
				},
			},
			// Friday 1pm (25h before Saturday 2pm) — author mentions in chat they might cancel
			{
				Offset:   47 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "各位不好意思，我周六可能有事来不了，桌游局大概率取消了。等我确认一下再正式取消",
				},
			},
			{
				Offset:   47*time.Hour + 5*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "啊好吧，确定了跟我们说",
				},
			},
			// Friday 3pm (23h before) — author finally hits cancel in app
			{
				Offset:   49 * time.Hour,
				Action:   ActionCancelPost,
				ActorRef: "author",
			},
			{
				Offset:   49*time.Hour + 1*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_1",
				Data: map[string]any{
					"type":   "activity_cancelled",
					"status": "delivered",
				},
			},
			{
				Offset:   49*time.Hour + 1*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_2",
				Data: map[string]any{
					"type":   "activity_cancelled",
					"status": "delivered",
				},
			},
			{
				Offset:   49*time.Hour + 1*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_3",
				Data: map[string]any{
					"type":   "activity_cancelled",
					"status": "read",
				},
			},
			// System auto-penalizes for <24h cancel
			{
				Offset:   49*time.Hour + 30*time.Minute,
				Action:   ActionCreditPenalty,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef":  "author",
					"delta":      -5,
					"sourceType": "late_cancel_penalty",
					"note":       "Activity cancelled less than 24h before start time",
				},
			},
			// Author disputes the penalty
			{
				Offset:   50 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "凭什么扣我分？我明明提前25小时在群里说了要取消",
				},
			},
			{
				Offset:   51 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "author",
				Data: map[string]any{
					"caseType":    "settlement_dispute",
					"targetRef":   "author",
					"description": "Author disputes late-cancel penalty. Chat message about cancellation was sent 25h before start, but system cancel action was recorded 23h before. Author argues chat notice should count.",
				},
			},
		},
	}
}
