// Command export-golden outputs a JSON manifest of all scenarios' ground truth
// for use by the Python agent evaluator.
// Usage: go run ./cmd/export-golden > ../agent/tests/golden_manifest.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"make_friends/backend/internal/seed/scenarios"
)

type GoldenEntry struct {
	ScenarioID  string   `json:"scenario_id"`
	CaseType    string   `json:"case_type"`
	Difficulty  string   `json:"difficulty"`
	Outcome     string   `json:"expected_outcome"`
	Responsible string   `json:"responsible_party"`
	PolicyRefs  []string `json:"policy_refs"`
	Required    []string `json:"required_evidence"`
	Forbidden   []string `json:"forbidden_claims"`
}

func main() {
	all := scenarios.AllScenarios()
	entries := make([]GoldenEntry, 0, len(all))

	for _, s := range all {
		entries = append(entries, GoldenEntry{
			ScenarioID:  s.ID,
			CaseType:    s.CaseType,
			Difficulty:  s.Difficulty,
			Outcome:     s.Truth.Outcome,
			Responsible: s.Truth.ResponsibleParty,
			PolicyRefs:  s.Truth.PolicyRefs,
			Required:    s.Truth.RequiredEvidence,
			Forbidden:   s.Truth.ForbiddenClaims,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
