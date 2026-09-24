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
