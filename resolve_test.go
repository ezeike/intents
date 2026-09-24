// resolve_test.go
package intents_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ezeike/intents"
)

func TestResolveState_InvalidSpecReturnsErrInvalidSpec(t *testing.T) {
	e := intents.NewEngine()
	bad := &intents.Node{Key: "", Kind: intents.KindClassify} // empty key, missing Classify
	_, err := e.ResolveState(context.Background(), bad, "input", nil)
	if !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("ResolveState() error = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestResolveState_UnregisteredBackendReturnsErrUnregisteredBackendBeforeAnyCall(t *testing.T) {
	fc := &fakeClassifier{choice: "a", confidence: 1}
	e := intents.NewEngine()
	// laya is NOT registered.
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	_, err := e.ResolveState(context.Background(), root, "input", nil)
	if !errors.Is(err, intents.ErrUnregisteredBackend) {
		t.Fatalf("ResolveState() error = %v, want errors.Is ErrUnregisteredBackend", err)
	}
	if fc.callCount() != 0 {
		t.Fatalf("Classify call count = %d, want 0 (checked before any backend call)", fc.callCount())
	}
}

func TestResolveState_CancelledContextReturnsErrorWrappingContextCanceledNotIncompleteError(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.ResolveState(ctx, root, "input", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveState() error = %v, want errors.Is context.Canceled", err)
	}
	if _, ok := err.(*intents.IncompleteError); ok {
		t.Fatalf("ResolveState() error is *IncompleteError, want a plain wrapped context error")
	}
}

func TestValidate_CachedPerNodePointerIdentity_SecondCallSkipsRevalidation(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	spec := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	if _, err := e.ResolveState(context.Background(), spec, "input", nil); err != nil {
		t.Fatalf("first ResolveState() error = %v, want nil", err)
	}

	// Mutate the same *Node pointer into an invalid shape after first successful use.
	// [Controller ruling, Task 11]: the original mutation here was `spec.Classify.Options = nil`,
	// which is invalid ONLY at Validate time — but resolveClassify's own runtime "unknown choice"
	// check (Task 5) reads cs.Options independently of Validate's cache, so that mutation also
	// breaks the classify dispatch itself, making the second call return *IncompleteError for a
	// reason unrelated to the cache (an unknown-choice failure, not a validation error) — the
	// test's intent (prove the SECOND call fully succeeds because validation was skipped) was
	// unreachable with that mutation. Fixed to mutate a field Validate rejects but the runtime
	// walk never reads: `spec.Key`.
	spec.Key = ""

	if _, err := e.ResolveState(context.Background(), spec, "input", nil); err != nil {
		t.Fatalf("second ResolveState() error = %v, want nil (cached validation, not re-walked)", err)
	}
}

// blockingClassifier signals entry, then blocks until its ctx is cancelled.
type blockingClassifier struct {
	entered chan struct{}
}

func (b *blockingClassifier) Classify(ctx context.Context, input string, grounding any) (intents.ChoiceAnswer, error) {
	close(b.entered)
	<-ctx.Done()
	return intents.ChoiceAnswer{}, ctx.Err()
}

var _ intents.Classifier = (*blockingClassifier)(nil)

func TestResolveState_CancelledDuringBackendCall_ReturnsWrappedContextErrorNotIncompleteError(t *testing.T) {
	bc := &blockingClassifier{entered: make(chan struct{})}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", bc)
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-bc.entered
		cancel()
	}()

	_, err := e.ResolveState(ctx, root, "input", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveState() error = %v, want errors.Is context.Canceled", err)
	}
	var incomplete *intents.IncompleteError
	if errors.As(err, &incomplete) {
		t.Fatalf("ResolveState() error is *IncompleteError, want a plain wrapped context error")
	}
}

type appAction struct {
	Key string
}

func TestResolve_PositivePathBuildsTFromState(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	got, err := intents.Resolve(context.Background(), e, root, "input", nil, func(s intents.State) (appAction, error) {
		return appAction{Key: s.Terminal}, nil
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.Key != "root" {
		t.Fatalf("got.Key = %q, want %q", got.Key, "root")
	}
}

func TestResolve_IncompleteGivesZeroTAndIncompleteErrorBuilderNotCalled(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	var buildCalled bool
	got, err := intents.Resolve(context.Background(), e, root, "input", nil, func(s intents.State) (appAction, error) {
		buildCalled = true
		return appAction{}, nil
	})
	if _, ok := err.(*intents.IncompleteError); !ok {
		t.Fatalf("Resolve() error = %v (%T), want *IncompleteError", err, err)
	}
	if got != (appAction{}) {
		t.Fatalf("got = %+v, want zero value", got)
	}
	if buildCalled {
		t.Fatalf("build was called despite incomplete resolution")
	}
}

func TestResolve_BuilderErrorReturnedWrapped(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}
	builderErr := errors.New("bad shape")

	_, err := intents.Resolve(context.Background(), e, root, "input", nil, func(s intents.State) (appAction, error) {
		return appAction{}, builderErr
	})
	if !errors.Is(err, builderErr) {
		t.Fatalf("Resolve() error = %v, want errors.Is builderErr", err)
	}
}

func TestResolve_NilBuildPanics(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}}

	defer func() {
		if recover() == nil {
			t.Fatalf("Resolve() with nil build did not panic")
		}
	}()
	_, _ = intents.Resolve[appAction](context.Background(), e, root, "input", nil, nil)
}
