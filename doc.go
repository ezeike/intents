// Package intents routes free-text user input to typed, populated Go structs using small, fast
// decision models (Laya/Jev-class classifiers) and entity-resolution lookups, instead of an LLM
// tool-calling loop. It exists for closed-world, enumerable action/query spaces — see "Boundary"
// below for where that stops applying.
//
// The primary use case is collecting all required structured data from user input before an LLM
// (or any other downstream system) ever sees it: given free text and a declarative spec of what
// data is needed, [Resolve] returns a populated struct or an error describing exactly what's
// still missing and why.
//
// # What lives here
//
// A declarative node graph ([Node], [ClassifySpec], [LookupSpec], [SlotSpec]) describing how to
// resolve an intent — four resolution strategies ([KindClassify], [KindLookup], [KindSlotDAG],
// [KindCustom]), composable in one tree since each node dispatches independently on its own
// Kind. Two backend seams ([Classifier], [Lookuper]) that adapter subpackages implement. An
// [Engine] that walks a spec against real backends, producing a [State] — the resolved
// [Value]s, any [Failure]s encountered (always populated, not just on error), and enough
// structure ([State.Terminal], dot-path [Failure.Path]) for a caller to dispatch on the result
// or ask a targeted follow-up question. [Engine.ResumeState] continues a partially-resolved
// [State] from either an already-known answer or new free text, keyed by which field failed.
//
// # What this package deliberately does not know
//
// It never executes an action. [Resolve] returns a struct; what the caller does with it —
// call a function, flip UI state, kick off an automation — is entirely outside this package's
// concern. There is no action registry, no dispatch table, and no Execute step anywhere in the
// engine.
//
// It never enforces confirmation or authorization. [RiskTier] is metadata only: inherited down
// a branch, surfaced on [Value] and [State], and left for the caller to act on. The engine will
// hand back a fully-populated struct for a RiskCritical result exactly as readily as a RiskLow
// one — deciding whether that requires a human to confirm first is policy, and policy belongs at
// the call site, not in this package (the same split the sibling packages/go tree uses
// throughout: a package here verifies/transports/computes, it never decides admission).
//
// It has no opinion on any specific backend. [Classifier] and [Lookuper] are the only seams;
// this package has zero dependency on Laya, Jev, any vector store, or any SQL driver. Real
// implementations live in adapter subpackages (intents/laya, intents/jev, intents/sqllookup,
// and any number of vector-store adapters — vector support is generic via [Lookuper], not tied
// to one vendor) so that importing the core engine never pulls in a backend client you don't use.
//
// It does not reflect into caller types. Result binding is a plain caller-written function,
// [Builder], not struct tags or reflection — this package never inspects the shape of T in
// [Resolve].
//
// # Boundary
//
// This substitutes for an LLM's tool-selection loop specifically when the action/query space is
// closed-world and enumerable ahead of time — a classify node needs a bounded option list, a
// lookup node needs an indexable set, a slot_dag node needs its fields named in advance. It
// breaks down for open-ended generation, genuinely unbounded step counts that can't be
// pre-designed into branch depth, and novel intermediate goals an LLM can synthesize but a
// pre-declared spec can't. When resolution genuinely can't be pre-declared, the right tool is an
// LLM — deliberately not something this package provides a seam for (see "Rejected
// alternatives").
//
// # Relationship to adapter subpackages
//
// intents/laya and intents/jev implement [Classifier]. intents/sqllookup and any vector-store
// adapter (e.g. a Qdrant or Pinecone client) implement [Lookuper]. All of them live in the same
// Go module as this package but are never imported by it — the dependency arrow points one way,
// from adapter to core, never back. A caller wires concrete backends in with
// [Engine.RegisterClassifier] / [Engine.RegisterLookuper]; this package only ever sees them
// through the two interfaces.
//
// intents/laya's [Classifier] implementation must populate [ChoiceAnswer]/[Value].Confidence from
// Laya's `confidence` field, never `action.act_probability` — Laya's own README documents that
// field as not yet a usable signal (AUROC 0.30 against correctness, saturates near 1.0 on almost
// every input), where `confidence` reaches AUROC 0.77 on the same evaluation. Confirmed against
// the live server in tests/integration/laya; `act_probability` isn't decoded into any Go type in
// that suite for this reason.
//
// # Rejected alternatives
//
// An LLMFallback interface, letting the engine call an LLM when classify/lookup couldn't
// resolve a value. Cut because it reintroduces the nondeterminism this package exists to avoid,
// and because an LLM asked to fill a typed slot needs a schema anyway — at which point it's most
// of the way back to a tool-calling loop for that one field. A scoped variant of this (an LLM
// used internally by a [Lookuper] adapter to do a harder multi-step entity search) doesn't need
// a new interface — it's just another [Lookuper] implementation.
//
// A dedicated terminal/leaf node kind (KindExecute). Leaf-ness is already fully determined by
// Node.Children being empty; a separate kind for it was redundant. Replaced by [State.Terminal],
// which the engine records structurally the moment it stops descending, rather than a spec
// author declaring in advance where a branch ends.
//
// An engine-enforced hard block on high-[RiskTier] results, refusing to build T without an
// explicit confirmed flag. Rejected because this package never executes anything — the block
// would only gate whether a caller receives the struct via [Resolve], and is trivially bypassed
// by calling [Engine.ResolveState] directly and building T by hand. It would add a policy
// decision to the engine for a guarantee the engine can't actually provide.
//
// Reflection-based struct binding via field tags. Rejected in favor of a caller-written
// [Builder] function — simpler mental model, no tag-parsing edge cases, and the caller's struct
// shape is never a contract this package has to honor across versions.
//
// # Cross-question batching
//
// [Classifier] stays single-question by design (decision 14) — it is not widened to accept
// multiple questions in one backend call. A node that needs to batch more than one question
// into a single backend round trip (e.g. a shared token budget, one request answering several
// questions at once) uses [KindCustom] with a [NodeFunc] that talks to its adapter directly,
// bypassing [ClassifySpec]/[Classifier] entirely for that node.
// tests/integration/laya/spec_action_test.go's askBatched/resolveGate is the reference
// implementation of this pattern — see [ClassifySpec]'s doc comment for the same note colocated
// with the type it's most often confused for a use case of.
//
// # What's still genuinely not done
//
// Construction-time [Validate], unbounded slot_dag wave concurrency, and a full unit test suite
// (external intents_test package, no table-driven tests, hand-written fakes) are done — see
// [Validate], [Engine.ResolveState]'s "How resolution works" behavior, and this module's
// *_test.go files. What remains genuinely undone: a resumable *session* wrapper beyond the
// [Engine.ResolveState]/[Engine.ResumeState] split (multi-step wizards, not just single-slot
// clarification, are a real motivating case — see the design doc); JSON (un)marshaling helpers
// for [State] (its fields are all exported and serializable in principle, but [Failure.Err]
// (an error) and [State.Context] (any) need caller-side handling a future session wrapper would
// own — see [State]'s and [Failure]'s doc comments); the adapter subpackages themselves
// (intents/laya, intents/jev, intents/sqllookup, vector-store adapters); and [ClassifySpec.Fallback]'s
// [BuildContext] field, which is intentionally never invoked — per [walkTreeFrom]'s algorithm,
// the fallback lookup reuses the classify node's own grounding instead. See
// docs/superpowers/specs/2026-09-24-intents-package-design.md for full design history,
// including which use cases motivated each decision.
package intents
