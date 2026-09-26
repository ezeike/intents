//go:build integration

// Jev counterpart to tests/integration/laya/escalation_alternatives_test.go: two alternatives to
// raw server `confidence` for deciding when spec_action should hand off to an LLM instead of
// trusting its own answer -- server confidence itself was measured in
// confidence_calibration_test.go. Both alternatives use the same calibrationCases() gold set for a
// like-for-like comparison:
//
//  1. Top-two probability margin (top1 - top2) -- already computed as margin() in
//     confidence_calibration_test.go, reused here rather than redefined.
//  2. An orthogonal noul, batched alongside spec_action in the same request (same pattern as
//     referential_gate_test.go's is_referential): ask the model to self-assess ambiguity directly,
//     rather than reading it off the choice distribution at all.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

const ambiguityInstructions = "Is it genuinely unclear or ambiguous which single action the user " +
	"wants from the available options, such that a person or a more capable system should decide " +
	"instead of guessing?"

func askSpecActionWithAmbiguity(t *testing.T, c *jev.Client, input string) (jev.ChoiceAnswer, jev.NoulAnswer) {
	t.Helper()
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: map[string]string{"input": input},
		Questions: map[string]jev.Question{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     specActionCriteria(),
			},
			"is_ambiguous": {
				Type:         "noul",
				Instructions: ambiguityInstructions,
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(%q): %v", input, err)
	}
	choice, err := resp.Choice("spec_action")
	if err != nil {
		t.Fatalf("Choice(%q): %v", input, err)
	}
	amb, err := resp.Noul("is_ambiguous")
	if err != nil {
		t.Fatalf("Noul(%q): %v", input, err)
	}
	return choice, amb
}

// thresholdSweep runs the same catches-wrong-vs-misflags-right analysis
// confidence_calibration_test.go used, generalized over any per-case scalar signal, so margin and
// is_ambiguous can be compared on equal footing. Also reused by out_of_scope_detection_test.go for
// in-scope-vs-out-of-scope separation.
type scoredSample struct {
	correct bool
	signal  float64
}

func thresholdSweep(t *testing.T, samples []scoredSample, thresholds []float64, belowMeansEscalate bool, signalName string) {
	t.Helper()
	var correctSum, incorrectSum float64
	var correctN, incorrectN int
	for _, s := range samples {
		if s.correct {
			correctSum += s.signal
			correctN++
		} else {
			incorrectSum += s.signal
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
	t.Logf("== %s: correct mean=%.3f (n=%d), incorrect mean=%.3f (n=%d), separation=%.3f ==",
		signalName, correctMean, correctN, incorrectMean, incorrectN, correctMean-incorrectMean)

	for _, th := range thresholds {
		var wrongCaught, wrongTotal, rightMisflagged, rightTotal int
		for _, s := range samples {
			escalate := s.signal < th
			if !belowMeansEscalate {
				escalate = s.signal > th
			}
			if s.correct {
				rightTotal++
				if escalate {
					rightMisflagged++
				}
			} else {
				wrongTotal++
				if escalate {
					wrongCaught++
				}
			}
		}
		t.Logf("== %s threshold=%.2f: catches %d/%d wrong answers, unnecessarily escalates %d/%d right answers ==",
			signalName, th, wrongCaught, wrongTotal, rightMisflagged, rightTotal)
	}
}

// TestSpecActionMargin_CorrectVsIncorrectSeparation checks whether top-two probability margin
// separates correct from incorrect better than raw server confidence did
// (confidence_calibration_test.go already reports this same comparison inline; this test isolates
// it against the threshold-sweep helper for direct comparison with is_ambiguous below).
func TestSpecActionMargin_CorrectVsIncorrectSeparation(t *testing.T) {
	c := requireJev(t)

	var samples []scoredSample
	for _, tc := range calibrationCases() {
		a := askSpecAction(t, c, tc.input)
		correct := a.Choice == tc.want
		m := margin(a.Probabilities)
		samples = append(samples, scoredSample{correct: correct, signal: m})
		t.Logf("[correct=%-5v want=%-16s got=%-16s margin=%.3f] %q", correct, tc.want, a.Choice, m, tc.input)
	}
	thresholdSweep(t, samples, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, true, "margin")
}

// TestSpecActionAmbiguityNoul_CorrectVsIncorrectSeparation checks whether a direct, orthogonal
// self-assessment ("is this ambiguous?") separates correct from incorrect better than reading
// confidence/margin off the choice distribution itself.
func TestSpecActionAmbiguityNoul_CorrectVsIncorrectSeparation(t *testing.T) {
	c := requireJev(t)

	var samples []scoredSample
	for _, tc := range calibrationCases() {
		a, amb := askSpecActionWithAmbiguity(t, c, tc.input)
		correct := a.Choice == tc.want
		samples = append(samples, scoredSample{correct: correct, signal: amb.Noul})
		t.Logf("[correct=%-5v want=%-16s got=%-16s is_ambiguous=%.3f] %q", correct, tc.want, a.Choice, amb.Noul, tc.input)
	}
	// belowMeansEscalate=false: a HIGH is_ambiguous score is what should trigger escalation here,
	// the opposite direction from confidence/margin.
	thresholdSweep(t, samples, []float64{0.3, 0.4, 0.5, 0.6, 0.7}, false, "is_ambiguous")
}
