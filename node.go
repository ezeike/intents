package intents

import "context"

// NodeKind selects which of Node's four resolution strategies applies.
type NodeKind string

const (
	KindClassify NodeKind = "classify"
	KindLookup   NodeKind = "lookup"
	KindSlotDAG  NodeKind = "slot_dag"
	KindCustom   NodeKind = "custom"
)

// Node is one vertex of the declarative spec tree. Exactly one of Classify, Lookup, Slots
// (non-empty), or Custom is set, matching Kind — Validate enforces this.
type Node struct {
	Key      string
	Kind     NodeKind
	Classify *ClassifySpec
	Lookup   *LookupSpec
	Slots    []SlotSpec // for Kind == KindSlotDAG
	Children map[string]*Node
	Custom   NodeFunc // for Kind == KindCustom
	RiskTier RiskTier
}

// ClassifySpec configures a KindClassify node: a closed-set choice over Options, resolved by
// the registered Classifier named Backend.
//
// For any node needing to batch more than one question into a single backend round trip
// (shared token budget, one request) — a real, measured need documented in
// docs/superpowers/specs/2026-09-24-intents-package-design.md decision 14 — do not widen this
// type or Classifier. Drop to a KindCustom node instead: its NodeFunc talks to the adapter
// directly, bypassing ClassifySpec/Classifier entirely.
// tests/integration/laya/spec_action_test.go's askBatched/resolveGate is the reference
// implementation of this pattern.
//
// BuildContext may be called concurrently from different goroutines during slot_dag wave
// execution (one call per slot in a wave), so it must be safe for concurrent use.
type ClassifySpec struct {
	Backend       string
	Options       map[string]string
	MinConfidence float64
	Fallback      *LookupSpec
	BuildContext  func(state State) (any, error)
}

// LookupSpec configures a KindLookup node: an entity-resolution query against the registered
// Lookuper named Backend. A LookupSpec reached only via ClassifySpec.Fallback never has its
// own BuildContext invoked — the fallback receives the classify node's grounding instead.
// BuildContext may be called concurrently from different goroutines during slot_dag wave
// execution, so it must be safe for concurrent use.
type LookupSpec struct {
	Backend      string
	Index        string
	BuildContext func(state State) (any, error)
}

// SlotSpec is one vertex of a slot_dag's dependency DAG, referenced by Name (adjacency-list
// representation, since a slot may have more than one incoming edge).
type SlotSpec struct {
	Name      string
	Kind      NodeKind // KindClassify | KindLookup | KindCustom
	Classify  *ClassifySpec
	Lookup    *LookupSpec
	Custom    NodeFunc
	DependsOn []string
	Required  bool
}

// NodeFunc is the KindCustom escape valve. It receives State (which already carries Input),
// not a bare string, so it can read Context and previously-resolved Values. A NodeFunc may be
// called concurrently from different goroutines during slot_dag wave execution, so it must be
// safe for concurrent use.
type NodeFunc func(ctx context.Context, state State) (Value, error)
