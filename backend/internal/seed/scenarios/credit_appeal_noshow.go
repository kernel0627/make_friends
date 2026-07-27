package scenarios

import "time"

// CreditAppealAfterNoShow — User was penalized for no-show, appeals claiming they
// notified in advance but the author didn't cancel them. Agent investigates chat history.
func CreditAppealAfterNoShow() *Scenario {
	return &Scenario{
		ID:         "credit_appeal_noshow_01",
		CaseType:   "credit_appeal",
		Difficulty: "medium",
		Summary:    "User appeals credit penalty for no-show claiming they gave advance notice of inability to attend",
		Truth: Truth{
			Outcome:          "upheld", // appeal upheld = penalty should be reversed
			ResponsibleParty: "",
			PolicyRefs:       []string{"settlement_no_show", "credit_reversal"},
			RequiredEvidence: []string{"chat_advance_notice", "credit_ledger_entry", "no_cancel_by_author"},
			ForbiddenClaims:  []string{"no_notice_given"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "组织者老李", CreditScore: 100},
			{Ref: "appellant", Nickname: "被罚同学", CreditScore: 90},
			{Ref: "participant_2", Nickname: "其他参与者", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "周五晚羽毛球双打",
					"description": "体育馆羽毛球场，打两小时。需要4人凑双打。自带球拍，球我来买。",
					"category":    "运动",
					"subCategory": "羽毛球",
					"address":     "校体育馆3号场",
					"maxCount":    4,
				},
			},
			{
				Offset:   1 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "appellant",
			},
			{
				Offset:   2 * time.Hour,
				Action:   ActionJoinPost,
				ActorRef: "participant_2",
			},
			{
				Offset:   3 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "期待周五！好久没打了",
				},
			},
			// Day before — appellant notifies can't make it
			{
				Offset:   20 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "appellant",
				Data: map[string]any{
					"content": "不好意思，我明天临时要加班，来不了了。你能帮我取消一下吗？还是我自己取消？",
				},
			},
			{
				Offset:   20*time.Hour + 30*time.Minute,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "啊这样啊…行吧我再找人",
				},
			},
			// Author never actually cancels the participant. No cancel action in timeline.
			// Event happens, participant is "no-show"
			{
				Offset:   28 * time.Hour,
				Action:   ActionClosePost,
				ActorRef: "author",
			},
			{
				Offset:   29 * time.Hour,
				Action:   ActionSubmitSettlement,
				ActorRef: "author",
				Data: map[string]any{
					"role":      "author",
					"decision":  "no_show",
					"note":      "没来",
					"targetRef": "appellant",
				},
			},
			// System auto-penalizes
			{
				Offset:   29*time.Hour + 30*time.Minute,
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
				Offset:   30 * time.Hour,
				Action:   ActionCreateCase,
				ActorRef: "appellant",
				Data: map[string]any{
					"caseType":    "credit_appeal",
					"targetRef":   "appellant",
					"description": "I told the organizer the day before that I couldn't make it. Check chat history. They acknowledged but never cancelled me.",
				},
			},
		},
	}
}
