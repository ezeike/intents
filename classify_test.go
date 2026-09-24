// classify_test.go
package intents_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ezeike/intents"
)

func TestClassify_DescendsThroughChildrenAndSetsTerminalToLeaf(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "leaf", confidence: 1})
	leaf := &intents.Node{Key: "leaf_node", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"leaf": "leaf"}}}
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"leaf": "leaf"}},
		Children: map[string]*intents.Node{"leaf": leaf},
	}

	state, err := e.ResolveState(context.Background(), root, "go to leaf", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Terminal != "leaf_node" {
		t.Fatalf("state.Terminal = %q, want %q", state.Terminal, "leaf_node")
	}
	if _, ok := state.Values["root"]; !ok {
		t.Fatalf("state.Values missing %q", "root")
	}
	if _, ok := state.Values["leaf_node"]; !ok {
		t.Fatalf("state.Values missing %q", "leaf_node")
	}
}

func TestClassify_LeafWithNoChildrenSetsTerminal(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Terminal != "root" {
		t.Fatalf("state.Terminal = %q, want %q", state.Terminal, "root")
	}
}

func TestClassify_BelowMinConfidenceGivesReasonLowConfidenceWithCandidate(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 0.2})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}, MinConfidence: 0.5}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if len(state.Failures) != 1 {
		t.Fatalf("len(state.Failures) = %d, want 1", len(state.Failures))
	}
	f := state.Failures[0]
	if f.Reason != intents.ReasonLowConfidence {
		t.Fatalf("f.Reason = %q, want %q", f.Reason, intents.ReasonLowConfidence)
	}
	if f.Candidate == nil || f.Candidate.Choice != "a" {
		t.Fatalf("f.Candidate = %+v, want Choice %q", f.Candidate, "a")
	}
}

func TestClassify_FallbackFiresOnReject(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})
	e.RegisterLookuper("sqllookup", &fakeLookuper{entity: intents.Entity{ID: "e1", Label: "Found"}})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend:  "laya",
			Options:  map[string]string{"a": "a"},
			Fallback: &intents.LookupSpec{Backend: "sqllookup"},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	v := state.Values["root"]
	if v.Kind != intents.ValueKindEntity || v.Entity == nil || v.Entity.ID != "e1" {
		t.Fatalf("state.Values[root] = %+v, want entity e1", v)
	}
	if len(state.Failures) != 0 {
		t.Fatalf("state.Failures = %+v, want empty (fallback succeeded)", state.Failures)
	}
}

func TestClassify_FallbackFiresOnLowConfidence(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 0.1})
	e.RegisterLookuper("sqllookup", &fakeLookuper{entity: intents.Entity{ID: "e1"}})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend:       "laya",
			Options:       map[string]string{"a": "a"},
			MinConfidence: 0.5,
			Fallback:      &intents.LookupSpec{Backend: "sqllookup"},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["root"].Entity.ID != "e1" {
		t.Fatalf("state.Values[root].Entity.ID = %q, want %q", state.Values["root"].Entity.ID, "e1")
	}
}

func TestClassify_FallbackFailureCarriesLookupReasonAndClassifierCandidate(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: intents.ErrNotFound})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend:  "laya",
			Options:  map[string]string{"a": "a"},
			Fallback: &intents.LookupSpec{Backend: "sqllookup"},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	f := state.Failures[0]
	if f.Reason != intents.ReasonNotFound {
		t.Fatalf("f.Reason = %q, want %q", f.Reason, intents.ReasonNotFound)
	}
	if f.Candidate == nil || f.Candidate.Choice != "other" {
		t.Fatalf("f.Candidate = %+v, want Choice %q", f.Candidate, "other")
	}
}

func TestClassify_RejectWithoutFallbackGivesReasonNotFound(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	f := state.Failures[0]
	if f.Reason != intents.ReasonNotFound {
		t.Fatalf("f.Reason = %q, want %q", f.Reason, intents.ReasonNotFound)
	}
	if f.Candidate == nil || f.Candidate.Choice != "other" {
		t.Fatalf("f.Candidate = %+v, want Choice %q", f.Candidate, "other")
	}
}

func TestClassify_BackendErrorGivesReasonBackendError(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{err: errors.New("boom")})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	f := state.Failures[0]
	if f.Reason != intents.ReasonBackendError {
		t.Fatalf("f.Reason = %q, want %q", f.Reason, intents.ReasonBackendError)
	}
	if !errors.Is(f.Err, errors.New("boom")) && f.Err == nil {
		t.Fatalf("f.Err = nil, want wrapped backend error")
	}
}

func TestClassify_UnknownChoiceNotInOptionsOrOtherGivesReasonBackendErrorWrappingErrUnknownChoice(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "not_declared", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	f := state.Failures[0]
	if f.Reason != intents.ReasonBackendError {
		t.Fatalf("f.Reason = %q, want %q", f.Reason, intents.ReasonBackendError)
	}
	if !errors.Is(f.Err, intents.ErrUnknownChoice) {
		t.Fatalf("f.Err = %v, want errors.Is ErrUnknownChoice", f.Err)
	}
}

func TestClassify_ValueHasChoiceKindConfidenceAndSource(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 0.9})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	v := state.Values["root"]
	if v.Kind != intents.ValueKindChoice || v.Choice != "a" || v.Confidence != 0.9 || v.Source != "laya" {
		t.Fatalf("state.Values[root] = %+v, want choice a/0.9/laya", v)
	}
}
