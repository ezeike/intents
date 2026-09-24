//go:build integration

// Option-count cost and hierarchy strategies on CLINC's 45 intents across
// three domains (banking, credit_cards, travel; 10 test-half rows per intent,
// n=450). Every strategy's questions are batched into ONE request per item:
// a flat 45-way choice, a 3-way domain choice, and one 15-way choice per
// domain. Descriptions are humanized label names.
//
// Recorded result (p16, laya 0.3.11, english):
//
//	flat 45-way                      0.844
//	oracle 15-way (gold domain)      0.922   (~8-point cost of 3x options)
//	hard hierarchy (domain, then 15) 0.793   p=0.027 vs flat -- worse
//	soft hierarchy P(d)*P(i|d)       0.847   n.s.
//	0.5*flat + 0.5*soft hierarchy    0.867   p=0.006 vs flat -- best
//
// A hard early routing commitment can't recover from its own errors (the
// domain question was only 86% right); averaging two differently-structured
// soft views can.
package laya_test

import "testing"

var clincDomainDesc = orderedCriteria{
	{Key: "banking", Desc: "bank accounts, balances, bills, payments, transfers, fraud"},
	{Key: "credit_cards", Desc: "credit cards, card limits, rewards, credit score, card applications"},
	{Key: "travel", Desc: "flights, hotels, trips, luggage, visas, travel logistics"},
}

func TestCLINC_HierarchyVsFlat(t *testing.T) {
	base := requireLaya(t)
	var all []string
	domainOf := map[string]string{}
	for _, d := range clincDomainOrder {
		all = append(all, clincDomains[d]...)
		for _, l := range clincDomains[d] {
			domainOf[l] = d
		}
	}
	items := loadCLINC(t)
	// The original run used test-half rows 10..19 per intent (the first 10 of
	// the test split), so take the first 10 test items per intent.
	_, test := clincSplit(items, all, false)
	perIntent := map[string]int{}
	var data []clincItem
	for _, it := range test {
		if perIntent[it.Intent] < 10 {
			data = append(data, it)
		}
		perIntent[it.Intent]++
	}

	var flat, oracle, hard, soft, mix, domainOK []bool
	for _, it := range data {
		qs := map[string]questionDef{
			"flat":   choiceQuestion(clincInstructions, humanized(all)),
			"domain": choiceQuestion("Which topic is this customer message about?", clincDomainDesc),
		}
		for _, d := range clincDomainOrder {
			qs[d] = choiceQuestion(clincInstructions, humanized(clincDomains[d]))
		}
		a := askProbs(t, base, it.Text, qs, "")

		pd := a["domain"].Probabilities
		joint := map[string]float64{}
		for _, d := range clincDomainOrder {
			for l, p := range a[d].Probabilities {
				joint[l] = pd[d] * p
			}
		}
		fp := a["flat"].Probabilities
		avg := make(map[string]float64, len(fp))
		for l, p := range fp {
			avg[l] = 0.5*p + 0.5*joint[l]
		}
		chosenDomain := argmaxLabel(pd)

		flat = append(flat, argmaxLabel(fp) == it.Intent)
		oracle = append(oracle, argmaxLabel(a[domainOf[it.Intent]].Probabilities) == it.Intent)
		domainOK = append(domainOK, chosenDomain == domainOf[it.Intent])
		hard = append(hard, argmaxLabel(a[chosenDomain].Probabilities) == it.Intent)
		soft = append(soft, argmaxLabel(joint) == it.Intent)
		mix = append(mix, argmaxLabel(avg) == it.Intent)
	}
	t.Logf("n=%d flat45=%.3f oracle15=%.3f domain-question-acc=%.3f", len(data), rate(flat), rate(oracle), rate(domainOK))
	logVsBaseline(t, "hard_hierarchy", flat, hard)
	logVsBaseline(t, "soft_hierarchy", flat, soft)
	logVsBaseline(t, "flat+soft_average", flat, mix)
}
