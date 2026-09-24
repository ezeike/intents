//go:build integration

// Shared fixtures for the CLINC150 technique experiments (clinc_*_test.go,
// negation_stripping_test.go). These measure which request-shaping techniques
// actually move Laya's accuracy on a public, realistic intent set with
// confusable neighbours, rather than on this package's own spec_action
// phrasings. Results and interpretation are recorded in research.db under
// laya-classification/techniques.
//
// Every comparison is paired (same items, same split) and reported with an
// exact McNemar test: with 300 test items only ~5-point differences are
// resolvable, so a non-significant delta is "inconclusive", not "no effect".
package laya_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"os"
	"testing"
)

type clincItem struct {
	Text   string `json:"text"`
	Intent string `json:"intent"`
}

const clincOOS = "oos"

// clincDomains lists the 15 intents per domain, in the option order the
// experiments were originally run with. Order matters: the english checkpoint
// has a strong first-slot bias (position_bias_test.go).
var clincDomains = map[string][]string{
	"banking": {
		"account_blocked", "interest_rate", "freeze_account", "bill_balance", "balance",
		"routing", "bill_due", "transactions", "report_fraud", "spending_history",
		"pin_change", "pay_bill", "order_checks", "transfer", "min_payment",
	},
	"credit_cards": {
		"redeem_rewards", "report_lost_card", "improve_credit_score", "credit_limit", "expiration_date",
		"card_declined", "rewards_balance", "credit_score", "damaged_card", "replacement_card_duration",
		"new_card", "application_status", "credit_limit_change", "apr", "international_fees",
	},
	"travel": {
		"exchange_rate", "car_rental", "vaccines", "international_visa", "translate",
		"carry_on", "book_flight", "timezone", "flight_status", "lost_luggage",
		"book_hotel", "plug_type", "travel_alert", "travel_notification", "travel_suggestion",
	},
}

var clincDomainOrder = []string{"banking", "credit_cards", "travel"}

const clincInstructions = "What does the customer want in this message?"

// bankingDesc is the hand-written baseline description set (written before
// any results were seen).
var bankingDesc = map[string]string{
	"account_blocked":  "asks why their bank account is blocked or locked",
	"interest_rate":    "asks what interest rate an account pays",
	"freeze_account":   "wants to freeze their bank account",
	"bill_balance":     "asks how much they owe on a bill",
	"balance":          "asks how much money is in their bank account",
	"routing":          "asks for the bank routing number",
	"bill_due":         "asks when a bill is due",
	"transactions":     "asks to see recent transactions on an account",
	"report_fraud":     "wants to report fraudulent or suspicious activity",
	"spending_history": "asks how much they spent on something over a period",
	"pin_change":       "wants to change their PIN",
	"pay_bill":         "wants to pay a bill",
	"order_checks":     "wants to order new checks",
	"transfer":         "wants to move money between accounts",
	"min_payment":      "asks for the minimum payment due on a card or bill",
}

// ---- ordered criteria ----

// orderedCriteria marshals as a JSON object whose keys keep slice order.
// encoding/json sorts map keys, so a map[string]string criteria is always
// sent in alphabetical option order -- which, given the slot bias, silently
// changes the question.
type orderedCriteria []criterion

type criterion struct{ Key, Desc string }

func (o orderedCriteria) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, c := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(c.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(c.Desc)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// criteriaFor builds ordered criteria for labels, looking each description up in desc.
func criteriaFor(labels []string, desc map[string]string) orderedCriteria {
	out := make(orderedCriteria, 0, len(labels))
	for _, l := range labels {
		out = append(out, criterion{Key: l, Desc: desc[l]})
	}
	return out
}

// humanized uses the label name with underscores as spaces as its description.
func humanized(labels []string) orderedCriteria {
	out := make(orderedCriteria, 0, len(labels))
	for _, l := range labels {
		d := []byte(l)
		for i := range d {
			if d[i] == '_' {
				d[i] = ' '
			}
		}
		out = append(out, criterion{Key: l, Desc: string(d)})
	}
	return out
}

func choiceQuestion(instructions string, criteria any) questionDef {
	return questionDef{Type: "choice", Instructions: instructions, Criteria: criteria}
}

// ---- data ----

func loadCLINC(t *testing.T) []clincItem {
	t.Helper()
	f, err := os.Open("testdata/clinc150_subset.jsonl")
	if err != nil {
		t.Fatalf("open clinc testdata: %v", err)
	}
	defer f.Close()
	var items []clincItem
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var it clincItem
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			t.Fatalf("decode clinc row: %v", err)
		}
		items = append(items, it)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan clinc testdata: %v", err)
	}
	return items
}

// clincSplit returns the fixed dev/test split for the given intents: per
// intent the first 10 rows are dev and the next 20 test. If withOOS, the
// first 100 oos rows join dev and the next 200 join test.
func clincSplit(items []clincItem, intents []string, withOOS bool) (dev, test []clincItem) {
	want := make(map[string]bool, len(intents))
	for _, i := range intents {
		want[i] = true
	}
	seen := map[string]int{}
	for _, it := range items {
		n := seen[it.Intent]
		seen[it.Intent] = n + 1
		switch {
		case want[it.Intent]:
			if n < 10 {
				dev = append(dev, it)
			} else if n < 30 {
				test = append(test, it)
			}
		case withOOS && it.Intent == clincOOS:
			if n < 100 {
				dev = append(dev, it)
			} else if n < 300 {
				test = append(test, it)
			}
		}
	}
	return dev, test
}

func inScope(items []clincItem) []clincItem {
	var out []clincItem
	for _, it := range items {
		if it.Intent != clincOOS {
			out = append(out, it)
		}
	}
	return out
}

// ---- asking ----

// askProbs sends one request (english checkpoint unless model is set) and
// returns each choice question's probability map by question id.
func askProbs(t *testing.T, base string, state any, qs map[string]questionDef, model string) map[string]choiceAnswer {
	t.Helper()
	if model == "" {
		model = "english"
	}
	resp := mustOK(t, base, systemOneRequest{State: state, Questions: qs, Model: model})
	out := make(map[string]choiceAnswer, len(qs))
	for id, q := range qs {
		if q.Type == "choice" {
			out[id] = decodeChoice(t, resp, id)
		}
	}
	return out
}

// runChoice asks one choice question per item and returns the probability maps.
func runChoice(t *testing.T, base string, items []clincItem, q questionDef, model string) []map[string]float64 {
	t.Helper()
	out := make([]map[string]float64, len(items))
	for i, it := range items {
		out[i] = askProbs(t, base, it.Text, map[string]questionDef{"q": q}, model)["q"].Probabilities
	}
	return out
}

func argmaxLabel(p map[string]float64) string {
	best, bestP := "", math.Inf(-1)
	for k, v := range p {
		if v > bestP || (v == bestP && k < best) {
			best, bestP = k, v
		}
	}
	return best
}

func correctness(preds []map[string]float64, items []clincItem) []bool {
	ok := make([]bool, len(items))
	for i, it := range items {
		ok[i] = argmaxLabel(preds[i]) == it.Intent
	}
	return ok
}

func rate(ok []bool) float64 {
	var n int
	for _, b := range ok {
		if b {
			n++
		}
	}
	return float64(n) / float64(len(ok))
}

func normalize(p map[string]float64) map[string]float64 {
	var s float64
	for _, v := range p {
		s += v
	}
	out := make(map[string]float64, len(p))
	for k, v := range p {
		out[k] = v / s
	}
	return out
}

// ---- statistics ----

// mcnemarExact returns how many items only a got right, only b got right,
// and the two-sided exact binomial p-value on those discordant pairs.
func mcnemarExact(a, b []bool) (aOnly, bOnly int, p float64) {
	for i := range a {
		switch {
		case a[i] && !b[i]:
			aOnly++
		case b[i] && !a[i]:
			bOnly++
		}
	}
	n := aOnly + bOnly
	if n == 0 {
		return 0, 0, 1
	}
	k := min(aOnly, bOnly)
	var tail float64
	for i := 0; i <= k; i++ {
		lc, _ := math.Lgamma(float64(n + 1))
		l1, _ := math.Lgamma(float64(i + 1))
		l2, _ := math.Lgamma(float64(n - i + 1))
		tail += math.Exp(lc - l1 - l2 - float64(n)*math.Ln2)
	}
	return aOnly, bOnly, min(1, 2*tail)
}

func logVsBaseline(t *testing.T, name string, baseline, variant []bool) {
	t.Helper()
	bOnly, vOnly, p := mcnemarExact(baseline, variant)
	t.Logf("%-22s acc=%.3f  vs baseline %.3f: baseline-only-right=%d variant-only-right=%d McNemar p=%.3f",
		name, rate(variant), rate(baseline), bOnly, vOnly, p)
}

// auroc is P(score of a positive > score of a negative), ties counted half.
func auroc(pos, neg []float64) float64 {
	var s float64
	for _, a := range pos {
		for _, b := range neg {
			switch {
			case a > b:
				s++
			case a == b:
				s += 0.5
			}
		}
	}
	return s / float64(len(pos)*len(neg))
}
