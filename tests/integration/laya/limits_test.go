//go:build integration

// Server-enforced request-size guardrails, per laya/serve.py:
// MAX_QUESTIONS = 64, MAX_STATE_CHARS = 50_000, MAX_BODY_BYTES = 2 * 1024 * 1024.
package laya_test

import (
	"fmt"
	"strings"
	"testing"
)

func buildNQuestions(n int) map[string]questionDef {
	qs := make(map[string]questionDef, n)
	for i := 0; i < n; i++ {
		qs[fmt.Sprintf("q%d", i)] = questionDef{
			Type:         "noul",
			Instructions: "is this a filler question?",
		}
	}
	return qs
}

func TestLimits_TooManyQuestions_Returns413(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State:     "x",
		Questions: buildNQuestions(65), // MAX_QUESTIONS = 64
	})

	if status != 413 {
		t.Fatalf("status = %d, want 413: %s", status, data)
	}
}

func TestLimits_ExactlyMaxQuestions_Accepted(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State:     "x",
		Questions: buildNQuestions(64), // MAX_QUESTIONS boundary, not over
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200 at the 64-question boundary: %s", status, data)
	}
}

func TestLimits_StateTooLarge_Returns413(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: strings.Repeat("a", 50001), // MAX_STATE_CHARS = 50000
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "x"},
		},
	})

	if status != 413 {
		t.Fatalf("status = %d, want 413: %s", status, data)
	}
}

func TestLimits_StateAtMaxChars_Accepted(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: strings.Repeat("a", 50000), // MAX_STATE_CHARS boundary, not over
		Questions: map[string]questionDef{
			"q1": {Type: "noul", Instructions: "x"},
		},
	})

	if status != 200 {
		t.Fatalf("status = %d, want 200 at the 50000-char boundary: %s", status, data)
	}
}

func TestLimits_BodyTooLarge_Returns413(t *testing.T) {
	base := requireLaya(t)

	// MAX_BODY_BYTES = 2MiB. Bloat via one question's instructions text --
	// the body-size check happens on Content-Length before JSON parsing at
	// all, so it doesn't matter which field carries the bulk.
	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {
				Type:         "noul",
				Instructions: strings.Repeat("a", 3*1024*1024), // 3MiB, over MAX_BODY_BYTES
			},
		},
	})

	if status != 413 {
		t.Fatalf("status = %d, want 413: %s", status, data)
	}
}
