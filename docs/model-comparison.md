# Laya vs Jev

Both are "System 1" decision models: non-autoregressive, answer typed questions (`choice`,
`score`, `noul`/boolean) in a single forward pass, no token-by-token generation. Neither writes
free text — they classify/score against a schema you define per call.

## Timeline

- **Jev** — launched by TypeSafe AI (Sept 15, 2026), out of stealth. Founder Diogo Almeida
  co-authored the original RLHF paper. Managed, hosted-only; no published weights or parameter
  count.
- **Laya** — technical writeup Sept 18, full public release (weights/code/docs) Sept 19, 2026, by
  ConvAI Innovations (Nandakishor Mukkunnoth) — 3-4 days after Jev, explicitly positioned as an
  open answer to it ("same idea, one closed and one open" per multiple sources). The underlying
  concept reportedly traces to a March 2025 paper (arXiv:2503.23303) predating Jev.

Both launches are recent as of this writing (within ~9 days) — treat third-party benchmark/review
claims about either as unverified until checked, the way `go-laya` (an in-progress third-party Go
port) turned out to have its actual model-execution layer (M6/M7) still unbuilt despite complete
scaffolding.

## Architecture

| | Laya | Jev |
|---|---|---|
| Backbone | ModernBERT-large, ~421M params (verified: 28 layers, hidden 1024, English/typed-decisions checkpoints) | Undisclosed |
| Deployment | Open weights (Apache 2.0), self-hosted | Managed hosted service only |
| Option-set handling | Shared token budget (~192-256) across all options in one question — degrades past ~20 options | Restricted-softmax two-stage scoring, natively handles up to 255 options |
| Training | RLCD-style (same acronym/lineage both use — likely shared, not a real difference) | RLCD ("Reinforcement Learning for Calibrated Decisions", TypeSafe's naming) |

## Measured latency

| | Latency |
|---|---|
| Laya (self-hosted, our own measurement) | ~21-38ms depending on question shape |
| Jev (TypeSafe published / independent benchmark) | 70-500ms end-to-end; ~127ms median in one third-party benchmark |

Caveat: Jev's numbers are against a hosted cloud endpoint; Laya's are self-hosted on local
hardware — part of the gap is plausibly network/hosting overhead, not purely compute. Jev's own
demo (reacting to game state ~10x/second, i.e. ~100ms/decision) suggests genuine per-decision
compute time is still meaningfully higher than Laya's, not just latency from the network hop.

## Accuracy / calibration trade-off

- Laya: higher raw top-1 accuracy on typed-decisions benchmark (0.766 vs Jev's 0.727)
- Jev: better out-of-the-box calibration (ECE 0.144 vs Laya's 0.213) and better soft accuracy —
  its full probability distribution more faithfully matches a reference distribution (0.580 vs
  0.471)

Practically: if only argmax matters, Laya's numbers are better. If downstream logic relies on the
probability distribution itself (confidence thresholds, top-2-probability compound-intent
detection — see `architecture.md` §4), Jev's better calibration is the more relevant number, even
though this repo's designs default to Laya for latency/self-hosting reasons.

## When to use which

**Laya** — latency-sensitive paths (debounced autocomplete preview), action taxonomies kept small
per node by design (the hierarchical tree in `architecture.md` §5 exists partly *because* of
Laya's option-budget limit), self-hosting required (privacy/compliance, e.g. blockchain wallet or
medical data), high call volume (near-zero marginal cost).

**Jev** — genuinely wide flat option sets (50-255) where restructuring into a tree isn't worth it,
calibration quality matters more than raw top-1 accuracy, no interest in owning GPU infra, low/
bursty volume where hosted pricing beats owning hardware.

For the architecture in this repo specifically: Laya fits nearly everywhere, since the designs
deliberately keep each `classify` node's option count small. Jev would only make more sense by
abandoning the tree/DAG shape in favor of one wide flat classifier, or to skip owning training/
infra entirely.

## CLM (Contrastive Language Models)

A third option, from Stanford's Hazy Research lab (Christopher Ré et al.), released Sept 23 2026 —
[github.com/Contrastive-LM/CLM](https://github.com/Contrastive-LM/CLM). Full research writeup at
`~/src/research/jev-models/clm-vs-laya/` (produced via the `research` skill). Architecturally
different from both Laya and Jev: a dual-encoder (frozen Qwen3-8B backbone + small trainable
projection heads per side) that embeds `state` and each candidate action separately and scores by
similarity, rather than a single fused classification head. This lets it cache action embeddings
and rank open-ended/dynamic candidate sets (best-of-N verification, large tool-routing sets) that
Laya's fixed-`choice` design structurally can't handle — at the cost of a heavier two-service
deployment (a vLLM pooling encoder plus `clm-serve`) and meaningfully more VRAM (Qwen3-8B needs
roughly 8-24GB depending on quantization, vs Laya's well-under-2GB footprint).

**Wire protocol is shape-compatible** with Laya/Jev's `/v1/systemone` — same endpoint, same
`choice`/`score`/`noul` request shapes — but is NOT a safe drop-in swap. Confirmed by actually
running CLM in place of Laya (same port, identical live requests) rather than just reading docs:

- **CLM's `noul` answers omit `confidence` entirely** — the response is bare `{"type":"noul","noul":p}`,
  not `{"type":"noul","noul":p,"confidence":c}` like Laya. Any code reading `answers.X.confidence`
  on a noul question (including this repo's `ActionHandler.MinConfidence` gating) gets nothing back
  from CLM, not a different number. Fix: derive it client-side —
  [`examples/adapters.go`](../examples/adapters.go)'s `NoulConfidence(p)` reproduces Laya's own
  exact formula (`max(p, 1-p)`), restoring a usable signal.
- **The two models can give meaningfully different judgments on identical input, not just
  differently-calibrated ones.** Same exact request through both: Laya scored a "how urgent is
  this?" question at 1.0857/3.0 with confidence 0.116 (spread across levels — leaning "soon" but
  uncertain); CLM scored the *identical* request at ~0.0000013/3.0 with confidence 0.9999990
  (near-total certainty it's "not urgent"). This is not a rounding or calibration-formula
  difference — one model reads the input as moderately urgent-and-uncertain, the other as
  confidently not urgent. `NoulConfidence` cannot fix this class of gap; any threshold tuned
  against one model's behavior needs independent re-validation against the other, not just a
  field-shape adapter.

**Practical guidance**: don't treat a Laya→CLM (or reverse) swap as a config change. Wire shape
being compatible is necessary but not sufficient — re-run real examples through both and diff the
actual answers before trusting a swap in anything with a confidence-gated action behind it.

### Test-run findings (2026-09-24, `tests/integration/laya/` pointed at CLM on p16:8000)

Ran the existing Laya integration suite against CLM directly (wire-compatible endpoint, per
above). The suite hardcodes `Model: "typed-decisions"` throughout — CLM rejects that name
(`unknown model 'typed-decisions'; available: ['clm-latest', 'clm-raw']`); results below are
against `clm-latest` after a scratch swap of the model field, not a permanent test-suite change.

- **Confidence gap reproduces exactly as described above**: `noul` answers from CLM omit
  `confidence` entirely — `TestNoul_Confidence_EqualsMaxOfNoulAndComplement` reads it as `0`
  instead of `max(noul, 1-noul)`.
- **CLM validates malformed requests more loosely than Laya.** Two cases where Laya's contract is
  "reject with 422" instead got a real `200` answer from CLM:
  - an invalid `criteria` key on a `noul` question — CLM answered anyway (`noul: 0.306`)
  - `labels` present on a `choice` question — CLM answered the `choice` question anyway instead
    of rejecting the malformed shape
  Neither is a wire-shape difference — same request bytes, different acceptance policy. Anything
  that relies on Laya's 422 as a client-side sanity check for malformed specs silently loses that
  guard against CLM.
- **spec_action accuracy is meaningfully worse on CLM, not just differently calibrated.**
  `TestSpecActionConfidence_CorrectVsIncorrectSeparation`'s 30-case accept/discard/propose_spec
  set: CLM scored **7/30 correct (~23%)**, mostly collapsing accept/discard phrasings into
  `propose_spec` — a different failure mode than the single-question calibration/urgency
  divergence documented above. `confidence` does not separate correct from incorrect answers on
  CLM in this domain either (mean 0.648 correct vs 0.603 incorrect, ~0 separation) — a
  `MinConfidence`-gated escalation policy tuned for Laya would not catch these on CLM.

Net: for `spec_action`-shaped zero-shot classification specifically, CLM is not a viable swap for
Laya as of this test — both the raw accuracy and the confidence-based escalation signal are
substantially worse, on top of the known wire/validation gaps above.

### Full-suite run (same session — every test in `tests/integration/laya/`, incl. the 3-question
### batched gate)

Ran the entire suite (40 pass / 22 fail) with the model field swapped to `clm-latest` throughout
(scratch edit, reverted — not committed). Failures cluster into real behavioral/contract
differences, not test flakiness:

**Contract differences (not accuracy — same request, different server behavior):**
- **HTTP status convention differs.** Laya returns `400` for a malformed-request-shape error
  (bad JSON, missing `questions` field, `questions` as a list not an object); CLM returns `422`
  for the same cases — `validation_test.go` fails wholesale on this alone, not on CLM answering
  incorrectly.
- **CLM enforces some schema rules Laya doesn't, and skips some Laya enforces** — not uniformly
  stricter or looser:
  - Stricter: rejects a single-level `score` question's criteria (Laya returns a valid `0` score
    for it); rejects `criteria` given as a plain list instead of an object (Laya accepts both
    forms); rejects an empty `questions: {}` map (Laya returns empty answers, zero usage).
  - Looser: accepts an invalid `criteria` key on a `noul` question and answers anyway; accepts
    `labels` present on a `choice` question and answers anyway (both reproduce the single-test
    finding above, confirmed again at full-suite scale); silently has **no resource limits** —
    `TestLimits_TooManyQuestions/StateTooLarge/BodyTooLarge` (Laya's `413`s) all got real `200`
    answers instead, including a 65-question batch and a 6KB body. A client relying on Laya's
    `413` as a guard rail loses it entirely against CLM.
- **No `routing` metadata in the response**, and none of Laya's model-name aliases resolve —
  `"typed-decisions"`, a published-HF-id form, and `"english"` are all `unknown model`; an
  unrecognized model name doesn't silently fall back to auto-routing the way Laya's does, it's a
  hard `422`. **`/health`'s response shape also differs** — no `status` field the Laya test reads.

**Accuracy — a consistent positive/"accept" bias across every construct tested, not just
`spec_action`:**

| Test (grid, CLM) | Result |
|---|---|
| `TestReducedSpecAction_ProposalGrid` (raw classifier, no gate) | 7/30 — accept_proposal 4/10, discard_proposal **0/10**, propose_spec 3/10 |
| `TestReferentialGate_ProposalGrid` (the 3-question batched gate — spec_action + is_referential + decision, one request) | 13/30 @ threshold 0.5 (11/30 @ 0.4) — accept_proposal **10/10**, discard_proposal **1/10**, propose_spec 2/10 |
| `TestProposalGrid_StateVsLiteralWording` (state-grounded, no gate) | accept_proposal 10/10 (both states), discard_proposal 6/10 / 2/10, propose_spec 1/10 / 1/10 |
| `TestGeneralAcceptReject_Grid` (plain accept/reject, unrelated to spec vocabulary) | 15/20 — accept **10/10**, reject 5/10 |
| `TestOutOfScopeDetection_*` | 7/33 |

The batching/gate mechanism itself works mechanically on CLM (the `decision` sub-question does
override `spec_action`'s raw pick when the referential gate fires — `gate fired on 21/30 cases`
at threshold 0.5) — but CLM's own `decision` answers are themselves biased toward `accept`, so
the gate's correction doesn't help: **discard_proposal accuracy stays ~1/10 with or without the
gate** (0/10 raw → 1/10 gated), a rounding difference, not the ~17/20 recovery Laya sees from the
same mechanism (see "Spec action" worked example above). This is the same "accept" skew visible
independently in `TestGeneralAcceptReject_Grid` (10/10 accept, 5/10 reject) — a general property
of this CLM checkpoint's zero-shot behavior on this task family, not an artifact of the
spec-vocabulary framing.

**Net, full-suite:** CLM is not a wire-compatible drop-in even ignoring accuracy — status-code
convention, resource limits, and model-name/routing metadata all differ in ways client code would
need to handle explicitly. On accuracy, CLM shows a strong, consistent positive/accept bias across
every independent test construct that measured it, making it specifically unsuitable for anything
that needs to reliably detect a *negative*/discard/reject signal, regardless of whether the
gating/batching mechanism designed for Laya is reused as-is.
