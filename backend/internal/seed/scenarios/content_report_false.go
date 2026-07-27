package scenarios

import "time"

// ContentReportFalsePositive — Reporter has a pattern of false reports (low credibility).
// The reported content is perfectly fine. Agent should reject and note reporter behavior.
func ContentReportFalsePositive() *Scenario {
	return &Scenario{
		ID:         "content_report_false_01",
		CaseType:   "content_report",
		Difficulty: "easy",
		Summary:    "Activity post reported for inappropriate content; post appears to be a normal social gathering",
		Truth: Truth{
			Outcome:          "rejected",
			ResponsibleParty: "",
			PolicyRefs:       []string{"content_commercial", "content_off_platform"},
			RequiredEvidence: []string{"post_content", "chat_messages"},
			ForbiddenClaims:  []string{"policy_violation", "commercial_activity", "inappropriate_content"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "电影爱好者", CreditScore: 100},
			{Ref: "reporter", Nickname: "爱举报的人", CreditScore: 75}, // Low credit = history of issues
			{Ref: "participant_1", Nickname: "影迷同学", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周四晚看电影《奥本海默》",
					"description": "万达IMAX厅，晚上7:30场。看完可以一起讨论。各买各的票，座位我来协调选在一起。",
					"category":    "娱乐",
					"subCategory": "电影",
					"address":     "万达影城（大学城店）",
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
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "我选了F排中间的座位，你到时候选F排旁边的就行",
				},
			},
			{
				Offset:   3*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "收到！好期待这部电影",
				},
			},
			// Reporter submits a bogus report
			{
				Offset:   5 * time.Hour,
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"reason":     "感觉是黄牛在卖票",
					"targetType": "post",
				},
			},
			{
				Offset:   5*time.Hour + 30*time.Minute,
				Action:   ActionCreateCase,
				ActorRef: "reporter",
				Data: map[string]any{
					"caseType":    "content_report",
					"targetRef":   "author",
					"description": "Reporter alleges ticket scalping; post appears to be normal movie outing with separate ticket purchase",
				},
			},
		},
	}
}
