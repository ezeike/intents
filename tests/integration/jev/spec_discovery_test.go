//go:build integration

package jev_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ezeike/intents/jev"
)

// specDiscoveryRequestJSON is the exact POST /v1/systemone body from block-builder's
// spec-discovery design (docs/design-discussions/spec-discovery.md in block-builder): one batched
// call per user turn, judged against a token-vault plugin turn where the user answers the
// assistant's signers question and, in the same reply, adds an actor capability and introduces a
// transaction the spec doesn't know about yet ("Only the owner. Oh -- and admins should be able
// to freeze an account in an emergency."). Received via a cross-session request from the
// block-builder-cc session and run live against Jev on 2026-09-27; see that run's answers in each
// case below.
//
// The request's own "model" field ("jev-1.13.0") 400s through OpenRouter -- not a valid id on
// that gateway. specDiscoveryRequest strips it so the caller applies jevModel() instead, same as
// every other fixture in this package.
const specDiscoveryRequestJSON = `{
  "state": {
    "plugin": {
      "name": "token-vault",
      "purpose": ["Users deposit a fungible token into a vault and transfer balances to each other",
                  "The vault owner operates the vault; holders only move their own balances"]
    },
    "exchange": {
      "assistant_question": {
        "text": "Who should be allowed to call transfer — only the owner, any holder moving their own balance, or a multisig?",
        "target_node": "transactions[transfer].authorized_signers",
        "options": ["owner only", "any holder (own balance)", "multisig"]
      },
      "user_reply": "Only the owner. Oh — and admins should be able to freeze an account in an emergency."
    },
    "recent_turns": [
      { "role": "assistant", "text": "What fields does a transfer carry?" },
      { "role": "user", "text": "from, to, and an amount in base units" }
    ],
    "pending_proposal": { "exists": false },
    "known_transactions": ["deposit", "transfer"],
    "candidates": [
      { "path": "transactions[transfer].authorized_signers", "status": "open", "required": true,
        "describes": "Which account(s) must sign a transfer for it to be valid", "facts": [] },
      { "path": "transactions[transfer].validate", "status": "open", "required": true,
        "describes": "Conditions checked before a transfer executes (balances, limits, frozen accounts)",
        "facts": [] },
      { "path": "intent.actors", "status": "resolved", "required": true,
        "describes": "Who participates in the plugin and what each may do",
        "facts": ["owner: operates the vault", "holders: hold and move balances"] }
    ],
    "open_nodes": [
      { "path": "transactions[transfer].validate", "required": true },
      { "path": "transactions[transfer].effects", "required": true,
        "depends_on": ["state_objects[Balance]"] },
      { "path": "state_objects[Balance].key", "required": true },
      { "path": "transactions[deposit].authorized_signers", "required": true },
      { "path": "lifecycle.genesis", "required": false }
    ]
  },
  "questions": {
    "status_transfer_signers": {
      "type": "choice",
      "instructions": "The assistant asked ` + "`exchange.assistant_question.text`" + `. Judge only the user's reply ` + "`exchange.user_reply`" + `, together with the existing facts on the candidate ` + "`transactions[transfer].authorized_signers`" + `. How completely does it answer which accounts must sign a transfer?",
      "criteria": {
        "resolved": { "what": "The reply names who must sign clearly enough to write the signer rule into a spec with no further questions",
                      "examples": ["only the owner", "the sender, for their own balance"] },
        "partial":  { "what": "The reply says something about who signs but leaves a real ambiguity a spec author would have to ask about",
                      "examples": ["the owner, probably, or maybe some admins too"] },
        "not_addressed": { "what": "The reply does not say who signs a transfer" }
      }
    },
    "status_transfer_validate": {
      "type": "choice",
      "instructions": "Judge ` + "`exchange.user_reply`" + ` against the candidate ` + "`transactions[transfer].validate`" + `: the conditions checked before a transfer executes. How completely does the reply specify them?",
      "criteria": {
        "resolved": "The reply states the pre-execution checks for a transfer completely enough to write them into a spec",
        "partial": { "what": "The reply implies at least one check a transfer must pass but does not state the full set",
                     "examples": ["a frozen account should not be able to send"] },
        "not_addressed": "The reply implies no condition a transfer must pass"
      }
    },
    "actors_changed": {
      "type": "noul",
      "instructions": "Does ` + "`exchange.user_reply`" + ` add an actor, or a new capability for an actor, that is not already in the facts of ` + "`intent.actors`" + `?",
      "criteria": {
        "true": "A new role appears, or an existing role gains a power not already listed",
        "false": "Every role and power mentioned is already in the facts"
      }
    },
    "introduces_transaction": {
      "type": "noul",
      "instructions": "Does ` + "`exchange.user_reply`" + ` describe an on-chain action that is not one of ` + "`known_transactions`" + ` and would need its own transaction?",
      "criteria": {
        "true": "The user describes something a user or role does that changes chain state and is not deposit or transfer",
        "false": "Everything described is covered by an existing transaction, or is not a state-changing action"
      }
    },
    "next_node": {
      "type": "choice",
      "instructions": "Given ` + "`state`" + `, which open item should the assistant ask about next? Rules, in priority order: (1) the intent layer before the spec layer; (2) stay on the item currently being discussed rather than jumping; (3) a transaction's signers and fields before its validation and effects; (4) state objects a transaction depends on (` + "`depends_on`" + `) before that transaction's effects; (5) required before optional.",
      "criteria": {
        "transfer_validate": "The pre-execution checks for transfer (e.g. sufficient balance, account not frozen)",
        "transfer_effects": "What a transfer changes in state and which events it emits",
        "balance_key": "How the Balance state object is keyed (per account, per token, ...)",
        "deposit_signers": "Who must sign a deposit",
        "genesis": "Initial state at chain genesis (optional)",
        "offer_draft": "Nothing important remains to ask; the assistant should offer to draft the spec now",
        "other": "Something not listed here should be asked about next"
      }
    }
  }
}`

func specDiscoveryRequest(t *testing.T) jev.Request {
	t.Helper()
	var req jev.Request
	if err := json.Unmarshal([]byte(specDiscoveryRequestJSON), &req); err != nil {
		t.Fatalf("decode spec discovery request fixture: %v", err)
	}
	return req
}

// TestSpecDiscoveryTurn_TokenVaultFreezeCapability sends block-builder's fixture turn once, then
// checks every question's answer as its own subtest -- a table, not five separate test functions,
// because they share one live call and each row is a distinct judgment over the same response,
// not a distinct invariant needing its own setup (the shape reduced_spec_action_test.go and
// out_of_scope_detection_test.go already use for multi-question live responses).
//
// Three rows are hard-asserted: the live run landed confidently and unambiguously on the expected
// side for these. Two rows (status_transfer_validate, next_node) log instead of assert: the live
// run put them on a close, low-margin/low-confidence call that disagreed with block-builder's own
// expectation on one of them -- promote those to hard assertions only once a repeated run confirms
// which side is actually stable, per this package's grid-then-promote convention.
func TestSpecDiscoveryTurn_TokenVaultFreezeCapability(t *testing.T) {
	c := requireJev(t)
	req := specDiscoveryRequest(t)
	req.Model = jevModel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.SystemOne(ctx, req)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	cases := []struct {
		name   string
		assert func(t *testing.T)
	}{
		{
			name: "actors_changed is confidently true (admins gain a freeze capability)",
			assert: func(t *testing.T) {
				a, err := resp.Noul("actors_changed")
				if err != nil {
					t.Fatalf("actors_changed: %v", err)
				}
				if a.Noul < 0.5 {
					t.Errorf("actors_changed = %.2f, want >= 0.5", a.Noul)
				}
			},
		},
		{
			name: "introduces_transaction is confidently true (freeze needs its own transaction)",
			assert: func(t *testing.T) {
				a, err := resp.Noul("introduces_transaction")
				if err != nil {
					t.Fatalf("introduces_transaction: %v", err)
				}
				if a.Noul < 0.5 {
					t.Errorf("introduces_transaction = %.2f, want >= 0.5", a.Noul)
				}
			},
		},
		{
			name: "status_transfer_signers resolves (the reply directly answers the signer question)",
			assert: func(t *testing.T) {
				a, err := resp.Choice("status_transfer_signers")
				if err != nil {
					t.Fatalf("status_transfer_signers: %v", err)
				}
				if a.Choice != "resolved" {
					t.Errorf("status_transfer_signers = %q, want %q (probabilities: %v)", a.Choice, "resolved", a.Probabilities)
				}
			},
		},
		{
			name: "status_transfer_validate is a close, unresolved call -- logged, not asserted",
			assert: func(t *testing.T) {
				a, err := resp.Choice("status_transfer_validate")
				if err != nil {
					t.Fatalf("status_transfer_validate: %v", err)
				}
				// Live run: not_addressed 0.56 vs partial 0.44. block-builder expected "partial"
				// (the freeze mention implies a validate-time check) -- the model landed on the
				// other side of a genuinely close call.
				t.Logf("status_transfer_validate = %q (probabilities: %v, margin %.2f)",
					a.Choice, a.Probabilities, margin(a.Probabilities))
			},
		},
		{
			name: "next_node picks transfer_validate but with low confidence -- logged, not asserted",
			assert: func(t *testing.T) {
				a, err := resp.Choice("next_node")
				if err != nil {
					t.Fatalf("next_node: %v", err)
				}
				// Live run: transfer_validate 0.42 vs deposit_signers 0.28, server confidence
				// 0.32 -- too close to trust an argmax-only production gate without a confidence
				// floor or tie-break rule.
				t.Logf("next_node = %q (probabilities: %v, confidence %.2f, margin %.2f)",
					a.Choice, a.Probabilities, a.Confidence, margin(a.Probabilities))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.assert)
	}
}

// onlyQuestion returns a copy of req carrying just questions[qid] -- isolates one judgment from
// the rest of the batch so a follow-up experiment (a reworded reply, tightened criteria, a
// restricted option set) changes exactly one variable instead of re-running all five questions
// every time.
func onlyQuestion(req jev.Request, qid string) jev.Request {
	q, ok := req.Questions[qid]
	if !ok {
		panic("spec discovery fixture has no question " + qid)
	}
	req.Questions = map[string]jev.Question{qid: q}
	return req
}

// withUserReply overrides state.exchange.user_reply on a freshly decoded req (each call site gets
// its own req from specDiscoveryRequest(t), so mutating the decoded map here can't leak into
// another test).
func withUserReply(t *testing.T, req jev.Request, reply string) jev.Request {
	t.Helper()
	state, ok := req.State.(map[string]any)
	if !ok {
		t.Fatalf("state is %T, want map[string]any", req.State)
	}
	exchange, ok := state["exchange"].(map[string]any)
	if !ok {
		t.Fatalf("state.exchange is %T, want map[string]any", state["exchange"])
	}
	exchange["user_reply"] = reply
	return req
}

// withCriteria replaces qid's Criteria in place -- req must already carry that question (typically
// via onlyQuestion first).
func withCriteria(req jev.Request, qid string, criteria jev.OrderedCriteria) jev.Request {
	q := req.Questions[qid]
	q.Criteria = criteria
	req.Questions[qid] = q
	return req
}

// askChoice sends req (expected to carry exactly one question) and decodes that question's answer.
func askChoice(t *testing.T, c *jev.Client, req jev.Request) jev.ChoiceAnswer {
	t.Helper()
	if len(req.Questions) != 1 {
		t.Fatalf("askChoice: req has %d questions, want exactly 1", len(req.Questions))
	}
	req.Model = jevModel()
	resp, err := c.SystemOne(context.Background(), req)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	for qid := range req.Questions {
		a, err := resp.Choice(qid)
		if err != nil {
			t.Fatalf("choice %q: %v", qid, err)
		}
		return a
	}
	panic("unreachable")
}

// TestSpecDiscoveryValidate_RepeatedTrials_MeasuresStability answers the open question from the
// single-run result: is status_transfer_validate's near-even split (0.56/0.44, then 0.53/0.47 on
// a second run) a stable ambiguity, or noise that would swing wildly with more samples? Repeats
// the identical isolated question N times and logs every margin plus the aggregate choice count.
func TestSpecDiscoveryValidate_RepeatedTrials_MeasuresStability(t *testing.T) {
	c := requireJev(t)
	const trials = 8

	counts := map[string]int{}
	for i := 0; i < trials; i++ {
		req := onlyQuestion(specDiscoveryRequest(t), "status_transfer_validate")
		a := askChoice(t, c, req)
		counts[a.Choice]++
		t.Logf("trial %d: %s (probabilities: %v, margin %.2f)", i, a.Choice, a.Probabilities, margin(a.Probabilities))
	}
	t.Logf("across %d trials: %v", trials, counts)
}

// TestSpecDiscoveryValidate_ReplyIsolation checks whether status_transfer_validate's ambiguity
// comes from judging a compound message (signers answer + freeze aside) or is inherent to the
// freeze mention itself: if the freeze-only reply resolves cleanly to "partial" alone, the
// compound framing is the confound; if it's still ~even, the ambiguity is real regardless of what
// else is in the reply.
func TestSpecDiscoveryValidate_ReplyIsolation(t *testing.T) {
	c := requireJev(t)
	cases := []struct {
		name  string
		reply string
	}{
		{"compound reply, as sent (signers answer + freeze aside)",
			"Only the owner. Oh — and admins should be able to freeze an account in an emergency."},
		{"freeze mention alone, no signers answer",
			"Admins should be able to freeze an account in an emergency."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := onlyQuestion(withUserReply(t, specDiscoveryRequest(t), tc.reply), "status_transfer_validate")
			a := askChoice(t, c, req)
			t.Logf("%s (probabilities: %v, margin %.2f)", a.Choice, a.Probabilities, margin(a.Probabilities))
		})
	}
}

// originalValidateCriteria is status_transfer_validate's criteria exactly as block-builder sent
// it (flattened to plain strings -- OrderedCriteria.Description is a string, unlike the fixture's
// nested {"what":...,"examples":...} objects, so the "example" text is folded into the string).
func originalValidateCriteria() jev.OrderedCriteria {
	return jev.OrderedCriteria{
		{Key: "resolved", Description: "The reply states the pre-execution checks for a transfer completely enough to write them into a spec"},
		{Key: "partial", Description: `The reply implies at least one check a transfer must pass but does not state the full set (example: "a frozen account should not be able to send")`},
		{Key: "not_addressed", Description: "The reply implies no condition a transfer must pass"},
	}
}

// tightenedValidateCriteria adds the exact boundary case this fixture's live runs sit on directly
// into partial's description, per the jev-classifier skill's rule that boundary conditions belong
// in criteria wording, not left implicit -- a new actor capability (freezing) implying a
// transfer-time check counts as partial even when the reply is primarily about something else.
func tightenedValidateCriteria() jev.OrderedCriteria {
	return jev.OrderedCriteria{
		{Key: "resolved", Description: "The reply states the pre-execution checks for a transfer completely enough to write them into a spec"},
		{Key: "partial", Description: "The reply mentions a new actor capability or account condition (for example: an account can be frozen, paused, or restricted) that a transfer would need to check for at execution time, even if that capability is described as belonging to a different action -- this counts as partial, not not_addressed, because it implies a transfer-time condition that isn't yet fully specified"},
		{Key: "not_addressed", Description: "The reply implies no condition a transfer must pass, and does not describe any account state that could block a transfer"},
	}
}

// TestSpecDiscoveryValidate_CriteriaWording is the cheapest fix to try first per the jev-classifier
// skill: does spelling out the exact boundary case (a new capability belonging to a different
// action still implying a transfer-time check) move status_transfer_validate off its near-even
// split, without changing anything else about the request?
func TestSpecDiscoveryValidate_CriteriaWording(t *testing.T) {
	c := requireJev(t)
	cases := []struct {
		name     string
		criteria jev.OrderedCriteria
	}{
		{"original wording (as sent by block-builder)", originalValidateCriteria()},
		{"tightened wording (explicit freeze-implies-check boundary)", tightenedValidateCriteria()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := withCriteria(onlyQuestion(specDiscoveryRequest(t), "status_transfer_validate"), "status_transfer_validate", tc.criteria)
			a := askChoice(t, c, req)
			t.Logf("%s (probabilities: %v, margin %.2f)", a.Choice, a.Probabilities, margin(a.Probabilities))
		})
	}
}

// pairwiseNextNodeCriteria builds a 2-option next_node Choice from exactly the two contenders that
// stayed close across both original runs (transfer_validate ~0.4, deposit_signers ~0.3, in a
// 7-option field) -- isolates whether the ambiguity is inherent to that pair, or an artifact of
// splitting probability mass across 7 options while asking the model to apply 5 stacked priority
// rules at once.
func pairwiseNextNodeCriteria(order []string) jev.OrderedCriteria {
	descriptions := map[string]string{
		"transfer_validate": "The pre-execution checks for transfer (e.g. sufficient balance, account not frozen)",
		"deposit_signers":   "Who must sign a deposit",
	}
	c := make(jev.OrderedCriteria, len(order))
	for i, key := range order {
		c[i] = jev.Criterion{Key: key, Description: descriptions[key]}
	}
	return c
}

// TestSpecDiscoveryNextNode_PairwiseIsolation narrows next_node to just its two closest live
// contenders, keeping the original instructions (still describing all 5 priority rules) so the
// only change is the option set -- not a rewrite of what's being asked.
func TestSpecDiscoveryNextNode_PairwiseIsolation(t *testing.T) {
	c := requireJev(t)
	req := withCriteria(onlyQuestion(specDiscoveryRequest(t), "next_node"), "next_node",
		pairwiseNextNodeCriteria([]string{"transfer_validate", "deposit_signers"}))
	a := askChoice(t, c, req)
	t.Logf("transfer_validate vs deposit_signers, isolated: %s (probabilities: %v, margin %.2f)",
		a.Choice, a.Probabilities, margin(a.Probabilities))
}

// TestSpecDiscoveryNextNode_PositionBias reruns the same isolated pair with declaration order
// reversed. The chain-guideline priority tests found no position bias on a 2-option Choice; this
// is the first check of that claim on next_node specifically, whose full form has 7 options (the
// earlier check only ever covered 2).
func TestSpecDiscoveryNextNode_PositionBias(t *testing.T) {
	c := requireJev(t)

	forward := withCriteria(onlyQuestion(specDiscoveryRequest(t), "next_node"), "next_node",
		pairwiseNextNodeCriteria([]string{"transfer_validate", "deposit_signers"}))
	reversed := withCriteria(onlyQuestion(specDiscoveryRequest(t), "next_node"), "next_node",
		pairwiseNextNodeCriteria([]string{"deposit_signers", "transfer_validate"}))

	fa := askChoice(t, c, forward)
	ra := askChoice(t, c, reversed)
	t.Logf("forward  (transfer_validate, deposit_signers): %s (probabilities: %v)", fa.Choice, fa.Probabilities)
	t.Logf("reversed (deposit_signers, transfer_validate): %s (probabilities: %v)", ra.Choice, ra.Probabilities)
}
