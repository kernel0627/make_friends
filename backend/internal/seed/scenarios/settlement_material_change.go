package scenarios

import "time"

// SettlementMaterialChange — Author significantly changes the activity after participants joined.
// Participant disputes because original plan was different. Agent should side with participant.
func SettlementMaterialChange() *Scenario {
	return &Scenario{
		ID:         "settlement_material_change_01",
		CaseType:   "settlement_dispute",
		Difficulty: "medium",
		Summary:    "Participant disputes settlement claiming activity was materially changed after they joined",
		Truth: Truth{
			Outcome:          "upheld",
			ResponsibleParty: "author",
			PolicyRefs:       []string{"settlement_material_change"},
			RequiredEvidence: []string{"content_snapshot_before", "content_snapshot_after", "domain_event_update", "notification_status"},
			ForbiddenClaims:  []string{"participant_no_show", "participant_fault"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "活动发起人", CreditScore: 95},
			{Ref: "participant_1", Nickname: "被坑参与者", CreditScore: 100},
			{Ref: "participant_2", Nickname: "无所谓同学", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周六爬山+野餐",
					"description": "白云山轻松路线，预计3小时。山顶野餐，我带帐篷和餐垫。大家各自带点吃的分享就好，AA制缆车票。",
					"category":    "户外",
					"subCategory": "爬山",
					"address":     "白云山南门",
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
				Offset:   5 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "太好了！我正好想爬山，我带三明治和水果",
				},
			},
			// Author changes plan significantly the night before
			{
				Offset:   20 * time.Hour,
				Action:   ActionUpdatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周六密室逃脱",
					"description": "改计划了！发现一个超好玩的密室，6人本。费用AA，每人128元。地点在天河城B2。时间改为下午3点。",
					"address":     "天河城B2 迷境密室",
				},
			},
			// Notification was sent but not all participants saw it
			{
				Offset:   20*time.Hour + 5*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_1",
				Data: map[string]any{
					"type":   "activity_changed",
					"status": "sent", // sent but not necessarily read
				},
			},
			{
				Offset:   20*time.Hour + 5*time.Minute,
				Action:   ActionSendNotification,
				ActorRef: "participant_2",
				Data: map[string]any{
					"type":   "activity_changed",
					"status": "read",
				},
			},
			{
				Offset:   21 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "密室也行！我都可以",
				},
			},
			// Day of — participant_1 sees the change too late
			{
				Offset:   24 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "等等？？什么时候改密室了？我准备了爬山的东西啊，而且我没有128的预算",
				},
			},
			{
				Offset:   24*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "昨晚就改了啊，你没看通知吗",
				},
			},
			{
				Offset:   24*time.Hour + 15*time.Minute,
				Action:   ActionCancelJoin,
				ActorRef: "participant_1",
			},
			// Settlement
			{
				Offset:   30 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			{
				Offset:   31 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":      "author",
					"decision":  "dispute",
					"note":      "被坑参与者临时取消，影响了活动人数",
					"targetRef": "participant_1",
				},
			},
			{
				Offset:   31*time.Hour + 30*time.Minute,
				Action:   ActionSubmitSettlement,
				ActorRef: "participant_1",
				Data: map[string]any{
					"role":     "participant",
					"decision": "dispute",
					"note":     "活动从爬山变成密室，地点时间费用全变了，我根本没同意",
				},
			},
			{
				Offset:   32 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "participant_1",
				Data: map[string]any{
					"caseType":    "settlement_dispute",
					"targetRef":   "author",
					"description": "Activity materially changed (outdoor→indoor, free→128元, morning→afternoon, different location) after participant joined",
				},
			},
		},
	}
}
