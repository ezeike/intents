//go:build integration

package laya_test

import (
	"math"
	"testing"
)

func TestNoul_ReturnsValueBetweenZeroAndOne(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "yes, go ahead and send it",
		Questions: map[string]questionDef{
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user already confirmed this action?",
			},
		},
		Model: "typed-decisions",
	})

	a := decodeNoul(t, resp, "confirmed")
	if a.Type != "noul" {
		t.Errorf("type = %q, want %q", a.Type, "noul")
	}
	if a.Noul < 0 || a.Noul > 1 {
		t.Errorf("noul = %v, out of [0,1]", a.Noul)
	}
}

func TestNoul_OmittedCriteria_IsValid(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "yes, confirmed",
		Questions: map[string]questionDef{
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user confirmed?",
				// Criteria intentionally omitted -- spec: "criteria optional".
			},
		},
		Model: "typed-decisions",
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200: %s", status, data)
	}
}

func TestNoul_CriteriaWithOnlyFalseKey_Accepted(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "not yet",
		Questions: map[string]questionDef{
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user confirmed?",
				Criteria:     map[string]string{"false": "user has not confirmed"},
			},
		},
		Model: "typed-decisions",
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200: %s", status, data)
	}
}

func TestNoul_CriteriaWithOnlyTrueKey_Accepted(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "yes",
		Questions: map[string]questionDef{
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user confirmed?",
				Criteria:     map[string]string{"true": "user has confirmed"},
			},
		},
		Model: "typed-decisions",
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200: %s", status, data)
	}
}

func TestNoul_Confidence_EqualsMaxOfNoulAndComplement(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "absolutely yes, do it now",
		Questions: map[string]questionDef{
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user confirmed?",
			},
		},
		Model: "typed-decisions",
	})

	a := decodeNoul(t, resp, "confirmed")
	want := math.Max(a.Noul, 1.0-a.Noul)
	if math.Abs(a.Confidence-want) > 0.001 {
		t.Errorf("confidence = %v, want max(noul, 1-noul) = %v (noul=%v)", a.Confidence, want, a.Noul)
	}
}

func TestNoul_InvalidCriteriaKey_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "noul",
				Instructions: "x",
				Criteria:     map[string]string{"maybe": "y"}, // only "true"/"false" are valid keys
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
	e := decodeError(t, data)
	if e.Detail == "" {
		t.Error("detail is empty, want an explanation naming the valid keys")
	}
}

func TestNoul_LabelsOnChoiceQuestion_Returns422(t *testing.T) {
	base := requireLaya(t)

	// `labels` is documented as noul-only; using it on a choice question must be rejected.
	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "choice",
				Instructions: "pick one",
				Criteria:     map[string]string{"a": "a", "b": "b"},
				Labels:       map[string]string{"a": "A", "b": "B"},
			},
		},
	})

	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}
