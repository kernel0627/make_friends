package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"make_friends/backend/internal/model"
)

const (
	postsVersionKey   = "zgbe:cache:version:posts"
	postDetailPattern = "zgbe:post:detail:%s:v%s"
	postDetailTTL     = 90 * time.Second
)

type cachedPostDetail struct {
	Post         model.Post              `json:"post"`
	Participants []model.PostParticipant `json:"participants"`
}

func queryIntOrDefault(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// NOTE: there is deliberately no cache for the hot post list.
//
// A previous version of this file carried canUsePostsListCache /
// getCachedHotPosts / setCachedHotPosts, keyed only by page. They were never
// called, and calling them would have been a bug: the hot feed is ranked per
// viewer (tags, click history, embedding and location, see
// buildRecommendedFeed), so a page-keyed entry would serve one user's
// personalised ordering to everyone else. The cost problem they were meant to
// solve is handled instead by bounding the candidate set in ListPosts.

func (s *Server) postsVersion(ctx context.Context) string {
	if !s.UseRedis || s.RedisClient == nil {
		return "1"
	}
	if err := s.RedisClient.SetNX(ctx, postsVersionKey, "1", 0).Err(); err != nil {
		log.Printf("redis setnx posts version failed: %v", err)
		return "1"
	}
	v, err := s.RedisClient.Get(ctx, postsVersionKey).Result()
	if err != nil || strings.TrimSpace(v) == "" {
		return "1"
	}
	return strings.TrimSpace(v)
}

func (s *Server) getCachedPostDetail(ctx context.Context, postID string) (model.Post, []model.PostParticipant, bool) {
	if !s.UseRedis || s.RedisClient == nil {
		return model.Post{}, nil, false
	}
	version := s.postsVersion(ctx)
	key := fmt.Sprintf(postDetailPattern, postID, version)
	raw, err := s.RedisClient.Get(ctx, key).Result()
	if err != nil {
		return model.Post{}, nil, false
	}
	var value cachedPostDetail
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return model.Post{}, nil, false
	}
	return value.Post, value.Participants, true
}

func (s *Server) setCachedPostDetail(ctx context.Context, post model.Post, participants []model.PostParticipant) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	version := s.postsVersion(ctx)
	key := fmt.Sprintf(postDetailPattern, post.ID, version)
	value := cachedPostDetail{Post: post, Participants: participants}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if err := s.RedisClient.Set(ctx, key, raw, postDetailTTL).Err(); err != nil {
		log.Printf("redis set post detail cache failed: %v", err)
	}
}

func (s *Server) invalidatePostsCache(ctx context.Context) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	if err := s.RedisClient.Incr(ctx, postsVersionKey).Err(); err != nil {
		log.Printf("redis bump posts cache version failed: %v", err)
	}
}
