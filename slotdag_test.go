// slotdag_test.go
package intents_test

import (
	"context"
	"testing"
	"time"

	"github.com/ezeike/intents"
)

func TestSlotDAG_IndependentWaveResolvesConcurrently(t *testing.T) {
	barrier := newBarrierClassifier(2) // blocks until 2 concurrent Classify calls arrive
	e := intents.NewEngine()
	e.RegisterClassifier("laya", barrier)
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "invitee", Kind: intents.KindClassify, Classify: classify, Required: true},
			{Name: "permission", Kind: intents.KindClassify, Classify: classify, Required: true},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	state, err := e.ResolveState(ctx, root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil (a serial implementation would time out here)", err)
	}
	if len(state.Values) != 2 {
		t.Fatalf("len(state.Values) = %d, want 2", len(state.Values))
	}
}

func TestSlotDAG_DependentSlotSeesDependencyValueInBuildContextState(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{entity: intents.Entity{ID: "acct1"}})
	var sawDependencyValue bool
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "account", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}, Required: true},
			{
				Name:      "amount",
				Kind:      intents.KindLookup,
				Required:  true,
				DependsOn: []string{"account"},
				Lookup: &intents.LookupSpec{
					Backend: "sqllookup",
					BuildContext: func(state intents.State) (any, error) {
						_, sawDependencyValue = state.Values["account"]
						return nil, nil
					},
				},
			},
		},
	}

	_, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if !sawDependencyValue {
		t.Fatalf("dependent slot's BuildContext did not see its dependency's Value")
	}
}

func TestSlotDAG_RequiredSlotFailureReturnsIncompleteError(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: intents.ErrNotFound})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "invitee", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}, Required: true},
		},
	}

	_, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, ok := err.(*intents.IncompleteError); !ok {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
}

func TestSlotDAG_OptionalSlotFailureRecordedWithoutIncompleteError(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: intents.ErrNotFound})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "nickname", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}, Required: false},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil (optional failure doesn't block)", err)
	}
	if len(state.Failures) != 1 || state.Failures[0].Reason != intents.ReasonNotFound {
		t.Fatalf("state.Failures = %+v, want one ReasonNotFound failure", state.Failures)
	}
}

func TestSlotDAG_SlotWithUnresolvedDependencyGivesReasonUnresolved(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: intents.ErrNotFound})
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "account", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}, Required: true},
			{
				Name:      "amount",
				Kind:      intents.KindClassify,
				Classify:  &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}},
				DependsOn: []string{"account"},
				Required:  true,
			},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, ok := err.(*intents.IncompleteError); !ok {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	var amountFailure *intents.Failure
	for i := range state.Failures {
		if state.Failures[i].Path == "root.amount" {
			amountFailure = &state.Failures[i]
		}
	}
	if amountFailure == nil {
		t.Fatalf("no failure recorded for path %q", "root.amount")
	}
	if amountFailure.Reason != intents.ReasonUnresolved {
		t.Fatalf("amount failure reason = %q, want %q", amountFailure.Reason, intents.ReasonUnresolved)
	}
}

func TestSlotDAG_TerminalSetToNodeKeyOnceWavesExhausted(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{
		Key:  "invite_collaborator",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "permission", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}, Required: true},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Terminal != "invite_collaborator" {
		t.Fatalf("state.Terminal = %q, want %q", state.Terminal, "invite_collaborator")
	}
}

func TestSlotDAG_InheritsAncestorRiskTierForAllSlots(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}
	dag := &intents.Node{
		Key:      "invite",
		Kind:     intents.KindSlotDAG,
		RiskTier: intents.RiskLow,
		Slots: []intents.SlotSpec{
			{Name: "invitee", Kind: intents.KindClassify, Classify: classify, Required: true},
			{Name: "permission", Kind: intents.KindClassify, Classify: classify, Required: true},
		},
	}
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: classify,
		RiskTier: intents.RiskHigh,
		Children: map[string]*intents.Node{"a": dag},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	for _, name := range []string{"invitee", "permission"} {
		v, ok := state.Values[name]
		if !ok {
			t.Fatalf("state.Values[%q] missing", name)
		}
		if v.RiskTier != intents.RiskHigh {
			t.Fatalf("state.Values[%q].RiskTier = %v, want %v (inherited from ancestor)", name, v.RiskTier, intents.RiskHigh)
		}
	}
}
