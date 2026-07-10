package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"make_friends/backend/internal/db"
	"make_friends/backend/internal/seed"
)

func main() {
	var reset bool
	flag.BoolVar(&reset, "reset", true, "seed before clearing business tables")
	flag.Parse()

	database, err := db.OpenSQLite()
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}

	result, err := seed.RunFull(database, seed.FullOptions{Reset: reset})
	if err != nil {
		log.Fatalf("seed-full failed: %v", err)
	}
	queueRecommendationRebuildJobs()

	fmt.Printf(
		"seed-full done: users=%d behavior_profiles=%d posts=%d participants=%d messages=%d reviews=%d exposures=%d clicks=%d\n",
		result.Users, result.BehaviorProfiles, result.Posts, result.Participants, result.Messages, result.Reviews, result.Exposures, result.Clicks,
	)
}

func queueRecommendationRebuildJobs() {
	if strings.TrimSpace(os.Getenv("USE_REDIS")) != "true" {
		return
	}
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
	})
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return
	}
	now := time.Now().UnixMilli()
	_, _ = client.XAdd(ctx, &redis.XAddArgs{
		Stream: "zgbe:rec:jobs",
		Values: map[string]any{
			"type":        "rebuild_all_embeddings",
			"requestedAt": now,
		},
	}).Result()
	_, _ = client.XAdd(ctx, &redis.XAddArgs{
		Stream: "zgbe:rec:jobs",
		Values: map[string]any{
			"type":        "train_ranking_model",
			"requestedAt": now,
		},
	}).Result()
}
