//go:build integration

// Validates the one open assumption flagged in jev/doc.go: Classifier.Classify returns the
// top-two probability margin as ChoiceAnswer.Confidence, not Jev's own "confidence" field,
// matching intents/laya's behavior even though Jev is advertised (TypeSafe's own benchmark) as
// better-calibrated than Laya's stock checkpoint. That default was never measured against real
// Jev traffic when it was chosen -- this file is
// tests/integration/laya/confidence_calibration_test.go's counterpart for Jev, reusing the same
// 30-phrasing accept_proposal/discard_proposal/propose_spec gold set (proposalGridTargets in
// tests/integration/laya/proposal_grid_test.go) for a like-for-like comparison against the
// numbers already recorded for Laya in the laya skill.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

type calibrationCase struct{ input, want string }

// calibrationCases mirrors tests/integration/laya/confidence_calibration_test.go's
// calibrationCases exactly (same 30 inputs, same wants) -- retyped here rather than shared
// because tests/integration/laya and tests/integration/jev are separate packages (both
// package X_test, no importable production code between them) and duplicating 30 short strings
// is cheaper than introducing a shared test-helper package for it.
func calibrationCases() []calibrationCase {
	return []calibrationCase{
		{"yes that looks right, commit it", "accept_proposal"},
		{"I approve this proposal", "accept_proposal"},
		{"looks good, ship it", "accept_proposal"},
		{"accept the proposal as-is", "accept_proposal"},
		{"do it", "accept_proposal"},
		{"the proposal looks solid, apply it", "accept_proposal"},
		{"confirmed, lock it in", "accept_proposal"},
		{"go ahead and finalize the proposal", "accept_proposal"},
		{"sign off on this one", "accept_proposal"},
		{"that proposal works for me, commit it", "accept_proposal"},

		{"no, scratch that, go back to what it was", "discard_proposal"},
		{"reject the proposal", "discard_proposal"},
		{"never mind, forget it", "discard_proposal"},
		{"throw away the proposal", "discard_proposal"},
		{"undo that", "discard_proposal"},
		{"I don't want that proposal, discard it", "discard_proposal"},
		{"not what I meant, cancel it", "discard_proposal"},
		{"toss the proposal", "discard_proposal"},
		{"revert, that's wrong", "discard_proposal"},
		{"drop the proposal, it's not right", "discard_proposal"},

		{"add a staking transaction type with a minimum lockup of 30 days", "propose_spec"},
		{"make it use segwit-style signatures instead", "propose_spec"},
		{"can we add support for multi-sig transactions", "propose_spec"},
		{"propose bumping the block reward to 50 tokens", "propose_spec"},
		{"I want to change the fee schedule so gas is cheaper", "propose_spec"},
		{"let's add a new rule primitive for token burns", "propose_spec"},
		{"switch the consensus mechanism to proof of stake", "propose_spec"},
		{"propose adding a cooldown period between withdrawals", "propose_spec"},
		{"what if we capped the max validator count at 100", "propose_spec"},
		{"draft a change that requires two signatures for governance votes", "propose_spec"},
	}
}

func specActionCriteria() jev.OrderedCriteria {
	return jev.OrderedCriteria{
		{Key: "accept_proposal", Description: "accept/commit the most recently proposed spec"},
		{Key: "discard_proposal", Description: "throw away the currently staged proposal"},
		{Key: "propose_spec", Description: "propose a new spec change, described in natural language, to be drafted by the assistant"},
	}
}

func margin(p map[string]float64) float64 {
	var top, second float64
	for _, v := range p {
		switch {
		case v > top:
			second = top
			top = v
		case v > second:
			second = v
		}
	}
	return top - second
}

// TestSpecActionConfidence_MarginVsServerConfidenceSeparation measures, against live Jev traffic,
// whether the server's own "confidence" field or the top-two margin better separates correct
// answers from incorrect ones on this 30-case gold set -- the same question
// tests/integration/laya/confidence_calibration_test.go answers for Laya (there: margin 0.269
// separation vs confidence's 0.128, per the laya skill). No hard assertion: like that file, the
// logged numbers are the result being measured, not a pass/fail on any one case.
func TestSpecActionConfidence_MarginVsServerConfidenceSeparation(t *testing.T) {
	c := requireJev(t)

	type sample struct {
		correct          bool
		serverConfidence float64
		margin           float64
	}
	var samples []sample
	for _, tc := range calibrationCases() {
		resp, err := c.SystemOne(context.Background(), jev.Request{
			State: tc.input,
			Questions: map[string]jev.Question{
				"spec_action": {
					Type:         "choice",
					Instructions: "What does the user want to do with this plugin's spec?",
					Criteria:     specActionCriteria(),
				},
			},
			Model: jevModel(),
		})
		if err != nil {
			t.Fatalf("SystemOne(%q): %v", tc.input, err)
		}
		a, err := resp.Choice("spec_action")
		if err != nil {
			t.Fatalf("Choice(%q): %v", tc.input, err)
		}
		correct := a.Choice == tc.want
		m := margin(a.Probabilities)
		samples = append(samples, sample{correct: correct, serverConfidence: a.Confidence, margin: m})
		t.Logf("[correct=%-5v want=%-16s got=%-16s server_confidence=%.3f margin=%.3f] %q",
			correct, tc.want, a.Choice, a.Confidence, m, tc.input)
	}

	report := func(name string, get func(sample) float64) {
		var correctSum, incorrectSum float64
		var correctN, incorrectN int
		for _, s := range samples {
			if s.correct {
				correctSum += get(s)
				correctN++
			} else {
				incorrectSum += get(s)
				incorrectN++
			}
		}
		var correctMean, incorrectMean float64
		if correctN > 0 {
			correctMean = correctSum / float64(correctN)
		}
		if incorrectN > 0 {
			incorrectMean = incorrectSum / float64(incorrectN)
		}
		t.Logf("== %s: correct n=%d mean=%.3f | incorrect n=%d mean=%.3f | separation=%.3f ==",
			name, correctN, correctMean, incorrectN, incorrectMean, correctMean-incorrectMean)
	}
	report("server confidence", func(s sample) float64 { return s.serverConfidence })
	report("top-two margin", func(s sample) float64 { return s.margin })

	thresholds := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6}
	for _, th := range thresholds {
		var wrongCaught, wrongTotal, rightMisflagged, rightTotal int
		for _, s := range samples {
			if s.correct {
				rightTotal++
				if s.margin < th {
					rightMisflagged++
				}
			} else {
				wrongTotal++
				if s.margin < th {
					wrongCaught++
				}
			}
		}
		t.Logf("== margin threshold=%.1f: catches %d/%d wrong answers, unnecessarily escalates %d/%d right answers ==",
			th, wrongCaught, wrongTotal, rightMisflagged, rightTotal)
	}
}
