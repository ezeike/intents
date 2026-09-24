//go:build integration

// The `model` field: "english" | "multilingual" | "typed-decisions", a
// published HF id (mapped onto one of the three), or anything else --
// silently ignored, falling back to auto-routing. laya/serve.py:_resolve_model.
package laya_test

import "testing"

func TestModelSelection_ExplicitTypedDecisions_RoutesToTypedDecisions(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "send 10 cnpy to bob",
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "is this a send action?"},
		},
		Model: "typed-decisions",
	})

	if resp.Routing == nil {
		t.Fatal("routing is nil, want routing info")
	}
	if resp.Routing.Model != "typed-decisions" {
		t.Errorf("routing.model = %q, want %q", resp.Routing.Model, "typed-decisions")
	}
}

func TestModelSelection_PublishedHFId_MapsToTypedDecisions(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "send 10 cnpy to bob",
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "is this a send action?"},
		},
		Model: "convaiinnovations/laya-typed-decisions",
	})

	if resp.Routing == nil {
		t.Fatal("routing is nil, want routing info")
	}
	if resp.Routing.Model != "typed-decisions" {
		t.Errorf("routing.model = %q, want %q (published HF id must map onto the checkpoint)", resp.Routing.Model, "typed-decisions")
	}
}

func TestModelSelection_UnknownModelString_IgnoredFallsBackToAutoRouting(t *testing.T) {
	base := requireLaya(t)

	// "gpt-4" is not a Laya checkpoint name, alias, or published HF id --
	// per _resolve_model, an unrecognized value is silently ignored
	// (treated as "no explicit checkpoint"), not an error.
	status, data := postSystemOne(t, base, systemOneRequest{
		State: "hello there, how are you",
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "is this a greeting?"},
		},
		Model: "gpt-4",
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200 (unknown model silently ignored): %s", status, data)
	}
}

func TestModelSelection_OmittedModel_AutoRoutes(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "hello there, how are you",
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "is this a greeting?"},
		},
		// Model intentionally omitted.
	})

	if resp.Routing == nil {
		t.Fatal("routing is nil, want the router to still record an auto-routing decision")
	}
	if resp.Routing.Reason == "" {
		t.Error("routing.reason is empty, want an explanation of the auto-routing decision")
	}
}

func TestModelSelection_ExplicitEnglish_RoutesToEnglish(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "hello there, how are you",
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "is this a greeting?"},
		},
		Model: "english",
	})

	if resp.Routing == nil {
		t.Fatal("routing is nil, want routing info")
	}
	if resp.Routing.Model != "english" {
		t.Errorf("routing.model = %q, want %q", resp.Routing.Model, "english")
	}
}
