# Architecture

Progression of designs worked through, roughly in the order they came up.

## 1. Basic shapes: one Laya call, multiple typed questions

Laya answers `noul` (calibrated P(true)), `score` (ordinal rubric), and `choice` (pick one of N
options) questions, all in a single forward pass per call. Multiple questions can share one call:

```json
{
  "state": {"body": "We should add a way to export reports as PDF."},
  "questions": {
    "is_new_spec": {"type": "noul", "instructions": "..."},
    "urgency": {"type": "score", "instructions": "...", "criteria": ["not urgent", "soon", "critical"]}
  }
}
```

Batching questions into one call is cheaper than N separate calls, but cost scales with the
number of question *objects* and their instruction text length, not with total leaf count of an
underlying taxonomy — see `context-budgets.md`.

## 2. Independent `noul`s vs. a single `choice`

Two ways to represent "which of N things does this mean":

- **N independent `noul` questions** — each can independently be true or false. Correct when
  inputs can genuinely satisfy more than one category at once (compound intent).
- **One `choice` question with N options** — forces exactly one answer. Correct when categories
  are truly mutually exclusive, cheaper (one shared instruction vs. N repeated ones), and
  structurally prevents "both at once" from ever being represented — useful when that's actually
  the desired constraint.

**Decide by auditing real inputs for whether multi-intent is realistic before picking a shape.**
Don't assume exclusivity — check it.

Measured on our own deployment (p16): a 10-option `choice` averaged ~24.5ms; 10 independent
`noul`s averaged ~37.3ms (~1.5x slower) — the redundant per-question instruction text is the cost
driver, not raw option count.

## 3. Risk-tiered action registry

Map each resolved intent/action to a handler with its own confidence bar and execution target:

```go
type ActionHandler struct {
    Key           string
    Target        string  // "backend" | "frontend"
    MinConfidence float64
    Execute       func(ctx context.Context, params map[string]any) (Result, error)
    ClientAction  string  // for frontend-dispatched actions
}
```

**Never auto-execute an irreversible/high-stakes action from classifier confidence alone**
(financial transfers, medication changes, publishing). Route those to an explicit human
confirmation step regardless of confidence — the classifier pre-fills the confirmation, it doesn't
authorize the action. Safe/reversible actions can have low bars and execute directly.

## 4. Dual/compound intent via top-2 probability

Rather than only taking argmax, inspect the top-two `choice` probabilities. A close second place
(e.g. 61% / 29%) is a real signal of compound intent ("create an app *and* publish it once
ready") — treat it as a sequence of steps rather than discarding the signal.

```go
func routeAction(ans ChoiceAnswer) Decision {
    top, second := ans.TopTwo()
    switch {
    case top.Prob < registry[top.Key].MinConfidence:
        return Decision{Kind: "clarify", Options: []string{top.Key, second.Key}}
    case second.Prob > 0.25 && top.Key != second.Key:
        return Decision{Kind: "sequence", Steps: []string{top.Key, second.Key}}
    default:
        return Decision{Kind: "single", Action: top.Key}
    }
}
```

## 5. Hierarchical classify tree

A single flat `choice` with a large option count runs into a real constraint: Laya's `choice`/
`noul` questions share a fixed head token budget (~192-256 tokens default) across all options —
past roughly 20 options, each gets only a sliver of representation (this is the documented failure
mode on Banking77's 77-intent benchmark). Fix: split into stages instead of widening one list.

```
root [classify: domain]
├─ app_builder [classify: action]
│  ├─ create_app [execute]
│  ├─ publish_app [execute]
│  └─ define_data_model [classify: field_type]
└─ blockchain [classify: category]
   ├─ DeFi [classify: action] → swap_tokens | stake | lend | provide_liquidity
   ├─ NFT [classify: action] → mint_nft | transfer_nft | list_for_sale
   ├─ wallet_management [classify: action] → connect_wallet | check_balance | view_tx_history
   └─ governance [classify: action] → vote_proposal | delegate_votes
```

Each level stays small (~3-8 options). Latency scales with **tree depth**, not total leaf count —
a 3-level path costs ~3 classify calls regardless of how many leaves exist across the whole tree.
Risk tiers (from #3) should be inherited/compounded down a branch, not reset per node — a leaf
under a sensitive branch (`blockchain → DeFi → swap_tokens`) keeps or exceeds its ancestor's bar.

## 6. Heterogeneous node types: classify vs. lookup vs. execute

Not every branch point is a small fixed vocabulary. "Which specific device / account / instance"
is usually a large or dynamically-sized set — that's **entity resolution** (fuzzy search / index
lookup), not classification. Mixing them up is a common mistake: don't hand a classifier an
effectively unbounded, data-driven option set.

```
type Node struct {
    Key      string
    Type     string // "classify" | "lookup" | "execute"
    Options  map[string]string       // for classify
    Children map[string]*Node        // keyed by resolved option/lookup result
}
```

The tree's *shape* is pre-declared config (a struct literal, JSON/YAML, or DB rows) — Laya only
ever picks among children you already declared at a `classify` node; it never invents structure.

## 7. Recursive pattern instead of a hand-authored tree

When the same sequence (classify → lookup → classify → …) repeats at arbitrary depth, don't author
a bespoke node per level. Write one recursive function; the *pattern* is pre-declared, the actual
branches are generated at runtime from lookup results (e.g. resolving a specific cluster, then its
workloads, which weren't enumerable at config time).

```go
func Resolve(ctx context.Context, input string, scope Scope) (*Node, error) {
    choice := laya.Classify(ctx, input, scope.Options())
    if scope.IsTerminal(choice) {
        return &Node{Type: "execute", Key: choice}, nil
    }
    entity, err := fuzzySearch(ctx, input, scope.IndexFor(choice))
    if err != nil {
        return nil, err
    }
    return Resolve(ctx, input, scope.NarrowedBy(entity))
}
```

The only thing that must still be explicit is the termination condition (`scope.IsTerminal`) —
otherwise the recursion never knows when to stop.

## 8. Slot-filling DAG

A different shape for "for each input, resolve these N separate pieces of information" where the
pieces are largely independent named fields rather than a sequential descent:

```go
type SlotSpec struct {
    Name      string
    Resolver  string   // "classify" | "lookup"
    Options   map[string]string
    Index     string
    DependsOn []string // slot names that must resolve first, if any
}
```

- **Independent slots** (no `DependsOn`): resolve in one wave — batch same-input `classify` slots
  into a single Laya call (see #1), run `lookup` slots concurrently.
- **Dependent slots** (e.g. resolving which index to search depends on an earlier slot's answer):
  topologically order into waves; resolve each wave, then move to the next once its dependencies
  are satisfied.

## 9. Hybrid resolver: don't know ahead of time which mechanism applies

When a slot's answer might fall in a small known vocabulary *or* require open-ended search, and
you can't tell which in advance: classify first with an explicit `"other"` bucket, fall back to
lookup on low confidence or `"other"`. Mirrors the CLINC150 out-of-scope benchmark pattern.

```go
func ResolveHybrid(ctx context.Context, input string, spec HybridSlot) (Value, error) {
    ans := laya.Classify(ctx, input, spec.ClassifyOptions) // must include an explicit "other"
    if ans.Choice != "other" && ans.Confidence >= spec.ClassifyMinConfidence {
        return Value{Source: "laya", Result: ans.Choice, Confidence: ans.Confidence}, nil
    }
    entity, err := fuzzySearch(ctx, input, spec.LookupIndex)
    return Value{Source: "lookup", Result: entity}, err
}
```

Avoid "race both and compare scores" unless you've validated the two systems' confidence scales
are actually comparable — classify-then-fallback sidesteps that calibration problem entirely, and
only pays the lookup cost when classify actually punts.

## Autocomplete (a downstream application of the same pieces)

Split into two independent layers, not one:

- **Instant, no ML**: client-side fuzzy match against the static action registry (labels/aliases)
  for command-palette-style input. Zero latency, zero network calls.
- **Debounced live preview**: for free-text input, call Laya only after a pause in typing
  (~300-500ms debounce), gated by the same per-action `MinConfidence` as the registry, and only
  once the input ends on a word boundary — partial/mid-word fragments are out of distribution for
  a model trained on complete sentences and degrade unpredictably.

## Boundary of this whole approach

This substitutes for an LLM's tool-selection loop specifically when the action/query space is
closed-world and enumerable ahead of time. It breaks down for: novel intermediate goals an LLM can
synthesize but a pre-declared tree can't; open-ended generation (classify/score doesn't write
text); and genuinely unbounded step counts that can't be pre-designed into branch depth. The tell
for which regime you're in: is branching conceptual and small (classify), data-driven and
effectively unbounded (lookup), or genuinely unpredictable (still needs an LLM)?
