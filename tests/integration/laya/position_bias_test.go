//go:build integration

// Slot position bias: with k options whose text is identical, any
// non-uniform answer is pure position preference.
//
// Recorded result (p16, laya 0.3.11, english, mean over 5 states):
//
//	k=3   0.47 0.32 0.21
//	k=5   0.45 0.26 0.13 0.08 0.07
//	k=15  slot 0 = 0.62, every later slot <= 0.15
//
// Matches upstream PR #352's 3.59-logit spread across 5 slots. On distinct,
// well-described options this did not cost accuracy (order averaging left
// CLINC banking unchanged), but with near-duplicate options or ambiguous
// input earlier options win ties -- choose option order deliberately.
package laya_test

import "testing"

func TestPositionBias_IdenticalOptions(t *testing.T) {
	base := requireLaya(t)
	states := []string{"what's my balance", "I want to pay a bill", "tell me a joke", "N/A", "freeze my card"}
	for _, k := range []int{3, 5, 15} {
		crit := make(orderedCriteria, k)
		for i := range crit {
			crit[i] = criterion{Key: "o" + itoa(i), Desc: "a customer banking request"}
		}
		q := choiceQuestion(clincInstructions, crit)
		slots := make([]float64, k)
		for _, s := range states {
			p := askProbs(t, base, s, map[string]questionDef{"q": q}, "")["q"].Probabilities
			for i := range slots {
				slots[i] += p["o"+itoa(i)] / float64(len(states))
			}
		}
		t.Logf("k=%d slot probabilities (identical text): %.3f", k, slots)
		if slots[0] <= slots[k-1] {
			t.Errorf("k=%d: expected first-slot bias (slot0=%.3f > last=%.3f)", k, slots[0], slots[k-1])
		}
	}
}
