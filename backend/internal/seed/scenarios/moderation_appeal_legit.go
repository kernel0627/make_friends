package scenarios

import "time"

// ModerationAppealLegitimate — Post was incorrectly rejected by moderation.
// Author appeals. Agent should find the content snapshot, compare against policy,
// and determine the rejection was overzealous.
func ModerationAppealLegitimate() *Scenario {
	return &Scenario{
		ID:         "moderation_appeal_legit_01",
		CaseType:   "moderation_appeal",
		Difficulty: "medium",
		Summary:    "Author appeals moderation rejection claiming their post about a study group was incorrectly flagged as commercial",
		Truth: Truth{
			Outcome:          "upheld", // appeal upheld = post should be restored
			ResponsibleParty: "",       // system error, no user at fault
			PolicyRefs:       []string{"content_commercial"},
			RequiredEvidence: []string{"moderation_record", "content_snapshot", "user_history"},
			ForbiddenClaims:  []string{"commercial_intent", "policy_violation"},
		},
		Roles: []Role{
			{Ref: "author", Nickname: "学霸班长", CreditScore: 100},
		},
		Timeline: []Event{
			{
				Offset:   0,
				Action:   ActionCreatePost,
				ActorRef: "author",
				Data: map[string]any{
					"title":       "考研数学互助小组 免费答疑",
					"description": "考研倒计时150天！组个数学互助小组，每周三晚图书馆三楼讨论。我去年数学一130+，可以帮大家看看错题。完全免费，纯互助。需要的同学带好自己的资料就行。",
					"category":    "学习",
					"subCategory": "考试",
					"address":     "图书馆三楼研讨室",
					"maxCount":    6,
				},
			},
			// Post gets incorrectly rejected (triggered by "免费答疑" + "130+" being misread as advertising)
			{
				Offset:   30 * time.Minute,
				Action:   ActionModerationReject,
				ActorRef: "author",
				Data: map[string]any{
					"matchedPolicies": `["content_commercial"]`,
					"reason":          "Suspected commercial tutoring advertisement: mentions credentials and offers free service as potential lead generation",
				},
			},
			// Author appeals
			{
				Offset:   1 * time.Hour,
				Action:   ActionSendMessage,
				ActorRef: "author",
				Data: map[string]any{
					"content": "为什么我的帖子被驳回了？我就是想帮同学们复习数学，又没有收费",
				},
			},
			{
				Offset:   1*time.Hour + 30*time.Minute,
				Action:   ActionModerationAppeal,
				ActorRef: "author",
				Data: map[string]any{
					"targetRef": "author",
					"reason":    "Post is a genuine study group with no commercial intent. Author is sharing exam prep knowledge for free with classmates. No external links, no pricing, no off-platform solicitation.",
				},
			},
		},
	}
}
