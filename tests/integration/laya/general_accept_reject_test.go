//go:build integration

// General accept-vs-reject classification, deliberately kept domain-neutral:
// no "proposal", "spec", "commit", or any block-builder vocabulary anywhere
// in this file. proposal_grid_test.go found that the literal word "proposal"
// in an input was a far stronger predictor than either state or intent
// understanding (75% vs 30% accuracy) -- this file isolates the underlying
// question "can the model tell accept from reject at all" from that
// confound, using a single generic decision two people could be having about
// almost anything (an invitation, a request, a plan).
//
// Same measurement-grid shape and rationale as proposal_grid_test.go: no
// per-cell hard assertions, an aggregate accuracy summary is the result.
package laya_test

import "testing"

// acceptRejectCriteria is a plain two-way choice, no proposal/spec wording.
func acceptRejectCriteria() map[string]string {
	return map[string]string{
		"accept": "the person is agreeing to, approving of, or going along with something",
		"reject": "the person is declining, refusing, or turning down something",
	}
}

func askAcceptReject(t *testing.T, base, input string) choiceAnswer {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: map[string]string{"input": input},
		Questions: map[string]questionDef{
			"decision": {
				Type:         "choice",
				Instructions: "Is the person accepting/agreeing, or rejecting/declining?",
				Criteria:     acceptRejectCriteria(),
			},
		},
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "decision")
}

// acceptPhrasings and rejectPhrasings: 10 each, everyday register, covering
// direct statements, casual/slang phrasing, and understated phrasing -- on
// purpose, not just the cleanest textbook examples.
func acceptPhrasings() []string {
	return []string{
		"yes, I'm in",
		"sounds great, let's do it",
		"absolutely, go for it",
		"I'm on board",
		"sure, why not",
		"that works for me",
		"agreed",
		"count me in",
		"yeah, let's go",
		"perfect, let's move forward",
	}
}

func rejectPhrasings() []string {
	return []string{
		"no thanks",
		"I'll pass",
		"not interested",
		"that doesn't work for me",
		"nah, I'm out",
		"I disagree",
		"let's not",
		"no, I don't want to",
		"count me out",
		"I'd rather not",
	}
}

// TestGeneralAcceptReject_Grid runs both phrasing lists against the plain
// accept/reject choice and logs per-side and overall accuracy. No per-case
// hard assertions -- see proposal_grid_test.go's header for why this shape
// is used instead of individually-named assertions.
func TestGeneralAcceptReject_Grid(t *testing.T) {
	base := requireLaya(t)

	type side struct {
		want      string
		phrasings []string
	}
	sides := []side{
		{want: "accept", phrasings: acceptPhrasings()},
		{want: "reject", phrasings: rejectPhrasings()},
	}

	totalCorrect, totalCases := 0, 0
	for _, s := range sides {
		hits := 0
		for _, phrasing := range s.phrasings {
			a := askAcceptReject(t, base, phrasing)
			correct := a.Choice == s.want
			if correct {
				hits++
			}
			t.Logf("[want=%-6s] %-35q -> %-6s (accept=%.3f reject=%.3f)",
				s.want, phrasing, a.Choice, a.Probabilities["accept"], a.Probabilities["reject"])
		}
		t.Logf("== summary want=%s: %d/%d correct ==", s.want, hits, len(s.phrasings))
		totalCorrect += hits
		totalCases += len(s.phrasings)
	}
	t.Logf("== overall: %d/%d correct ==", totalCorrect, totalCases)
}
