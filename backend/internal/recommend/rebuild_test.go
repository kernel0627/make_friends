package recommend

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestDefaultRankingWeightsMatchPythonDefaults keeps the Go seed weights and
// the Python trainer's fallback weights describing the same feature space.
// The scorer looks weights up by name and treats a missing name as zero, so a
// feature present in one list and absent from the other is a silently dropped
// ranking signal rather than a visible failure.
func TestDefaultRankingWeightsMatchPythonDefaults(t *testing.T) {
	path := filepath.Join("..", "..", "recommender", "features.py")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("python recommender not available: %v", err)
	}

	// Collect the keys of _TRAINABLE_DEFAULT_WEIGHTS and UNTRAINABLE_WEIGHTS.
	blockRe := regexp.MustCompile(`(?s)(UNTRAINABLE_WEIGHTS|_TRAINABLE_DEFAULT_WEIGHTS) = \{(.*?)\n\}`)
	keyRe := regexp.MustCompile(`"([a-z0-9_]+)":`)

	pythonKeys := map[string]struct{}{}
	blocks := blockRe.FindAllStringSubmatch(string(raw), -1)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 weight blocks in features.py, found %d", len(blocks))
	}
	for _, block := range blocks {
		for _, m := range keyRe.FindAllStringSubmatch(block[2], -1) {
			pythonKeys[m[1]] = struct{}{}
		}
	}

	goWeights := DefaultRankingWeights()
	for name := range goWeights {
		if _, ok := pythonKeys[name]; !ok {
			t.Errorf("feature %q is in Go DefaultRankingWeights but not in features.py", name)
		}
	}
	for name := range pythonKeys {
		if _, ok := goWeights[name]; !ok {
			t.Errorf("feature %q is in features.py but not in Go DefaultRankingWeights", name)
		}
	}
}
