//go:build integration

// General, application-agnostic claims about Laya's behavior -- none of this
// uses block-builder vocabulary, unlike spec_action_test.go. Each test here
// empirically checks one specific claim (from Laya's own docs, or from
// findings earlier in the design conversation) using a neutral customer-
// service-style domain, the same domain Laya's own benchmarks use. Results
// feed back into ~/.claude/skills/laya/SKILL.md -- this file exists to
// verify claims before they're asserted as fact there, not to test
// intents-package-specific behavior.
package laya_test

import (
	"math"
	"testing"
)

// ---- Claim: an irrelevant bool-flag key added to `state` can shift answers
// on unrelated questions, even when the flag is false (see design
// conversation: adding "has_staged_proposal" moved general_chat's answer on
// an off-topic input that never referenced a proposal). Tested here on a
// generic domain to see whether this generalizes or was case-specific. ----

func TestGeneralClaim_UnrelatedBoolFlag_ComparedAgainstBareState(t *testing.T) {
	base := requireLaya(t)

	const input = "I was charged twice for my subscription this month, please refund the duplicate."
	criteria := map[string]string{
		"billing":  "a billing or payment issue",
		"account":  "an account access or login issue",
		"feature":  "a feature request or product question",
		"shipping": "a shipping or delivery issue",
	}

	bare := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"category": {Type: "choice", Instructions: "What category does this message belong to?", Criteria: criteria},
		},
		Model: "typed-decisions",
	})
	bareAnswer := decodeChoice(t, bare, "category")

	withFlag := mustOK(t, base, systemOneRequest{
		State: map[string]any{"input": input, "is_vip_customer": false},
		Questions: map[string]questionDef{
			"category": {Type: "choice", Instructions: "What category does this message belong to?", Criteria: criteria},
		},
		Model: "typed-decisions",
	})
	flagAnswer := decodeChoice(t, withFlag, "category")

	t.Logf("bare choice=%s %+v", bareAnswer.Choice, bareAnswer.Probabilities)
	t.Logf("with unrelated bool flag choice=%s %+v", flagAnswer.Choice, flagAnswer.Probabilities)

	var maxDelta float64
	for k, v := range bareAnswer.Probabilities {
		if d := math.Abs(v - flagAnswer.Probabilities[k]); d > maxDelta {
			maxDelta = d
		}
	}
	t.Logf("max per-option probability delta from adding an unrelated bool flag: %v", maxDelta)
	// Diagnostic, not asserted: this is exactly the open question from the
	// design conversation (one prior case showed contamination, one theory
	// says prose-shaped context avoids it). No hard assertion here on
	// purpose -- the log line above is the finding.
}

// ---- Claim (README.md:791-794): choice keys are rendered verbatim, and the
// model can follow labels like "true"/"false" instead of their descriptions
// -- i.e. swapping which description is attached to which label can flip the
// answer even when the actual input semantics didn't change. ----

func askCancelIntent(t *testing.T, base, input string, trueMeans, falseMeans string) choiceAnswer {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"q": {
				Type:         "choice",
				Instructions: "What does the user want?",
				Criteria:     map[string]string{"true": trueMeans, "false": falseMeans},
			},
		},
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "q")
}

func TestGeneralClaim_BooleanWordLabels_TrueDescribesCancellation(t *testing.T) {
	base := requireLaya(t)
	a := askCancelIntent(t, base, "I want to cancel my subscription immediately",
		"the user wants to cancel their subscription", "the user wants to keep their subscription")
	t.Logf("true=cancel/false=keep, input=cancel-intent -> choice=%s %+v", a.Choice, a.Probabilities)
	if a.Choice != "true" {
		t.Errorf("choice = %q, want %q (cancellation intent, and 'true' is labeled as cancellation)", a.Choice, "true")
	}
}

func TestGeneralClaim_BooleanWordLabels_LabelMeaningSwapped_FollowsDescriptionNotLabelWord(t *testing.T) {
	base := requireLaya(t)
	// Same cancellation-intent input, but now "false" is the description that
	// matches cancellation and "true" means keep. If the model tracks
	// descriptions, the correct answer flips to "false". If it instead just
	// follows the literal word "true"/"false" pattern (the documented risk),
	// it will incorrectly still favor "true" regardless of what's staged there.
	a := askCancelIntent(t, base, "I want to cancel my subscription immediately",
		"the user wants to keep their subscription", "the user wants to cancel their subscription")
	t.Logf("true=keep/false=cancel, input=cancel-intent -> choice=%s %+v", a.Choice, a.Probabilities)
	if a.Choice != "false" {
		t.Errorf("choice = %q, want %q -- if this fails, the model followed the literal label "+
			"word instead of the swapped description (the exact risk README.md:791-794 warns about)",
			a.Choice, "false")
	}
}

// ---- Claim: the model actually reads `state` rather than collapsing to a
// constant/prior regardless of input (per data/typed-decisions/README.md's
// "Prior" baseline concept: a model that ignores state just answers each
// question's label base rate every time). Hard-asserted: this is a basic
// sanity floor, not a close call. ----

func TestGeneralClaim_StateIsActuallyRead_UrgentVsNoRushDiffer(t *testing.T) {
	base := requireLaya(t)

	ask := func(input string) noulAnswer {
		resp := mustOK(t, base, systemOneRequest{
			State: map[string]string{"input": input},
			Questions: map[string]questionDef{
				"urgent": {Type: "noul", Instructions: "Does this message express urgency, needing action right now?"},
			},
			Model: "typed-decisions",
		})
		return decodeNoul(t, resp, "urgent")
	}

	noRush := ask("no rush at all, whenever you get a chance is fine, no hurry")
	urgent := ask("URGENT!! this needs to happen RIGHT NOW, emergency, act immediately")

	t.Logf("no_rush noul=%v, urgent noul=%v, gap=%v", noRush.Noul, urgent.Noul, urgent.Noul-noRush.Noul)

	if urgent.Noul <= noRush.Noul {
		t.Errorf("urgent (%v) did not score higher than no_rush (%v) -- model may not be reading state",
			urgent.Noul, noRush.Noul)
	}
	const minGap = 0.3
	if gap := urgent.Noul - noRush.Noul; gap < minGap {
		t.Errorf("gap = %v, want >= %v between an unambiguous urgent and unambiguous non-urgent input", gap, minGap)
	}
}

// ---- Claim (README.md:795-799): high-cardinality choice questions degrade
// under default head_max_len -- a 77-option question allocates only ~3-4
// tokens/label and accuracy falls off sharply (0.425 vs 0.870 on Banking77).
// head_max_len itself isn't settable over the wire protocol (Python-only,
// agent.cfg), so this only tests whether the documented degradation is
// visible at default server settings, not whether raising the budget fixes
// it -- that half of the claim isn't reachable through this HTTP surface. ----

func buildTopicCriteria(n int, correctKey, correctDesc string) map[string]string {
	c := map[string]string{correctKey: correctDesc}
	for i := 0; i < n; i++ {
		c[topicFillerKey(i)] = topicFillerDesc(i)
	}
	return c
}

func topicFillerKey(i int) string  { return "topic_" + itoa(i) }
func topicFillerDesc(i int) string { return "a message about unrelated topic number " + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestGeneralClaim_HighCardinalityChoice_LowCardinalityBaseline(t *testing.T) {
	base := requireLaya(t)

	const input = "I was charged twice for my subscription this month, please refund the duplicate."
	criteria := buildTopicCriteria(4, "billing", "a billing or payment issue")

	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"category": {Type: "choice", Instructions: "What category does this message belong to?", Criteria: criteria},
		},
		Model: "typed-decisions",
	})
	a := decodeChoice(t, resp, "category")
	t.Logf("5 options: choice=%s confidence=%v probabilities=%+v", a.Choice, a.Confidence, a.Probabilities)
	if a.Choice != "billing" {
		t.Errorf("choice = %q, want %q at low cardinality (5 options) -- this should be the easy control case", a.Choice, "billing")
	}
}

func TestGeneralClaim_HighCardinalityChoice_SixtyOptionsDegradesConfidence(t *testing.T) {
	base := requireLaya(t)

	const input = "I was charged twice for my subscription this month, please refund the duplicate."
	criteria := buildTopicCriteria(60, "billing", "a billing or payment issue")

	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"category": {Type: "choice", Instructions: "What category does this message belong to?", Criteria: criteria},
		},
		Model: "typed-decisions",
	})
	a := decodeChoice(t, resp, "category")
	t.Logf("61 options: choice=%s confidence=%v top probability=%v", a.Choice, a.Confidence, a.Probabilities[a.Choice])
	// Diagnostic, not hard-asserted on correctness: the documented claim is
	// about degraded accuracy/confidence under a token-starved option budget,
	// not that it necessarily gets the answer wrong outright on every case.
	// The log line is the finding -- compare its confidence against the
	// low-cardinality baseline test above.
}
