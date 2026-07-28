package api

import (
	"testing"

	"make_friends/backend/internal/model"
)

func TestIsAutoApproveEligible(t *testing.T) {
	cfg := autoApproveConfig{
		Threshold: 0.85,
		MaxDeduct: 10,
	}

	tests := []struct {
		name     string
		decision model.CaseDecision
		eligible bool
		reason   string
	}{
		{
			name: "high confidence low risk → eligible",
			decision: model.CaseDecision{
				Confidence: 0.90,
				Outcome:    "upheld",
				Actions:    `[{"action":"credit_deduct","amount":-5,"reason":"test"}]`,
			},
			eligible: true,
		},
		{
			name: "confidence below threshold → not eligible",
			decision: model.CaseDecision{
				Confidence: 0.70,
				Outcome:    "upheld",
				Actions:    `[{"action":"credit_deduct","amount":-5}]`,
			},
			eligible: false,
			reason:   "confidence",
		},
		{
			name: "large deduction → not eligible",
			decision: model.CaseDecision{
				Confidence: 0.95,
				Outcome:    "upheld",
				Actions:    `[{"action":"credit_deduct","amount":-20}]`,
			},
			eligible: false,
			reason:   "credit_deduct amount",
		},
		{
			name: "escalate outcome → not eligible",
			decision: model.CaseDecision{
				Confidence: 0.99,
				Outcome:    "escalate",
				Actions:    `[]`,
			},
			eligible: false,
			reason:   "escalate",
		},
		{
			name: "post_takedown at 0.88 → not eligible (requires 0.90)",
			decision: model.CaseDecision{
				Confidence: 0.88,
				Outcome:    "upheld",
				Actions:    `[{"action":"post_takedown","reason":"spam"}]`,
			},
			eligible: false,
			reason:   "post_takedown requires",
		},
		{
			name: "post_takedown at 0.92 → eligible",
			decision: model.CaseDecision{
				Confidence: 0.92,
				Outcome:    "upheld",
				Actions:    `[{"action":"post_takedown","reason":"spam"}]`,
			},
			eligible: true,
		},
		{
			name: "no actions → eligible (just a verdict)",
			decision: model.CaseDecision{
				Confidence: 0.90,
				Outcome:    "rejected",
				Actions:    `[]`,
			},
			eligible: true,
		},
		{
			name: "invalid actions JSON → not eligible",
			decision: model.CaseDecision{
				Confidence: 0.95,
				Outcome:    "upheld",
				Actions:    `not json`,
			},
			eligible: false,
			reason:   "cannot parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligible, reason := isAutoApproveEligible(tt.decision, cfg)
			if eligible != tt.eligible {
				t.Errorf("expected eligible=%v, got %v (reason: %s)", tt.eligible, eligible, reason)
			}
			if !tt.eligible && tt.reason != "" {
				if !contains(reason, tt.reason) {
					t.Errorf("expected reason to contain %q, got %q", tt.reason, reason)
				}
			}
		})
	}
}

func TestIsAutoApproveEligible_CustomThreshold(t *testing.T) {
	cfg := autoApproveConfig{
		Threshold: 0.95,
		MaxDeduct: 5,
	}
	d := model.CaseDecision{
		Confidence: 0.90,
		Outcome:    "upheld",
		Actions:    `[{"action":"credit_deduct","amount":-3}]`,
	}
	eligible, _ := isAutoApproveEligible(d, cfg)
	if eligible {
		t.Error("should not be eligible when confidence 0.90 < threshold 0.95")
	}

	d.Confidence = 0.96
	eligible, _ = isAutoApproveEligible(d, cfg)
	if !eligible {
		t.Error("should be eligible when confidence 0.96 >= threshold 0.95")
	}

	// But amount -8 > maxDeduct 5
	d.Actions = `[{"action":"credit_deduct","amount":-8}]`
	eligible, reason := isAutoApproveEligible(d, cfg)
	if eligible {
		t.Error("should not be eligible when deduct 8 > maxDeduct 5")
	}
	if !contains(reason, "credit_deduct") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && indexOfSubstr(s, substr) >= 0)
}

func indexOfSubstr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
