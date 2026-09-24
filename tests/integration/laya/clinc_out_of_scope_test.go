//go:build integration

// Out-of-scope detection on CLINC banking: 300 in-scope + 200 oos test
// items, thresholds chosen on dev to keep 95% of dev in-scope.
//
// Recorded result (p16, laya 0.3.11, english):
//
//	signal          AUROC   oos caught   in-scope kept
//	max-p           0.956   63.5%        97.3%
//	margin          0.955   68.5%        96.7%
//	entropy conf    0.958   27.0%        98.3%
//	explicit other  0.962   78.5%        95.7% (in-scope acc 0.870 -> 0.857)
//
// All signals rank oos equally well; the difference is where a dev-chosen
// threshold lands. CLINC oos is mostly off-domain, which is where "other"
// is strongest -- contrast out_of_scope_detection_test.go, where oos inputs
// were in-domain intents with no option to land on and "other" failed.
// Use "other" for junk plus a dev-tuned margin gate for near-misses.
package laya_test

import (
	"sort"
	"testing"
)

type oosSample struct {
	inScope bool
	correct bool
	maxP    float64
	margin  float64
	conf    float64
}

func TestCLINC_OutOfScopeSignals(t *testing.T) {
	base := requireLaya(t)
	dev, test := clincSplit(loadCLINC(t), bankingLabels, true)
	q := choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDesc))

	collect := func(items []clincItem) []oosSample {
		out := make([]oosSample, len(items))
		for i, it := range items {
			a := askProbs(t, base, it.Text, map[string]questionDef{"q": q}, "")["q"]
			top := argmaxLabel(a.Probabilities)
			out[i] = oosSample{
				inScope: it.Intent != clincOOS,
				correct: top == it.Intent,
				maxP:    a.Probabilities[top],
				margin:  marginOf(a.Probabilities),
				conf:    a.Confidence,
			}
		}
		return out
	}
	devS, testS := collect(dev), collect(test)

	signals := []struct {
		name string
		f    func(oosSample) float64
	}{
		{"max_p", func(s oosSample) float64 { return s.maxP }},
		{"margin", func(s oosSample) float64 { return s.margin }},
		{"entropy_confidence", func(s oosSample) float64 { return s.conf }},
	}
	for _, sig := range signals {
		var devIn, testIn, testOut []float64
		for _, s := range devS {
			if s.inScope {
				devIn = append(devIn, sig.f(s))
			}
		}
		for _, s := range testS {
			if s.inScope {
				testIn = append(testIn, sig.f(s))
			} else {
				testOut = append(testOut, sig.f(s))
			}
		}
		sort.Float64s(devIn)
		thr := devIn[len(devIn)*5/100] // keep 95% of dev in-scope
		var kept, caught int
		for _, v := range testIn {
			if v >= thr {
				kept++
			}
		}
		for _, v := range testOut {
			if v < thr {
				caught++
			}
		}
		t.Logf("%-18s AUROC=%.3f thr(dev)=%.3f  test: in-scope kept=%.3f oos caught=%.3f",
			sig.name, auroc(testIn, testOut), thr,
			float64(kept)/float64(len(testIn)), float64(caught)/float64(len(testOut)))
	}

	// Explicit reject option.
	withOther := append(criteriaFor(bankingLabels, bankingDesc),
		criterion{Key: "other", Desc: "anything else, not one of these banking requests"})
	qo := choiceQuestion(clincInstructions, withOther)
	var inN, inCorrect, inToOther, oosN, oosCaught int
	var pIn, pOut []float64
	for _, it := range test {
		p := askProbs(t, base, it.Text, map[string]questionDef{"q": qo}, "")["q"].Probabilities
		top := argmaxLabel(p)
		if it.Intent == clincOOS {
			oosN++
			if top == "other" {
				oosCaught++
			}
			pOut = append(pOut, 1-p["other"])
			continue
		}
		inN++
		if top == it.Intent {
			inCorrect++
		}
		if top == "other" {
			inToOther++
		}
		pIn = append(pIn, 1-p["other"])
	}
	t.Logf("explicit_other     AUROC=%.3f  in-scope acc=%.3f in-scope->other=%.3f oos caught=%.3f",
		auroc(pIn, pOut), float64(inCorrect)/float64(inN), float64(inToOther)/float64(inN), float64(oosCaught)/float64(oosN))
}
