//go:build integration

// Jev counterpart to tests/integration/laya/spec_action_test.go: domain-realistic classification,
// routing free-text SpecDraftChat messages (block-builder's spec-authoring surface) to one of
// accept_proposal, discard_proposal, propose_spec, per
// docs/functionality-tree-block-builder-designer.md. Terse referential commands ("commit it",
// "scratch that") are handled via the batched referential-gate resolver defined in
// referential_gate_test.go (askBatched, resolveGate, referentialGateThreshold) -- see that file's
// header for the failed approaches that preceded it.
//
// This is a zero-shot baseline, not a contract: Jev has never been fine-tuned on this domain, so
// these tests measure how far the stock checkpoint gets before any training happens, not a
// guarantee the wire protocol makes.
package jev_test

import "testing"

// ---- confident cases: unambiguous phrasing, hard equality assertion ----

func TestSpecAction_ExplicitCommit_AcceptProposal(t *testing.T) {
	c := requireJev(t)
	a := askBatched(t, c, "yes that looks right, commit it")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire for an explicit staged-proposal response; it fell through to spec_action instead")
	}
	if choice != "accept_proposal" {
		t.Errorf("choice = %q, want %q", choice, "accept_proposal")
	}
}

func TestSpecAction_ScratchThatWithStagedProposal_DiscardProposalViaGate(t *testing.T) {
	c := requireJev(t)
	a := askBatched(t, c, "no, scratch that, go back to what it was")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire; it fell through to spec_action instead")
	}
	if choice != "discard_proposal" {
		t.Errorf("choice = %q, want %q", choice, "discard_proposal")
	}
}

func TestSpecAction_NegativeWithoutStagedProposal_FallsThroughToClassifier(t *testing.T) {
	c := requireJev(t)
	// hasStagedProposal is caller-known app state -- when false, the gate must NEVER fire
	// regardless of what is_referential scores, since there's nothing staged to accept or discard.
	a := askBatched(t, c, "no, I don't think that's a good approach")
	_, viaGate := resolveGate(a, false, referentialGateThreshold)
	if viaGate {
		t.Error("gate fired with no staged proposal; should have fallen through to spec_action")
	}
}

func TestSpecAction_ConfirmedWithStagedProposal_ResolvesAccept(t *testing.T) {
	c := requireJev(t)
	// A second, distinct affirmative phrasing from ExplicitCommit_AcceptProposal above (not a
	// duplicate) -- both must resolve to accept_proposal via the gate, not just one.
	a := askBatched(t, c, "confirmed, lock it in")
	choice, viaGate := resolveGate(a, true, referentialGateThreshold)
	if !viaGate {
		t.Fatal("want the referential gate to fire on affirmative staged-proposal input")
	}
	if choice != "accept_proposal" {
		t.Errorf("choice = %q, want %q", choice, "accept_proposal")
	}
}
