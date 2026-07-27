package scenarios

import "time"

// ContentCommercialReport — User posts a gathering that's actually commercial advertising.
// The reporter notices WeChat/QQ group links and pricing. The agent must find the original
// content snapshot, verify the commercial indicators, and check the content policy.
func ContentCommercialReport() *Scenario {
	return &Scenario{
		ID:         "content_commercial_01",
		CaseType:   "content_report",
		Difficulty: "easy",
		Summary:    "User reported activity post containing suspected commercial advertising content",
		Truth: Truth{
			Outcome:          "upheld",
			ResponsibleParty: "author",
			PolicyRefs:       []string{"content_commercial"},
			RequiredEvidence: []string{"content_snapshot", "chat_commercial_link"},
			ForbiddenClaims:  []string{"insufficient_evidence"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "卖课小王", CreditScore: 85},
			{Ref: "reporter", Nickname: "路人甲", CreditScore: 100},
			{Ref: "participant_1", Nickname: "同学A", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周末一起学摄影～免费教学",
					"description": "周末有空的同学一起来学摄影吧！我是专业摄影师，可以带你入门，器材不限。地点待定，人多的话可以租个场地。",
					"category":    "学习",
					"subCategory": "技能交流",
					"address":     "校园咖啡厅",
					"maxCount":    6,
				},
			},
			{
				Offset:   1 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_1",
			},
			{
				Offset:   2 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "欢迎加入！先加我微信 photo_master_2026，我拉你进学习群",
				},
			},
			{
				Offset:   2*time.Hour + 5*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "对了，基础课是免费的，进阶课程的话有个小费用，具体看群公告～",
				},
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "participant_1",
				Data: map[string]any{
					"content": "所以还是收费的啊？",
				},
			},
			{
				Offset:   3*time.Hour + 10*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "进阶部分是的，详情看这个链接 https://photo-course.taobao.com/item/123456 早鸟价299",
				},
			},
			{
				Offset:   4 * time.Hour,
				Action:   ActionUpdatePost,
				ActorRef: "author",
				Data: map[string]any{
					"description": "周末摄影学习！免费基础课+可选进阶（进阶课优惠中）。加微信 photo_master_2026 进群。",
				},
			},
			{
				Offset:   5 * time.Hour,
				Action:   ActionCreateReport,
				ActorRef: "reporter",
				Data: map[string]any{
					"reason":     "这个帖子明显是在卖课打广告，不是真正的交友活动",
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
					"description": "Post contains commercial advertising (external payment link, WeChat solicitation)",
				},
			},
		},
	}
}
