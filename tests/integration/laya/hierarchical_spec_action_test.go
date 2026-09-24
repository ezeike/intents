//go:build integration

// Hierarchical decomposition of spec_action (query-shape option 1 from the
// design conversation): instead of packing accept_proposal/discard_proposal
// into the same flat 8-way choice as propose_change/validate/etc. (where
// propose_change acted as a gravity well for terse commands -- see the laya
// skill), triage first into a merged "respond_to_proposal" bucket, then
// delegate the accept-vs-reject decision to the plain binary classifier that
// scored 95% domain-neutral in general_accept_reject_test.go.
//
// Deliberately uses NO state/instructions grounding anywhere in this file --
// every grounding placement tried so far (bool-flag state, prose state,
// prose instructions) either failed to help or made things worse. This
// tests whether decomposing the *option set* succeeds where wording changes
// didn't.
package laya_test

import "testing"

// triageCriteria mirrors specActionCriteria but merges accept_proposal and
// discard_proposal into one bucket -- the binary accept/reject classifier
// resolves which one, once triage has confirmed the message is responding
// to something already proposed.
func triageCriteria() map[string]string {
	c := specActionCriteria()
	delete(c, "accept_proposal")
	delete(c, "discard_proposal")
	c["respond_to_proposal"] = "responding yes or no to a specific change that was already proposed to the user"
	return c
}

func askTriage(t *testing.T, base, input string) choiceAnswer {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     triageCriteria(),
			},
		},
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "spec_action")
}

// resolveSpecActionHierarchical is the two-stage resolver this file
// validates. viaBinary reports whether the accept/reject sub-decision ran
// (i.e. triage landed on respond_to_proposal).
func resolveSpecActionHierarchical(t *testing.T, base, input string) (choice string, viaBinary bool) {
	t.Helper()
	triage := askTriage(t, base, input)
	t.Logf("triage(%q) = %s (distribution: %+v)", input, triage.Choice, triage.Probabilities)
	if triage.Choice != "respond_to_proposal" {
		return triage.Choice, false
	}
	ar := askAcceptReject(t, base, input)
	t.Logf("accept_reject(%q) = %s (accept=%.3f reject=%.3f)", input, ar.Choice, ar.Probabilities["accept"], ar.Probabilities["reject"])
	if ar.Choice == "accept" {
		return "accept_proposal", true
	}
	return "discard_proposal", true
}

// TestHierarchicalSpecAction_ProposalGrid reruns the exact 20 accept/discard
// phrasings from proposal_grid_test.go's proposalGridTargets() through the
// hierarchical resolver, with no context at all, and logs accuracy for
// direct comparison against the flat 8-way baseline recorded in the laya
// skill (45%-60% depending on state-grounding variant).
func TestHierarchicalSpecAction_ProposalGrid(t *testing.T) {
	base := requireLaya(t)

	totalCorrect, totalCases := 0, 0
	triageMisses := 0
	for _, target := range proposalGridTargets() {
		hits := 0
		for _, phrasing := range target.phrasings {
			choice, viaBinary := resolveSpecActionHierarchical(t, base, phrasing)
			correct := choice == target.want
			if correct {
				hits++
			}
			if !viaBinary {
				triageMisses++
			}
			t.Logf("[want=%-16s] %-45q -> %-16s (via_binary=%v, correct=%v)",
				target.want, phrasing, choice, viaBinary, correct)
		}
		t.Logf("== summary target=%s: %d/%d correct ==", target.want, hits, len(target.phrasings))
		totalCorrect += hits
		totalCases += len(target.phrasings)
	}
	t.Logf("== overall hierarchical: %d/%d correct, %d/%d cases triage sent elsewhere (not respond_to_proposal) ==",
		totalCorrect, totalCases, triageMisses, totalCases)
}

// TestHierarchicalSpecAction_NonProposalCasesUnaffected checks the six
// spec_action targets that never touch accept/discard still resolve
// correctly through triage's unchanged branches -- the merge into
// respond_to_proposal must not regress everything else in the option set.
func TestHierarchicalSpecAction_NonProposalCasesUnaffected(t *testing.T) {
	base := requireLaya(t)

	cases := []struct {
		input string
		want  string
	}{
		{"add a staking transaction type with a minimum lockup of 30 days", "propose_change"},
		{"just set the chain_id field to canopy-mainnet-1 directly, don't ask the assistant", "direct_edit"},
		{"is this spec actually valid right now?", "validate"},
		{"what does the spec look like right now", "fetch_current"},
		{"what transaction primitives am I allowed to use here", "list_primitives"},
		{"why would someone even want a bonding curve for a token launch", "general_chat"},
	}
	hits := 0
	for _, c := range cases {
		choice, viaBinary := resolveSpecActionHierarchical(t, base, c.input)
		correct := choice == c.want
		if correct {
			hits++
		}
		t.Logf("[want=%-16s] %-70q -> %-16s (via_binary=%v, correct=%v)", c.want, c.input, choice, viaBinary, correct)
	}
	t.Logf("== summary non-proposal cases: %d/%d correct ==", hits, len(cases))
}
