package main

import (
	"fmt"
	"log"
	"time"

	"make_friends/backend/internal/db"
	"make_friends/backend/internal/seed"
)

func main() {
	database, err := db.OpenSQLite()
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}

	count, err := seed.BackfillBehaviorProfiles(database, time.Now().UnixMilli())
	if err != nil {
		log.Fatalf("seed behavior profiles failed: %v", err)
	}

	fmt.Printf("seed behavior profiles done: behavior_profiles=%d\n", count)
}
