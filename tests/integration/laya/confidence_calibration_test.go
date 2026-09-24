//go:build integration

// Answers a question the intents design spec assumes but this session had
// never actually tested: is spec_action's own `confidence` a trustworthy
// signal for deciding when to escalate to an LLM instead of trusting the
// classifier (ClassifySpec.MinConfidence)? Motivating evidence this file
// exists to check systematically: several WRONG answers logged earlier this
// session were confidently wrong, not hedged --
//   "confirmed, lock it in" -> validate (wrong) at confidence 0.71
//   "yes that looks right, commit it" -> validate (wrong) at confidence 0.54
// A naive "escalate if confidence < 0.5" policy would sail past both. Laya's
// own README benchmarks `confidence` at AUROC 0.77 (vs act_probability's
// 0.30) for separating correct from incorrect -- but that's on Laya's own
// 396-case benchmark domain, not spec_action's 8-option set. This file
// measures it directly, in-domain, rather than assuming the generic number
// transfers.
//
// Deliberately uses raw askSpecAction (flat classifier, no context, no
// referential gate) -- this measures the signal MinConfidence would actually
// gate on for the base classify call itself, not the gate-corrected answer.
package laya_test

import "testing"

type calibrationCase struct {
	input, want string
}

// calibrationCases is the 20 accept/discard phrasings from
// proposal_grid_test.go's proposalGridTargets() -- reusing that gold data
// rather than retyping it, so this file's cases stay in sync with the
// phrasings already validated elsewhere in the package. Originally also
// included six non-proposal spec_action cases (propose_change, direct_edit,
// validate, fetch_current, list_primitives, general_chat); those categories
// were cut from the whole test suite once spec_action's scope narrowed to
// accept_proposal/discard_proposal only (spec_action_test.go).
func calibrationCases() []calibrationCase {
	var cases []calibrationCase
	for _, target := range proposalGridTargets() {
		for _, p := range target.phrasings {
			cases = append(cases, calibrationCase{input: p, want: target.want})
		}
	}
	return cases
}

// TestSpecActionConfidence_CorrectVsIncorrectSeparation measures whether
// spec_action's `confidence` field actually separates correct answers from
// incorrect ones in this domain, and sweeps candidate MinConfidence
// thresholds to see whether any of them would catch wrong answers without
// needlessly flagging right ones. No hard assertion -- like the other grid
// files, the logged numbers are the result, not a pass/fail on one case.
func TestSpecActionConfidence_CorrectVsIncorrectSeparation(t *testing.T) {
	base := requireLaya(t)

	type sample struct {
		correct    bool
		confidence float64
	}
	var samples []sample
	for _, c := range calibrationCases() {
		a := askSpecAction(t, base, c.input)
		correct := a.Choice == c.want
		samples = append(samples, sample{correct: correct, confidence: a.Confidence})
		t.Logf("[correct=%-5v want=%-16s got=%-16s confidence=%.3f] %q",
			correct, c.want, a.Choice, a.Confidence, c.input)
	}

	var correctSum, incorrectSum float64
	var correctN, incorrectN int
	for _, s := range samples {
		if s.correct {
			correctSum += s.confidence
			correctN++
		} else {
			incorrectSum += s.confidence
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
	t.Logf("== correct answers: n=%d mean_confidence=%.3f ==", correctN, correctMean)
	t.Logf("== incorrect answers: n=%d mean_confidence=%.3f ==", incorrectN, incorrectMean)
	t.Logf("== separation (correct_mean - incorrect_mean): %.3f (negative or near-zero means confidence "+
		"does NOT separate correct from incorrect in this domain) ==", correctMean-incorrectMean)

	// Threshold sweep: for an "escalate to LLM if confidence < threshold"
	// policy, how much of each group falls below each candidate threshold?
	// A useful threshold catches most wrong answers (high wrongCaught rate)
	// while unnecessarily escalating few right ones (low rightMisflagged rate).
	thresholds := []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	for _, th := range thresholds {
		var wrongCaught, wrongTotal, rightMisflagged, rightTotal int
		for _, s := range samples {
			if s.correct {
				rightTotal++
				if s.confidence < th {
					rightMisflagged++
				}
			} else {
				wrongTotal++
				if s.confidence < th {
					wrongCaught++
				}
			}
		}
		t.Logf("== threshold=%.1f: catches %d/%d wrong answers (would escalate to LLM), "+
			"unnecessarily escalates %d/%d right answers ==", th, wrongCaught, wrongTotal, rightMisflagged, rightTotal)
	}
}
