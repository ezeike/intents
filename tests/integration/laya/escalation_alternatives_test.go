//go:build integration

// Two alternatives to raw `confidence` for deciding when spec_action should
// hand off to an LLM instead of trusting its own answer -- confidence itself
// was measured and rejected in confidence_calibration_test.go (correct mean
// 0.431, incorrect mean 0.304, no threshold both high-recall and
// high-precision). Both alternatives use the same calibrationCases() gold
// set for a like-for-like comparison:
//
//  1. Top-two probability margin (top1 - top2). Already used elsewhere in
//     this package for compound-intent detection (docs/architecture.md's
//     TopTwo pattern) -- tests whether "how much did the winner beat the
//     runner-up by" separates correct from incorrect better than the
//     model's own confidence score does.
//  2. An orthogonal noul, batched alongside spec_action in the same request
//     (same pattern as referential_gate_test.go's is_referential): ask the
//     model to self-assess ambiguity directly, rather than reading it off
//     the choice distribution at all.
package laya_test

import "testing"

// marginOf returns the gap between the top and second-highest probability
// in a choice distribution -- 0 if there's only one option or a tie.
func marginOf(p map[string]float64) float64 {
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

const ambiguityInstructions = "Is it genuinely unclear or ambiguous which single action the user " +
	"wants from the available options, such that a person or a more capable system should decide " +
	"instead of guessing?"

func askSpecActionWithAmbiguity(t *testing.T, base, input string) (choiceAnswer, noulAnswer) {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
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
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "spec_action"), decodeNoul(t, resp, "is_ambiguous")
}

// thresholdSweep runs the same catches-wrong-vs-misflags-right analysis
// confidence_calibration_test.go used, generalized over any per-case scalar
// signal, so margin and is_ambiguous can be compared on equal footing.
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

// TestSpecActionMargin_CorrectVsIncorrectSeparation checks whether top-two
// probability margin separates correct from incorrect better than raw
// confidence did.
func TestSpecActionMargin_CorrectVsIncorrectSeparation(t *testing.T) {
	base := requireLaya(t)

	var samples []scoredSample
	for _, c := range calibrationCases() {
		a := askSpecAction(t, base, c.input)
		correct := a.Choice == c.want
		margin := marginOf(a.Probabilities)
		samples = append(samples, scoredSample{correct: correct, signal: margin})
		t.Logf("[correct=%-5v want=%-16s got=%-16s margin=%.3f] %q", correct, c.want, a.Choice, margin, c.input)
	}
	thresholdSweep(t, samples, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, true, "margin")
}

// TestSpecActionAmbiguityNoul_CorrectVsIncorrectSeparation checks whether a
// direct, orthogonal self-assessment ("is this ambiguous?") separates
// correct from incorrect better than reading confidence off the choice
// distribution itself.
func TestSpecActionAmbiguityNoul_CorrectVsIncorrectSeparation(t *testing.T) {
	base := requireLaya(t)

	var samples []scoredSample
	for _, c := range calibrationCases() {
		a, amb := askSpecActionWithAmbiguity(t, base, c.input)
		correct := a.Choice == c.want
		samples = append(samples, scoredSample{correct: correct, signal: amb.Noul})
		t.Logf("[correct=%-5v want=%-16s got=%-16s is_ambiguous=%.3f] %q", correct, c.want, a.Choice, amb.Noul, c.input)
	}
	// belowMeansEscalate=false: a HIGH is_ambiguous score is what should
	// trigger escalation here, the opposite direction from confidence/margin.
	thresholdSweep(t, samples, []float64{0.3, 0.4, 0.5, 0.6, 0.7}, false, "is_ambiguous")
}
