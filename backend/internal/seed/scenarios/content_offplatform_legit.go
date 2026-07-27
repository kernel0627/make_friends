package scenarios

import "time"

// ContentOffPlatformLegitimate — A post reported for "off-platform" behavior, but
// the content is actually a legitimate activity where sharing contact info is reasonable.
// Agent should reject the report (false positive).
func ContentOffPlatformLegitimate() *Scenario {
	return &Scenario{
		ID:         "content_offplatform_legit_01",
		CaseType:   "content_report",
		Difficulty: "medium",
		Summary:    "User reported activity post for off-platform solicitation; reporter alleges WeChat group recruitment",
		Truth: Truth{
			Outcome:          "rejected",
			ResponsibleParty: "",
			PolicyRefs:       []string{"content_off_platform"},
			RequiredEvidence: []string{"chat_context", "post_content"},
			ForbiddenClaims:  []string{"commercial_activity", "policy_violation"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "跑步达人", CreditScore: 100},
			{Ref: "reporter", Nickname: "举报侠", CreditScore: 95},
			{Ref: "participant_1", Nickname: "新手跑者", CreditScore: 100},
			{Ref: "participant_2", Nickname: "周末选手", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周六晨跑约伴 5km",
					"description": "每周六早7点校园东门集合，跑5公里。配速自由，互相鼓励就好。需要提前一天确认出席。",
					"category":    "运动",
					"subCategory": "跑步",
					"address":     "校园东门",
					"maxCount":    8,
				},
			},
			{
				Offset:   3 * time.Hour,
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
					"content": "大家平时怎么联系确认出席？有群吗？",
				},
			},
			{
				Offset:   5*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "平台上聊就行，每周五晚我会在这里发消息确认。如果下雨的话我也会提前通知。",
				},
			},
			{
				Offset:   5*time.Hour + 20*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "好的！那周六见",
				},
			},
			{
				Offset:   6 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "对了跑完可以一起吃个早餐，我知道一家不错的粥店",
				},
			},
			{
				Offset:   24 * time.Hour,
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"reason":     "这个人在拉微信群，把人引到平台外面去",
					"targetType": "post",
				},
			},
			{
				Offset:   24*time.Hour + 30*time.Minute,
				Action:   ActionCreateCase,
				ActorRef: "reporter",
				Data: map[string]any{
					"caseType":    "content_report",
					"targetRef":   "author",
					"description": "Reporter claims off-platform solicitation but chat shows author explicitly staying on-platform",
				},
			},
		},
	}
}
