// resume_test.go
package intents_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ezeike/intents"
)

func classifySpecFor(t *testing.T, backend string, options map[string]string) *intents.ClassifySpec {
	t.Helper()
	return &intents.ClassifySpec{Backend: backend, Options: options}
}

func TestResumeState_PositivePathWithAnswerValueSkipsReResolution(t *testing.T) {
	fc := &fakeClassifier{choice: "other", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"})}

	prior, err := e.ResolveState(context.Background(), root, "input", nil)
	if _, ok := err.(*intents.IncompleteError); !ok {
		t.Fatalf("setup ResolveState() error = %v (%T), want *IncompleteError", err, err)
	}
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a", Source: "user"}
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{
		"root": {Value: &answerVal},
	})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if state.Values["root"].Choice != "a" {
		t.Fatalf("state.Values[root].Choice = %q, want %q", state.Values["root"].Choice, "a")
	}
	if fc.callCount() != 1 {
		t.Fatalf("Classify call count = %d, want 1 (only the original ResolveState call)", fc.callCount())
	}
}

func TestResumeState_PositivePathWithAnswerRawReRunsBackend(t *testing.T) {
	fc := &fakeClassifier{choice: "other", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"})}

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	fc.choice = "a" // simulate the re-run producing a resolvable answer
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{
		"root": {Raw: "clarified input"},
	})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if state.Values["root"].Choice != "a" {
		t.Fatalf("state.Values[root].Choice = %q, want %q", state.Values["root"].Choice, "a")
	}
	if fc.callCount() != 2 {
		t.Fatalf("Classify call count = %d, want 2 (original + re-run)", fc.callCount())
	}
	if fc.lastInput != "clarified input" {
		t.Fatalf("Classify lastInput = %q, want %q", fc.lastInput, "clarified input")
	}
	if state.Input != "input" {
		t.Fatalf("state.Input = %q, want unchanged %q", state.Input, "input")
	}
}

func TestResumeState_AnsweredTreeNodeContinuesDescending(t *testing.T) {
	child := &intents.Node{Key: "child", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"c": "c"})}
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"}),
		Children: map[string]*intents.Node{"a": child},
	}
	fc := &fakeClassifier{choice: "other", confidence: 1}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	fc.choice = "c"
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root": {Value: &answerVal}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if state.Terminal != "child" {
		t.Fatalf("state.Terminal = %q, want %q (descended into child after answer)", state.Terminal, "child")
	}
}

func TestResumeState_AnswerUnblocksPreviouslyUnresolvedSlot(t *testing.T) {
	classify := classifySpecFor(t, "laya", map[string]string{"a": "a"})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "first", Kind: intents.KindClassify, Classify: classify, Required: true},
			{Name: "second", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"first"}, Required: true},
		},
	}
	fc := &fakeClassifier{err: errors.New("boom")}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fc)

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	fc.mu.Lock()
	fc.err = nil
	fc.choice = "a"
	fc.confidence = 1
	fc.mu.Unlock()

	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root.first": {Value: &answerVal}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if _, ok := state.Values["second"]; !ok {
		t.Fatalf("state.Values missing %q, want it unblocked and resolved", "second")
	}
}

func TestResumeState_UnrelatedResolvedSlotIsNotReRun(t *testing.T) {
	classify := classifySpecFor(t, "laya", map[string]string{"a": "a"})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "resolved", Kind: intents.KindClassify, Classify: classify, Required: true},
			{Name: "failing", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya2", map[string]string{"a": "a"}), Required: true},
		},
	}
	fcOK := &fakeClassifier{choice: "a", confidence: 1}
	fcFail := &fakeClassifier{err: errors.New("boom")}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", fcOK)
	e.RegisterClassifier("laya2", fcFail)

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	_, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root.failing": {Value: &answerVal}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if fcOK.callCount() != 1 {
		t.Fatalf("resolved slot's Classify call count = %d, want 1 (never re-run)", fcOK.callCount())
	}
}

func TestResumeState_FailedForAnotherReasonOptionalSlotStaysFailedWhenUnanswered(t *testing.T) {
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "required", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"}), Required: true},
			{Name: "optional", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya2", map[string]string{"a": "a"}), Required: false},
		},
	}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{err: errors.New("boom")})
	e.RegisterClassifier("laya2", &fakeClassifier{err: errors.New("boom2")})

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root.required": {Value: &answerVal}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	var stillFailed bool
	for _, f := range state.Failures {
		if f.Path == "root.optional" {
			stillFailed = true
		}
	}
	if !stillFailed {
		t.Fatalf("optional slot failure was cleared without an answer, want it to stay failed")
	}
}

func TestResumeState_IncompleteAnswersReturnsErrUnansweredFailure(t *testing.T) {
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"})}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})

	prior, _ := e.ResolveState(context.Background(), root, "input", nil)
	_, err := e.ResumeState(context.Background(), root, prior, intents.Answers{})
	if !errors.Is(err, intents.ErrUnansweredFailure) {
		t.Fatalf("ResumeState() error = %v, want errors.Is ErrUnansweredFailure", err)
	}
}

func TestResumeState_UnknownAnswerPathReturnsErrUnknownAnswerPath(t *testing.T) {
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"})}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})

	prior, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("setup ResolveState() error = %v, want nil", err)
	}
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	_, err = e.ResumeState(context.Background(), root, prior, intents.Answers{"no.such.path": {Value: &answerVal}})
	if !errors.Is(err, intents.ErrUnknownAnswerPath) {
		t.Fatalf("ResumeState() error = %v, want errors.Is ErrUnknownAnswerPath", err)
	}
}

func TestResumeState_InputAndContextUnchanged(t *testing.T) {
	root := &intents.Node{Key: "root", Kind: intents.KindClassify, Classify: classifySpecFor(t, "laya", map[string]string{"a": "a"})}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "other", confidence: 1})

	prior, _ := e.ResolveState(context.Background(), root, "original input", "original context")
	answerVal := intents.Value{Kind: intents.ValueKindChoice, Choice: "a"}
	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root": {Value: &answerVal}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if state.Input != "original input" || state.Context != "original context" {
		t.Fatalf("state.Input/Context = %q/%v, want unchanged", state.Input, state.Context)
	}
}

func TestResumeState_RawReRunOfCustomNodeOverwritesReturnedRiskTier(t *testing.T) {
	calls := 0
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindCustom,
		RiskTier: intents.RiskMedium,
		Custom: func(ctx context.Context, state intents.State) (intents.Value, error) {
			calls++
			if calls == 1 {
				return intents.Value{}, intents.ErrInvalidInput
			}
			return intents.Value{Kind: intents.ValueKindChoice, Choice: "x", RiskTier: intents.RiskCritical}, nil
		},
	}
	e := intents.NewEngine()
	prior, _ := e.ResolveState(context.Background(), root, "input", nil)

	state, err := e.ResumeState(context.Background(), root, prior, intents.Answers{"root": {Raw: "clarified"}})
	if err != nil {
		t.Fatalf("ResumeState() error = %v, want nil", err)
	}
	if got := state.Values["root"].RiskTier; got != intents.RiskMedium {
		t.Fatalf("state.Values[root].RiskTier = %v, want %v (computed tier, not NodeFunc's)", got, intents.RiskMedium)
	}
}

func TestResumeState_CancelledRawReRunKeepsAnsweredFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindCustom,
		Custom: func(context.Context, intents.State) (intents.Value, error) {
			calls++
			if calls == 2 {
				cancel()
			}
			return intents.Value{}, intents.ErrInvalidInput
		},
	}
	e := intents.NewEngine()
	prior, _ := e.ResolveState(context.Background(), root, "input", nil)

	state, err := e.ResumeState(ctx, root, prior, intents.Answers{"root": {Raw: "clarified"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResumeState() error = %v, want errors.Is context.Canceled", err)
	}
	if len(state.Failures) != 1 || state.Failures[0].Path != "root" {
		t.Fatalf("state.Failures = %+v, want the answered Failure at %q kept", state.Failures, "root")
	}
	if _, err := e.ResumeState(context.Background(), root, state, intents.Answers{"root": {Raw: "again"}}); errors.Is(err, intents.ErrUnknownAnswerPath) {
		t.Fatalf("retry ResumeState() error = %v, want the partial State to remain resumable", err)
	}
}
