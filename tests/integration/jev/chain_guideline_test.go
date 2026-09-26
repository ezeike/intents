//go:build integration

// Exploratory suite for a new classification target: block-builder's chain-guideline drafting
// prompt (~/src/machina/block-builder/internal/prompts/chain_guideline_system.prompt) needs to
// know, per incoming chat message, (1) which of its four fixed categories the message addresses,
// and (2) which categories the conversation has already settled well enough that the assistant
// shouldn't re-ask about them. Those are two different kinds of judgment (message topic vs.
// conversation state), so per the jev-classifier skill's "never compound conditions into one
// question" rule they're modeled as five separate questions, batched into one request rather than
// folded together or split across round trips:
//
//   - addresses_category: a Choice over the prompt's four named categories plus an "other"
//     catch-all (greetings, confirmations, meta-conversation) -- exactly one bucket per message.
//   - {category}_settled: four independent Nouls, one per category, asking whether the
//     conversation so far already answered that category well enough to stop asking about it.
//
// This intentionally deviates from the project's usual "no table-driven tests" rule
// (packages/CLAUDE.md) for the same reason proposal_grid_test.go does: this is a data-gathering
// instrument with no known-correct answer per cell yet, not a set of named security/correctness
// invariants.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

// guidelineCriteria mirrors chain_guideline_system.prompt's four categories verbatim, plus an
// "other" catch-all per the jev-classifier skill's rule that a Choice option set always needs one
// unless it's provably exhaustive (it isn't: greetings, confirmations, and meta-conversation don't
// belong to any category).
func guidelineCriteria() jev.OrderedCriteria {
	return jev.OrderedCriteria{
		{
			Key: "core_functionality",
			Description: "what the chain/plugin actually does, who the actors are (validators, delegators, " +
				"chain operators, end users), and what each actor needs from it -- includes incentive questions",
		},
		{
			Key:         "success_metrics",
			Description: "a small number of measurable criteria for what counts as this project having worked",
		},
		{
			Key: "scope_boundary",
			Description: "what's explicitly out of scope for a first version, or a hard technical/economic " +
				"constraint (token, fee, staking, performance, compliance) -- includes economic-model questions",
		},
		{
			Key: "general_entities",
			Description: "the core state/domain nouns the system revolves around (accounts, chains, " +
				"transactions, state objects) and how they relate",
		},
		{
			Key: "other",
			Description: "a greeting, meta-conversation, confirmation/rejection of a summary, or anything " +
				"that isn't actually describing chain content",
		},
	}
}

// guidelineState is the state shape sent to Jev: conversation_so_far and latest_message are kept
// as distinct fields (per the jev-classifier skill's state-design rule) rather than concatenated
// into one prose blob, since addresses_category cares only about latest_message while the four
// *_settled Nouls care about conversation_so_far.
type guidelineState struct {
	ConversationSoFar string `json:"conversation_so_far,omitempty"`
	LatestMessage     string `json:"latest_message"`
}

type guidelineAnswers struct {
	category jev.ChoiceAnswer
	settled  map[string]jev.NoulAnswer // keyed by category: core_functionality, success_metrics, scope_boundary, general_entities
}

var guidelineSettledQuestions = map[string]string{
	"core_functionality": "Has the conversation so far already given a clear, sufficient answer for the " +
		"core_functionality category (what the chain does, its actors, what each actor needs)?",
	"success_metrics": "Has the conversation so far already given a clear, sufficient answer for the " +
		"success_metrics category (measurable definition-of-done criteria)?",
	"scope_boundary": "Has the conversation so far already given a clear, sufficient answer for the " +
		"scope_boundary category (what's out of scope, plus hard constraints)?",
	"general_entities": "Has the conversation so far already given a clear, sufficient answer for the " +
		"general_entities category (the core domain nouns and their relationships)?",
}

// askGuideline sends addresses_category and all four *_settled nouls as five questions in ONE
// /v1/systemone request -- one shared state, one round trip, per the skill's fan-out pattern.
func askGuideline(t *testing.T, c *jev.Client, conversationSoFar, latestMessage string) guidelineAnswers {
	t.Helper()

	questions := map[string]jev.Question{
		"addresses_category": {
			Type:         "choice",
			Instructions: "Which guideline category does the user's latest message address?",
			Criteria:     guidelineCriteria(),
		},
	}
	for category, instructions := range guidelineSettledQuestions {
		questions[category+"_settled"] = jev.Question{Type: "noul", Instructions: instructions}
	}

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State:     guidelineState{ConversationSoFar: conversationSoFar, LatestMessage: latestMessage},
		Questions: questions,
		Model:     jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(latest=%q): %v", latestMessage, err)
	}

	category, err := resp.Choice("addresses_category")
	if err != nil {
		t.Fatalf("Choice(addresses_category, %q): %v", latestMessage, err)
	}

	settled := make(map[string]jev.NoulAnswer, len(guidelineSettledQuestions))
	for cat := range guidelineSettledQuestions {
		a, err := resp.Noul(cat + "_settled")
		if err != nil {
			t.Fatalf("Noul(%s_settled, %q): %v", cat, latestMessage, err)
		}
		settled[cat] = a
	}

	return guidelineAnswers{category: category, settled: settled}
}

// guidelineGridTarget is one cell's expected addresses_category classification, for scoring.
type guidelineGridTarget struct {
	want      string
	phrasings []string
}

func guidelineGridTargets() []guidelineGridTarget {
	return []guidelineGridTarget{
		{
			want: "core_functionality",
			phrasings: []string{
				"it's a lending protocol where validators approve loan requests and delegators earn a cut of interest",
				"end users deposit collateral and borrow against it, chain operators just run nodes",
				"the main actors are validators, delegators, and borrowers",
				"this chain lets people swap tokens directly, validators just secure the network",
				"delegators stake to validators and get a share of block rewards",
				"chain operators need low resource requirements to run a node cheaply",
				"end users interact through a mobile wallet to send and receive payments",
				"validators need strong incentives to keep uptime high",
			},
		},
		{
			want: "success_metrics",
			phrasings: []string{
				"we'll call it done when we can process 100 transactions per second",
				"success means at least 50 validators join in the first month",
				"if average finality is under 2 seconds, that's a win",
				"we want zero downtime incidents in the first quarter",
				"done means the testnet survives a week without a hard fork",
				"success is measured by total value locked exceeding a million tokens",
				"we're done when the audit finds no critical vulnerabilities",
				"a good outcome is under 1% failed transactions",
			},
		},
		{
			want: "scope_boundary",
			phrasings: []string{
				"we're not doing cross-chain bridges in v1",
				"no smart contracts for the first version, just native transactions",
				"fees should stay below a cent per transaction",
				"we're capping max validator count at 100 for now",
				"governance voting is out of scope until v2",
				"the staking lockup period must be at least 7 days",
				"we won't support fiat on-ramps at launch",
				"compliance requires kyc for any transaction over 10k",
			},
		},
		{
			want: "general_entities",
			phrasings: []string{
				"the main objects are accounts, transactions, and validator sets",
				"we need a state object to track loan balances per account",
				"chains, blocks, and transactions are the core nouns here",
				"every account has a balance and a list of active stakes",
				"a loan links a borrower account to a collateral state object",
				"transactions reference an account and a chain id",
				"we'll have a validator entity and a delegator entity linked by stake records",
				"state objects represent open positions per account",
			},
		},
		{
			want: "other",
			phrasings: []string{
				"hey, let's get started",
				"sounds good, let's move to the next part",
				"can you summarize what we've covered so far",
				"yes that's right",
				"what do you think we should call this chain",
				"thanks, this is helpful",
			},
		},
	}
}

// TestGuidelineCategoryGrid_AddressesCategory runs the full 5-category x phrasing grid (bare
// latest_message, no conversation_so_far) and logs a per-combination accuracy summary. No per-cell
// hard assertions: zero-shot correctness on a brand-new domain isn't a known ground truth we're
// defending yet, it's the thing being measured.
func TestGuidelineCategoryGrid_AddressesCategory(t *testing.T) {
	c := requireJev(t)

	totalCorrect, totalCases := 0, 0
	for _, target := range guidelineGridTargets() {
		hits := 0
		for _, phrasing := range target.phrasings {
			a := askGuideline(t, c, "", phrasing)
			correct := a.category.Choice == target.want
			if correct {
				hits++
			}
			t.Logf("[want=%-20s] %-90q -> %-20s (correct=%v) distribution=%+v",
				target.want, phrasing, a.category.Choice, correct, a.category.Probabilities)
		}
		t.Logf("== summary target=%s: %d/%d correct ==", target.want, hits, len(target.phrasings))
		totalCorrect += hits
		totalCases += len(target.phrasings)
	}
	t.Logf("== overall addresses_category: %d/%d correct ==", totalCorrect, totalCases)
}

// guidelineSettledContext returns natural-language grounding stating that category was already
// covered, built from one of that category's own grid phrasings -- mirrors
// tests/integration/laya/spec_action_test.go's stagedProposalContext pattern (prose, not a bool
// flag, since a raw boolean field measurably shifted unrelated answers in that suite).
func guidelineSettledContext(category string) string {
	for _, target := range guidelineGridTargets() {
		if target.want == category {
			return "Earlier in the conversation, the user already said: \"" + target.phrasings[0] + "\""
		}
	}
	return ""
}

// TestGuidelineSettled_TracksConversationContext checks, for each category, whether its
// *_settled noul actually moves when conversation_so_far contains a clear answer for that
// category, versus an empty conversation -- both asked about the SAME neutral follow-up message,
// so any difference in the noul is attributable to conversation_so_far, not latest_message. No
// hard assertion (first measurement on a new domain): the logged per-category separation is the
// result, matching this package's other measurement-grid tests.
func TestGuidelineSettled_TracksConversationContext(t *testing.T) {
	c := requireJev(t)

	const neutralFollowUp = "okay, what's next"

	baseline := askGuideline(t, c, "", neutralFollowUp)
	t.Logf("[baseline, no context] settled=%+v", logNouls(baseline.settled))

	for category := range guidelineSettledQuestions {
		ctx := guidelineSettledContext(category)
		withContext := askGuideline(t, c, ctx, neutralFollowUp)
		before := baseline.settled[category].Noul
		after := withContext.settled[category].Noul
		t.Logf("[category=%-20s] context=%q settled_before=%.3f settled_after=%.3f delta=%+.3f",
			category, ctx, before, after, after-before)
	}
}

// guidelineRicherSettledContext concatenates three of a category's grid phrasings instead of one,
// testing the hypothesis TestGuidelineSettled_TracksConversationContext's result raised:
// scope_boundary and general_entities barely moved their settled noul off a single-example
// context (deltas of +0.25 and +0.13, both landing well under a 0.5 gate) because those two
// categories genuinely ask for a SET of things (multiple constraints, multiple nouns) that one
// example under-fills -- unlike success_metrics/core_functionality, where one sentence swung the
// noul past 0.5 (+0.80 and +0.44). This isolates whether that's a context-richness problem
// (fixable by how much is in conversation_so_far) rather than a wording/threshold problem with the
// settled question itself.
func guidelineRicherSettledContext(category string) string {
	for _, target := range guidelineGridTargets() {
		if target.want == category {
			quoted := make([]string, 0, 3)
			for _, p := range target.phrasings[:3] {
				quoted = append(quoted, "\""+p+"\"")
			}
			return "Earlier in the conversation, the user already said: " + quoted[0] + ", " + quoted[1] + ", and " + quoted[2] + "."
		}
	}
	return ""
}

// TestGuidelineSettled_RicherContextForWeakCategories reruns the same before/after comparison as
// TestGuidelineSettled_TracksConversationContext, but only for the two categories that responded
// weakly to a single-example context, this time with three examples concatenated into
// conversation_so_far. No hard assertion (still exploratory): if the delta closes toward
// success_metrics/core_functionality's single-example response, the settled noul is workable and
// the earlier result was a context-richness gap, not a broken question.
func TestGuidelineSettled_RicherContextForWeakCategories(t *testing.T) {
	c := requireJev(t)

	const neutralFollowUp = "okay, what's next"
	weakCategories := []string{"scope_boundary", "general_entities"}

	baseline := askGuideline(t, c, "", neutralFollowUp)
	t.Logf("[baseline, no context] settled=%+v", logNouls(baseline.settled))

	for _, category := range weakCategories {
		singleCtx := guidelineSettledContext(category)
		richCtx := guidelineRicherSettledContext(category)

		single := askGuideline(t, c, singleCtx, neutralFollowUp)
		rich := askGuideline(t, c, richCtx, neutralFollowUp)

		before := baseline.settled[category].Noul
		afterSingle := single.settled[category].Noul
		afterRich := rich.settled[category].Noul
		t.Logf("[category=%-20s] baseline=%.3f single_example=%.3f (delta=%+.3f) rich_context=%.3f (delta=%+.3f)",
			category, before, afterSingle, afterSingle-before, afterRich, afterRich-before)
		t.Logf("  rich context used: %q", richCtx)
	}
}

func logNouls(settled map[string]jev.NoulAnswer) map[string]float64 {
	out := make(map[string]float64, len(settled))
	for k, v := range settled {
		out[k] = v.Noul
	}
	return out
}
