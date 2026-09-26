//go:build integration

// Jev counterpart to tests/integration/laya/referential_gate_test.go. That file's solution came
// after four failed attempts (bool-flag state, prose state, prose instructions, hierarchical
// decomposition -- all either did nothing or made things worse against live Laya, per the laya
// skill) at getting proposal-staging context to actually influence spec_action. The fix that
// worked: don't ask the model whether a proposal is staged -- that's app state, known for
// certain by the caller. Instead ask a narrow "is this message referring back to something
// already shown to the user" noul, batched in the SAME request as spec_action (one round trip,
// one state, no new state/instructions fields), then combine with the caller's own certain
// knowledge in code:
//
//	if hasStagedProposal && is_referential > threshold: use the accept/reject binary to decide
//	  accept_proposal vs discard_proposal, overriding spec_action's raw pick
//	else: trust spec_action's raw pick
//
// This file reruns that exact pattern against live Jev, using the same proposal grid, to see
// whether the gate's improvement over the raw classifier (measured for Laya: 45-60% baseline ->
// 80% via the gate) reproduces on Jev's different underlying checkpoint.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

const referentialInstructions = "Does this message refer back to something already proposed or " +
	"shown to the user -- using words like \"it\", \"that\", or a short confirmation/rejection -- " +
	"rather than describing something new from scratch?"

// referentialGateThreshold is the is_referential cutoff spec_action_test.go and this file's own
// tests use in production-style calls (as opposed to the threshold-comparison sweep below).
// Inherited from the laya suite's tuned value (0.4 beat 0.5 there: 17/20 vs 16/20 on the proposal
// grid, identical precision) -- not yet independently retuned against Jev traffic; the sweep test
// below is what would confirm or correct it for this backend.
const referentialGateThreshold = 0.4

type batchedAnswers struct {
	specAction   jev.ChoiceAnswer
	referential  jev.NoulAnswer
	acceptReject jev.ChoiceAnswer
}

// askBatched sends spec_action, is_referential, and decision (accept/reject, reusing
// acceptRejectCriteria from general_accept_reject_test.go) as three questions in ONE
// /v1/systemone request -- one shared state, one round trip.
func askBatched(t *testing.T, c *jev.Client, input string) batchedAnswers {
	t.Helper()
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: map[string]string{"input": input},
		Questions: map[string]jev.Question{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     specActionCriteria(),
			},
			"is_referential": {
				Type:         "noul",
				Instructions: referentialInstructions,
			},
			"decision": {
				Type:         "choice",
				Instructions: "Is the person accepting/agreeing, or rejecting/declining?",
				Criteria:     acceptRejectCriteria(),
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(%q): %v", input, err)
	}

	specAction, err := resp.Choice("spec_action")
	if err != nil {
		t.Fatalf("Choice(spec_action, %q): %v", input, err)
	}
	referential, err := resp.Noul("is_referential")
	if err != nil {
		t.Fatalf("Noul(is_referential, %q): %v", input, err)
	}
	acceptReject, err := resp.Choice("decision")
	if err != nil {
		t.Fatalf("Choice(decision, %q): %v", input, err)
	}

	return batchedAnswers{specAction: specAction, referential: referential, acceptReject: acceptReject}
}

// resolveGate is the pure decision this file validates, split out from the network call
// (askBatched) so the same forward-pass result can be scored against multiple thresholds without
// duplicating live requests. hasStagedProposal is caller-known app state, never asked of the
// model.
func resolveGate(a batchedAnswers, hasStagedProposal bool, threshold float64) (choice string, viaGate bool) {
	if hasStagedProposal && a.referential.Noul > threshold {
		if a.acceptReject.Choice == "accept" {
			return "accept_proposal", true
		}
		return "discard_proposal", true
	}
	return a.specAction.Choice, false
}

// gateThresholds are compared side by side against the exact same live forward-pass results: 0.5
// is the laya-tuned baseline, 0.4 is the laya-tuned improvement -- both retested here against Jev
// to see whether the same threshold ordering holds on a different checkpoint.
var gateThresholds = []float64{0.5, 0.4}

// TestReferentialGate_ProposalGrid reruns the same 30 accept/discard/propose_spec phrasings from
// proposal_grid_test.go, all with hasStagedProposal=true (matching that grid's proposal_exists
// condition), through the batched referential-gate resolver at both thresholds. Compare against
// the laya skill's recorded Laya numbers (flat baseline 45-60%, hierarchical 20%, gate@0.5 80%).
func TestReferentialGate_ProposalGrid(t *testing.T) {
	c := requireJev(t)

	totalCorrect := map[float64]int{}
	gateFired := map[float64]int{}
	totalCases := 0
	for _, target := range proposalGridTargets() {
		hits := map[float64]int{}
		for _, phrasing := range target.phrasings {
			a := askBatched(t, c, phrasing)
			t.Logf("batched(%q): spec_action=%s is_referential=%.3f decision=%s (accept=%.3f reject=%.3f)",
				phrasing, a.specAction.Choice, a.referential.Noul, a.acceptReject.Choice,
				a.acceptReject.Probabilities["accept"], a.acceptReject.Probabilities["reject"])
			for _, th := range gateThresholds {
				choice, viaGate := resolveGate(a, true, th)
				correct := choice == target.want
				if correct {
					hits[th]++
					totalCorrect[th]++
				}
				if viaGate {
					gateFired[th]++
				}
				t.Logf("  [threshold=%.1f want=%-16s] -> %-16s (via_gate=%v, correct=%v)",
					th, target.want, choice, viaGate, correct)
			}
		}
		for _, th := range gateThresholds {
			t.Logf("== summary target=%s threshold=%.1f: %d/%d correct ==", target.want, th, hits[th], len(target.phrasings))
		}
		totalCases += len(target.phrasings)
	}
	for _, th := range gateThresholds {
		t.Logf("== overall referential-gate threshold=%.1f: %d/%d correct, gate fired on %d/%d cases ==",
			th, totalCorrect[th], totalCases, gateFired[th], totalCases)
	}
}
