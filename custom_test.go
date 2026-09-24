// custom_test.go
package intents_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
)

func TestCustom_NodeFuncValueIsRecorded(t *testing.T) {
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindCustom,
		Custom: func(ctx context.Context, state intents.State) (intents.Value, error) {
			return intents.Value{Kind: intents.ValueKindRaw, Raw: state.Input}, nil
		},
	}
	e := intents.NewEngine()

	state, err := e.ResolveState(context.Background(), root, "hello", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["root"].Raw != "hello" {
		t.Fatalf("state.Values[root].Raw = %q, want %q", state.Values["root"].Raw, "hello")
	}
}

func TestCustom_ErrInvalidInputGivesReasonInvalidInput(t *testing.T) {
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindCustom,
		Custom: func(ctx context.Context, state intents.State) (intents.Value, error) {
			return intents.Value{}, intents.ErrInvalidInput
		},
	}
	e := intents.NewEngine()

	state, err := e.ResolveState(context.Background(), root, "garbage", nil)
	if _, isIncomplete := err.(*intents.IncompleteError); !isIncomplete {
		t.Fatalf("ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	if state.Failures[0].Reason != intents.ReasonInvalidInput {
		t.Fatalf("state.Failures[0].Reason = %q, want %q", state.Failures[0].Reason, intents.ReasonInvalidInput)
	}
}

func TestCustom_SetsSourceToCustomOnlyWhenEmpty(t *testing.T) {
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindCustom,
		Custom: func(ctx context.Context, state intents.State) (intents.Value, error) {
			return intents.Value{Kind: intents.ValueKindRaw, Raw: "x", Source: "my_adapter"}, nil
		},
	}
	e := intents.NewEngine()

	state, err := e.ResolveState(context.Background(), root, "x", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["root"].Source != "my_adapter" {
		t.Fatalf("state.Values[root].Source = %q, want %q (caller-set Source preserved)", state.Values["root"].Source, "my_adapter")
	}
}

func TestCustom_DefaultsSourceToCustomWhenEmpty(t *testing.T) {
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindCustom,
		Custom: func(ctx context.Context, state intents.State) (intents.Value, error) {
			return intents.Value{Kind: intents.ValueKindRaw, Raw: "x"}, nil
		},
	}
	e := intents.NewEngine()

	state, err := e.ResolveState(context.Background(), root, "x", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["root"].Source != "custom" {
		t.Fatalf("state.Values[root].Source = %q, want %q", state.Values["root"].Source, "custom")
	}
}
