package scenarios

import "time"

// SettlementDisputeAmbiguous — Both parties have valid points. Author made a minor change,
// participant overreacted. Difficult case where "insufficient_evidence" for clear blame is correct.
func SettlementDisputeAmbiguous() *Scenario {
	return &Scenario{
		ID:         "settlement_ambiguous_01",
		CaseType:   "settlement_dispute",
		Difficulty: "hard",
		Summary:    "Both parties dispute settlement; author made minor time change, participant claims breach",
		Truth: Truth{
			Outcome:          "rejected", // dispute rejected — minor change doesn't warrant penalty
			ResponsibleParty: "",
			PolicyRefs:       []string{"settlement_material_change"},
			RequiredEvidence: []string{"content_snapshot_before", "content_snapshot_after", "chat_acknowledgment"},
			ForbiddenClaims:  []string{"material_change"}, // it wasn't material enough
		},
		Roles: []Role{
			{Ref: "author", Nickname: "篮球队长", CreditScore: 100},
			{Ref: "participant_1", Nickname: "较真同学", CreditScore: 95},
			{Ref: "participant_2", Nickname: "随和选手", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周日上午打篮球",
					"description": "篮球场全场，打半场或全场看人数。上午9点开始，打到11点。水平不限，开心就好。",
					"category":    "运动",
					"subCategory": "篮球",
					"address":     "北区篮球场",
					"maxCount":    10,
				},
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_1",
			},
			{
				Offset:   5 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			// Minor change: 9am → 9:30am
			{
				Offset:   18 * time.Hour,
				Action:   ActionUpdatePost,
				ActorRef: "author",
				Data: map[string]any{
					"description": "篮球场全场，打半场或全场看人数。上午9:30开始（推迟半小时），打到11:30。水平不限，开心就好。",
				},
			},
			{
				Offset:   18*time.Hour + 5*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_1",
				Data: map[string]any{
					"type":   "activity_changed",
					"status": "delivered",
				},
			},
			{
				Offset:   18*time.Hour + 5*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_2",
				Data: map[string]any{
					"type":   "activity_changed",
					"status": "read",
				},
			},
			{
				Offset:   18*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "大家注意，推迟半小时哈，9:30开始。场地那边说9点有人包场。",
				},
			},
			{
				Offset:   19 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "没问题👌",
				},
			},
			// participant_1 doesn't respond, shows up at 9am, nobody there, leaves
			{
				Offset:   24 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "我9点到的一个人都没有，等了15分钟就走了。你改时间也不提前多说一声",
				},
			},
			{
				Offset:   24*time.Hour + 5*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "我发了通知也在群里说了啊…你没看消息吗",
				},
			},
			{
				Offset:   26 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			{
				Offset:   27 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_1",
				Data: map[string]any{
					"role":     "participant",
					"decision": "dispute",
					"note":     "活动时间被改了，我白跑一趟",
				},
			},
			{
				Offset:   27*time.Hour + 30*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":      "author",
					"decision":  "completed",
					"note":      "活动正常进行了，就是较真同学自己没看通知",
					"targetRef": "participant_1",
				},
			},
			{
				Offset:   28 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "participant_1",
				Data: map[string]any{
					"caseType":    "settlement_dispute",
					"targetRef":   "author",
					"description": "Author changed activity time without adequate notice. 30min change the night before.",
				},
			},
		},
	}
}
