//go:build integration

package laya_test

import "testing"

func TestMultiQuestion_MixedTypesInSingleRequest_ReturnsAllAnswers(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "URGENT, send now, yes I confirm",
		Questions: map[string]questionDef{
			"asset": {
				Type:         "choice",
				Instructions: "Which token?",
				Criteria:     map[string]string{"cnpy": "the native token", "chainx": "chain x token"},
			},
			"urgency": {
				Type:         "score",
				Instructions: "How urgent, 0-3?",
				Criteria:     []string{"none", "mild", "urgent", "very urgent"},
			},
			"confirmed": {
				Type:         "noul",
				Instructions: "Has the user confirmed?",
			},
		},
		Model: "typed-decisions",
	})

	if len(resp.Answers) != 3 {
		t.Fatalf("got %d answers, want 3: %+v", len(resp.Answers), resp.Answers)
	}

	choice := decodeChoice(t, resp, "asset")
	if choice.Type != "choice" {
		t.Errorf("asset.type = %q, want %q", choice.Type, "choice")
	}
	score := decodeScore(t, resp, "urgency")
	if score.Type != "score" {
		t.Errorf("urgency.type = %q, want %q", score.Type, "score")
	}
	noul := decodeNoul(t, resp, "confirmed")
	if noul.Type != "noul" {
		t.Errorf("confirmed.type = %q, want %q", noul.Type, "noul")
	}
}

func TestMultiQuestion_AnswersKeyedByQuestionID_NotByDeclarationOrder(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State: "send 10 chainx to bob",
		Questions: map[string]questionDef{
			"zzz_last_alphabetically": {
				Type:         "noul",
				Instructions: "Is this urgent?",
			},
			"aaa_first_alphabetically": {
				Type:         "noul",
				Instructions: "Is this a send action?",
			},
		},
		Model: "typed-decisions",
	})

	if _, ok := resp.Answers["zzz_last_alphabetically"]; !ok {
		t.Error("answers missing zzz_last_alphabetically")
	}
	if _, ok := resp.Answers["aaa_first_alphabetically"]; !ok {
		t.Error("answers missing aaa_first_alphabetically")
	}
}

func TestMultiQuestion_EmptyQuestions_ReturnsEmptyAnswersZeroUsage(t *testing.T) {
	base := requireLaya(t)

	resp := mustOK(t, base, systemOneRequest{
		State:     "anything",
		Questions: map[string]questionDef{},
	})

	if len(resp.Answers) != 0 {
		t.Errorf("answers = %+v, want empty", resp.Answers)
	}
	if resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		t.Errorf("usage = %+v, want zero tokens for zero questions", resp.Usage)
	}
}

func TestMultiQuestion_OneInvalidQuestionAmongValid_RejectsWholeRequest(t *testing.T) {
	base := requireLaya(t)

	status, data := postSystemOne(t, base, systemOneRequest{
		State: "send 10 cnpy to alice",
		Questions: map[string]questionDef{
			"valid_one": {
				Type:         "noul",
				Instructions: "Is this a send action?",
			},
			"invalid_one": {
				Type:         "bogus", // unknown type
				Instructions: "x",
			},
		},
	})

	// Validation runs over the whole batch before any inference: a single bad
	// question fails the entire request, none of the others get answered either.
	if status != 422 {
		t.Fatalf("status = %d, want 422 (whole batch rejected): %s", status, data)
	}
}
