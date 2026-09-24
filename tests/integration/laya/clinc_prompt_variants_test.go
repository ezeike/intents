//go:build integration

// Prompt-side techniques on the CLINC banking 15-way set (300 test items),
// each compared against the same baseline on the same items.
//
// Recorded result (p16, laya 0.3.11, english checkpoint): baseline 0.870.
// Significant moves were all DOWN: contrastive "(not X)" descriptions 0.800
// (p=0.001), the multilingual checkpoint 0.793 (p=0.004), and a one-word
// instruction ("intent") 0.847 (p=0.016). Label names (opaque/bare),
// description length, a domain-specific instruction, contextual/batch
// calibration, option-order averaging, description ensembling (+1.7-2,
// p=0.15-0.23) and a pairwise top-2 tie-break were all within noise.
package laya_test

import (
	"math/rand/v2"
	"testing"
)

var bankingLabels = clincDomains["banking"]

// bankingDescLong is a second, longer hand-written phrasing set.
var bankingDescLong = map[string]string{
	"account_blocked":  "my account is locked or restricted and I want to know why",
	"interest_rate":    "question about the rate of interest earned on savings or checking",
	"freeze_account":   "request to put a freeze or hold on the account",
	"bill_balance":     "how much is owed on a utility or other bill",
	"balance":          "how much money is currently available in the account",
	"routing":          "what is the routing or ABA number of the bank",
	"bill_due":         "the due date of an upcoming bill",
	"transactions":     "list recent purchases or charges on the account",
	"report_fraud":     "tell the bank about a charge or activity the customer did not make",
	"spending_history": "total spent on a category or merchant over some time",
	"pin_change":       "set a new PIN number for a card",
	"pay_bill":         "make a payment toward a bill",
	"order_checks":     "request a new checkbook or more paper checks",
	"transfer":         "send money from one account to another",
	"min_payment":      "the smallest amount that must be paid on a credit card",
}

// bankingDescShort is a near-bare third phrasing set.
var bankingDescShort = map[string]string{
	"account_blocked":  "account blocked",
	"interest_rate":    "interest rate",
	"freeze_account":   "freeze account",
	"bill_balance":     "bill balance owed",
	"balance":          "account balance",
	"routing":          "routing number",
	"bill_due":         "bill due date",
	"transactions":     "recent transactions",
	"report_fraud":     "report fraud",
	"spending_history": "spending history",
	"pin_change":       "change PIN",
	"pay_bill":         "pay a bill",
	"order_checks":     "order checks",
	"transfer":         "transfer money",
	"min_payment":      "minimum payment",
}

// bankingDescContrast adds "(not X)" contrasts to the confusable options,
// written from dev-split errors only.
func bankingDescContrast() map[string]string {
	d := make(map[string]string, len(bankingDesc))
	for k, v := range bankingDesc {
		d[k] = v
	}
	d["account_blocked"] = "asks why the account is blocked/locked or how to UNLOCK or unblock it (not a request to freeze it)"
	d["freeze_account"] = "wants the bank to freeze, lock or stop all access to their account now (not unlocking it)"
	d["report_fraud"] = "reports a charge, purchase or transaction on the account they did not make or do not recognize"
	d["transactions"] = "wants a list of or detail about specific recent transactions (not a total spent, not fraud)"
	d["spending_history"] = "asks for a TOTAL amount spent on a category or merchant over time (not a list of transactions)"
	d["bill_balance"] = "asks how much is owed in total on a bill (not the minimum payment, not the due date)"
	d["min_payment"] = "asks for the MINIMUM or lowest amount they are allowed to pay on a card or bill"
	d["bill_due"] = "asks WHEN a bill is due or the latest date to pay it (not paying it now)"
	d["pay_bill"] = "wants to make a payment on a bill now"
	d["balance"] = "asks how much money is in their account or whether they have enough funds for a purchase"
	return d
}

func bankingBaseline(t *testing.T, base string, items []clincItem) []map[string]float64 {
	t.Helper()
	return runChoice(t, base, items, choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDesc)), "")
}

// TestCLINC_PromptVariants compares label naming, description phrasing,
// instruction wording, contrastive descriptions and checkpoint choice.
func TestCLINC_PromptVariants(t *testing.T) {
	base := requireLaya(t)
	_, test := clincSplit(loadCLINC(t), bankingLabels, false)
	baseOK := correctness(bankingBaseline(t, base, test), test)
	t.Logf("baseline acc=%.3f (n=%d)", rate(baseOK), len(test))

	// Opaque keys A..O carrying the same descriptions, mapped back to labels.
	opaque := make(orderedCriteria, len(bankingLabels))
	for i, l := range bankingLabels {
		opaque[i] = criterion{Key: string(rune('A' + i)), Desc: bankingDesc[l]}
	}
	opaqueProbs := runChoice(t, base, test, choiceQuestion(clincInstructions, opaque), "")
	for i, p := range opaqueProbs {
		m := make(map[string]float64, len(p))
		for j, l := range bankingLabels {
			m[l] = p[string(rune('A'+j))]
		}
		opaqueProbs[i] = m
	}
	logVsBaseline(t, "opaque_keys", baseOK, correctness(opaqueProbs, test))

	logVsBaseline(t, "bare_label_list", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion(clincInstructions, bankingLabels), ""), test))
	logVsBaseline(t, "desc_long", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDescLong)), ""), test))
	logVsBaseline(t, "desc_short", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDescShort)), ""), test))
	logVsBaseline(t, "instr_banking", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion("Which banking request is this customer making?", criteriaFor(bankingLabels, bankingDesc)), ""), test))
	logVsBaseline(t, "instr_one_word", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion("intent", criteriaFor(bankingLabels, bankingDesc)), ""), test))
	logVsBaseline(t, "desc_contrastive", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDescContrast())), ""), test))
	logVsBaseline(t, "multilingual_ckpt", baseOK,
		correctness(runChoice(t, base, test, choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDesc)), "multilingual"), test))
}

// TestCLINC_Calibration tries two prior-correction methods from the LLM
// literature: contextual calibration (divide by the answer to content-free
// states) and batch calibration (divide by the mean answer over unlabeled
// dev traffic, oos included).
func TestCLINC_Calibration(t *testing.T) {
	base := requireLaya(t)
	dev, test := clincSplit(loadCLINC(t), bankingLabels, true)
	test = inScope(test)
	testProbs := bankingBaseline(t, base, test)
	baseOK := correctness(testProbs, test)

	q := choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDesc))
	ctxPrior := map[string]float64{}
	contentFree := []string{"N/A", "none", "."}
	for _, s := range contentFree {
		for l, v := range askProbs(t, base, s, map[string]questionDef{"q": q}, "")["q"].Probabilities {
			ctxPrior[l] += v / float64(len(contentFree))
		}
	}
	t.Logf("content-free prior: %v", ctxPrior)

	batchPrior := map[string]float64{}
	for _, p := range bankingBaseline(t, base, dev) {
		for l, v := range p {
			batchPrior[l] += v / float64(len(dev))
		}
	}

	apply := func(prior map[string]float64) []map[string]float64 {
		out := make([]map[string]float64, len(testProbs))
		for i, p := range testProbs {
			m := make(map[string]float64, len(p))
			for l, v := range p {
				m[l] = v / max(prior[l], 1e-6)
			}
			out[i] = normalize(m)
		}
		return out
	}
	logVsBaseline(t, "contextual_calibration", baseOK, correctness(apply(ctxPrior), test))
	logVsBaseline(t, "batch_calibration", baseOK, correctness(apply(batchPrior), test))
}

// TestCLINC_Ensembles averages several views of the same question batched
// into ONE request: 4 option orders, and 3 description phrasings.
func TestCLINC_Ensembles(t *testing.T) {
	base := requireLaya(t)
	_, test := clincSplit(loadCLINC(t), bankingLabels, false)
	baseOK := correctness(bankingBaseline(t, base, test), test)

	rng := rand.New(rand.NewPCG(1, 2))
	orders := [][]string{bankingLabels}
	for len(orders) < 4 {
		o := append([]string(nil), bankingLabels...)
		rng.Shuffle(len(o), func(i, j int) { o[i], o[j] = o[j], o[i] })
		orders = append(orders, o)
	}
	descSets := []map[string]string{bankingDesc, bankingDescLong, bankingDescShort}

	permProbs := make([]map[string]float64, len(test))
	descProbs := make([]map[string]float64, len(test))
	for i, it := range test {
		qs := map[string]questionDef{}
		for j, o := range orders {
			qs["perm"+itoa(j)] = choiceQuestion(clincInstructions, criteriaFor(o, bankingDesc))
		}
		for j, d := range descSets {
			qs["desc"+itoa(j)] = choiceQuestion(clincInstructions, criteriaFor(bankingLabels, d))
		}
		a := askProbs(t, base, it.Text, qs, "")
		permProbs[i], descProbs[i] = map[string]float64{}, map[string]float64{}
		for _, l := range bankingLabels {
			for j := range orders {
				permProbs[i][l] += a["perm"+itoa(j)].Probabilities[l] / float64(len(orders))
			}
			for j := range descSets {
				descProbs[i][l] += a["desc"+itoa(j)].Probabilities[l] / float64(len(descSets))
			}
		}
	}
	logVsBaseline(t, "order_average_x4", baseOK, correctness(permProbs, test))
	logVsBaseline(t, "desc_ensemble_x3", baseOK, correctness(descProbs, test))
}

// TestCLINC_PairwiseTieBreak re-asks the top two options as a 2-way
// question (both orders, averaged) whenever the top-two margin is below tau.
func TestCLINC_PairwiseTieBreak(t *testing.T) {
	base := requireLaya(t)
	_, test := clincSplit(loadCLINC(t), bankingLabels, false)
	baseProbs := bankingBaseline(t, base, test)
	baseOK := correctness(baseProbs, test)

	for _, tau := range []float64{0.2, 0.4, 0.6} {
		out := make([]map[string]float64, len(test))
		var fired int
		for i, it := range test {
			p := baseProbs[i]
			if marginOf(p) >= tau {
				out[i] = p
				continue
			}
			fired++
			a := argmaxLabel(p)
			rest := make(map[string]float64, len(p))
			for k, v := range p {
				if k != a {
					rest[k] = v
				}
			}
			b := argmaxLabel(rest)
			ans := askProbs(t, base, it.Text, map[string]questionDef{
				"ab": choiceQuestion(clincInstructions, criteriaFor([]string{a, b}, bankingDesc)),
				"ba": choiceQuestion(clincInstructions, criteriaFor([]string{b, a}, bankingDesc)),
			}, "")
			pa := (ans["ab"].Probabilities[a] + ans["ba"].Probabilities[a]) / 2
			q := make(map[string]float64, len(p))
			for k, v := range p {
				q[k] = v
			}
			q[a], q[b] = pa, 1-pa
			out[i] = q
		}
		t.Logf("tau=%.1f fired on %d/%d", tau, fired, len(test))
		logVsBaseline(t, "pairwise_tau"+itoa(int(tau*10)), baseOK, correctness(out, test))
	}
}
