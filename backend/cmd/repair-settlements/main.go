package main

import (
	"log"
	"os"
	"time"

	"make_friends/backend/internal/db"
	"make_friends/backend/internal/model"
	"make_friends/backend/internal/score"
)

func main() {
	database, err := db.OpenSQLite()
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}

	var posts []model.Post
	if err := database.Where("status = ?", "closed").Order("updated_at DESC").Find(&posts).Error; err != nil {
		log.Fatalf("query closed posts failed: %v", err)
	}

	nowMS := time.Now().UnixMilli()
	repaired := 0
	failed := 0
	for _, post := range posts {
		if err := score.RecalculatePostActivityScores(database, post.ID, nowMS); err != nil {
			failed++
			log.Printf("repair post %s failed: %v", post.ID, err)
			continue
		}
		repaired++
	}

	log.Printf("settlement repair finished: repaired=%d failed=%d", repaired, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
