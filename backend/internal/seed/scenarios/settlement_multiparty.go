package scenarios

import "time"

// SettlementMultipartyDispute — Multi-party dispute with 4 participants.
// Author no-shows but claims the activity was cancelled. 2 participants confirm
// author was absent, 1 participant sides with author (was told privately).
// Settlement records conflict. Agent must weigh majority testimony + system records.
func SettlementMultipartyDispute() *Scenario {
	return &Scenario{
		ID:         "settlement_multiparty_01",
		CaseType:   "settlement_dispute",
		Difficulty: "hard",
		Summary:    "4-participant activity where author allegedly no-showed; conflicting settlement claims from multiple parties",
		Truth: Truth{
			Outcome:          "upheld", // dispute upheld — author did no-show
			ResponsibleParty: "author",
			PolicyRefs:       []string{"settlement_no_show"},
			RequiredEvidence: []string{"settlement_records", "chat_messages", "participant_testimony"},
			ForbiddenClaims:  []string{"activity_cancelled", "author_attended"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "跑步发起人", CreditScore: 95},
			{Ref: "participant_1", Nickname: "早起达人", CreditScore: 100},
			{Ref: "participant_2", Nickname: "健身小王", CreditScore: 100},
			{Ref: "participant_3", Nickname: "作者室友", CreditScore: 85},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周六晨跑5公里",
					"description": "操场集合，6:30出发跑5公里。配速随意，跑完一起吃早餐。不要迟到哦！",
					"category":    "运动",
					"subCategory": "跑步",
					"address":     "田径场西门",
					"maxCount":    5,
				},
			},
			{
				Offset:   2 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_1",
			},
			{
				Offset:   4 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   6 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_3",
			},
			// Day before — normal excitement
			{
				Offset:   20 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "明早见！闹钟定好了💪",
				},
			},
			{
				Offset:   20*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "6:30准时出发！大家别迟到",
				},
			},
			// Morning of — author doesn't show up. No cancel action in timeline.
			{
				Offset:   24*time.Hour + 40*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "我们三个到了，发起人呢？打了电话没接",
				},
			},
			{
				Offset:   24*time.Hour + 45*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "等了15分钟了，人没来也没消息",
				},
			},
			// participant_3 privately knows author overslept but covers for them
			{
				Offset:   24*time.Hour + 50*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_3",
				Data: map[string]any{
					"content": "他昨晚跟我说可能取消了…你们没收到消息吗",
				},
			},
			{
				Offset:   24*time.Hour + 55*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "没有啊，群里没说取消，通知也没有",
				},
			},
			// Author surfaces hours later
			{
				Offset:   28 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "不好意思！！昨晚太晚睡了…我以为室友帮我跟你们说了取消",
				},
			},
			{
				Offset:   30 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			// Conflicting settlements
			{
				Offset:   31 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_1",
				Data: map[string]any{
					"role":      "participant",
					"decision":  "no_show",
					"note":      "发起人没来，我们白等了半小时",
					"targetRef": "author",
				},
			},
			{
				Offset:   31*time.Hour + 10*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_2",
				Data: map[string]any{
					"role":      "participant",
					"decision":  "no_show",
					"note":      "组织者爽约，没通知取消",
					"targetRef": "author",
				},
			},
			// participant_3 sides with author
			{
				Offset:   31*time.Hour + 20*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_3",
				Data: map[string]any{
					"role":     "participant",
					"decision": "completed",
					"note":     "他提前跟我说取消了，可能忘了在群里通知",
				},
			},
			// Author claims cancellation
			{
				Offset:   31*time.Hour + 30*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":     "author",
					"decision": "dispute",
					"note":     "我让室友帮忙通知了取消，不是故意爽约",
				},
			},
			{
				Offset:   32 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "participant_1",
				Data: map[string]any{
					"caseType":    "settlement_dispute",
					"targetRef":   "author",
					"description": "Author no-showed with no system cancel. Claims told roommate but no notification sent. 2 of 3 participants confirm no-show.",
				},
			},
		},
	}
}
