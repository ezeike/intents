//go:build integration

// Domain-realistic classification: routing free-text SpecDraftChat messages
// (block-builder's spec-authoring surface) to one of the eight real
// Spec Authoring RPCs/UI actions, per
// docs/functionality-tree-block-builder-designer.md and the spec_action
// worked example from the design conversation.
//
// This is a zero-shot baseline, not a contract: `typed-decisions` has never
// been fine-tuned on this domain, so these tests measure how far the stock
// checkpoint gets before any training happens, not a guarantee the wire
// protocol makes. The eight test cases below split into two groups:
//
//   - "confident" cases, where the phrasing is unambiguous enough that a
//     reasonable classifier should get the top choice right zero-shot --
//     asserted with a hard equality check.
//   - "ambiguous" cases ("ship it", "make it use segwit instead"), where our
//     own hand-labeled gold data is itself a split distribution (not 1.0) --
//     asserted only via TopTwo membership, mirroring the compound-intent
//     detection pattern from docs/architecture.md, since demanding a single
//     right answer here would be testing our own labeling confidence, not
//     the model.
//
// Terse referential commands ("commit it", "scratch that") are handled
// separately, via the batched referential-gate resolver defined in
// referential_gate_test.go (askBatched, resolveGate, referentialGateThreshold)
// -- see that file's header for why: four other approaches (bool-flag state,
// prose state, prose instructions, hierarchical decomposition) were tried
// first and all either did nothing or made things worse, all recorded in the
// laya skill. The referential gate is the one that actually worked (85% on
// the 20-case proposal grid at threshold 0.4, 100% precise whenever it
// fires), so it's the pattern this file's own accept/discard tests use below.
package laya_test

import "testing"

// specActionCriteria mirrors the eight-option spec_action classify node from
// the design conversation's block-builder worked example.
//
// An earlier version of this file tried lengthening these descriptions with
// explicit "trigger words" (fix 2 from the design conversation). That
// regressed two previously-passing cases (fetch_current, general_chat) while
// only partially helping the case it targeted -- reverted. The reliable fix
// for the case it was trying to solve is the batched referential gate in
// referential_gate_test.go: a narrow, separate noul judgment (batched into
// the same request, not a longer prompt) combined with real Go logic, not a
// conditional rule asked of the classifier itself.
func specActionCriteria() map[string]string {
	return map[string]string{
		"propose_change":   "propose a new change to the spec, in natural language",
		"accept_proposal":  "accept/commit the most recently proposed spec",
		"discard_proposal": "throw away the currently staged proposal",
		"direct_edit":      "replace the committed spec directly, bypassing the LLM conversation",
		"validate":         "validate a spec and get back its built IR",
		"fetch_current":    "fetch the plugin's current committed spec",
		"list_primitives":  "list the known-audited rule primitives available",
		"general_chat":     "not spec-related, general conversation about the plugin",
	}
}

// specActionState carries situational grounding (fix 1) that a bare input
// string can't: "commit it" / "scratch that" are referential -- they only
// resolve correctly if the model knows whether a proposal is actually staged.
//
// Context is prose, not a bool flag. Per the laya skill (~/.claude/skills/laya):
// every real state example in Laya's own docs uses natural-language-shaped
// keys (from/subject/body/message) mapped to prose -- never a bare boolean.
// An earlier version of this file used {"input": "...", "has_staged_proposal":
// true} and that measurably shifted answers on *unrelated* cases even when
// the flag was false -- plausibly because a raw bool key is out-of-distribution
// for what the model has actually seen. Context is omitempty so an untouched
// caller (askSpecAction) sends exactly the original bare {"input": "..."}
// shape, not an empty-string field pinned in place.
type specActionState struct {
	Context string `json:"context,omitempty"`
	Input   string `json:"input"`
}

// stagedProposalContext is the natural-language grounding used whenever a
// case needs the model to know a proposal is currently staged.
const stagedProposalContext = "There is currently a proposed change staged, awaiting the user's decision to accept or discard it."

func askSpecActionWithContext(t *testing.T, base, input, context string) choiceAnswer {
	t.Helper()
	resp := mustOK(t, base, systemOneRequest{
		State: specActionState{Input: input, Context: context},
		Questions: map[string]questionDef{
			"spec_action": {
				Type:         "choice",
				Instructions: "What does the user want to do with this plugin's spec?",
				Criteria:     specActionCriteria(),
			},
		},
		Model: "typed-decisions",
	})
	return decodeChoice(t, resp, "spec_action")
}

func askSpecAction(t *testing.T, base, input string) choiceAnswer {
	t.Helper()
	return askSpecActionWithContext(t, base, input, "")
}

// topTwo returns the two highest-probability keys, matching the compound-
// intent-detection shape from docs/architecture.md's ChoiceAnswer.TopTwo.
func topTwo(p map[string]float64) (first, second string) {
	var firstP, secondP float64
	for k, v := range p {
		switch {
		case v > firstP:
			second, secondP = first, firstP
			first, firstP = k, v
		case v > secondP:
			second, secondP = k, v
		}
	}
	return first, second
}

func assertTop2Contains(t *testing.T, a choiceAnswer, want string) {
	t.Helper()
	first, second := topTwo(a.Probabilities)
	if first != want && second != want {
		t.Errorf("top-2 = [%s, %s], want %q somewhere in there (full distribution: %+v)",
			first, second, want, a.Probabilities)
	}
	t.Logf("input top-2: [%s, %s], full distribution: %+v", first, second, a.Probabilities)
}

// ---- confident cases: unambiguous phrasing, hard equality assertion ----

func TestSpecAction_NewFeatureRequest_ProposeChange(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "add a staking transaction type with a minimum lockup of 30 days")
	if a.Choice != "propose_change" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "propose_change", a.Probabilities)
	}
}

func TestSpecAction_ExplicitCommit_AcceptProposal(t *testing.T) {
	base := requireLaya(t)
	a := askBatched(t, base, "yes that looks right, commit it")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire for an explicit staged-proposal response; it fell through to spec_action instead")
	}
	if choice != "accept_proposal" {
		t.Errorf("choice = %q, want %q", choice, "accept_proposal")
	}
}

// TestSpecAction_ScratchThatGoBack_DiscardProposal used to assert this
// directly against the 8-way classifier and failed zero-shot (picked
// direct_edit, then propose_change after fix 2's criteria rewrite). Replaced
// below by the referential-gate tests, which get this case right by
// construction rather than hoping the classifier learns the rule zero-shot.

func TestSpecAction_ScratchThatWithStagedProposal_DiscardProposalViaGate(t *testing.T) {
	base := requireLaya(t)
	a := askBatched(t, base, "no, scratch that, go back to what it was")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire; it fell through to spec_action instead")
	}
	if choice != "discard_proposal" {
		t.Errorf("choice = %q, want %q", choice, "discard_proposal")
	}
}

func TestSpecAction_NegativeWithoutStagedProposal_FallsThroughToClassifier(t *testing.T) {
	base := requireLaya(t)
	// hasStagedProposal is caller-known app state -- when false, the gate
	// must NEVER fire regardless of what is_referential scores, since
	// there's nothing staged to accept or discard.
	a := askBatched(t, base, "no, I don't think that's a good approach")
	_, viaGate := resolveGate(a, false, referentialGateThreshold)
	if viaGate {
		t.Error("gate fired with no staged proposal; should have fallen through to spec_action")
	}
}

func TestSpecAction_ConfirmedWithStagedProposal_ResolvesAccept(t *testing.T) {
	base := requireLaya(t)
	// A second, distinct affirmative phrasing from ExplicitCommit_AcceptProposal
	// above (not a duplicate) -- both must resolve to accept_proposal via the
	// gate, not just one.
	a := askBatched(t, base, "confirmed, lock it in")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire on affirmative staged-proposal input")
	}
	if choice != "accept_proposal" {
		t.Errorf("choice = %q, want %q", choice, "accept_proposal")
	}
}

func TestSpecAction_DirectlyDontAskAssistant_DirectEdit(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "just set the chain_id field to canopy-mainnet-1 directly, don't ask the assistant")
	if a.Choice != "direct_edit" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "direct_edit", a.Probabilities)
	}
}

func TestSpecAction_IsThisValid_Validate(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "is this spec actually valid right now?")
	if a.Choice != "validate" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "validate", a.Probabilities)
	}
}

func TestSpecAction_WhatDoesItLookLike_FetchCurrent(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "what does the spec look like right now")
	if a.Choice != "fetch_current" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "fetch_current", a.Probabilities)
	}
	// validate is the closest neighbor in this option set (see design conversation) --
	// log it even on a pass, since a near-tie here is a real signal this pair needs
	// contrastive fine-tuning data even if today's top-1 happens to be right.
	t.Logf("fetch_current vs validate margin: fetch_current=%v validate=%v",
		a.Probabilities["fetch_current"], a.Probabilities["validate"])
}

func TestSpecAction_WhichPrimitivesCanIUse_ListPrimitives(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "what transaction primitives am I allowed to use here")
	if a.Choice != "list_primitives" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "list_primitives", a.Probabilities)
	}
}

func TestSpecAction_OffTopicQuestion_GeneralChat(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "why would someone even want a bonding curve for a token launch")
	if a.Choice != "general_chat" {
		t.Errorf("choice = %q, want %q (distribution: %+v)", a.Choice, "general_chat", a.Probabilities)
	}
}

// ---- ambiguous cases: our own gold labels are split, so only check top-2 ----

func TestSpecAction_MakeItUseSegwitInstead_ProposeChangeInTop2(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "make it use segwit-style signatures instead")
	// gold: propose_change 0.85 / direct_edit 0.15 -- "make it X" doesn't say *how*,
	// so a reasonable classifier could split between "describe a change" and
	// "edit it for me directly." Both are defensible; propose_change should be
	// somewhere in the top two regardless of which one wins outright.
	assertTop2Contains(t, a, "propose_change")
}

func TestSpecAction_ShipIt_AcceptProposalInTop2(t *testing.T) {
	base := requireLaya(t)
	a := askSpecAction(t, base, "ship it")
	// gold: accept_proposal 0.8 / propose_change 0.2 -- genuinely ambiguous without
	// knowing whether a proposal is currently staged.
	assertTop2Contains(t, a, "accept_proposal")
}
