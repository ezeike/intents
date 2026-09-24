//go:build integration

package laya_test

import (
	"math"
	"testing"
)

func TestScore_ReturnsScoreWithinLegendRange(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "URGENT!! send this right now, hurry",
		Questions: map[string]questionDef{
			"urgency": {
				Type:         "score",
				Instructions: "How urgent does this request sound, from 0 (no urgency) to 3 (very urgent)?",
				Criteria:     []string{"not urgent", "mild", "urgent", "very urgent"},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeScore(t, resp, "urgency")
	if a.Type != "score" {
		t.Errorf("type = %q, want %q", a.Type, "score")
	}
	if a.Score < 0 || a.Score > 3 {
		t.Errorf("score = %v, want within [0,3] for a 4-level criteria list", a.Score)
	}
}

func TestScore_Legend_MatchesCriteriaOrder(t *testing.T) {
	base := requireLaya(t)

	criteria := []string{"none", "mild", "urgent", "very urgent"}
	resp := mustOK(t, base, systemOneRequest{
		State: "whenever you get a chance, no rush",
		Questions: map[string]questionDef{
			"urgency": {
				Type:         "score",
				Instructions: "How urgent, 0-3?",
				Criteria:     criteria,
			},
		},
		Model: "typed-decisions",
	})

	a := decodeScore(t, resp, "urgency")
	if len(a.Legend) != len(criteria) {
		t.Fatalf("legend has %d entries, want %d: %+v", len(a.Legend), len(criteria), a.Legend)
	}
	for i, want := range criteria {
		key := string(rune('0' + i))
		if got := a.Legend[key]; got != want {
			t.Errorf("legend[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestScore_Probabilities_SumToOneAcrossLevels(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "please do this soon",
		Questions: map[string]questionDef{
			"urgency": {
				Type:         "score",
				Instructions: "How urgent, 0-2?",
				Criteria:     []string{"low", "medium", "high"},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeScore(t, resp, "urgency")
	if len(a.Probabilities) != 3 {
		t.Fatalf("got %d probabilities, want 3: %+v", len(a.Probabilities), a.Probabilities)
	}
	sum := sumProbabilities(a.Probabilities)
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("probabilities sum = %v, want ~1.0: %+v", sum, a.Probabilities)
	}
}

func TestScore_SingleLevel_ReturnsZeroScore(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "anything at all",
		Questions: map[string]questionDef{
			"only_level": {
				Type:         "score",
				Instructions: "degenerate single-level score",
				Criteria:     []string{"the only level"},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeScore(t, resp, "only_level")
	if a.Score != 0 {
		t.Errorf("score = %v, want 0 for a single-level criteria list (only index 0 exists)", a.Score)
	}
}

func TestScore_EmptyCriteria_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "score",
				Instructions: "how much?",
				Criteria:     []string{},
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}

func TestScore_CriteriaAsDictNotList_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "score",
				Instructions: "how much?",
				Criteria:     map[string]string{"lvl0": "x"}, // score requires a list, not a dict
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}
