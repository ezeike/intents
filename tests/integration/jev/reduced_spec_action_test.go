//go:build integration

// Jev counterpart to tests/integration/laya/reduced_spec_action_test.go. In the laya suite this
// file was the historical record of an 8-way -> 2-way -> 3-way cardinality-reduction progression;
// against Jev, specActionCriteria() (confidence_calibration_test.go) already starts at the
// production 3-way accept_proposal/discard_proposal/propose_spec scope, so this file exists to
// give out_of_scope_detection_test.go the shared helpers it needs (specActionCriteriaWithOther,
// askSpecActionWithCriteria) and to confirm that scope against the same 30-phrasing proposal grid
// used throughout this package.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

// specActionCriteria2 is specActionCriteria()'s (confidence_calibration_test.go) canonical
// criteria set, under the name this file's other helpers and out_of_scope_detection_test.go
// already reference -- kept as its own name for continuity with the laya suite this mirrors.
func specActionCriteria2() jev.OrderedCriteria {
	return specActionCriteria()
}

// specActionCriteriaWithOther adds an explicit "other" catch-all option to the
// accept/discard/propose_spec set. out_of_scope_detection_test.go's laya counterpart found
// margin-thresholding on the forced 3-way catches out-of-scope inputs only partially, because a
// forced choice has no way to express "none of these" -- this tests whether giving the model an
// explicit extra option to pick instead is a real fix, or costs accuracy on genuinely in-scope
// cases, against live Jev traffic specifically (the laya result measurably found it was not a
// real fix there).
func specActionCriteriaWithOther() jev.OrderedCriteria {
	c := append(jev.OrderedCriteria{}, specActionCriteria2()...)
	return append(c, jev.Criterion{
		Key:         "other",
		Description: "none of the above -- the message is not accepting or rejecting a staged proposal at all",
	})
}

func askSpecActionWithCriteria(t *testing.T, c *jev.Client, input string, criteria jev.OrderedCriteria) jev.ChoiceAnswer {
	t.Helper()
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: map[string]string{"input": input},
		Questions: map[string]jev.Question{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     criteria,
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(%q): %v", input, err)
	}
	a, err := resp.Choice("spec_action")
	if err != nil {
		t.Fatalf("Choice(%q): %v", input, err)
	}
	return a
}

// askSpecAction is askSpecActionWithCriteria pinned to the production 3-way criteria set --
// shared by escalation_alternatives_test.go.
func askSpecAction(t *testing.T, c *jev.Client, input string) jev.ChoiceAnswer {
	t.Helper()
	return askSpecActionWithCriteria(t, c, input, specActionCriteria2())
}

// runReducedGrid runs the 30-phrasing proposal grid through an arbitrary criteria set and logs
// per-case results plus overall accuracy. Shared by this file's own test and
// out_of_scope_detection_test.go's in-scope half.
func runReducedGrid(t *testing.T, c *jev.Client, criteria jev.OrderedCriteria, label string) {
	t.Helper()
	totalCorrect, totalCases := 0, 0
	for _, target := range proposalGridTargets() {
		hits := 0
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, c, phrasing, criteria)
			correct := a.Choice == target.want
			if correct {
				hits++
			}
			t.Logf("[want=%-16s] %-45q -> %-16s (correct=%v) distribution=%+v",
				target.want, phrasing, a.Choice, correct, a.Probabilities)
		}
		t.Logf("== summary target=%s: %d/%d correct ==", target.want, hits, len(target.phrasings))
		totalCorrect += hits
		totalCases += len(target.phrasings)
	}
	t.Logf("== overall %s: %d/%d correct ==", label, totalCorrect, totalCases)
}

// TestReducedSpecAction_ProposalGrid confirms the production 3-way scope
// (accept_proposal/discard_proposal/propose_spec) against the full proposal grid.
func TestReducedSpecAction_ProposalGrid(t *testing.T) {
	c := requireJev(t)
	runReducedGrid(t, c, specActionCriteria2(), "reduced (accept/discard/propose_spec)")
}
