//go:build integration

// Domain-realistic classification: routing free-text SpecDraftChat messages
// (block-builder's spec-authoring surface) to one of the real Spec Authoring
// RPCs/UI actions, per docs/functionality-tree-block-builder-designer.md and
// the spec_action worked example from the design conversation.
//
// Scoped to three actions: accept_proposal, discard_proposal, propose_spec
// (create and send a new proposal -- the option previously named
// propose_change in the original 8-way design; renamed on reintroduction).
// direct_edit, validate, fetch_current, list_primitives, and general_chat
// remain cut from the whole test suite -- not needed for this node's actual
// production scope, and every one of them acted as noise or a genuine
// confusable competitor in the wider option sets tested earlier (see the
// laya skill's reduced-cardinality findings: a plain 2-way
// accept_proposal/discard_proposal classify, with nothing else in the same
// call, scored a perfect 20/20 on the hardest terse-referential-command
// cases that every other approach struggled with). propose_spec was cut
// alongside them for the same reason, then reinstated: production needs a
// third outcome distinct from accept/discard -- input that wants a brand
// new proposal drafted and sent, which must be forwarded to the LLM rather
// than resolved locally either way.
//
// This is a zero-shot baseline, not a contract: `typed-decisions` has never
// been fine-tuned on this domain, so these tests measure how far the stock
// checkpoint gets before any training happens, not a guarantee the wire
// protocol makes.
//
// Terse referential commands ("commit it", "scratch that") are handled via
// the batched referential-gate resolver defined in referential_gate_test.go
// (askBatched, resolveGate, referentialGateThreshold) -- see that file's
// header for why: four other approaches (bool-flag state, prose state,
// prose instructions, hierarchical decomposition) were tried first and all
// either did nothing or made things worse, all recorded in the laya skill.
// The referential gate got this case to 85%; reducing spec_action itself to
// this pure 2-way set (reduced_spec_action_test.go) later got the same
// cases to 100% with far less machinery -- both remain in the suite since
// the gate's batching pattern is independently useful (see
// out_of_scope_detection_test.go), even though it's no longer strictly
// required for accept/discard accuracy alone.
package laya_test

import "testing"

// specActionCriteria mirrors the spec_action classify node's actual scope:
// accept_proposal, discard_proposal, propose_spec. Originally an
// eight-option set (design conversation's block-builder worked example);
// direct_edit, validate, fetch_current, list_primitives, and general_chat
// were all cut -- see this file's header. propose_spec is the former
// propose_change, reinstated under its production name.
//
// An earlier version of this file tried lengthening these descriptions with
// explicit "trigger words" (fix 2 from the design conversation), back when
// this was still the eight-option set. That regressed two previously-passing
// cases (fetch_current, general_chat) while only partially helping the case
// it targeted -- reverted. The reliable fix for the case it was trying to
// solve is the batched referential gate in referential_gate_test.go: a
// narrow, separate noul judgment (batched into the same request, not a
// longer prompt) combined with real Go logic, not a conditional rule asked
// of the classifier itself.
func specActionCriteria() map[string]string {
	return map[string]string{
		"accept_proposal":  "accept/commit the most recently proposed spec",
		"discard_proposal": "throw away the currently staged proposal",
		"propose_spec":     "propose a new spec change, described in natural language, to be drafted by the assistant",
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

// ---- confident cases: unambiguous phrasing, hard equality assertion ----

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
