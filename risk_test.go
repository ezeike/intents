// risk_test.go
package intents_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
)

func TestRiskTier_InheritedAsMaxOverAncestorsOnValue(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "child", confidence: 1})
	child := &intents.Node{
		Key:      "child",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"child": "child"}},
		RiskTier: intents.RiskLow,
	}
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"child": "child"}},
		Children: map[string]*intents.Node{"child": child},
		RiskTier: intents.RiskHigh,
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["child"].RiskTier != intents.RiskHigh {
		t.Fatalf("state.Values[child].RiskTier = %v, want %v (inherited from root)", state.Values["child"].RiskTier, intents.RiskHigh)
	}
}

func TestRiskTier_StateRiskTierIsMaxOverResolvedValues(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}}
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "low", Kind: intents.KindClassify, Classify: classify, Required: true},
		},
		RiskTier: intents.RiskCritical,
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.RiskTier != intents.RiskCritical {
		t.Fatalf("state.RiskTier = %v, want %v", state.RiskTier, intents.RiskCritical)
	}
}

func TestRiskTier_CriticalResultIsStillReturnedNotBlocked(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a"}},
		RiskTier: intents.RiskCritical,
	}

	state, err := e.ResolveState(context.Background(), root, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil (engine never blocks on RiskTier)", err)
	}
	if _, ok := state.Values["root"]; !ok {
		t.Fatalf("state.Values missing %q despite RiskCritical", "root")
	}
}
