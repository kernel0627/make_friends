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
	postsVersionKey    = "zgbe:cache:version:posts"
	hotPostsKeyPattern = "zgbe:post:list:hot:page:%d:v%s"
	postDetailPattern  = "zgbe:post:detail:%s:v%s"
	postsListTTL       = 45 * time.Second
	postDetailTTL      = 90 * time.Second
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

func (s *Server) canUsePostsListCache(cachingCtx interface{ Query(string) string }, sortBy string) bool {
	if !s.UseRedis || s.RedisClient == nil {
		return false
	}
	if sortBy != "hot" {
		return false
	}
	return strings.TrimSpace(cachingCtx.Query("category")) == "" &&
		strings.TrimSpace(cachingCtx.Query("subCategory")) == "" &&
		strings.TrimSpace(cachingCtx.Query("keyword")) == ""
}

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

func (s *Server) getCachedHotPosts(ctx context.Context, page int) ([]model.Post, bool) {
	version := s.postsVersion(ctx)
	key := fmt.Sprintf(hotPostsKeyPattern, page, version)
	raw, err := s.RedisClient.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}
	var posts []model.Post
	if err := json.Unmarshal([]byte(raw), &posts); err != nil {
		return nil, false
	}
	return posts, true
}

func (s *Server) setCachedHotPosts(ctx context.Context, page int, posts []model.Post) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	version := s.postsVersion(ctx)
	key := fmt.Sprintf(hotPostsKeyPattern, page, version)
	raw, err := json.Marshal(posts)
	if err != nil {
		return
	}
	if err := s.RedisClient.Set(ctx, key, raw, postsListTTL).Err(); err != nil {
		log.Printf("redis set posts list cache failed: %v", err)
	}
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
