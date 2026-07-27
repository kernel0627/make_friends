package scenarios

import "time"

// ContentRepeatReporter — A user with a history of 4 rejected reports files yet another
// false report on a legitimate study group post. The report claims "商业推广" but the post
// is clearly free peer tutoring. Agent should reject and note the reporter's pattern.
func ContentRepeatReporter() *Scenario {
	return &Scenario{
		ID:         "content_repeat_reporter_01",
		CaseType:   "content_report",
		Difficulty: "medium",
		Summary:    "Study group post reported for commercial promotion; reporter has history of rejected reports",
		Truth: Truth{
			Outcome:          "rejected",
			ResponsibleParty: "",
			PolicyRefs:       []string{"content_commercial"},
			RequiredEvidence: []string{"post_content", "reporter_history", "chat_messages"},
			ForbiddenClaims:  []string{"commercial_activity", "policy_violation", "off_platform_contact"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "英语角发起人", CreditScore: 100},
			{Ref: "reporter", Nickname: "举报专业户", CreditScore: 60}, // Very low credit from repeated false reports
			{Ref: "participant_1", Nickname: "四级备考生", CreditScore: 100},
			{Ref: "participant_2", Nickname: "口语练习者", CreditScore: 95},
		},
		Timeline: []Event{
			// Reporter's history: 4 previous false reports (domain events showing rejections)
			{
				Offset:   -14 * 24 * time.Hour, // 14 days ago
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetType": "post",
					"reason":     "这个人在卖东西",
				},
			},
			{
				Offset:   -14*24*time.Hour + time.Hour,
				Action:   ActionCreditPenalty,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetRef":  "reporter",
					"delta":      -10,
					"sourceType": "false_report_penalty_1",
					"note":       "Report rejected: no commercial activity found",
				},
			},
			{
				Offset:   -10 * 24 * time.Hour, // 10 days ago
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetType": "post",
					"reason":     "广告帖，在推销课程",
				},
			},
			{
				Offset:   -10*24*time.Hour + time.Hour,
				Action:   ActionCreditPenalty,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetRef":  "reporter",
					"delta":      -10,
					"sourceType": "false_report_penalty_2",
					"note":       "Report rejected: free tutoring is not commercial",
				},
			},
			{
				Offset:   -6 * 24 * time.Hour, // 6 days ago
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetType": "user",
					"reason":     "这个人每次都发广告",
				},
			},
			{
				Offset:   -6*24*time.Hour + time.Hour,
				Action:   ActionCreditPenalty,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetRef":  "reporter",
					"delta":      -10,
					"sourceType": "false_report_penalty_3",
					"note":       "Report rejected: targeted user has no violations",
				},
			},
			{
				Offset:   -3 * 24 * time.Hour, // 3 days ago
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetType": "message",
					"reason":     "群里发微信号，拉人去别的平台",
				},
			},
			{
				Offset:   -3*24*time.Hour + time.Hour,
				Action:   ActionCreditPenalty,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetRef":  "reporter",
					"delta":      -10,
					"sourceType": "false_report_penalty_4",
					"note":       "Report rejected: message contained dorm room number, not social media account",
				},
			},
			// Current legitimate post
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "英语角｜每周三晚免费练口语",
					"description": "每周三晚7点，食堂二楼角落聚一下练英语口语。话题自由，从日常对话到时事讨论都行。所有水平欢迎，不收费不卖课，纯粹想找人练口语。我是英语专业大三的，可以帮忙纠正发音。",
					"category":    "学习",
					"subCategory": "语言",
					"address":     "二食堂2楼东侧",
					"maxCount":    8,
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
				Offset:   3*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "太好了！我四级口语一直不太行，想练练日常对话",
				},
			},
			{
				Offset:   3*time.Hour + 40*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "欢迎欢迎！不用紧张，大家水平都差不多，主要是多开口",
				},
			},
			{
				Offset:   4 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_2",
				Data: map[string]any{
					"content": "我想练雅思口语part2，可以安排topic讨论吗",
				},
			},
			{
				Offset:   4*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "当然可以！每次可以轮流出话题，想练什么都行",
				},
			},
			// 5th false report
			{
				Offset:   6 * time.Hour,
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"targetType": "post",
					"reason":     "商业推广，打着免费旗号推销英语课",
				},
			},
			{
				Offset:   6*time.Hour + 30*time.Minute,
				Action:   ActionCreateCase,
				ActorRef: "reporter",
				Data: map[string]any{
					"caseType":    "content_report",
					"targetRef":   "author",
					"description": "Reporter with 4 prior rejected reports alleges commercial English tutoring promotion. Post appears to be free peer practice.",
				},
			},
		},
	}
}
