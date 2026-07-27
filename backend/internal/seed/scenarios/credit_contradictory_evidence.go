package scenarios

import "time"

// CreditContradictoryEvidence — User penalized for no-show appeals with contradictory evidence.
// System settlement says no-show, but chat messages indicate the user did arrive and the
// activity was cut short due to rain. Another participant corroborates. Agent must weigh
// chat evidence against system record and conclude the appeal is valid.
func CreditContradictoryEvidence() *Scenario {
	return &Scenario{
		ID:         "credit_contradictory_evidence_01",
		CaseType:   "credit_appeal",
		Difficulty: "hard",
		Summary:    "User appeals no-show penalty; chat evidence suggests they attended but activity ended early due to weather",
		Truth: Truth{
			Outcome:          "upheld", // appeal upheld — penalty should be reversed
			ResponsibleParty: "",
			PolicyRefs:       []string{"credit_reversal"},
			RequiredEvidence: []string{"chat_arrival_message", "participant_corroboration", "settlement_record", "weather_context"},
			ForbiddenClaims:  []string{"no_show_confirmed", "appellant_absent"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "户外活动王", CreditScore: 100},
			{Ref: "appellant", Nickname: "小明同学", CreditScore: 85},
			{Ref: "participant_2", Nickname: "摄影爱好者", CreditScore: 100},
			{Ref: "participant_3", Nickname: "新生小张", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周日下午户外写生+野餐",
					"description": "带上画板或者相机，到湖边写生拍照，顺便野餐。下午2点湖边亭子集合，预计玩到5点。我带垫子和零食，大家各带各的画具。",
					"category":    "生活",
					"subCategory": "户外",
					"address":     "校内人工湖西侧亭子",
					"maxCount":    6,
				},
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "appellant",
			},
			{
				Offset:   5 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   8 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_3",
			},
			{
				Offset:   20 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "明天见！我带了新买的水彩本",
				},
			},
			// Day of activity — appellant says they're on the way
			{
				Offset:   25*time.Hour + 50*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "我出门了，5分钟到",
				},
			},
			{
				Offset:   26 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "我们在亭子这边，你过来吧",
				},
			},
			// Appellant arrives, but shortly after it starts raining heavily
			{
				Offset:   26*time.Hour + 15*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "小明到了但是下雨了大家都散了",
				},
			},
			{
				Offset:   26*time.Hour + 20*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "雨太大了跑回宿舍了😭 刚到就下雨也太惨了",
				},
			},
			{
				Offset:   26*time.Hour + 25*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_3",
				Data: map[string]any{
					"content": "哈哈我也是刚躲进图书馆，太突然了",
				},
			},
			// Author closes post later. Incorrectly marks appellant as no-show
			// (maybe confused because appellant left quickly, or just sloppy settlement)
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
					"decision":  "no_show",
					"note":      "小明没怎么参加活动",
					"targetRef": "appellant",
				},
			},
			// System auto-penalizes
			{
				Offset:   31*time.Hour + 30*time.Minute,
				Action:   ActionCreditPenalty,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef":  "appellant",
					"delta":      -5,
					"sourceType": "no_show_penalty",
					"note":       "Activity no-show: auto penalty",
				},
			},
			// Appellant appeals
			{
				Offset:   32 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "什么？！我明明去了好吗！是下雨了大家才散的，凭什么说我没去",
				},
			},
			{
				Offset:   33 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "appellant",
				Data: map[string]any{
					"caseType":    "credit_appeal",
					"targetRef":   "appellant",
					"description": "Appellant was penalized for no-show but chat messages show they arrived. Activity ended early due to rain. Another participant confirms appellant was present. System settlement contradicts chat evidence.",
				},
			},
		},
	}
}
