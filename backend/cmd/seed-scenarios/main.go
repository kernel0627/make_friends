// Command seed-scenarios populates a test database with scenario-driven agent case data.
// Usage: go run ./cmd/seed-scenarios [--db path] [--scenario id] [--clean]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/seed/scenarios"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	dbPath := flag.String("db", "test_agent.db", "SQLite database path")
	scenarioID := flag.String("scenario", "", "Run a specific scenario by ID (empty = all)")
	clean := flag.Bool("clean", false, "Delete DB file before seeding")
	list := flag.Bool("list", false, "List available scenarios and exit")
	flag.Parse()

	if *list {
		for _, s := range scenarios.AllScenarios() {
			fmt.Printf("  %-35s [%s] %s\n", s.ID, s.Difficulty, s.Summary)
		}
		return
	}

	if *clean {
		os.Remove(*dbPath)
	}

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	// Migrate all tables
	if err := db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.PostParticipant{},
		&model.ChatMessage{},
		&model.PostInvitation{},
		&model.Review{},
		&model.ActivityScore{},
		&model.PostParticipantSettlement{},
		&model.CreditLedger{},
		&model.AdminCase{},
		&model.ModerationRecord{},
		&model.CaseEvent{},
		&model.OutboxEvent{},
		&model.DomainEvent{},
		&model.AgentRun{},
		&model.AgentStep{},
		&model.ContentSnapshot{},
		&model.Notification{},
		&model.Report{},
		&model.CaseEvidence{},
		&model.CaseDecision{},
	); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	allScenarios := scenarios.AllScenarios()

	// Filter if specific scenario requested
	if *scenarioID != "" {
		var filtered []*scenarios.Scenario
		for _, s := range allScenarios {
			if s.ID == *scenarioID {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			fmt.Println("Available scenarios:")
			for _, s := range scenarios.AllScenarios() {
				fmt.Printf("  %s\n", s.ID)
			}
			log.Fatalf("scenario %q not found", *scenarioID)
		}
		allScenarios = filtered
	}

	fmt.Printf("Seeding %d scenario(s) into %s\n", len(allScenarios), *dbPath)

	for _, s := range allScenarios {
		gen := scenarios.NewGenerator(db, s)
		caseID, err := gen.Generate()
		if err != nil {
			log.Fatalf("scenario %s: %v", s.ID, err)
		}
		fmt.Printf("  ✓ %-35s → case %s\n", s.ID, caseID)
	}

	fmt.Println("\nDone.")
}
