package api

import (
	"strings"
	"time"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

func (s *Server) refreshClosedPostDerivedState(post model.Post) error {
	if strings.TrimSpace(post.ID) == "" || post.Status != "closed" {
		return nil
	}
	return score.RecalculatePostActivityScores(s.DB, post.ID, time.Now().UnixMilli())
}

func (s *Server) refreshClosedPostsDerivedState(posts []model.Post) error {
	seen := make(map[string]struct{}, len(posts))
	for _, post := range posts {
		if post.Status != "closed" {
			continue
		}
		postID := strings.TrimSpace(post.ID)
		if postID == "" {
			continue
		}
		if _, ok := seen[postID]; ok {
			continue
		}
		seen[postID] = struct{}{}
		if err := score.RecalculatePostActivityScores(s.DB, postID, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func pendingSettlementCount(relations []model.PostParticipant, settlements map[string]model.PostParticipantSettlement) int {
	count := 0
	for _, relation := range relations {
		if settlementNeedsAttention(settlements[relation.UserID]) {
			count++
		}
	}
	return count
}

func hasAuthorSettlementWork(relations []model.PostParticipant, settlements map[string]model.PostParticipantSettlement) bool {
	return pendingSettlementCount(relations, settlements) > 0
}

func hasParticipantSettlementWork(viewerID string, settlements map[string]model.PostParticipantSettlement) bool {
	if strings.TrimSpace(viewerID) == "" {
		return false
	}
	return settlementNeedsAttention(settlements[viewerID])
}

func aggregateAuthorSettlementStatus(post model.Post, relations []model.PostParticipant, settlements map[string]model.PostParticipantSettlement) string {
	if post.CancelledAt > 0 {
		return score.SettlementCancelled
	}
	if post.Status != "closed" {
		return score.SettlementPending
	}
	if len(relations) == 0 {
		return score.SettlementCompleted
	}

	hasPending := false
	hasDispute := false
	hasCompleted := false
	hasNoShow := false
	allCancelled := true

	for _, relation := range relations {
		status := strings.TrimSpace(settlements[relation.UserID].FinalStatus)
		switch status {
		case score.SettlementPending:
			hasPending = true
			allCancelled = false
		case score.SettlementDisputed:
			hasDispute = true
			allCancelled = false
		case score.SettlementCompleted:
			hasCompleted = true
			allCancelled = false
		case score.SettlementNoShow:
			hasNoShow = true
			allCancelled = false
		case score.SettlementCancelled:
		default:
			allCancelled = false
		}
	}

	switch {
	case hasDispute:
		return score.SettlementDisputed
	case hasCompleted:
		return score.SettlementCompleted
	case hasPending:
		return score.SettlementPending
	case hasNoShow:
		return score.SettlementNoShow
	case allCancelled:
		return score.SettlementCancelled
	default:
		return score.SettlementPending
	}
}

func buildHomeSettlementState(
	post model.Post,
	subjectUserID string,
	relations []model.PostParticipant,
	settlements map[string]model.PostParticipantSettlement,
	reviewState reviewStateView,
) settlementStateView {
	state := settlementStateView{
		ReviewDeadlineAt: score.ReviewDeadlineAt(post),
		ProjectCancelled: post.CancelledAt > 0,
	}
	subjectID := strings.TrimSpace(subjectUserID)
	if subjectID == "" {
		return state
	}

	subjectIsAuthor := subjectID == strings.TrimSpace(post.AuthorID)
	if post.Status != "closed" {
		state.FlowLabel = "进行中"
		if subjectIsAuthor {
			state.FinalStatus = score.SettlementPending
			state.HasDispute = false
			return state
		}
		row := settlements[subjectID]
		state.FinalStatus = strings.TrimSpace(row.FinalStatus)
		state.HasDispute = state.FinalStatus == score.SettlementDisputed
		state.ParticipantDecision = strings.TrimSpace(row.ParticipantDecision)
		state.AuthorDecision = strings.TrimSpace(row.AuthorDecision)
		return state
	}

	stage := "done"
	if state.ProjectCancelled {
		stage = "cancelled"
	}

	if subjectIsAuthor {
		state.PendingMemberCount = pendingSettlementCount(relations, settlements)
		state.CanAuthorConfirm = post.Status == "closed" && !state.ProjectCancelled && state.PendingMemberCount > 0
		state.CanCancelAll = state.CanAuthorConfirm
		state.FinalStatus = aggregateAuthorSettlementStatus(post, relations, settlements)
		state.HasDispute = state.FinalStatus == score.SettlementDisputed
		if state.CanAuthorConfirm {
			stage = "settlement"
		} else if reviewState.CanReview {
			stage = "review"
		}
		state.FlowLabel = settlementFlowLabel(true, stage)
		state.CanOpenFlow = state.CanAuthorConfirm || reviewState.CanReview
		return state
	}

	state = buildSettlementItemState(post, subjectID, subjectID, settlements[subjectID])
	if state.ProjectCancelled {
		stage = "cancelled"
	} else if state.CanParticipantConfirm {
		stage = "settlement"
	} else if reviewState.CanReview {
		stage = "review"
	}
	state.FlowLabel = settlementFlowLabel(false, stage)
	state.CanOpenFlow = state.CanParticipantConfirm || reviewState.CanReview
	return state
}
