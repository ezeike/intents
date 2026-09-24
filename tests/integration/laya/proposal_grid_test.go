//go:build integration

// Exploratory measurement grid for the two proposal-related spec_action
// targets (accept_proposal, discard_proposal): does the model's answer track
// the *state* (is a proposal actually staged) or does it just key off the
// literal word "proposal" appearing in the input text? Each phrasing list
// below deliberately mixes cases that use the word "proposal" and cases that
// express the identical intent without it, so that signal falls out of the
// grid rather than needing its own loop.
//
// This intentionally deviates from the project's usual "no table-driven
// tests" rule (packages/CLAUDE.md) -- that rule protects named,
// individually-asserted security/correctness invariants from being flattened
// into indistinguishable rows. This file isn't that: it's a data-gathering
// instrument with no known-correct answer per cell, structured as a grid on
// purpose so the summary itself is the result. See spec_action_test.go for
// the individually-asserted invariant tests.
package laya_test

import (
	"strings"
	"testing"
)

// noStagedProposalContext is stagedProposalContext's opposite: explicit
// natural-language grounding that nothing is currently staged.
const noStagedProposalContext = "There is no proposed change currently staged for this plugin."

// proposalGridCase is one cell's expected classification, for scoring.
type proposalGridTarget struct {
	want      string   // the spec_action option this phrasing is meant to trigger
	phrasings []string // 10 different ways to express `want`, mixing literal "proposal" wording and not
}

func proposalGridTargets() []proposalGridTarget {
	return []proposalGridTarget{
		{
			want: "accept_proposal",
			phrasings: []string{
				"yes that looks right, commit it",
				"I approve this proposal",
				"looks good, ship it",
				"accept the proposal as-is",
				"do it",
				"the proposal looks solid, apply it",
				"confirmed, lock it in",
				"go ahead and finalize the proposal",
				"sign off on this one",
				"that proposal works for me, commit it",
			},
		},
		{
			want: "discard_proposal",
			phrasings: []string{
				"no, scratch that, go back to what it was",
				"reject the proposal",
				"never mind, forget it",
				"throw away the proposal",
				"undo that",
				"I don't want that proposal, discard it",
				"not what I meant, cancel it",
				"toss the proposal",
				"revert, that's wrong",
				"drop the proposal, it's not right",
			},
		},
	}
}

// TestProposalGrid_StateVsLiteralWording runs the full spec_action(2) x
// state(2) x phrasing(10) = 40-cell grid and logs a per-combination accuracy
// summary. No per-cell hard assertions: zero-shot correctness on 40
// individual cells isn't a known ground truth we're defending, it's the
// thing being measured. Read the t.Log output (or the updated laya skill,
// once this run's findings are folded in) for the actual results.
func TestProposalGrid_StateVsLiteralWording(t *testing.T) {
	base := requireLaya(t)

	type stateCase struct {
		name    string
		context string
	}
	states := []stateCase{
		{name: "proposal_exists", context: stagedProposalContext},
		{name: "no_proposal_exists", context: noStagedProposalContext},
	}

	type cellResult struct {
		target, state, phrasing, got string
		containsWord                 bool
		correct                      bool
	}
	var results []cellResult

	for _, target := range proposalGridTargets() {
		for _, st := range states {
			hits := 0
			for _, phrasing := range target.phrasings {
				a := askSpecActionWithContext(t, base, phrasing, st.context)
				correct := a.Choice == target.want
				if correct {
					hits++
				}
				results = append(results, cellResult{
					target:       target.want,
					state:        st.name,
					phrasing:     phrasing,
					got:          a.Choice,
					containsWord: containsProposalWord(phrasing),
					correct:      correct,
				})
				t.Logf("[target=%-16s state=%-18s word=%-5v] %-45q -> %-16s (want %s) distribution=%+v",
					target.want, st.name, containsProposalWord(phrasing), phrasing, a.Choice, target.want, a.Probabilities)
			}
			t.Logf("== summary target=%s state=%s: %d/%d correct ==", target.want, st.name, hits, len(target.phrasings))
		}
	}

	// Aggregate cut: does accuracy differ depending on whether the input
	// literally contains the word "proposal", independent of what the state
	// said? This is the signal the whole grid exists to expose.
	var withWord, withoutWord, withWordCorrect, withoutWordCorrect int
	for _, r := range results {
		if r.containsWord {
			withWord++
			if r.correct {
				withWordCorrect++
			}
		} else {
			withoutWord++
			if r.correct {
				withoutWordCorrect++
			}
		}
	}
	t.Logf("== literal-word cut: contains 'proposal' %d/%d correct, does not contain %d/%d correct ==",
		withWordCorrect, withWord, withoutWordCorrect, withoutWord)

	// Aggregate cut: does accuracy differ depending on what state said,
	// independent of phrasing?
	var stateExists, stateNone, stateExistsCorrect, stateNoneCorrect int
	for _, r := range results {
		if r.state == "proposal_exists" {
			stateExists++
			if r.correct {
				stateExistsCorrect++
			}
		} else {
			stateNone++
			if r.correct {
				stateNoneCorrect++
			}
		}
	}
	t.Logf("== state cut: proposal_exists %d/%d correct, no_proposal_exists %d/%d correct ==",
		stateExistsCorrect, stateExists, stateNoneCorrect, stateNone)
}

func containsProposalWord(s string) bool {
	return strings.Contains(strings.ToLower(s), "proposal")
}
