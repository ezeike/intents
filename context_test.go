// context_test.go
package intents_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
)

type fakeGrounding struct {
	HasStagedProposal bool
}

func TestBuildContext_ClassifyRoundTripsTypedStateContext(t *testing.T) {
	fc := &fakeClassifier{choice: "a", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend: "laya",
			Options: map[string]string{"a": "a"},
			BuildContext: func(state intents.State) (any, error) {
				g, _ := state.Context.(fakeGrounding)
				return g, nil
			},
		},
	}

	_, err := e.ResolveState(context.Background(), root, "input", fakeGrounding{HasStagedProposal: true})
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	got, ok := fc.lastGround.(fakeGrounding)
	if !ok || !got.HasStagedProposal {
		t.Fatalf("Classify received grounding = %+v, want fakeGrounding{HasStagedProposal: true}", fc.lastGround)
	}
}

func TestBuildContext_NilBuildContextPassesStateContextVerbatim(t *testing.T) {
	fc := &fakeClassifier{choice: "a", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	_, err := e.ResolveState(context.Background(), root, "input", fakeGrounding{HasStagedProposal: true})
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	got, ok := fc.lastGround.(fakeGrounding)
	if !ok || !got.HasStagedProposal {
		t.Fatalf("Classify received grounding = %+v, want state.Context passed through", fc.lastGround)
	}
}

func TestBuildContext_LookupRoundTripsTypedStateContext(t *testing.T) {
	fl := &fakeLookuper{entity: intents.Entity{ID: "e1"}}
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", fl)
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindLookup,
		Lookup: &intents.LookupSpec{
			Backend: "sqllookup",
			BuildContext: func(state intents.State) (any, error) {
				g, _ := state.Context.(fakeGrounding)
				return g, nil
			},
		},
	}

	_, err := e.ResolveState(context.Background(), root, "input", fakeGrounding{HasStagedProposal: true})
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	got, ok := fl.lastGround.(fakeGrounding)
	if !ok || !got.HasStagedProposal {
		t.Fatalf("Lookup received grounding = %+v, want fakeGrounding{HasStagedProposal: true}", fl.lastGround)
	}
}

func TestBuildContext_ErrorGivesReasonBackendErrorWithoutCallingBackend(t *testing.T) {
	fc := &fakeClassifier{choice: "a", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend: "laya",
			Options: map[string]string{"a": "a"},
			BuildContext: func(state intents.State) (any, error) {
				return nil, intents.ErrInvalidInput
			},
		},
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if state.Failures[0].Reason != intents.ReasonBackendError {
		t.Fatalf("state.Failures[0].Reason = %q, want %q", state.Failures[0].Reason, intents.ReasonBackendError)
	}
	if fc.callCount() != 0 {
		t.Fatalf("Classify call count = %d, want 0 (BuildContext failed before backend call)", fc.callCount())
	}
}
