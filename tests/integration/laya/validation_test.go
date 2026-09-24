//go:build integration

// Malformed/invalid request bodies. Status-code split, per laya/serve.py and
// laya/agent.py: 400 for structurally-bad requests (bad JSON, wrong shape),
// 422 for structurally-valid-but-semantically-invalid question definitions.
package laya_test

import (
	"strings"
	"testing"
)

func TestValidation_MalformedJSON_Returns400(t *testing.T) {
	base := requireLaya(t)

	status, data := rawPostSystemOne(t, base, []byte(`{not valid json`))
	if status != 400 {
		t.Fatalf("status = %d, want 400: %s", status, data)
	}
}

func TestValidation_EmptyBody_Returns400(t *testing.T) {
	base := requireLaya(t)

	status, data := rawPostSystemOne(t, base, []byte(``))
	if status != 400 {
		t.Fatalf("status = %d, want 400: %s", status, data)
	}
}

func TestValidation_MissingQuestionsField_Returns400(t *testing.T) {
	base := requireLaya(t)

	status, data := rawPostSystemOne(t, base, []byte(`{"state":"x"}`))
	if status != 400 {
		t.Fatalf("status = %d, want 400: %s", status, data)
	}
}

func TestValidation_QuestionsAsListNotObject_Returns400(t *testing.T) {
	base := requireLaya(t)

	status, data := rawPostSystemOne(t, base, []byte(`{"state":"x","questions":[]}`))
	if status != 400 {
		t.Fatalf("status = %d, want 400: %s", status, data)
	}
}

func TestValidation_QuestionDefNotAnObject_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := rawPostSystemOne(t, base, []byte(`{"state":"x","questions":{"q1":"nope"}}`))
	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}

func TestValidation_UnknownQuestionType_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {Type: "bogus", Instructions: "x"},
		},
	})
	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}

func TestValidation_ChoiceCriteriaWrongType_Returns422(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"q1": {Type: "choice", Instructions: "x", Criteria: "nope"}, // must be dict or list
		},
	})
	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
}

func TestValidation_ErrorDetail_NamesTheOffendingQuestion(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "x",
		Questions: map[string]questionDef{
			"my_question_id": {Type: "bogus", Instructions: "x"},
		},
	})
	if status != 422 {
		t.Fatalf("status = %d, want 422: %s", status, data)
	}
	e := decodeError(t, data)
	if !strings.Contains(e.Detail, "my_question_id") {
		t.Errorf("detail = %q, want it to name the offending question %q", e.Detail, "my_question_id")
	}
}
