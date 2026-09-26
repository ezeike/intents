//go:build integration

// Jev counterpart to tests/integration/laya/general_accept_reject_test.go: a domain-neutral
// accept/reject two-way choice (no "proposal"/"spec" vocabulary), used specifically because it
// gives confidence_calibration_test.go's spec_action gold set a harder companion — Laya's own
// run of this exact set had one confident miss ("I'll pass" -> accept, wrong), so it's a
// reasonable place to look for a Jev miss too, since the spec_action set alone produced zero
// errors to measure calibration against.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

func acceptRejectCriteria() jev.OrderedCriteria {
	return jev.OrderedCriteria{
		{Key: "accept", Description: "the person is agreeing to, approving of, or going along with something"},
		{Key: "reject", Description: "the person is declining, refusing, or turning down something"},
	}
}

func acceptPhrasings() []string {
	return []string{
		"yes, I'm in", "sounds great, let's do it", "absolutely, go for it", "I'm on board",
		"sure, why not", "that works for me", "agreed", "count me in", "yeah, let's go",
		"perfect, let's move forward",
	}
}

func rejectPhrasings() []string {
	return []string{
		"no thanks", "I'll pass", "not interested", "that doesn't work for me", "nah, I'm out",
		"I disagree", "let's not", "no, I don't want to", "count me out", "I'd rather not",
	}
}

// TestGeneralAcceptReject_Grid_AndCalibration runs the 20-phrasing accept/reject set against live
// Jev, logging per-case accuracy plus confidence/margin, then reports whether either signal
// separates correct from incorrect on whatever errors actually show up. No per-case hard
// assertion, matching the measurement-grid convention this file's laya counterpart uses — the
// logged numbers are the result.
func TestGeneralAcceptReject_Grid_AndCalibration(t *testing.T) {
	c := requireJev(t)

	type sample struct {
		input, want, got string
		correct          bool
		serverConfidence float64
		margin           float64
	}
	ask := func(input string) sample {
		resp, err := c.SystemOne(context.Background(), jev.Request{
			State: map[string]string{"input": input},
			Questions: map[string]jev.Question{
				"decision": {
					Type:         "choice",
					Instructions: "Is the person accepting/agreeing, or rejecting/declining?",
					Criteria:     acceptRejectCriteria(),
				},
			},
			Model: jevModel(),
		})
		if err != nil {
			t.Fatalf("SystemOne(%q): %v", input, err)
		}
		a, err := resp.Choice("decision")
		if err != nil {
			t.Fatalf("Choice(%q): %v", input, err)
		}
		return sample{serverConfidence: a.Confidence, margin: margin(a.Probabilities), got: a.Choice}
	}

	var samples []sample
	var acceptHits, rejectHits int
	for _, p := range acceptPhrasings() {
		s := ask(p)
		s.input, s.want, s.correct = p, "accept", s.got == "accept"
		if s.correct {
			acceptHits++
		}
		samples = append(samples, s)
		t.Logf("[correct=%-5v want=accept got=%-6s server_confidence=%.3f margin=%.3f] %q", s.correct, s.got, s.serverConfidence, s.margin, p)
	}
	for _, p := range rejectPhrasings() {
		s := ask(p)
		s.input, s.want, s.correct = p, "reject", s.got == "reject"
		if s.correct {
			rejectHits++
		}
		samples = append(samples, s)
		t.Logf("[correct=%-5v want=reject got=%-6s server_confidence=%.3f margin=%.3f] %q", s.correct, s.got, s.serverConfidence, s.margin, p)
	}
	t.Logf("== accept: %d/%d correct | reject: %d/%d correct | overall: %d/%d ==",
		acceptHits, len(acceptPhrasings()), rejectHits, len(rejectPhrasings()), acceptHits+rejectHits, len(samples))

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
}
