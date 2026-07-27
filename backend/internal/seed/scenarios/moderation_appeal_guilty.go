package scenarios

import "time"

// ModerationAppealGuilty — Author appeals moderation rejection but the content genuinely
// violates policy. Agent should reject the appeal.
func ModerationAppealGuilty() *Scenario {
	return &Scenario{
		ID:         "moderation_appeal_guilty_01",
		CaseType:   "moderation_appeal",
		Difficulty: "medium",
		Summary:    "Author appeals moderation rejection; post contains clear off-platform solicitation with external group link",
		Truth: Truth{
			Outcome:          "rejected",
			ResponsibleParty: "author",
			PolicyRefs:       []string{"content_off_platform"},
			RequiredEvidence: []string{"content_snapshot", "moderation_record"},
			ForbiddenClaims:  []string{"false_positive", "system_error"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "社群达人", CreditScore: 80},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "读书会招新 每周共读一本书",
					"description": "我们有一个50人的读书会QQ群（群号：87654321），每周共读一本书然后讨论。想加入的直接搜群号加入就行！这里只是发个通知。",
					"category":    "学习",
					"subCategory": "读书",
					"address":     "线上为主",
					"maxCount":    10,
				},
			},
			// Moderation catches the QQ group solicitation
			{
				Offset:   20 * time.Minute,
				Action:   ActionModerationReject,
				ActorRef: "author",
				Data: map[string]any{
					"matchedPolicies": `["content_off_platform"]`,
					"reason":          "Post contains explicit off-platform group recruitment (QQ group number) and states platform is only used for notification, not actual activity coordination",
				},
			},
			// Author appeals
			{
				Offset:   1 * time.Hour,
				Action:   ActionModerationAppeal,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef": "author",
					"reason":    "读书会是正常的学习活动啊，又没有收费，为什么不让发？",
				},
			},
		},
	}
}
