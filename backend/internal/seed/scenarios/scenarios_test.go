package scenarios_test

import (
	"testing"

	"make_friends/backend/internal/model"
	"make_friends/backend/internal/seed/scenarios"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	err = db.AutoMigrate(
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
	)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAllScenariosGenerate(t *testing.T) {
	for _, s := range scenarios.AllScenarios() {
		t.Run(s.ID, func(t *testing.T) {
			db := setupTestDB(t)
			gen := scenarios.NewGenerator(db, s)
			caseID, err := gen.Generate()
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}
			if caseID == "" {
				t.Fatal("Generate() returned empty case ID")
			}

			// Verify case exists
			var adminCase model.AdminCase
			if err := db.First(&adminCase, "id = ?", caseID).Error; err != nil {
				t.Fatalf("case not found: %v", err)
			}
			if adminCase.CaseType != s.CaseType {
				t.Errorf("case type = %q, want %q", adminCase.CaseType, s.CaseType)
			}
			if adminCase.Status != "open" {
				t.Errorf("case status = %q, want open", adminCase.Status)
			}

			// Verify users created
			var userCount int64
			db.Model(&model.User{}).Count(&userCount)
			if int(userCount) != len(s.Roles) {
				t.Errorf("user count = %d, want %d", userCount, len(s.Roles))
			}

			// Verify at least one domain event
			var eventCount int64
			db.Model(&model.DomainEvent{}).Count(&eventCount)
			if eventCount == 0 {
				t.Error("no domain events generated")
			}
		})
	}
}

func TestScenarioTruthNotEmpty(t *testing.T) {
	for _, s := range scenarios.AllScenarios() {
		t.Run(s.ID, func(t *testing.T) {
			if s.Truth.Outcome == "" {
				t.Error("Truth.Outcome is empty")
			}
			if len(s.Truth.PolicyRefs) == 0 {
				t.Error("Truth.PolicyRefs is empty")
			}
			if len(s.Truth.RequiredEvidence) == 0 {
				t.Error("Truth.RequiredEvidence is empty")
			}
		})
	}
}
