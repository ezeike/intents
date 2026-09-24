//go:build integration

// Negated clauses pull Laya toward the intent being ruled out ("don't
// freeze my account, I just want my balance" -> freeze_account). Tests a
// NegEx-style pre-filter that drops a leading negated clause before
// classifying, and whether the model can flag negation itself via a noul.
//
// Recorded result (p16, laya 0.3.11, english, banking 15-way): plain 17/20,
// all 3 misses on the negated intent; stripped 20/20 (McNemar p=0.25, n.s.).
// The is_negated noul scored 0.26-0.83 with no separation -- don't ask the
// model to detect its own weak spot.
//
// Caveat: the 20 cases were hand-written in the exact "negated clause,
// real request" template the regex targets. The regex misses mid-sentence
// negation and would corrupt genuine negative requests ("I don't recognize
// this charge" is report_fraud) -- treat this as evidence for the failure
// mode, not as a production-ready rule.
package laya_test

import (
	"regexp"
	"testing"
)

var leadingNegatedClause = regexp.MustCompile(`(?i)^[^,]*\b(?:don't|do not|not|no need to|no|never mind|forget|wasn't|isn't|i'm not|i don't)\b[^,]*,\s*`)

func stripLeadingNegation(s string) string {
	return leadingNegatedClause.ReplaceAllString(s, "")
}

func negationCases() []clincItem {
	return []clincItem{
		{"don't freeze my account, I just want to know my balance", "balance"},
		{"I'm not trying to transfer money, what's my routing number", "routing"},
		{"I don't want to pay the bill yet, when is it due", "bill_due"},
		{"no need to report fraud, just show my recent transactions", "transactions"},
		{"don't change my pin, I want to order new checks", "order_checks"},
		{"I'm not asking about the minimum payment, how much do I owe in total on my phone bill", "bill_balance"},
		{"not looking to pay anything, what interest rate does my savings account earn", "interest_rate"},
		{"don't show me transactions, how much did I spend on groceries this month", "spending_history"},
		{"I don't need my balance, please freeze my account", "freeze_account"},
		{"I'm not reporting fraud, I just want to know why my account is locked", "account_blocked"},
		{"don't move any money, I want to pay my electric bill", "pay_bill"},
		{"forget the due date, what's the minimum I have to pay on my card", "min_payment"},
		{"don't unlock anything, I want to change my PIN", "pin_change"},
		{"I don't want to order checks, send 200 dollars from checking to savings", "transfer"},
		{"never mind my bill, someone made purchases on my card that weren't me", "report_fraud"},
		{"not my spending, just list the last five transactions", "transactions"},
		{"I wasn't asking for the routing number, what's my account balance", "balance"},
		{"no transfer please, when is my credit card bill due", "bill_due"},
		{"don't freeze it, why is my account blocked", "account_blocked"},
		{"I don't care about interest, how much do I owe on the water bill", "bill_balance"},
	}
}

func TestNegation_LeadingClauseStripping(t *testing.T) {
	base := requireLaya(t)
	q := choiceQuestion(clincInstructions, criteriaFor(bankingLabels, bankingDesc))
	negQ := questionDef{
		Type:         "noul",
		Instructions: "Does the message say the customer does NOT want some action?",
		Criteria:     map[string]string{"true": "the customer explicitly rules out or declines an action", "false": "no action is ruled out"},
		Labels:       map[string]string{"true": "A", "false": "B"},
	}
	var plain, stripped []bool
	for _, c := range negationCases() {
		resp := mustOK(t, base, systemOneRequest{
			State:     c.Text,
			Questions: map[string]questionDef{"q": q, "is_negated": negQ},
			Model:     "english",
		})
		p := decodeChoice(t, resp, "q")
		n := decodeNoul(t, resp, "is_negated")
		s := askProbs(t, base, stripLeadingNegation(c.Text), map[string]questionDef{"q": q}, "")["q"]
		plain = append(plain, p.Choice == c.Intent)
		stripped = append(stripped, s.Choice == c.Intent)
		t.Logf("plain=%-16s stripped=%-16s is_negated=%.2f want=%-16s | %s", p.Choice, s.Choice, n.Noul, c.Intent, c.Text)
	}
	logVsBaseline(t, "negation_stripped", plain, stripped)
}
