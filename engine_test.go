// engine_test.go
package intents_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents"
)

func TestNewEngine_ReturnsUsableEngine(t *testing.T) {
	e := intents.NewEngine()
	if e == nil {
		t.Fatalf("NewEngine() = nil")
	}
	// Must not panic on first Register call — the zero-value-Engine failure mode decision 9 rules out.
	e.RegisterClassifier("laya", &fakeClassifier{})
	e.RegisterLookuper("sqllookup", &fakeLookuper{})
}

func TestRegisterClassifier_EmptyName_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("RegisterClassifier(\"\", ...) did not panic")
		}
	}()
	intents.NewEngine().RegisterClassifier("", &fakeClassifier{})
}

func TestRegisterClassifier_NilImplementation_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("RegisterClassifier(name, nil) did not panic")
		}
	}()
	intents.NewEngine().RegisterClassifier("laya", nil)
}

func TestRegisterLookuper_EmptyName_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("RegisterLookuper(\"\", ...) did not panic")
		}
	}()
	intents.NewEngine().RegisterLookuper("", &fakeLookuper{})
}

func TestRegisterLookuper_NilImplementation_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("RegisterLookuper(name, nil) did not panic")
		}
	}()
	intents.NewEngine().RegisterLookuper("sqllookup", nil)
}

func TestRegisterClassifier_ReregisterSameName_ReplacesImplementation(t *testing.T) {
	e := intents.NewEngine()
	e.RegisterClassifier("laya", &fakeClassifier{choice: "a", confidence: 1})
	e.RegisterClassifier("laya", &fakeClassifier{choice: "b", confidence: 1})

	spec := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "a", "b": "b"}},
	}
	state, err := e.ResolveState(context.Background(), spec, "input", nil)
	if err != nil {
		t.Fatalf("ResolveState() error = %v, want nil", err)
	}
	if state.Values["root"].Choice != "b" {
		t.Fatalf("state.Values[root].Choice = %q, want %q (replaced implementation)", state.Values["root"].Choice, "b")
	}
}
