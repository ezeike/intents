// lookup_test.go
package intents_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
)

func TestLookup_SuccessGivesEntityValue(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{entity: intents.Entity{ID: "e1", Label: "Bob", Score: 0.9}})
	root := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup", Index: "users"}}

	state, err := e.ResolveState(context.Background(), root, "bob", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	v := state.Values["root"]
	if v.Kind != intents.ValueKindEntity || v.Entity == nil || v.Entity.ID != "e1" || v.Source != "sqllookup" {
		t.Fatalf("state.Values[root] = %+v, want entity e1/sqllookup", v)
	}
}

func TestLookup_ErrNotFoundGivesReasonNotFound(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: intents.ErrNotFound})
	root := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}}

	state, err := e.ResolveState(context.Background(), root, "nobody", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if state.Failures[0].Reason != intents.ReasonNotFound {
		t.Fatalf("state.Failures[0].Reason = %q, want %q", state.Failures[0].Reason, intents.ReasonNotFound)
	}
}

func TestLookup_NilErrorWithEmptyEntityIDGivesReasonNotFound(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{entity: intents.Entity{ID: ""}}) // nil error, no ID
	root := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}}

	state, err := e.ResolveState(context.Background(), root, "nobody", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if state.Failures[0].Reason != intents.ReasonNotFound {
		t.Fatalf("state.Failures[0].Reason = %q, want %q", state.Failures[0].Reason, intents.ReasonNotFound)
	}
}

func TestLookup_OtherBackendErrorGivesReasonBackendError(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterLookuper("sqllookup", &fakeLookuper{err: context.DeadlineExceeded})
	root := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if state.Failures[0].Reason != intents.ReasonBackendError {
		t.Fatalf("state.Failures[0].Reason = %q, want %q", state.Failures[0].Reason, intents.ReasonBackendError)
	}
}
