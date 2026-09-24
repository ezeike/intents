# intents

Design notes and reference implementation sketches for routing free-text user input to
application actions using small, fast "decision models" (Laya / Jev-class: `choice`, `score`,
`noul` typed questions answered in one forward pass) instead of an LLM tool-calling loop.

Core idea: if your action/query space is closed and enumerable ahead of time, you don't need a
generative model to pick among options at each decision point — a calibrated classifier is
sufficient, much cheaper, and much faster. This repo captures the architecture patterns worked out
for that problem, plus example Go code.

**Not covered here**: open-ended generation (drafting text, writing code) or genuinely novel
multi-step reasoning where the next step can't be anticipated in advance — that's still an LLM's
job. This approach only replaces the "pick the next tool from a known set" part of an agent loop.

## Contents

- [`docs/architecture.md`](docs/architecture.md) — the progression of designs: flat choice → typed
  question shapes → risk-tiered action registry → hierarchical classify tree → heterogeneous
  classify/lookup tree → recursive pattern → slot-filling DAG → hybrid classify-then-lookup
  resolver.
- [`docs/model-comparison.md`](docs/model-comparison.md) — Laya vs Jev: what they are, technical
  differences, when to use which.
- [`docs/context-budgets.md`](docs/context-budgets.md) — the two separate context limits (state
  window vs. head budget) and why they matter for option-set sizing.
- [`examples/`](examples/) — illustrative Go code for the action registry, tree traversal, DAG
  slot resolution, and hybrid classify/lookup fallback. **Sketches, not a built/tested library** —
  written to demonstrate shape, not to import as-is.

## Origin

Distilled from a design conversation exploring how to route commands in a no-code app-builder
(UI actions) and later extended to a blockchain-action taxonomy (DeFi/NFT/wallet/governance),
using [Laya](https://github.com/NandhaKishorM/laya) (open-weights, self-hosted, ModernBERT-large
backbone) as the decision model, benchmarked against
[Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev) (TypeSafe AI's hosted
System-1 model).
