//go:build integration

// Hard-asserted invariants for chain_guideline_test.go's addresses_category classification,
// promoted after TestGuidelineCategoryGrid_AddressesCategory scored 38/38 with near-unanimous
// distributions (worst case 0.72/0.28, most cells exactly 1.0/0.0) -- same promotion pattern
// tests/integration/laya/spec_action_test.go used after its own proposal grid came back clean:
// the grid stays as the exploratory measurement instrument, and the cases it found unambiguous
// get named, individually-asserted tests here. One phrasing per category (the grid's first,
// unanimously-scored example for each), plus the "other" catch-all -- not the full 38-case set,
// since these are meant as a fast regression check, not a re-run of the grid.
package jev_test

import "testing"

func TestGuidelineCategory_LendingProtocolDescription_CoreFunctionality(t *testing.T) {
	c := requireJev(t)
	a := askGuideline(t, c, "", "it's a lending protocol where validators approve loan requests and delegators earn a cut of interest")
	if a.category.Choice != "core_functionality" {
		t.Errorf("Choice = %q, want %q", a.category.Choice, "core_functionality")
	}
}

func TestGuidelineCategory_ThroughputTarget_SuccessMetrics(t *testing.T) {
	c := requireJev(t)
	a := askGuideline(t, c, "", "we'll call it done when we can process 100 transactions per second")
	if a.category.Choice != "success_metrics" {
		t.Errorf("Choice = %q, want %q", a.category.Choice, "success_metrics")
	}
}

func TestGuidelineCategory_NoCrossChainBridgesInV1_ScopeBoundary(t *testing.T) {
	c := requireJev(t)
	a := askGuideline(t, c, "", "we're not doing cross-chain bridges in v1")
	if a.category.Choice != "scope_boundary" {
		t.Errorf("Choice = %q, want %q", a.category.Choice, "scope_boundary")
	}
}

func TestGuidelineCategory_AccountsTransactionsValidatorSets_GeneralEntities(t *testing.T) {
	c := requireJev(t)
	a := askGuideline(t, c, "", "the main objects are accounts, transactions, and validator sets")
	if a.category.Choice != "general_entities" {
		t.Errorf("Choice = %q, want %q", a.category.Choice, "general_entities")
	}
}

func TestGuidelineCategory_GreetingMessage_Other(t *testing.T) {
	c := requireJev(t)
	a := askGuideline(t, c, "", "hey, let's get started")
	if a.category.Choice != "other" {
		t.Errorf("Choice = %q, want %q", a.category.Choice, "other")
	}
}
