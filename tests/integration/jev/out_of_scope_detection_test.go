//go:build integration

// Jev counterpart to tests/integration/laya/out_of_scope_detection_test.go. spec_action's
// production scope (accept_proposal, discard_proposal, propose_spec) scores well on the proposal
// grid when the input genuinely belongs to one of those three. But a real deployment still needs
// to know when an input DOESN'T belong there at all, so it can escalate to the LLM instead of
// forcing a confident-looking wrong answer between options that don't apply. This measures whether
// confidence or margin on the classifier itself -- no new question, no extra call -- separates
// genuinely in-scope inputs from out-of-scope ones, against live Jev traffic.
package jev_test

import "testing"

// outOfScopeCases is genuinely unrelated input -- nothing about accepting, rejecting, or
// proposing a spec change.
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

// TestOutOfScopeDetection_ConfidenceAndMargin runs the full proposal grid (accept/discard/
// propose_spec phrasings) and the out-of-scope cases above through the SAME classifier
// (specActionCriteria2), then compares confidence and margin between the two groups -- same
// threshold-sweep method as escalation_alternatives_test.go, but for scope detection instead of
// correctness detection.
func TestOutOfScopeDetection_ConfidenceAndMargin(t *testing.T) {
	c := requireJev(t)

	var samples []scopeSample
	for _, target := range proposalGridTargets() {
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, c, phrasing, specActionCriteria2())
			m := margin(a.Probabilities)
			samples = append(samples, scopeSample{inScope: true, confidence: a.Confidence, margin: m})
			t.Logf("[in_scope=true  want=%-16s] %-45q -> %-16s confidence=%.3f margin=%.3f",
				target.want, phrasing, a.Choice, a.Confidence, m)
		}
	}
	for _, input := range outOfScopeCases() {
		a := askSpecActionWithCriteria(t, c, input, specActionCriteria2())
		m := margin(a.Probabilities)
		samples = append(samples, scopeSample{inScope: false, confidence: a.Confidence, margin: m})
		t.Logf("[in_scope=false] %-70q -> %-16s confidence=%.3f margin=%.3f", input, a.Choice, a.Confidence, m)
	}

	scoreConfidence := make([]scoredSample, len(samples))
	scoreMargin := make([]scoredSample, len(samples))
	for i, s := range samples {
		// Reuse scoredSample/thresholdSweep from escalation_alternatives_test.go: "correct" there
		// maps to "inScope" here -- same separation question, different label.
		scoreConfidence[i] = scoredSample{correct: s.inScope, signal: s.confidence}
		scoreMargin[i] = scoredSample{correct: s.inScope, signal: s.margin}
	}
	// belowMeansEscalate=true: a LOW confidence/margin is what should trigger escalation to the
	// LLM (the classifier isn't sure any of the 3 options fit).
	thresholdSweep(t, scoreConfidence, []float64{0.3, 0.4, 0.5, 0.6, 0.7}, true, "scope_confidence")
	thresholdSweep(t, scoreMargin, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, true, "scope_margin")
}

// TestOutOfScopeDetection_ExplicitOtherOption tests the architectural fix for margin-thresholding's
// gap: an explicit fourth "other" option (specActionCriteriaWithOther), rather than reading
// uncertainty off a forced 3-way. In-scope inputs are correct if the model still picks
// accept_proposal/discard_proposal/propose_spec correctly; out-of-scope inputs are correct only if
// the model explicitly picks "other".
func TestOutOfScopeDetection_ExplicitOtherOption(t *testing.T) {
	c := requireJev(t)

	inScopeHits, inScopeTotal := 0, 0
	for _, target := range proposalGridTargets() {
		for _, phrasing := range target.phrasings {
			a := askSpecActionWithCriteria(t, c, phrasing, specActionCriteriaWithOther())
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
		a := askSpecActionWithCriteria(t, c, input, specActionCriteriaWithOther())
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
