// Package laya is an intents.Classifier adapter for Laya (convaiinnovations/laya), a
// non-autoregressive "System 1" decision model served over the Jev-compatible POST /v1/systemone
// HTTP protocol. It never generates text: given a state and a set of typed questions, it returns
// calibrated-probability answers in a single forward pass. See ~/.claude/skills/laya (this
// repo's living Laya findings doc) for the empirical basis of every non-obvious decision below.
//
// # What lives here
//
// [Client] speaks the raw wire protocol ([Request]/[Response], [Question] of type "choice",
// "score", or "noul") and is usable standalone for the cross-question batching pattern
// (docs/superpowers/specs/2026-09-24-intents-package-design.md decision 14) that a single
// [Classifier] deliberately can't express — see [Response.Choice], [Response.Score],
// [Response.Noul]. [Classifier] wraps one [Client] plus one fixed question definition
// (instructions/criteria/model) to implement intents.Classifier for a single classify node.
// [OrderedCriteria] preserves caller-declared option order onto the wire, working around
// encoding/json's alphabetical map-key ordering.
//
// # What this package deliberately does not know
//
// It has no opinion on option-set design (tree depth, cardinality, which options are noise) —
// that's the calling spec's job; see docs/architecture.md and docs/context-budgets.md. It never
// retries a request or implements request batching/coalescing across separate Classify calls;
// a caller wanting either supplies its own http.RoundTripper via Client.HTTPClient or drops to
// KindCustom with a shared Client, per the design spec's decision 14. It does not compute or
// cache a "recommended" MinConfidence/margin threshold for any given option set — the laya skill
// is explicit that this needs to be measured per option set, not assumed from this package's
// defaults.
//
// # Confidence vs. margin
//
// [Classifier.Classify] sets the returned intents.ChoiceAnswer.Confidence to the top-two
// probability margin (top1 - top2), not Laya's own "confidence" field. Measured on a real option
// set (laya skill, "confidence calibration"): raw confidence barely separates correct from
// incorrect answers (mean 0.431 vs 0.304, heavy overlap, one confident wrong answer at 0.501),
// while top-two margin separates them meaningfully better (0.444 vs 0.176) at no extra cost — the
// full distribution is already in the response. This directly feeds ClassifySpec.MinConfidence:
// gating on the model's own confidence field is not a reliable escalation signal for a
// classify-node-shaped option set, and this adapter does not reproduce that unreliability.
//
// # What's still genuinely not done
//
// No fine-tuning/training-data helpers (see the laya skill's "Fine-tuning" section) — this
// package only calls an already-served model. No automatic model-name routing beyond passing
// Classifier.Model through verbatim; auto-routing hazards (short-text language misdetection) are
// the caller's to avoid by pinning Model. No client-side enforcement of the server's documented
// hard limits (≤64 questions, state ≤50,000 chars, body ≤2MiB) — a request that exceeds them
// gets the server's own 413 back, wrapped as [ErrLimitExceeded].
package laya
