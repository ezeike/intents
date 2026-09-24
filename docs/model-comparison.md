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
