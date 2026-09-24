# Context budgets

Two separate, easily-confused limits govern a Laya call.

## 1. State (input) context — the backbone's window

Laya's backbone is ModernBERT-large. Verified directly against the deployed checkpoints' actual
`config.json` (not just vendor claims) on our own inference host:

| Checkpoint    | `max_position_embeddings` | Layers | Hidden size |
|---------------|---------------------------|--------|-------------|
| English (default) | 8192 | 28 | 1024 |
| Multilingual  | 8192 | 22 | 768 |
| Typed-decisions | 8192 | 28 | 1024 |

This is the budget for `state.body` — the actual input text being classified. A web search result
during initial research claimed "laya-multilingual supports 1,024 context" — that figure does not
match the live config and should be treated as wrong (or referring to something else entirely,
e.g. a serving-layer default) rather than trusted. **Prefer checking the actual deployed config
over any secondary source when this number is load-bearing.**

For short commands/sentences (the primary use case here), this window is not a practical
constraint. It becomes relevant only if passing much longer state (full documents, multi-turn
conversation history) for more context-aware routing.

## 2. Head budget — `head_max_len`

Separate, much smaller pool: the space reserved for a question's **instructions + option labels +
marker tokens** (default 192 tokens, English). This is what actually degrades with large option
counts — not state length. The Banking77 intent-classification benchmark (77 fine-grained,
near-duplicate intents) is the documented stress case: enough options with enough label text blows
past this budget and accuracy collapses. A 10-option `choice` question, by contrast, is nowhere
near this ceiling at default settings.

## Practical implications

- Multi-question calls (e.g. 10 independent `noul`s vs. one 10-option `choice`) consume *head*
  budget per question — this, not state length, is the source of the ~1.5x latency gap measured
  between those two shapes (see `architecture.md` §2).
- If ever passing longer state, sanity-check total sequence length doesn't silently truncate —
  truncation behavior (which end gets cut, silent vs. explicit error) was not verified against
  Laya's actual tokenization code as of this writing; treat as an open question if it becomes
  load-bearing, rather than assuming either behavior.
- The head-budget ceiling, not the state window, is the real argument for going hierarchical
  (architecture.md §5) once an action taxonomy grows past ~15-20 options.

## Jev comparison

Jev's restricted-softmax/two-stage scoring reportedly supports up to 255 options natively without
the same shared-budget dilution — a genuine architectural difference from Laya for wide flat
option sets. See `model-comparison.md`.
