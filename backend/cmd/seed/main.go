package main

import (
	"flag"
	"fmt"
	"log"

	"make_friends/backend/internal/db"
	"make_friends/backend/internal/seed"
)

func main() {
	var users int
	var posts int
	var messages int
	var reset bool

	flag.IntVar(&users, "users", 20, "mock user count")
	flag.IntVar(&posts, "posts", 80, "mock post count")
	flag.IntVar(&messages, "messages", 5, "messages per post")
	flag.BoolVar(&reset, "reset", true, "clear existing business data before seeding")
	flag.Parse()

	database, err := db.OpenSQLite()
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}

	result, err := seed.Run(database, seed.Options{
		Reset:           reset,
		Users:           users,
		Posts:           posts,
		MessagesPerPost: messages,
	})
	if err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	fmt.Printf(
		"seed done: users=%d behavior_profiles=%d posts=%d participants=%d messages=%d reviews=%d exposures=%d clicks=%d\n",
		result.Users, result.BehaviorProfiles, result.Posts, result.Participants, result.Messages, result.Reviews, result.Exposures, result.Clicks,
	)
}
