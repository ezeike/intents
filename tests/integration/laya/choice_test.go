//go:build integration

package laya_test

import (
	"math"
	"testing"
)

func TestChoice_ClearSingleOption_ReturnsExpectedChoice(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "send 50 CNPY to alice",
		Questions: map[string]questionDef{
			"asset": {
				Type:         "choice",
				Instructions: "Which token does the user want to send?",
				Criteria: map[string]string{
					"cnpy":   "the native CNPY token",
					"chainx": "the chain_x token",
				},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeChoice(t, resp, "asset")
	if a.Type != "choice" {
		t.Errorf("type = %q, want %q", a.Type, "choice")
	}
	if a.Choice != "cnpy" {
		t.Errorf("choice = %q, want %q", a.Choice, "cnpy")
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		t.Errorf("confidence = %v, out of [0,1]", a.Confidence)
	}
}

func TestChoice_CriteriaAsPlainList_AcceptsListForm(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "send 50 CNPY to alice",
		Questions: map[string]questionDef{
			"asset": {
				Type:         "choice",
				Instructions: "Which token does the user want to send?",
				Criteria:     []string{"cnpy", "chainx"},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeChoice(t, resp, "asset")
	if _, ok := a.Probabilities["cnpy"]; !ok {
		t.Errorf("probabilities missing option %q: %+v", "cnpy", a.Probabilities)
	}
	if _, ok := a.Probabilities["chainx"]; !ok {
		t.Errorf("probabilities missing option %q: %+v", "chainx", a.Probabilities)
	}
}

func TestChoice_Probabilities_SumToOneAcrossAllOptions(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "check my balance",
		Questions: map[string]questionDef{
			"action": {
				Type:         "choice",
				Instructions: "What does the user want to do?",
				Criteria: map[string]string{
					"send":         "send funds",
					"view_balance": "check wallet balance",
					"view_chain":   "view a chain's details",
				},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeChoice(t, resp, "action")
	if len(a.Probabilities) != 3 {
		t.Fatalf("got %d probabilities, want 3: %+v", len(a.Probabilities), a.Probabilities)
	}
	sum := sumProbabilities(a.Probabilities)
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("probabilities sum = %v, want ~1.0: %+v", sum, a.Probabilities)
	}
}

func TestChoice_ChosenKey_MatchesArgmaxOfProbabilities(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "view chain xyz-123 status",
		Questions: map[string]questionDef{
			"action": {
				Type:         "choice",
				Instructions: "What does the user want to do?",
				Criteria: map[string]string{
					"send":       "send funds",
					"view_chain": "view a chain's details",
				},
			},
		},
		Model: "typed-decisions",
	})

	a := decodeChoice(t, resp, "action")
	var bestKey string
	var bestProb float64
	for k, v := range a.Probabilities {
		if v > bestProb {
			bestProb = v
			bestKey = k
		}
	}
	if a.Choice != bestKey {
		t.Errorf("choice = %q, but argmax of probabilities is %q (%+v)", a.Choice, bestKey, a.Probabilities)
	}
}

func TestChoice_ManyOptions_AllPresentInResponse(t *testing.T) {
	base := requireLaya(t)

	criteria := map[string]string{
		"wallet_send":       "send an asset",
		"wallet_view_chain": "view a chain",
		"launchpad_create":  "create a new chain",
		"launchpad_view":    "view launchpad status",
		"help":              "general help",
	}

	resp := mustOK(t, base, systemOneRequest{
		State: "I want to create a brand new chain",
		Questions: map[string]questionDef{
			"app_action": {
				Type:         "choice",
				Instructions: "Which top-level action does the user want?",
				Criteria:     criteria,
			},
		},
		Model: "typed-decisions",
	})

	a := decodeChoice(t, resp, "app_action")
	for k := range criteria {
		if _, ok := a.Probabilities[k]; !ok {
			t.Errorf("probabilities missing option %q: %+v", k, a.Probabilities)
		}
	}
}

func TestChoice_EmptyCriteria_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "choice",
				Instructions: "pick one",
				Criteria:     map[string]string{},
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
	e := decodeError(t, data)
	if e.Detail == "" {
		t.Error("detail is empty, want an explanation naming the question")
	}
}

func TestChoice_MissingInstructions_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:     "choice",
				Criteria: map[string]string{"a": "a"},
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}
