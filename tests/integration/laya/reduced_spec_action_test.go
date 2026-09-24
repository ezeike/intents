//go:build integration

// Historical record of the cardinality-reduction progression that led to
// spec_action_test.go's 2-way accept_proposal/discard_proposal-only scope.
// The 8-way set's noise/confusable options (propose_change, direct_edit,
// validate, fetch_current, list_primitives, general_chat) were removed one
// at a time in earlier revisions of this file -- 4-way, then 3-way, then
// 2-way -- each step recorded in the laya skill with real accuracy numbers
// (65% -> 85% -> 100% on the 20-phrasing proposal grid). propose_spec
// (formerly propose_change) was later reinstated in spec_action_test.go:
// production needs a third outcome -- "wants a new proposal drafted and
// sent" -- distinct from accept/discard and from general out-of-scope input,
// so specActionCriteria() is now 3-way again. specActionCriteria2 below
// still just delegates to it rather than duplicating it -- kept as its own
// name for callers (out_of_scope_detection_test.go) that specifically mean
// "the reduced/final criteria," not "whatever spec_action_test.go happens to
// define today."
package laya_test

import "testing"

// specActionCriteria2 is spec_action_test.go's canonical criteria set, under
// the name this file's other helpers (specActionCriteriaWithOther,
// out_of_scope_detection_test.go) already reference. No longer strictly
// 2-way (propose_spec was reinstated) -- name kept for continuity.
func specActionCriteria2() map[string]string {
	return specActionCriteria()
}

// specActionCriteriaWithOther adds an explicit reject-option extra choice to
// the accept/discard/propose_spec set, per the design spec's ClassifySpec.
// Fallback pattern ("tried if Choice == \"other\" or below MinConfidence").
// out_of_scope_detection_test.go found margin-thresholding on the FORCED
// binary catches out-of-scope inputs only partially, because a forced binary
// has no way to express "neither of these." This tests whether giving the
// model an explicit third option to pick instead is a real fix rather than a
// threshold workaround -- it measurably is not (see that file): recall on
// out-of-scope inputs dropped rather than improved, and it cost one
// previously-correct in-scope case too.
func specActionCriteriaWithOther() map[string]string {
	c := specActionCriteria2()
	c["other"] = "none of the above -- the message is not accepting or rejecting a staged proposal at all"
	return c
}

func askSpecActionWithCriteria(t *testing.T, base, input string, criteria map[string]string) choiceAnswer {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     criteria,
			},
		},
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "spec_action")
}

// runReducedGrid runs the 20-phrasing proposal grid through an arbitrary
// criteria set and logs per-case results plus overall accuracy. Shared by
// this file's own test and out_of_scope_detection_test.go's in-scope half.
func runReducedGrid(t *testing.T, base string, criteria map[string]string, label string) {
	t.Helper()
	totalCorrect, totalCases := 0, 0
	for _, target := range proposalGridTargets() {
		hits := 0
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, base, phrasing, criteria)
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

// TestReducedSpecAction_ProposalGrid confirms spec_action_test.go's actual
// production scope (accept_proposal/discard_proposal/propose_spec) against
// the full proposal grid, now including propose_spec's own phrasings.
func TestReducedSpecAction_ProposalGrid(t *testing.T) {
	base := requireLaya(t)
	runReducedGrid(t, base, specActionCriteria2(), "reduced (accept/discard/propose_spec)")
}
