//go:build integration

// Fifth attempt at getting proposal-staging context to actually influence
// spec_action, after four failures logged in the laya skill (bool-flag
// state, prose state, prose instructions, hierarchical decomposition -- all
// either did nothing or made things worse). This one is mechanistically
// different from all four: instead of trying to get the MODEL to learn
// whether a proposal is staged (something the caller already knows for
// certain -- it's app state, not a judgment call), ask a narrow
// "is this message referring back to something already shown to the user"
// noul, batched in the SAME request as spec_action (one round trip, one
// state, no new state/instructions fields), then combine with the caller's
// own certain knowledge in code:
//
//	if hasStagedProposal && is_referential > 0.5: use the accept/reject
//	  binary (95% accurate standalone) to decide accept_proposal vs
//	  discard_proposal, overriding spec_action's raw pick
//	else: trust spec_action's raw pick
//
// This differs from the is_negative gate this file replaced in
// spec_action_test.go in two ways: (1) it targets referentiality, not
// sentiment -- the old gate only ever caught the discard side, this one
// catches both accept and discard terse commands; (2) it's batched into ONE
// request via multi-question, not a separate round-trip call, so it shares
// Laya's exact same tokenized state without adding any state/instructions
// field that has previously proven contamination-prone.
package laya_test

import "testing"

const referentialInstructions = "Does this message refer back to something already proposed or " +
	"shown to the user -- using words like \"it\", \"that\", or a short confirmation/rejection -- " +
	"rather than describing something new from scratch?"

// referentialGateThreshold is the is_referential cutoff spec_action_test.go
// and this file's own tests use in production-style calls (as opposed to
// the threshold-comparison sweep below). 0.4, not the initially-tried 0.5:
// retested at both and 0.4 scored strictly better (17/20 vs 16/20 on the
// proposal grid) with identical precision (100% correct when fired) and no
// new false positives -- see the laya skill for the full comparison.
const referentialGateThreshold = 0.4

type batchedAnswers struct {
	specAction   choiceAnswer
	referential  noulAnswer
	acceptReject choiceAnswer
}

// askBatched sends spec_action, is_referential, and decision (accept/reject)
// as three questions in ONE /v1/systemone request -- one shared state, one
// round trip.
func askBatched(t *testing.T, base, input string) batchedAnswers {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
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
		Model: "typed-decisions",
	})
	return batchedAnswers{
		specAction:   decodeChoice(t, resp, "spec_action"),
		referential:  decodeNoul(t, resp, "is_referential"),
		acceptReject: decodeChoice(t, resp, "decision"),
	}
}

// resolveGate is the pure decision this file validates, split out from the
// network call (askBatched) so the same forward-pass result can be scored
// against multiple thresholds without duplicating live requests.
// hasStagedProposal is caller-known app state, never asked of the model.
func resolveGate(a batchedAnswers, hasStagedProposal bool, threshold float64) (choice string, viaGate bool) {
	if hasStagedProposal && a.referential.Noul > threshold {
		if a.acceptReject.Choice == "accept" {
			return "accept_proposal", true
		}
		return "discard_proposal", true
	}
	return a.specAction.Choice, false
}

// gateThresholds are compared side by side against the exact same live
// forward-pass results: 0.5 is the confirmed baseline (80% grid accuracy,
// 7/7 precise when fired); 0.4 tests whether lowering the cutoff recovers
// the under-threshold false negatives ("do it" 0.22, "undo that" 0.28,
// "looks good, ship it" 0.37, "revert, that's wrong" 0.44) without costing
// precision.
var gateThresholds = []float64{0.5, 0.4}

// TestReferentialGate_ProposalGrid reruns the same 20 accept/discard
// phrasings from proposal_grid_test.go, all with hasStagedProposal=true
// (matching that grid's proposal_exists condition), through the batched
// referential-gate resolver at both thresholds. Compare against the laya
// skill's recorded numbers: flat baseline 45-60%, hierarchical 20%, gate@0.5
// 80%.
func TestReferentialGate_ProposalGrid(t *testing.T) {
	base := requireLaya(t)

	totalCorrect := map[float64]int{}
	gateFired := map[float64]int{}
	totalCases := 0
	for _, target := range proposalGridTargets() {
		hits := map[float64]int{}
		for _, phrasing := range target.phrasings {
			a := askBatched(t, base, phrasing)
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

// TestReferentialGate_NonProposalCasesUnaffected (propose_change, direct_edit,
// validate, fetch_current, list_primitives, general_chat cases) was removed
// once spec_action's scope narrowed to a pure accept_proposal/discard_proposal
// 2-way set (spec_action_test.go) -- there's no other category left for
// spec_action to answer, so "does the gate misfire on a non-proposal target"
// no longer has a case to test against. See out_of_scope_detection_test.go
// for the current version of "does this input belong here at all," which
// tests genuinely out-of-scope/unrelated input instead of other spec_action
// categories that no longer exist.
