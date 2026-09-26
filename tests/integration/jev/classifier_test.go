//go:build integration

package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
	"github.com/ezeike/intents/jev"
)

// TestClassifier_Classify_EndToEndAgainstLiveJev exercises the actual intents.Classifier seam
// (not just the raw Client) against the live endpoint, mirroring how an *intents.Engine would
// call it via RegisterClassifier.
func TestClassifier_Classify_EndToEndAgainstLiveJev(t *testing.T) {
	requireJev(t) // ensures a key is configured and the endpoint is reachable before building c below

	c := &jev.Classifier{
		Client:       jev.NewClient(jevBaseURL(), jevAPIKey()),
		Question:     "action",
		Instructions: "What does the user want to do?",
		Criteria: jev.OrderedCriteria{
			{Key: "send", Description: "send funds to another account"},
			{Key: "view_balance", Description: "check the current wallet balance"},
		},
		Model: jevModel(),
	}

	ans, err := c.Classify(context.Background(), "what's my balance right now?", nil)
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	if ans.Choice != "view_balance" {
		t.Errorf("Choice = %q, want %q", ans.Choice, "view_balance")
	}
	if ans.Confidence < 0 {
		t.Errorf("Confidence = %v, want >= 0 (top-two margin)", ans.Confidence)
	}
	if len(ans.Distribution) != 2 {
		t.Errorf("Distribution = %+v, want 2 entries", ans.Distribution)
	}
}

// TestClassifier_ThroughEngine_ResolvesADeclaredChoice wires the same Classifier into a real
// intents.Engine, confirming the adapter satisfies the seam end to end (registration, dispatch,
// Value population) against live traffic, not just a direct Classify() call.
func TestClassifier_ThroughEngine_ResolvesADeclaredChoice(t *testing.T) {
	requireJev(t)

	e := intents.NewEngine()
	e.RegisterClassifier("jev", &jev.Classifier{
		Client:       jev.NewClient(jevBaseURL(), jevAPIKey()),
		Question:     "action",
		Instructions: "What does the user want to do?",
		Criteria: jev.OrderedCriteria{
			{Key: "send", Description: "send funds to another account"},
			{Key: "view_balance", Description: "check the current wallet balance"},
		},
		Model: jevModel(),
	})
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "jev", Options: map[string]string{"send": "send", "view_balance": "view_balance"}},
	}

	state, err := e.ResolveState(context.Background(), root, "send 10 CNPY to bob", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	v := state.Values["root"]
	if v.Choice != "send" {
		t.Errorf("Values[root].Choice = %q, want %q", v.Choice, "send")
	}
	if v.Source != "jev" {
		t.Errorf("Values[root].Source = %q, want %q", v.Source, "jev")
	}
}
