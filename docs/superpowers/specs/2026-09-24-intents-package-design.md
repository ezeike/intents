# `intents` package design

> **Status: scratch / in progress.** This captures everything worked out in the design
> conversation so far. It is NOT frozen, has not been through spec self-review, and has open
> questions called out explicitly at the bottom. Do not hand this to `/forge init` yet.

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
	// classifiers, lookupers map[string]... — registered via:
}

func (e *Engine) RegisterClassifier(name string, c Classifier)
func (e *Engine) RegisterLookuper(name string, l Lookuper)

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

The "negative sentiment + a proposal is actually staged ⇒ discard_proposal" rule
(`resolveSpecAction` in the test file) is a hard rule computed in Go, not a conditional the
8-way classifier is asked to apply zero-shot — modeled as block-builder's `Builder[T]` inspecting
`state.Context.(SpecActionContext).HasStagedProposal` plus a narrow preceding `noul`
`is_negative` question, not as spec-tree structure. `intents` itself doesn't need to know this
rule exists; it's caller-side composition on top of `Resolve[T]`'s output, same as the
"General app action" registry dispatch above.

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

## Open questions (not yet decided — scratch doc)

- Exact `Engine` internal fields (`classifiers`/`lookupers` maps) and constructor shape
  (`NewEngine()` vs. zero-value-usable struct).
- Sentinel `Err*` variables for `errors.go` — not yet enumerated.
- Cycle detection in `slot_dag` nodes: validated at spec-construction time, or lazily at
  first `ResolveState` walk? Needs a decision before `errors.go` can be finalized.
- Concurrency limits for wave execution in `slot_dag` (unbounded goroutines per wave vs.
  a cap) — not discussed yet.
- Whether `ClassifySpec`/`SlotSpec` validation (e.g. `MinConfidence` in `[0,1]`, at least one
  `Option`) happens at spec-construction or first-use.
- ~~`doc.go` content~~ — written, see `/doc.go` at module root.
- Testing strategy for the engine itself — not discussed at all yet.
- Whether `ResumeState`'s dependency-graph re-entry needs to re-run downstream waves that depended
  on a slot being answered via `Answer.Raw` (i.e., does answering `token` via Resume need to
  re-open `amount`'s wave if `amount` had already failed for an unrelated reason) — not walked
  through.
- Full "how resolution works" narrative section (referenced above) — currently only exists as a
  worked trace in conversation, not written into this doc.
