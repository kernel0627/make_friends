package seed

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/recommend"
	"make_friends/backend/internal/score"
)

const (
	defaultFullUsers         = 77
	defaultFullPosts         = 500
	defaultFullFeedPerUser   = 15
	defaultFullFeedPageSize  = 20
	closedPostReviewTargets  = 2
	closedPostRatioNumerator = 7
	closedPostRatioDenom     = 20
)

type FullOptions struct {
	Reset bool
}

type fullPersona struct {
	Name               string
	PrimaryTemplates   []int
	SecondaryTemplates []int
	ExploreTemplates   []int
	CityLocations      []int
}

type seededPost struct {
	Post          model.Post
	TemplateIndex int
	LocationIndex int
	PersonaIndex  int
}

var fullPersonas = []fullPersona{
	{Name: "campus-racket", PrimaryTemplates: []int{0, 1}, SecondaryTemplates: []int{2}, ExploreTemplates: []int{4, 8}, CityLocations: []int{0, 1, 2}},
	{Name: "endurance-outdoor", PrimaryTemplates: []int{2, 3}, SecondaryTemplates: []int{0}, ExploreTemplates: []int{8, 12}, CityLocations: []int{3, 4, 5}},
	{Name: "nightlife-music", PrimaryTemplates: []int{4, 6}, SecondaryTemplates: []int{7}, ExploreTemplates: []int{1, 13}, CityLocations: []int{3, 4, 5}},
	{Name: "games-table", PrimaryTemplates: []int{5, 7}, SecondaryTemplates: []int{4}, ExploreTemplates: []int{6, 12}, CityLocations: []int{6, 7}},
	{Name: "reading-exam", PrimaryTemplates: []int{8, 9}, SecondaryTemplates: []int{10}, ExploreTemplates: []int{2, 11}, CityLocations: []int{10, 11}},
	{Name: "coding-project", PrimaryTemplates: []int{10, 11}, SecondaryTemplates: []int{8}, ExploreTemplates: []int{3, 9}, CityLocations: []int{12, 13}},
	{Name: "pet-photo", PrimaryTemplates: []int{12, 15}, SecondaryTemplates: []int{14}, ExploreTemplates: []int{6, 8}, CityLocations: []int{14}},
	{Name: "food-market", PrimaryTemplates: []int{13, 14}, SecondaryTemplates: []int{12}, ExploreTemplates: []int{5, 15}, CityLocations: []int{15}},
	{Name: "sport-social", PrimaryTemplates: []int{1, 4}, SecondaryTemplates: []int{5}, ExploreTemplates: []int{0, 6}, CityLocations: []int{8, 9}},
	{Name: "study-sports", PrimaryTemplates: []int{2, 9}, SecondaryTemplates: []int{8}, ExploreTemplates: []int{10, 3}, CityLocations: []int{8, 9}},
	{Name: "city-discovery", PrimaryTemplates: []int{6, 13}, SecondaryTemplates: []int{14}, ExploreTemplates: []int{4, 15}, CityLocations: []int{12, 13}},
	{Name: "outdoor-learning", PrimaryTemplates: []int{3, 10}, SecondaryTemplates: []int{11}, ExploreTemplates: []int{2, 8}, CityLocations: []int{14, 15}},
}

func RunFull(db *gorm.DB, opt FullOptions) (Result, error) {
	if opt.Reset {
		if err := clearAllBusinessTables(db); err != nil {
			return Result{}, err
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return Result{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UnixMilli()
	users := buildSeedUsers(defaultFullUsers, string(hash), now, true)
	if err := db.Create(&users).Error; err != nil {
		return Result{}, fmt.Errorf("create users: %w", err)
	}

	normalUsers := make([]model.User, 0, len(users))
	for _, user := range users {
		if user.Role == model.UserRoleAdmin || user.DeletedAt > 0 {
			continue
		}
		normalUsers = append(normalUsers, user)
	}

	personaByUserID := buildPersonaByUser(normalUsers)
	behaviorProfiles := buildSeedBehaviorProfiles(normalUsers, personaByUserID, now)
	if len(behaviorProfiles) > 0 {
		if err := db.CreateInBatches(behaviorProfiles, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create user behavior profiles: %w", err)
		}
	}
	seededPosts, participants, messages, reviews := buildFullPosts(normalUsers, personaByUserID, now)

	posts := make([]model.Post, 0, len(seededPosts))
	closedPostIDs := make([]string, 0, len(seededPosts)/3)
	for _, item := range seededPosts {
		posts = append(posts, item.Post)
		if item.Post.Status == "closed" {
			closedPostIDs = append(closedPostIDs, item.Post.ID)
		}
	}

	if err := db.CreateInBatches(posts, 120).Error; err != nil {
		return Result{}, fmt.Errorf("create posts: %w", err)
	}
	if len(participants) > 0 {
		if err := db.CreateInBatches(participants, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create participants: %w", err)
		}
	}
	if len(messages) > 0 {
		if err := db.CreateInBatches(messages, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create messages: %w", err)
		}
	}
	if len(reviews) > 0 {
		if err := db.CreateInBatches(reviews, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create reviews: %w", err)
		}
	}
	for _, postID := range closedPostIDs {
		if err := score.RecalculatePostActivityScores(db, postID, now); err != nil {
			return Result{}, fmt.Errorf("recalc activity scores for %s: %w", postID, err)
		}
	}
	if err := injectSeedDisputeCases(db, seededPosts, participants, users, now); err != nil {
		return Result{}, fmt.Errorf("inject dispute cases: %w", err)
	}

	exposures, clicks := buildFullFeedLogs(normalUsers, personaByUserID, seededPosts, now)
	if len(exposures) > 0 {
		if err := db.CreateInBatches(exposures, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create exposures: %w", err)
		}
	}
	if len(clicks) > 0 {
		if err := db.CreateInBatches(clicks, 200).Error; err != nil {
			return Result{}, fmt.Errorf("create clicks: %w", err)
		}
	}

	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	if err := recommend.EnsureDefaultRecommendationModel(db, now); err != nil {
		return Result{}, fmt.Errorf("init default recommendation model: %w", err)
	}
	if err := recommend.RebuildUserTags(db, userIDs, now); err != nil {
		return Result{}, fmt.Errorf("rebuild user tags: %w", err)
	}

	return Result{
		Users:            len(users),
		BehaviorProfiles: len(behaviorProfiles),
		Posts:            len(posts),
		Participants:     len(participants),
		Messages:         len(messages),
		Reviews:          len(reviews),
		Exposures:        len(exposures),
		Clicks:           len(clicks),
	}, nil
}

func injectSeedDisputeCases(db *gorm.DB, posts []seededPost, participants []model.PostParticipant, users []model.User, now int64) error {
	type disputeSpec struct {
		CaseStatus string
		Resolution string
	}
	specs := []disputeSpec{
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "open"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "in_review"},
		{CaseStatus: "resolved", Resolution: score.SettlementCompleted},
		{CaseStatus: "resolved", Resolution: score.SettlementCompleted},
		{CaseStatus: "resolved", Resolution: score.SettlementCancelled},
		{CaseStatus: "resolved", Resolution: score.SettlementCancelled},
		{CaseStatus: "resolved", Resolution: score.SettlementNoShow},
		{CaseStatus: "resolved", Resolution: score.SettlementNoShow},
		{CaseStatus: "resolved", Resolution: score.SettlementCompleted},
	}

	participantByPost := make(map[string][]string)
	for _, relation := range participants {
		participantByPost[relation.PostID] = append(participantByPost[relation.PostID], relation.UserID)
	}
	adminID := ""
	reviewerAdminID := ""
	for _, user := range users {
		if user.Nickname == "admin" {
			adminID = user.ID
		}
		if user.Nickname == "admin1" {
			reviewerAdminID = user.ID
		}
	}
	if adminID == "" {
		return nil
	}
	if reviewerAdminID == "" {
		reviewerAdminID = adminID
	}

	closedPosts := make([]seededPost, 0, len(posts))
	for _, post := range posts {
		if post.Post.Status == "closed" && post.Post.DeletedAt == 0 && len(participantByPost[post.Post.ID]) > 0 {
			closedPosts = append(closedPosts, post)
		}
	}
	if len(closedPosts) == 0 {
		return nil
	}

	participantNotes := []string{
		"我提前十分钟到场，还在群里发过定位和现场照片，希望后台帮忙核实。",
		"我到了集合点后等了很久，也有聊天截图可以证明自己确实到场。",
		"现场我已经签到并和其他参与者打过招呼，发起人的记录和实际情况不一致。",
		"我在群里说过自己已经到了入口附近，想请管理员结合聊天记录再确认一次。",
	}
	authorNotes := []string{
		"发起人点名时没有看到这位参与者到场，现场签到记录也对不上。",
		"根据当时群内沟通，这位参与者迟迟没有出现，所以先标记为异常待复核。",
		"现场等待超过约定时间仍未见人，记录为未按约到场，后续交由后台确认。",
		"发起人认为对方并未完成签到，但聊天记录存在分歧，需要后台统一判断。",
	}
	resolutionNotes := map[string]string{
		score.SettlementCompleted: "管理员核对聊天、签到说明和现场反馈后，确认参与者已正常到场并完成活动。",
		score.SettlementCancelled: "管理员确认双方已协商取消，本次活动按取消结算，不再按爽约处理。",
		score.SettlementNoShow:    "管理员综合聊天记录、签到缺失和现场反馈，最终判定为爽约。",
	}

	for index, spec := range specs {
		post := closedPosts[index%len(closedPosts)].Post
		candidates := participantByPost[post.ID]
		targetUserID := candidates[index%len(candidates)]
		baseTime := maxInt64(post.CreatedAt+int64((30+index*3))*int64(time.Hour/time.Millisecond), now-int64((110-index*3))*int64(time.Hour/time.Millisecond))
		row := model.PostParticipantSettlement{
			PostID:                 post.ID,
			UserID:                 targetUserID,
			ParticipantDecision:    score.DecisionCompleted,
			AuthorDecision:         score.DecisionNoShow,
			ParticipantNote:        participantNotes[index%len(participantNotes)],
			AuthorNote:             authorNotes[index%len(authorNotes)],
			ParticipantConfirmedAt: baseTime,
			AuthorConfirmedAt:      baseTime + 100,
			CreatedAt:              baseTime,
			UpdatedAt:              baseTime + 100,
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "post_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"participant_decision":     row.ParticipantDecision,
				"author_decision":          row.AuthorDecision,
				"participant_note":         row.ParticipantNote,
				"author_note":              row.AuthorNote,
				"participant_confirmed_at": row.ParticipantConfirmedAt,
				"author_confirmed_at":      row.AuthorConfirmedAt,
				"updated_at":               row.UpdatedAt,
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if err := score.RecalculatePostActivityScores(db, post.ID, baseTime+200); err != nil {
			return err
		}

		var adminCase model.AdminCase
		if err := db.Where("post_id = ? AND target_user_id = ? AND case_type = ?", post.ID, targetUserID, score.AdminCaseSettlementDispute).
			Order("created_at DESC").
			First(&adminCase).Error; err != nil {
			return err
		}

		switch spec.CaseStatus {
		case "in_review":
			if err := db.Model(&model.AdminCase{}).Where("id = ?", adminCase.ID).Updates(map[string]any{
				"status":     "in_review",
				"updated_at": baseTime + 300,
			}).Error; err != nil {
				return err
			}
		case "resolved":
			settlementUpdates := map[string]any{
				"updated_at":  baseTime + 400,
				"settled_at":  baseTime + 400,
				"author_note": "管理员已根据聊天记录、签到说明和现场反馈给出最终结论。",
			}
			switch spec.Resolution {
			case score.SettlementCompleted:
				settlementUpdates["participant_decision"] = score.DecisionCompleted
				settlementUpdates["author_decision"] = score.DecisionCompleted
			case score.SettlementCancelled:
				settlementUpdates["participant_decision"] = score.DecisionCancelled
				settlementUpdates["author_decision"] = score.DecisionCancelled
			default:
				settlementUpdates["author_decision"] = score.DecisionNoShow
				settlementUpdates["final_status"] = score.SettlementNoShow
			}
			if err := db.Model(&model.PostParticipantSettlement{}).
				Where("post_id = ? AND user_id = ?", post.ID, targetUserID).
				Updates(settlementUpdates).Error; err != nil {
				return err
			}
			if err := score.RecalculatePostActivityScores(db, post.ID, baseTime+500); err != nil {
				return err
			}
			if err := db.Model(&model.AdminCase{}).Where("id = ?", adminCase.ID).Updates(map[string]any{
				"status":           "resolved",
				"resolver_user_id": reviewerAdminID,
				"resolution":       spec.Resolution,
				"resolution_note":  resolutionNotes[spec.Resolution],
				"resolved_at":      baseTime + 600,
				"updated_at":       baseTime + 600,
			}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func buildFullPosts(users []model.User, personaByUserID map[string]int, now int64) ([]seededPost, []model.PostParticipant, []model.ChatMessage, []model.Review) {
	posts := make([]seededPost, 0, defaultFullPosts)
	participants := make([]model.PostParticipant, 0, 1800)
	messages := make([]model.ChatMessage, 0, 1800)
	reviews := make([]model.Review, 0, 800)
	startAt := now - int64(120*24*time.Hour/time.Millisecond)
	step := int64((112 * 24 * time.Hour / time.Millisecond) / time.Duration(defaultFullPosts))
	if step <= 0 {
		step = int64(4 * time.Hour / time.Millisecond)
	}

	for i := 0; i < defaultFullPosts; i++ {
		author := users[(i*7)%len(users)]
		personaIndex := personaByUserID[author.ID]
		persona := fullPersonas[personaIndex]
		templateIndex := weightedTemplateIndex(persona, i)
		locationIndex := weightedLocationIndex(persona, i)
		content := buildSeedActivityFromChoice(templateIndex, locationIndex, i, i+personaIndex, author)

		postID := fmt.Sprintf("post_seed_%03d", i+1)
		participantCount := 2 + (i % 5)
		maxCount := participantCount + 2 + (i % 4)
		createdAt := startAt + int64(i)*step + int64((i%6)*37)*int64(time.Minute/time.Millisecond)
		timeMode := "range"
		timeDays := 2 + (i % 12)
		fixedTime := ""
		status := "open"
		if i%closedPostRatioDenom < closedPostRatioNumerator {
			status = "closed"
		}
		if i%3 == 0 {
			timeMode = "fixed"
			timeDays = 0
			if status == "closed" {
				closedTs := createdAt + int64((30+i%80))*int64(time.Hour/time.Millisecond)
				if closedTs > now-int64(2*time.Hour/time.Millisecond) {
					closedTs = now - int64((10+i%60))*int64(time.Hour/time.Millisecond)
				}
				fixedTime = time.UnixMilli(closedTs).Format(time.RFC3339)
			} else {
				futureTs := now + int64((18+i%168))*int64(time.Hour/time.Millisecond)
				fixedTime = time.UnixMilli(futureTs).Format(time.RFC3339)
			}
		}
		deletedAt := int64(0)
		deletedBy := ""
		if i > 0 && i%41 == 0 {
			deletedAt = createdAt + int64((12+i%16))*int64(time.Hour/time.Millisecond)
			deletedBy = "system_seed"
		}

		post := model.Post{
			ID:           postID,
			AuthorID:     author.ID,
			Title:        content.Title,
			Description:  content.Description,
			Category:     content.Category,
			SubCategory:  content.SubCategory,
			TimeMode:     timeMode,
			TimeDays:     timeDays,
			FixedTime:    fixedTime,
			Address:      content.Address,
			Lat:          content.Lat,
			Lng:          content.Lng,
			MaxCount:     maxCount,
			CurrentCount: participantCount + 1,
			Status:       status,
			DeletedAt:    deletedAt,
			DeletedBy:    deletedBy,
			CreatedAt:    createdAt,
			UpdatedAt:    maxInt64(createdAt, deletedAt),
		}
		posts = append(posts, seededPost{
			Post:          post,
			TemplateIndex: templateIndex,
			LocationIndex: locationIndex,
			PersonaIndex:  personaIndex,
		})

		participantUsers := selectParticipantUsers(users, personaByUserID, author.ID, templateIndex, participantCount, i)
		for idx, participant := range participantUsers {
			participants = append(participants, model.PostParticipant{
				PostID:   postID,
				UserID:   participant.ID,
				JoinedAt: createdAt + int64((idx+1)*25),
			})
		}

		msgCount := 2 + (i % 4)
		messages = append(messages, buildSeedMessages(post, author, participantUsers, msgCount, i)...)

		if status == "closed" {
			reviewTargets := closedPostReviewTargets
			if reviewTargets > len(participantUsers) {
				reviewTargets = len(participantUsers)
			}
			for j := 0; j < reviewTargets; j++ {
				participant := participantUsers[j]
				reviewTime := maxInt64(createdAt+int64((j+1)*400), now-int64((i%96))*int64(time.Hour/time.Millisecond))
				reviews = append(reviews, buildSeedReview(postID, participant.ID, author.ID, reviewTime, i+j))
				reviews = append(reviews, buildSeedReview(postID, author.ID, participant.ID, reviewTime+40, i+j+1))
			}
		}
	}

	return posts, participants, messages, reviews
}

func buildFullFeedLogs(users []model.User, personaByUserID map[string]int, posts []seededPost, now int64) ([]model.FeedExposure, []model.FeedClick) {
	activePosts := make([]seededPost, 0, len(posts))
	for _, post := range posts {
		if post.Post.DeletedAt == 0 {
			activePosts = append(activePosts, post)
		}
	}
	if len(activePosts) == 0 {
		return nil, nil
	}

	exposures := make([]model.FeedExposure, 0, len(users)*defaultFullFeedPerUser*defaultFullFeedPageSize)
	clicks := make([]model.FeedClick, 0, len(users)*defaultFullFeedPerUser*4)
	baseTime := now - int64(90*24*time.Hour/time.Millisecond)
	requestStep := int64((85 * 24 * time.Hour / time.Millisecond) / time.Duration(max(1, len(users)*defaultFullFeedPerUser)))
	if requestStep <= 0 {
		requestStep = int64(30 * time.Minute / time.Millisecond)
	}

	for userIndex, user := range users {
		persona := fullPersonas[personaByUserID[user.ID]]
		sessionID := fmt.Sprintf("session_seed_%03d", userIndex+1)
		for round := 0; round < defaultFullFeedPerUser; round++ {
			requestID := fmt.Sprintf("feed_seed_%03d_%02d", userIndex+1, round+1)
			sortedPosts := rankSeedPostsForUser(persona, user.ID, activePosts, round)
			limit := defaultFullFeedPageSize
			if limit > len(sortedPosts) {
				limit = len(sortedPosts)
			}
			requestTime := baseTime + int64(userIndex*defaultFullFeedPerUser+round)*requestStep
			for pos := 0; pos < limit; pos++ {
				post := sortedPosts[pos]
				scoreValue := seedFeedScore(persona, user.ID, post, pos, round)
				exposures = append(exposures, model.FeedExposure{
					RequestID: requestID,
					UserID:    user.ID,
					PostID:    post.Post.ID,
					Position:  pos + 1,
					Strategy:  "seed_feed",
					Score:     scoreValue,
					SessionID: sessionID,
					CreatedAt: requestTime + int64(pos),
				})
				if shouldSeedClick(persona, user.ID, post, pos, round) {
					clicks = append(clicks, model.FeedClick{
						RequestID: requestID,
						UserID:    user.ID,
						PostID:    post.Post.ID,
						Position:  pos + 1,
						Strategy:  "seed_feed",
						Score:     scoreValue,
						SessionID: sessionID,
						CreatedAt: requestTime + int64(pos) + 200,
					})
				}
			}
		}
	}
	return exposures, clicks
}

func selectParticipantUsers(users []model.User, personaByUserID map[string]int, authorID string, templateIndex, count, seed int) []model.User {
	type candidate struct {
		User     model.User
		Affinity int
		Bias     int
	}
	candidates := make([]candidate, 0, len(users))
	for idx, user := range users {
		if user.ID == authorID {
			continue
		}
		persona := fullPersonas[personaByUserID[user.ID]]
		candidates = append(candidates, candidate{
			User:     user,
			Affinity: templateAffinity(persona, templateIndex),
			Bias:     positiveModulo(idx-seed, len(users)),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Affinity != candidates[j].Affinity {
			return candidates[i].Affinity < candidates[j].Affinity
		}
		return candidates[i].Bias < candidates[j].Bias
	})
	result := make([]model.User, 0, count)
	for _, item := range candidates {
		result = append(result, item.User)
		if len(result) == count {
			break
		}
	}
	return result
}

func rankSeedPostsForUser(persona fullPersona, viewerID string, posts []seededPost, round int) []seededPost {
	out := make([]seededPost, len(posts))
	copy(out, posts)
	sort.SliceStable(out, func(i, j int) bool {
		left := seedFeedScore(persona, viewerID, out[i], i, round)
		right := seedFeedScore(persona, viewerID, out[j], j, round)
		if left == right {
			return out[i].Post.CreatedAt > out[j].Post.CreatedAt
		}
		return left > right
	})
	return out
}

func seedFeedScore(persona fullPersona, viewerID string, post seededPost, position, round int) float64 {
	affinity := templateAffinity(persona, post.TemplateIndex)
	score := 0.0
	switch affinity {
	case 0:
		score += 1.0
	case 1:
		score += 0.72
	case 2:
		score += 0.46
	default:
		score += 0.18
	}
	if viewerID == post.Post.AuthorID {
		score -= 0.2
	}
	if post.Post.Status == "closed" {
		score -= 0.1
	}
	score += 0.05 * float64(len(persona.CityLocations)-positiveModulo(position+round, len(persona.CityLocations)+1))
	score += 0.03 * float64(post.Post.CurrentCount) / 6
	return score
}

func shouldSeedClick(persona fullPersona, viewerID string, post seededPost, position, round int) bool {
	if viewerID == post.Post.AuthorID {
		return false
	}
	threshold := 4
	switch templateAffinity(persona, post.TemplateIndex) {
	case 0:
		threshold = 20
	case 1:
		threshold = 13
	case 2:
		threshold = 8
	default:
		threshold = 3
	}
	if post.Post.Status == "open" && post.Post.CurrentCount < post.Post.MaxCount {
		threshold += 2
	}
	if post.Post.TimeMode == "fixed" {
		threshold += 1
	}
	code := positiveModulo(len(viewerID)*31+post.TemplateIndex*17+position*13+round*19, 100)
	return code < threshold
}

func weightedTemplateIndex(persona fullPersona, seq int) int {
	switch seq % 10 {
	case 0, 1, 2, 3, 4, 5, 6:
		return persona.PrimaryTemplates[positiveModulo(seq, len(persona.PrimaryTemplates))]
	case 7, 8:
		return persona.SecondaryTemplates[positiveModulo(seq, len(persona.SecondaryTemplates))]
	default:
		return persona.ExploreTemplates[positiveModulo(seq, len(persona.ExploreTemplates))]
	}
}

func weightedLocationIndex(persona fullPersona, seq int) int {
	return persona.CityLocations[positiveModulo(seq, len(persona.CityLocations))]
}

func templateAffinity(persona fullPersona, templateIndex int) int {
	if containsInt(persona.PrimaryTemplates, templateIndex) {
		return 0
	}
	if containsInt(persona.SecondaryTemplates, templateIndex) {
		return 1
	}
	if containsInt(persona.ExploreTemplates, templateIndex) {
		return 2
	}
	return 3
}

func buildPersonaByUser(users []model.User) map[string]int {
	result := make(map[string]int, len(users))
	for idx, user := range users {
		personaIndex := idx / 4
		if personaIndex >= len(fullPersonas) {
			personaIndex = positiveModulo(idx, len(fullPersonas))
		}
		result[user.ID] = personaIndex
	}
	return result
}

func containsInt(list []int, target int) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func clearAllBusinessTables(db *gorm.DB) error {
	if err := db.Where("1 = 1").Delete(&model.UserEmbedding{}).Error; err != nil {
		return fmt.Errorf("clear user_embeddings: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.PostEmbedding{}).Error; err != nil {
		return fmt.Errorf("clear post_embeddings: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.UserTag{}).Error; err != nil {
		return fmt.Errorf("clear user_tags: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.UserBehaviorProfile{}).Error; err != nil {
		return fmt.Errorf("clear user_behavior_profiles: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.FeedClick{}).Error; err != nil {
		return fmt.Errorf("clear feed_clicks: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.FeedExposure{}).Error; err != nil {
		return fmt.Errorf("clear feed_exposures: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.RecommendationModel{}).Error; err != nil {
		return fmt.Errorf("clear recommendation_models: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.AdminCase{}).Error; err != nil {
		return fmt.Errorf("clear admin_cases: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.CreditLedger{}).Error; err != nil {
		return fmt.Errorf("clear credit_ledgers: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.PostParticipantSettlement{}).Error; err != nil {
		return fmt.Errorf("clear post_participant_settlements: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.ActivityScore{}).Error; err != nil {
		return fmt.Errorf("clear activity_scores: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.Review{}).Error; err != nil {
		return fmt.Errorf("clear reviews: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.ChatMessage{}).Error; err != nil {
		return fmt.Errorf("clear chat_messages: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.PostParticipant{}).Error; err != nil {
		return fmt.Errorf("clear post_participants: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.Post{}).Error; err != nil {
		return fmt.Errorf("clear posts: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("clear refresh_tokens: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.RevokedAccessToken{}).Error; err != nil {
		return fmt.Errorf("clear revoked_access_tokens: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&model.User{}).Error; err != nil {
		return fmt.Errorf("clear users: %w", err)
	}
	return nil
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
