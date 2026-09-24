//go:build integration

// spec_action_test.go's actual production scope (accept_proposal,
// discard_proposal, propose_spec) scored well on the proposal grid when the
// input genuinely belongs to one of those three. But a real deployment still
// needs to know when an input DOESN'T belong there at all -- direct_edit,
// validate, fetch_current, list_primitives, and general_chat remain cut from
// the test suite (a real system presumably routes them through other nodes
// entirely, out of scope for these tests), so the remaining out-of-scope
// question is: does the classifier know when an input isn't about
// accepting/rejecting/proposing anything at all, so it can escalate to the
// LLM instead of forcing a confident-looking wrong answer between options
// that don't apply.
//
// This measures whether confidence or margin on the classifier itself -- no
// new question, no extra call -- separates genuinely in-scope inputs from
// out-of-scope ones, the same way confidence_calibration_test.go and
// escalation_alternatives_test.go measured correct-vs-incorrect separation.
package laya_test

import "testing"

// outOfScopeCases is now just genuinely unrelated input -- no longer
// spanning other spec_action categories, since those were cut from the
// suite entirely rather than just excluded from this one node's option set.
func outOfScopeCases() []string {
	return []string{
		"what's the weather like today",
		"tell me a joke",
		"how do I reset my password",
	}
}

type scopeSample struct {
	inScope    bool
	confidence float64
	margin     float64
}

// TestOutOfScopeDetection_ConfidenceAndMargin runs the full proposal grid
// (accept/discard/propose_spec phrasings) and the out-of-scope cases above
// through the SAME classifier (reduced_spec_action_test.go's
// specActionCriteria2), then compares confidence and margin between the
// two groups -- same
// threshold-sweep method as escalation_alternatives_test.go, but for scope
// detection instead of correctness detection.
func TestOutOfScopeDetection_ConfidenceAndMargin(t *testing.T) {
	base := requireLaya(t)

	var samples []scopeSample
	for _, target := range proposalGridTargets() {
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, base, phrasing, specActionCriteria2())
			margin := marginOf(a.Probabilities)
			samples = append(samples, scopeSample{inScope: true, confidence: a.Confidence, margin: margin})
			t.Logf("[in_scope=true  want=%-16s] %-45q -> %-16s confidence=%.3f margin=%.3f",
				target.want, phrasing, a.Choice, a.Confidence, margin)
		}
	}
	for _, input := range outOfScopeCases() {
		a := askSpecActionWithCriteria(t, base, input, specActionCriteria2())
		margin := marginOf(a.Probabilities)
		samples = append(samples, scopeSample{inScope: false, confidence: a.Confidence, margin: margin})
		t.Logf("[in_scope=false] %-70q -> %-16s confidence=%.3f margin=%.3f", input, a.Choice, a.Confidence, margin)
	}

	scoreConfidence := make([]scoredSample, len(samples))
	scoreMargin := make([]scoredSample, len(samples))
	for i, s := range samples {
		// Reuse scoredSample/thresholdSweep from escalation_alternatives_test.go:
		// "correct" there maps to "inScope" here -- same separation question,
		// different label.
		scoreConfidence[i] = scoredSample{correct: s.inScope, signal: s.confidence}
		scoreMargin[i] = scoredSample{correct: s.inScope, signal: s.margin}
	}
	// belowMeansEscalate=true: a LOW confidence/margin is what should trigger
	// escalation to the LLM (the classifier isn't sure any of the 3 options fit).
	thresholdSweep(t, scoreConfidence, []float64{0.3, 0.4, 0.5, 0.6, 0.7}, true, "scope_confidence")
	thresholdSweep(t, scoreMargin, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, true, "scope_margin")
}

// TestOutOfScopeDetection_ExplicitOtherOption tests the architectural fix
// for margin-thresholding's gap: an explicit third "other" option
// (specActionCriteriaWithOther), rather than reading uncertainty off a
// forced binary. In-scope inputs are correct if the model still picks
// accept_proposal/discard_proposal correctly; out-of-scope inputs are
// correct only if the model explicitly picks "other".
func TestOutOfScopeDetection_ExplicitOtherOption(t *testing.T) {
	base := requireLaya(t)

	inScopeHits, inScopeTotal := 0, 0
	for _, target := range proposalGridTargets() {
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, base, phrasing, specActionCriteriaWithOther())
			correct := a.Choice == target.want
			if correct {
				inScopeHits++
			}
			inScopeTotal++
			t.Logf("[in_scope=true  want=%-16s] %-45q -> %-16s (correct=%v) distribution=%+v",
				target.want, phrasing, a.Choice, correct, a.Probabilities)
		}
	}
	t.Logf("== in-scope accuracy with explicit 'other' option: %d/%d ==", inScopeHits, inScopeTotal)

	outOfScopeHits, outOfScopeTotal := 0, 0
	for _, input := range outOfScopeCases() {
		a := askSpecActionWithCriteria(t, base, input, specActionCriteriaWithOther())
		correct := a.Choice == "other"
		if correct {
			outOfScopeHits++
		}
		outOfScopeTotal++
		t.Logf("[in_scope=false] %-70q -> %-16s (correctly flagged as other=%v) distribution=%+v",
			input, a.Choice, correct, a.Probabilities)
	}
	t.Logf("== out-of-scope correctly flagged as 'other': %d/%d ==", outOfScopeHits, outOfScopeTotal)
	t.Logf("== overall: %d/%d ==", inScopeHits+outOfScopeHits, inScopeTotal+outOfScopeTotal)
}
