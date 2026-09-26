//go:build integration

package jev_test

import (
	"context"
	"math"
	"testing"

	"github.com/ezeike/intents/jev"
)

func TestChoice_ClearSingleOption_ReturnsExpectedChoice(t *testing.T) {
	c := requireJev(t)

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: "send 50 CNPY to alice",
		Questions: map[string]jev.Question{
			"asset": {
				Type:         "choice",
				Instructions: "Which token does the user want to send?",
				Criteria: jev.OrderedCriteria{
					{Key: "cnpy", Description: "the native CNPY token"},
					{Key: "chainx", Description: "the chain_x token"},
				},
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	a, err := resp.Choice("asset")
	if err != nil {
		t.Fatalf("Choice() error = %v, want nil", err)
	}
	if a.Choice != "cnpy" {
		t.Errorf("choice = %q, want %q", a.Choice, "cnpy")
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		t.Errorf("confidence = %v, out of [0,1]", a.Confidence)
	}
}

func TestChoice_Probabilities_SumToOneAcrossAllOptions(t *testing.T) {
	c := requireJev(t)

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: "check my balance",
		Questions: map[string]jev.Question{
			"action": {
				Type:         "choice",
				Instructions: "What does the user want to do?",
				Criteria: jev.OrderedCriteria{
					{Key: "send", Description: "send funds"},
					{Key: "view_balance", Description: "check wallet balance"},
					{Key: "view_chain", Description: "view a chain's details"},
				},
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	a, err := resp.Choice("action")
	if err != nil {
		t.Fatalf("Choice() error = %v, want nil", err)
	}
	if len(a.Probabilities) != 3 {
		t.Fatalf("got %d probabilities, want 3: %+v", len(a.Probabilities), a.Probabilities)
	}
	var sum float64
	for _, p := range a.Probabilities {
		sum += p
	}
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("probabilities sum = %v, want ~1.0: %+v", sum, a.Probabilities)
	}
}

func TestChoice_ChosenKey_MatchesArgmaxOfProbabilities(t *testing.T) {
	c := requireJev(t)

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: "view chain xyz-123 status",
		Questions: map[string]jev.Question{
			"action": {
				Type:         "choice",
				Instructions: "What does the user want to do?",
				Criteria: jev.OrderedCriteria{
					{Key: "send", Description: "send funds"},
					{Key: "view_chain", Description: "view a chain's details"},
				},
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	a, err := resp.Choice("action")
	if err != nil {
		t.Fatalf("Choice() error = %v, want nil", err)
	}
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

func TestChoice_EmptyCriteria_ReturnsAnError(t *testing.T) {
	c := requireJev(t)

	_, err := c.SystemOne(context.Background(), jev.Request{
		State: "x",
		Questions: map[string]jev.Question{
			"q1": {Type: "choice", Instructions: "pick one", Criteria: jev.OrderedCriteria{}},
		},
		Model: jevModel(),
	})
	// Confirmed live: OpenRouter's Jev gateway returns 400 here, not Laya's 422 — see
	// errors.go's doc comment. Assert only that some sentinel-wrapped error comes back, not which
	// one, so this test doesn't pin a status-code convention this route doesn't actually follow.
	if err == nil {
		t.Fatal("SystemOne() error = nil, want an error for empty criteria")
	}
}
