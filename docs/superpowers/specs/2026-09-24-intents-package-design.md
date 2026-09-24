# `intents` package design

> **Status: design finalized, pending human review.** All open questions from the prior draft
> have been resolved (see "Resolved during design finalization" at the bottom, replacing the old
> "Open questions" section) and folded into the relevant sections above. Spec self-review has been
> run. Awaiting the human's read-through before handoff to `/forge init`.

## Purpose

Route free-text user input to application actions using small, fast decision models (Laya/
Jev-class classifiers) and entity-resolution lookups, instead of an LLM tool-calling loop — for
closed-world, enumerable action/query spaces. Primary use case: **collect all required
structured data from user input before an LLM (or any downstream system) sees it.**

The package classifies and extracts. It never executes actions and never decides policy — see
"Non-goals" below.

Module: `github.com/ezeike/intents` (single Go module, own git repo, not part of the
`machina/packages` shared Go module tree).

## Non-goals (explicitly out of scope)

- **No execution.** The engine never calls an action handler. `Resolve[T]` returns a populated
  struct; the caller decides what to do with it.
- **No confirmation/authorization enforcement.** `RiskTier` is advisory metadata only — no hard
  block in the engine. Policy stays at the call site (matches the "mechanism here, policy at the
  call site" convention already used elsewhere in this codebase's sibling `packages/go` tree).
- **No LLM fallback seam.** Considered and cut (see "Cut during design" below).
- **No backend implementations in the core module.** Laya/Jev/vector-store/SQL clients live in
  separate subpackages; the root package only defines interfaces.

## Core design decisions

1. **Spec shape: hybrid declarative with a custom escape valve.** Most resolution is expressed as
   declarative data (`Node` graph); a `custom` node kind lets a spec author drop to a plain Go
   function for shapes that don't fit the built-in kinds.
2. **Result binding: typed builder per intent, no reflection.** Caller writes their own
   `func(State) (T, error)`; the package never inspects struct tags or reflects into caller types.
3. **Backend scope: interfaces only, adapters in separate subpackages.** Core module has zero
   third-party deps beyond what the engine itself needs. `intents/laya`, `intents/jev`,
   `intents/sqllookup`, and any number of vector-store adapters (`intents/qdrant`, etc. — vector
   support is generic via `Lookuper`, not tied to any one vendor) each implement `Classifier` or
   `Lookuper` independently.
4. **Multi-turn state: one-shot now, resumable-ready.** `Resolve[T]` is a single call, no built-in
   session object — but the intermediate `State` is fully serializable and `ResolveState`/
   `ResumeState` are already split, so a session wrapper can be added later with no breaking
   change to the core engine.
5. **Failure reporting: `State.Failures` always populated**, not just on error — even a fully
   successful resolution can carry near-misses on optional fields.
6. **Risk tiers are advisory, not enforced.** Inherited/compounded down a branch; surfaced on
   `Value`/`State`; the caller decides what to do with them. No hard-block confirmation gate in
   the engine (considered, rejected — see below).
7. **Vector-store support is generic.** `Lookuper` has no Postgres/pgvector-specific anything;
   any vector database is just another `Lookuper` implementation in its own subpackage.
8. **Situational context is first-class, via `State.Context` + `ClassifySpec`/`LookupSpec.
   BuildContext`.** A bare `input string` plus static per-node config (`Options`, `Index`) covers
   most cases (see "Functional update" above — per-request scoping baked into the spec at
   construction time still works fine and needs no `Context` machinery). But some backend calls
   need typed, dynamic, often `omitempty`-sensitive grounding the caller can't know until resolve
   time and can't bake into the spec tree (e.g. "does this conversation currently have a staged
   proposal?" — see the "Spec action" worked example below). `State.Context` (opaque `any`, set
   once at `ResolveState`/`Resolve[T]`, unchanged by `Resume`) plus a per-node `BuildContext` hook
   solves this without widening `Classifier`/`Lookuper` to a fixed shape every backend must adapt
   to. Rejected alternative: `options map[string]string` — can't express typed non-string fields,
   and forces every caller to hand-roll zero-value omission (empirically load-bearing: a `false`
   boolean present-but-zero measurably shifted an unrelated classifier answer vs. the key being
   omitted entirely).
9. **`Engine` requires a constructor, not a zero-value struct.** `NewEngine()` returns a ready-to-use
   `*Engine` with its `classifiers`/`lookupers` maps allocated. A zero-value `Engine` would panic on
   the first `RegisterClassifier`/`RegisterLookuper` call unless every such method carried lazy-init
   nil-checking — standard Go convention for a struct with internal maps wins over marginal
   zero-value ergonomics.
10. **Spec validation — cycles and field checks together, at construction time.** One
    `Validate(spec *Node) error` walks the whole tree once and reports every structural problem: a
    `slot_dag` cycle in `DependsOn`, `ClassifySpec.MinConfidence` outside `[0,1]`, a `ClassifySpec`/
    `SlotSpec` with no `Options`/no slots, an unreachable `Children` entry, etc. Callers run it once
    after building a spec (or the engine runs it automatically the first time a given `*Node`
    pointer reaches `ResolveState`, caching the result keyed by pointer identity so repeat calls
    don't re-walk). A bad spec never reaches resolution — no separate "lazy, first-use" validation
    path to reason about alongside it.
11. **Wave execution in `slot_dag` is unbounded per wave.** Every slot in a topological level
    resolves concurrently with no engine-level cap (`errgroup.Group`, no semaphore). Waves are
    bounded by how many slots a spec author put in one DAG level — a spec-authoring decision, not
    request volume — and any real rate-limiting belongs to the backend adapter (Laya/Jev/sqllookup/
    vector-store client), matching "mechanism here, policy at the call site." No `MaxConcurrency`
    knob on `Engine`.
12. **`ResumeState` only re-resolves what `Answers` targets.** Answering a slot via `Answer.Raw`/
    `Answer.Value` re-runs exactly the nodes/slots named by `Answers` (keyed by `Failure.Path`) plus
    anything newly unblocked by those answers (e.g. a `slot_dag` slot whose `DependsOn` is now
    satisfied) — it never re-opens a wave that already produced a `Value` for an unrelated slot,
    even if that slot's "correct" answer could theoretically depend on the newly-answered one.
    Matches "`State` is a checkpoint, not a recompute": a slot that failed for an unrelated reason
    stays failed until the caller explicitly answers it too. No dependency-invalidation tracking.
13. **Testing strategy: same conventions as the sibling `packages/go` tree**, even though `intents`
    is its own module/repo — `intents_test` external package, hand-written fakes (`fakeClassifier`,
    `fakeLookuper`; no mocking framework), one behavior per
    `Test<Subject>_<Condition>_<Outcome>` function, no table-driven tests, negative invariants
    (cycle detected, low confidence, backend error, unresolved `DependsOn`, incomplete `Answers` on
    `ResumeState`) as first-class named tests, not rows in a shared table.
14. **`Classifier` stays single-question; cross-question batching is a documented `KindCustom`
    escape hatch, not a widened interface.** The spec_action worked example's measured-best
    resolution rule (`askBatched`/`resolveGate` — see below) needs one backend request answering
    three questions in a shared token budget; `Classifier.Classify(ctx, input, context)` asks only
    one. Rather than widen the core interface for a need demonstrated once so far (batching is
    inherently backend-specific — shared token budget, one round trip — so it doesn't generalize
    cleanly), `doc.go` and `ClassifySpec`'s doc comment formally bless dropping to `KindCustom` with
    a `NodeFunc` that talks to its adapter directly, bypassing `ClassifySpec`/`Classifier` entirely,
    as the supported pattern for any node needing cross-question batching.
    `spec_action_test.go`'s `askBatched`/`resolveGate` is the reference implementation of this
    pattern.

## Type definitions (current state)

```go
package intents

// ---- Spec (declarative, with a custom escape valve) ----

type NodeKind string

const (
	KindClassify NodeKind = "classify"
	KindLookup   NodeKind = "lookup"
	KindSlotDAG  NodeKind = "slot_dag" // real DAG scheduling (topological waves) once a slot has DependsOn edges
	KindCustom   NodeKind = "custom"
)

type Node struct {
	Key      string
	Kind     NodeKind
	Classify *ClassifySpec
	Lookup   *LookupSpec
	Slots    []SlotSpec        // for Kind == KindSlotDAG
	Children map[string]*Node  // empty/nil => this branch terminates here (no separate "terminal" kind)
	Custom   NodeFunc          // for Kind == KindCustom
	RiskTier RiskTier          // this node's own floor; effective tier = max(this, ancestors')
}

type ClassifySpec struct {
	Backend       string // registered Classifier name, e.g. "laya"
	Options       map[string]string
	MinConfidence float64
	Fallback      *LookupSpec // tried if Choice == "other" or below MinConfidence (reject-option pattern)

	// BuildContext shapes exactly what this node's backend call receives beyond the bare input
	// string — situational grounding pulled from state.Context (caller-supplied, set once at
	// resolve start) and/or state.Values (sibling/ancestor nodes already resolved earlier in a
	// tree walk). Colocated with Criteria/Options so a node's full backend contract lives in one
	// place. Nil means "pass state.Context straight through, unmodified" — most nodes don't need
	// grounding beyond the bare input and shouldn't be forced to write a no-op hook.
	//
	// Why this exists: a bare `options map[string]string` can't express a typed, omitempty-
	// sensitive field (e.g. a bool that must be OMITTED, not sent false, to avoid measurably
	// shifting an unrelated answer — see intents/tests/integration/laya/spec_action_test.go's
	// specActionState). Returning `any` lets BuildContext hand the adapter a real Go struct with
	// its own json tags instead of forcing every caller to hand-roll zero-value omission.
	BuildContext func(state State) (context any, err error)
}

type LookupSpec struct {
	Backend string // registered Lookuper name, e.g. "vectorstore", "sqllookup"
	Index   string // caller bakes in any per-request scoping (e.g. a doc ID) when constructing the spec

	// BuildContext: same purpose and default-nil behavior as ClassifySpec.BuildContext, for
	// lookups that need situational grounding beyond the bare query string (e.g. scoping to the
	// caller's current project/org).
	BuildContext func(state State) (context any, err error)
}

// SlotSpec is a DAG vertex. DependsOn is the edge list, referenced by Name (adjacency-list
// representation) — not by pointer, because a slot can have more than one incoming edge, which
// the tree-shaped Node.Children cannot express.
type SlotSpec struct {
	Name      string
	Kind      NodeKind // KindClassify | KindLookup | KindCustom
	Classify  *ClassifySpec
	Lookup    *LookupSpec
	Custom    NodeFunc // mirrors Node.Custom — for a slot needing bespoke logic (e.g. amount
	                    // parsing that depends on another slot's resolved Value)
	DependsOn []string
	Required  bool
}

// NodeFunc is the custom-node escape valve. Receives State (which already carries Input), not a
// separate raw string.
type NodeFunc func(ctx context.Context, state State) (Value, error)

// ---- Backend seams (implemented by adapter subpackages, exported so external packages can
// satisfy them and the engine's registration API can reference them) ----

// context is the value produced by ClassifySpec.BuildContext (or state.Context verbatim, when
// BuildContext is nil) — an opaque, backend-shaped payload. The adapter owns marshaling it onto
// the wire (e.g. the laya adapter JSON-marshals it directly into systemOneRequest.State).
type Classifier interface {
	Classify(ctx context.Context, input string, context any) (ChoiceAnswer, error)
}

type Lookuper interface {
	Lookup(ctx context.Context, query string, index string, context any) (Entity, error)
}

type ChoiceAnswer struct {
	Choice       string
	Confidence   float64
	Distribution map[string]float64 // full option -> probability; needed for TopTwo (compound-intent detection)
}

func (a ChoiceAnswer) TopTwo() (top, second Scored)

type Scored struct {
	Key  string
	Prob float64
}

type Entity struct {
	ID    string
	Label string
	Score float64 // similarity/match score — NOT a calibrated probability, don't conflate with Confidence
}

// ---- Result types ----

type RiskTier int

const (
	RiskLow RiskTier = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

type ValueKind string

const (
	ValueKindChoice ValueKind = "choice"
	ValueKindEntity ValueKind = "entity"
	ValueKindRaw    ValueKind = "raw"
)

type Value struct {
	Kind       ValueKind
	Choice     string   // populated when Kind == ValueKindChoice
	Entity     *Entity  // populated when Kind == ValueKindEntity
	Raw        string   // populated when Kind == ValueKindRaw
	Confidence float64
	RiskTier   RiskTier // effective, post-inheritance
	Source     string   // registered backend name that produced this
}

type FailureReason string

const (
	ReasonLowConfidence FailureReason = "low_confidence"
	ReasonNotFound      FailureReason = "not_found"
	ReasonBackendError  FailureReason = "backend_error"
	ReasonInvalidInput  FailureReason = "invalid_input" // custom node: input didn't match any expected shape
	ReasonUnresolved    FailureReason = "unresolved"     // DependsOn never satisfied, node unreached
)

type Failure struct {
	Path      string // dot path through the node tree, e.g. "transfer.amount"
	Reason    FailureReason
	Err       error
	Candidate *Value // best-effort near-miss, if any (e.g. top choice below MinConfidence)
}

type State struct {
	Input    string             // set once at the start of resolution; unchanged by Resume
	Context  any                // caller-supplied situational grounding; set once at the start of
	                              // resolution (same lifecycle as Input — unchanged by Resume). The
	                              // engine never inspects it; it exists purely for ClassifySpec/
	                              // LookupSpec.BuildContext (and custom NodeFunc, which already
	                              // receives the full State) to read from. E.g. block-builder sets
	                              // this to a struct carrying HasStagedProposal, current org/user,
	                              // conversation id — anything the engine has no way to derive
	                              // itself from Input alone.
	Values   map[string]Value   // every successfully resolved node/slot, keyed by Key/Name
	Terminal string             // Key of the leaf node reached — set the moment Children == nil
	Failures []Failure          // ALWAYS populated, required-or-not, even on overall success
	RiskTier RiskTier           // max across all resolved Values
}

// ---- Engine ----

type Engine struct {
	// classifiers, lookupers map[string]... — allocated by NewEngine, populated via Register*.
}

// NewEngine returns a ready-to-use *Engine. There is no zero-value-usable Engine — Register*
// requires its maps allocated.
func NewEngine() *Engine

func (e *Engine) RegisterClassifier(name string, c Classifier)
func (e *Engine) RegisterLookuper(name string, l Lookuper)

// Validate walks the whole spec tree once and reports every structural problem: a slot_dag
// dependency cycle, a ClassifySpec/SlotSpec with no Options/no slots, MinConfidence outside
// [0,1], etc. Call it once after building a spec — ResolveState also runs it automatically the
// first time a given *Node pointer is seen, caching the result by pointer identity, so a spec
// built once and reused across calls is validated once, not per-call.
func Validate(spec *Node) error

// callerContext seeds State.Context (see State's field doc) — pass nil when a spec's nodes need
// no situational grounding beyond the bare input.
func (e *Engine) ResolveState(ctx context.Context, spec *Node, input string, callerContext any) (State, error)

// Answer is either an already-known Value (caller resolved it themselves, e.g. user picked from
// a Candidate list — skip re-resolution entirely) or new free text (caller has no picker UI, just
// forwards the user's next message — re-run through the node's declared backend).
type Answer struct {
	Value *Value
	Raw   string
}
type Answers map[string]Answer // keyed by Failure.Path

// No separate callerContext param: prior.Context already carries it forward (Context is set once
// at ResolveState and unchanged thereafter, same as Input).
func (e *Engine) ResumeState(ctx context.Context, spec *Node, prior State, answers Answers) (State, error)

// ---- Caller-facing entrypoint ----

type Builder[T any] func(State) (T, error)

func Resolve[T any](ctx context.Context, e *Engine, spec *Node, input string, callerContext any, build Builder[T]) (T, error)

// ---- Errors ----

type IncompleteError struct {
	State State
}
func (e *IncompleteError) Error() string

// Sentinel errors, one block in errors.go, wrapped via fmt.Errorf("intents: ...: %w", ...) so
// callers use errors.Is. IncompleteError stays a distinct exported struct (not a sentinel) because
// it carries State — callers errors.As into it to recover the checkpoint for ResumeState.
var (
	ErrCyclicSlotDAG       = errors.New("intents: slot_dag contains a dependency cycle")
	ErrInvalidSpec         = errors.New("intents: spec failed validation")
	ErrUnregisteredBackend = errors.New("intents: no Classifier/Lookuper registered under this name")
	ErrUnansweredFailure   = errors.New("intents: resume answers do not cover all required failures")
)
```

## Node kinds — plain name, CS term, meaning

| Plain name | CS term | What it actually is |
|---|---|---|
| `classify` (flat, `Children` empty) | Closed-set multiclass classification | One decision over a fixed, enumerable label set. |
| `classify` (nested via `Children`) | Top-down hierarchical classification over a decision tree | Root-to-leaf traversal of a statically-materialized *n*-ary tree; each internal node is itself a closed-set classifier. |
| `lookup` | Entity resolution via nearest-neighbor / similarity search over an open set | Not classification — a query into an index (vector or relational) whose candidate set isn't enumerable ahead of time. |
| `slot_dag` | DAG scheduling with topological (level/wave) ordering | `DependsOn` edges define a DAG over `Slots`; engine computes topological levels ("waves"), resolves each level concurrently before advancing. Same structure as build-system dependency graphs (Make/Bazel/Terraform/npm) — those tools use the identical flat-list-plus-named-references representation and call it a DAG. Represented as an adjacency list (`[]SlotSpec` + `DependsOn` by name), not as nested `*Node` pointers, because a slot can have more than one incoming edge. |
| `custom` (`NodeFunc`) | User-defined callback / injected higher-order function | Arbitrary function supplied by the caller, invoked by the engine. |
| `ClassifySpec.Fallback` | Reject-option classification with fallback retrieval | Classifier given an explicit reject class (`"other"`); on reject or low confidence, control routes to a lookup instead of committing to a wrong label. |
| `ResumeState` | Resumable computation from a serialized checkpoint | `State` is a checkpoint; `ResumeState` is a continuation from it, not a recompute. |
| Recursive re-entry pattern | Lazy (on-demand) tree/graph expansion | A `lookup`/`custom` node's result determines the next subtree at runtime — the graph isn't materialized upfront, it's expanded incrementally during traversal. No new type; a composition of existing kinds. |

Note: a single `Node` tree can freely mix kinds — e.g. a `classify` descent whose leaf is a
`slot_dag` node. Each node dispatches on its own `Kind` independently; there is no
requirement that a whole spec be one uniform shape (architecture.md §6, "heterogeneous node
types").

## How resolution works

`ResolveState` walks `spec` starting at the root `Node`:

1. **Dispatch on `Node.Kind`.**
   - `classify`: call the registered `Classifier` (via `ClassifySpec.Backend`) with `state.Input`
     and the context produced by `ClassifySpec.BuildContext(state)` (or `state.Context` verbatim if
     `BuildContext` is nil). If the answer is below `MinConfidence` or lands on the reject option
     and `Fallback` is set, try the fallback `LookupSpec` instead. Record the result as a `Value` in
     `state.Values` keyed by `Node.Key`. If `Children[choice]` exists, descend into it and repeat
     step 1 from there; otherwise this node is a leaf — set `state.Terminal = Node.Key` and stop
     descending.
   - `lookup`: call the registered `Lookuper` (via `LookupSpec.Backend`) with `state.Input`,
     `LookupSpec.Index`, and the `BuildContext` result. Record the `Entity` as a `Value`. Same
     `Children`/`Terminal` handling as `classify`.
   - `slot_dag`: compute topological levels ("waves") over `Slots` from their `DependsOn` edges
     (already validated cycle-free by `Validate`). Resolve every slot in a wave concurrently
     (unbounded), advance wave-by-wave once a level completes. A slot resolves the same way a
     `classify`/`lookup`/`custom` node would, individually. Once every `Required` slot has a `Value`
     (or the DAG is exhausted with unresolved required slots left over), set
     `state.Terminal = Node.Key`.
   - `custom`: invoke `Node.Custom(ctx, state)` directly — the `NodeFunc` owns its own backend
     calls entirely (this is the mechanism used for cross-question-batched nodes like `spec_action`
     — see decision 14 above). Its returned `Value` is recorded the same way; `Children`/`Terminal`
     handling is unchanged.
2. **Every reached value lands in `state.Values`.** Every miss — low confidence, not-found,
   backend error, invalid custom input, or a `slot_dag` required slot whose `DependsOn` never got
   satisfied — becomes a `Failure` in `state.Failures` (always populated, even on an otherwise
   successful resolution, per decision 5), carrying `Reason` and a best-effort `Candidate` when one
   exists (e.g. the top choice that fell below `MinConfidence`).
3. **Resolution stops, it doesn't error, the moment a required node/slot can't be satisfied.**
   `ResolveState` returns the accumulated `State` wrapped in `*IncompleteError` so the caller can
   collect `Answers` for the unresolved `Failure.Path`s and call `ResumeState` — which re-resolves
   only what `Answers` targets plus anything newly unblocked by them (decision 12), never
   re-opening a wave that already produced an unrelated `Value`.

**Worked trace** ("General app action" example above): `"invite bob@company.com as an editor"` →
root `classify` picks `app_action` → descends into the `app_action` hierarchical classify tree →
picks `collaboration` → descends to `invite_collaborator`, a `slot_dag` leaf with two independent
slots (`invitee` via `sqllookup.Lookup`, `permission` via `classify`), both in wave 1 since neither
`DependsOn` the other → both resolve concurrently → `state.Terminal = "invite_collaborator"` →
`Resolve[T]`'s `Builder` reads `state.Values["invitee"]`/`state.Values["permission"]` into an
`AppAction{Key: "invite_collaborator", Params: ...}` → caller looks up
`registry["invite_collaborator"]` and dispatches.

## Package layout

```
intents/                     module github.com/ezeike/intents
├── go.mod
├── doc.go              # package doc: what lives here, deliberate omissions, relationship to adapters
├── node.go             # Node, NodeKind, ClassifySpec, LookupSpec, SlotSpec, NodeFunc
├── engine.go            # Engine, RegisterClassifier/RegisterLookuper, ResolveState, ResumeState, Resolve[T]
├── state.go             # State, Value, ValueKind, Failure, FailureReason, RiskTier
├── backend.go           # Classifier, Lookuper interfaces + ChoiceAnswer, Entity, Scored
├── errors.go            # IncompleteError, sentinel Err* vars
├── laya/                 # Classifier adapter
├── jev/                   # Classifier adapter
├── sqllookup/             # Lookuper adapter (SQL-backed)
├── qdrant/, pinecone/, etc. # Lookuper adapters, vector-store backed — none privileged/canonical
├── examples/              # already exists — sketches, not imported
└── docs/                  # design notes (this file included)
```

Single module, flat subpackages — matches the "flat by design, no `pkg/`/`cmd/`/`util/`
layering" convention already used in the sibling `packages/go` tree, even though `intents` is its
own separate module.

## Use cases worked through (product design app)

Context: a product design app where a user is looking at a specific spec/design doc already open
(scoped entry point — "which document" is known from context, not resolved by `intents`).

All three sit under one top-level classify:

```go
docIntentSpec := &Node{
	Key: "doc_intent", Kind: KindClassify,
	Classify: &ClassifySpec{Backend: "laya", Options: map[string]string{
		"new_proposal":      "user wants a new or substantially revised specification proposed",
		"functional_update": "user is describing a product behavior change the design doc should reflect",
		"app_action":        "a general application command (rename, invite, export, etc.)",
	}},
	Children: map[string]*Node{
		"new_proposal":      newProposalSpec,
		"functional_update": functionalUpdateSpec(docID),
		"app_action":        appActionSpec(),
	},
}
```

### 1. New proposal

```go
type NewProposalRequest struct {
	Description string // state.Input, verbatim
	Category    string // classified: new_feature | redesign | deprecation
}
```

`newProposalSpec` is a `slot_dag` node with one required `category` slot (classify).
Caller uses the result to trigger an automated action (e.g. kick off drafting) — `intents`
doesn't know or care what that action is (confirmed: the package's contract stops at "classify +
extract," not "decide what happens next").

### 2. Functional update

```go
type FunctionalUpdateRequest struct {
	Description string // state.Input, verbatim
	Section     string // resolved via lookup against this doc's section index
}
```

`functionalUpdateSpec(docID)` is built per-request so the vector-store lookup's `Index` can be
scoped to the currently-open document (`"doc_sections:" + docID`) — no engine-level context
machinery needed, the caller just bakes the scoping into the spec before calling `Resolve`.

Worked example: `"the checkout flow needs to support saved payment methods now"` →
`vectorstore.Lookup` embeds the input, nearest-neighbor search over this doc's section
embeddings, returns `Entity{Label: "Payment Processing", Score: 0.84}`. Caller opens the doc
editor scrolled to that section, pre-filled with the raw description.

### 3. General app action

```go
type AppAction struct {
	Key    string                   // state.Terminal — the leaf action to dispatch on
	Params map[string]Value         // whatever slots that leaf resolved
}
```

Hierarchical `classify` tree grouped by domain (`document`, `collaboration`, ...), with
`slot_dag` leaves for actions needing arguments. Worked example: `"invite
bob@company.com as an editor"` → `app_action → collaboration → invite_collaborator` (dependency
graph: `invitee` via `sqllookup` lookup, `permission` via classify, both wave 1, no edges between
them). `State.Terminal = "invite_collaborator"`. Caller looks up `registry["invite_collaborator"]`
(a plain `map[string]Handler` maintained entirely in caller code, not in `intents`) and calls it
with `Params`.

Full sequence trace (classify → descend → slot_dag wave resolution → Terminal set →
builder called → caller dispatch) is captured in the design conversation; worth lifting into a
proper "how resolution works" doc section before this spec is frozen.

### 4. Spec action (block-builder integration, `Context`/`BuildContext` motivating case)

Routes free-text `SpecDraftChat` input to one of block-builder's eight spec-authoring
RPCs/UI actions (`docs/functionality-tree-block-builder-designer.md`). Zero-shot baseline
prototyped directly against Laya in `intents/tests/integration/laya/spec_action_test.go`
(`specActionCriteria`, `specActionState`) before this `Context`/`BuildContext` mechanism existed
in the core package — that test file is the reference behavior this Node/Builder must reproduce.

```go
type SpecActionContext struct {
	HasStagedProposal bool // omit, don't send false — see decision 8 above
}

specActionSpec := &Node{
	Key: "spec_action", Kind: KindClassify,
	Classify: &ClassifySpec{
		Backend: "laya",
		Options: map[string]string{
			"propose_change":   "propose a new change to the spec, in natural language",
			"accept_proposal":  "accept/commit the most recently proposed spec",
			"discard_proposal": "throw away the currently staged proposal",
			"direct_edit":      "replace the committed spec directly, bypassing the LLM conversation",
			"validate":         "validate a spec and get back its built IR",
			"fetch_current":    "fetch the plugin's current committed spec",
			"list_primitives":  "list the known-audited rule primitives available",
			"general_chat":     "not spec-related, general conversation about the plugin",
		},
		BuildContext: func(state State) (any, error) {
			ctx, _ := state.Context.(SpecActionContext) // zero value if caller passed nil
			return ctx, nil
		},
	},
}
```

**The accept/discard resolution rule, updated from an earlier design.** An initial attempt
(`resolveSpecAction`/`is_negative` in the test file's history) used a single preceding `noul`
("is this negative?") to catch only the discard side, as a hard Go rule gated on
`HasStagedProposal` — never asked of the 8-way classifier as a conditional it had to apply
zero-shot. That version worked but only ever caught `discard_proposal`; live measurement against
five alternative approaches (bool-flag state, prose state, prose instructions, hierarchical
decomposition, and a batched referential gate — see `~/.claude/skills/laya/SKILL.md` for the full
comparison with real accuracy numbers) found a strictly better version, now what
`spec_action_test.go` actually implements (`askBatched`/`resolveGate`):

- Batch **three** questions into **one** backend request: `spec_action` (the 8-way choice above),
  `is_referential` (a `noul` — "does this refer back to something already shown to the user?"),
  and `decision` (a plain `accept`/`reject` `choice`, unrelated to spec vocabulary).
- Combine in Go: if `HasStagedProposal` (caller-known app state, never asked of the model) **and**
  `is_referential > 0.4`, trust `decision` (mapped `accept`→`accept_proposal`,
  `reject`→`discard_proposal`) over `spec_action`'s raw pick; otherwise trust `spec_action`.

Measured on the same 20-phrasing accept/discard test set: **85% (17/20)**, up from 45-60% for
every state/instructions-grounding variant tried and 20% for hierarchical decomposition — and,
unlike the earlier `is_negative`-only rule, it catches both the accept and discard sides, not just
discard. Still caller-side composition, not spec-tree structure — `intents` itself doesn't need to
know this rule exists, same as the "General app action" registry dispatch above.

**Open design gap this exposes**: the win here specifically depends on batching all three
questions into *one* backend request (shared token budget, one round trip) — a sequential version
(ask `is_negative` as a separate call, as the earlier rule did) measurably underperformed. The
`Classifier` interface as currently drafted (`Classify(ctx, input, context) (ChoiceAnswer, error)`)
only asks one question per call and has no batching shape. Reproducing this node's actual measured
behavior through the generic interface isn't possible yet — for now this has to be implemented as
a `KindCustom` node whose `NodeFunc` makes a raw, adapter-specific batched call directly, bypassing
`ClassifySpec`/`Classifier` entirely for this one node. Worth resolving before freeze: either widen
`Classifier` to accept multiple questions in one call, or formally bless "drop to `KindCustom` and
talk to the adapter directly" as the supported pattern for any node that needs cross-question
batching.

## Cut during design (with reasoning, so it isn't re-proposed later)

- **`LLMFallback` interface.** Considered three jobs it could do: (A) LLM resolves the value
  itself — rejected, reintroduces the nondeterminism this package exists to avoid, and would need
  structured output from the LLM anyway (circular). (B) LLM generates a clarifying question, not
  a value — legitimate but thin enough to not need a dedicated interface; folds into
  caller-side handling of `IncompleteError`/`State.Failures`. (C) LLM used for scoped agentic
  lookup on a hard entity-resolution case — not a new seam at all, just another `Lookuper`
  implementation (e.g. `intents/llmsearch`). Net: no dedicated `LLMFallback` type.
- **`KindExecute` / an explicit terminal-node marker.** Leaf-ness is fully determined by
  `Children == nil` — a dedicated kind was redundant. Replaced by `State.Terminal`, which the
  engine sets structurally when it stops descending, rather than a spec author declaring "this is
  where it ends."
- **Engine-enforced hard block on high-risk results.** Considered making `Resolve[T]` refuse to
  build `T` for `RiskTier >= RiskHigh` without an explicit `confirmed` flag. Rejected: since the
  engine never executes anything, the "block" would only prevent the caller from getting the
  struct via `Resolve[T]` — trivially bypassable by calling `ResolveState` directly. Would add
  policy into the engine for a guarantee it can't actually provide. `RiskTier` stays advisory,
  surfaced on `State`/`Value`, enforcement entirely at the call site.
- **Postgres/pgvector-specific naming.** `Lookuper` was always backend-agnostic; corrected the
  package layout to not privilege one vector-store vendor as "the" adapter.

## Resolved during design finalization (2026-09-24)

All items previously listed here as open are now decided and folded into "Core design decisions"
(9-14) and their corresponding sections above:

- ~~Exact `Engine` internal fields and constructor shape~~ — `NewEngine()`, decision 9.
- ~~Sentinel `Err*` variables for `errors.go`~~ — enumerated, see the type definitions section.
- ~~Cycle detection timing~~ — construction-time, decision 10.
- ~~Concurrency limits for wave execution~~ — unbounded per wave, decision 11.
- ~~`ClassifySpec`/`SlotSpec` validation timing~~ — construction-time, folded into decision 10's
  single `Validate` pass.
- ~~`doc.go` content~~ — written, see `/doc.go` at module root.
- ~~Testing strategy for the engine itself~~ — same conventions as `packages/go`, decision 13.
- ~~`ResumeState`'s dependency-graph re-entry scope~~ — targets `Answers` only, no cascade,
  decision 12.
- ~~Full "how resolution works" narrative section~~ — written, see that section above.
- ~~`Classifier` batching gap for `spec_action`~~ — `KindCustom` blessed as the supported escape
  hatch, decision 14.

No open questions remain. Spec self-review (below) has been run against this revision.

## Spec self-review

- **Placeholder scan:** no `TBD`/`TODO` remain; every section above has concrete content.
- **Internal consistency:** `Engine.RegisterClassifier`/`RegisterLookuper` now presuppose
  `NewEngine()`-allocated maps (decision 9) — consistent with the `Engine` struct's own comment.
  `Validate`'s construction-time timing (decision 10) is consistent with `ResolveState`'s "How
  resolution works" step 1, which assumes cycle-freedom before computing topological waves.
  `ResumeState`'s signature and decision 12's no-cascade scope agree.
- **Scope check:** still one cohesive package (engine + spec types + backend seams) with adapters
  correctly pushed to subpackages; no drift into adapter-specific concerns in the core module's
  type definitions.
- **Ambiguity check:** `Validate`'s caching behavior ("first time a given `*Node` pointer is seen")
  is now explicit rather than implied, resolving a possible double-validation-cost ambiguity.
  `KindCustom` as the batching escape hatch is now documented as a *formally supported* pattern,
  not just a workaround the spec_action example happened to use.
