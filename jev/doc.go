// Package jev is an intents.Classifier adapter for Jev (TypeSafe AI's managed "System 1"
// decision model), served over the same POST /v1/systemone HTTP protocol Laya was built to be
// compatible with. Structurally this package is a near-mirror of the sibling intents/laya
// adapter — same wire shapes, same [Classifier] contract — with the differences the two models'
// deployment models and (as advertised, not yet independently measured) calibration actually
// warrant. See docs/model-comparison.md and ~/.claude/skills/laya for the comparison this
// package's decisions are based on.
//
// # What lives here
//
// [Client] speaks the wire protocol ([Request]/[Response], [Question] of type "choice", "score",
// or "noul") against a hosted Jev endpoint — unlike Laya, there is no self-hosted deployment, so
// [NewClient] requires an API key up front rather than treating auth as optional. [Classifier]
// wraps one [Client] plus a fixed question definition to implement intents.Classifier for a
// single classify node. [OrderedCriteria] preserves caller-declared option order on the wire, for
// the same reason as intents/laya's: encoding/json's map-key ordering, and because "does this
// model actually have zero position bias" isn't something this package assumes without measuring
// it, whatever the vendor's own docs claim.
//
// # What this package deliberately does not know
//
// Whether Jev's `confidence` field is actually well-calibrated for any specific option set.
// TypeSafe's own published benchmark reports meaningfully better calibration than Laya's stock
// checkpoint (ECE 0.144 vs 0.213) and better soft-distribution accuracy (0.580 vs 0.471) — but
// that is a vendor-published number on a different benchmark, not a measurement against this
// package's own live traffic. Until an integration suite exists here doing for Jev what
// tests/integration/laya/confidence_calibration_test.go did for Laya (raw confidence barely
// separated correct from incorrect answers there), [Classifier.Classify] makes the same
// conservative choice intents/laya makes: it returns the top-two probability margin as
// ChoiceAnswer.Confidence, not the server's own confidence field. This also keeps the two
// adapters' Confidence semantics identical, which matters if the point of building this package
// is to A/B a node's ClassifySpec.MinConfidence behavior across both backends on equal terms. The
// server's own confidence is still available uninterpreted via [Response.Choice] for anyone
// running that comparison.
//
// It has no opinion on whether Jev's wider native option-set handling (TypeSafe's restricted-
// softmax two-stage scoring, claimed to handle up to 255 options without Laya's ~20-option
// head-budget degradation) changes how a spec tree should be shaped for this backend — that
// remains the calling spec's decision, and is exactly the kind of vendor claim this package
// treats as unverified until checked against real requests (see docs/model-comparison.md's
// "treat third-party benchmark/review claims about either as unverified" note, written before
// either model had been in general use for even two weeks).
//
// # What's still genuinely not done
//
// No integration test suite against a live Jev endpoint exists yet (intents/laya has one under
// tests/integration/laya/ against a self-hosted server; nothing comparable exists for Jev here) —
// this package is untested against real Jev traffic. No retry/backoff for a hosted, rate-limited
// endpoint — a caller wanting either supplies its own http.RoundTripper via Client.HTTPClient. No
// cross-question batching beyond one Client.SystemOne call — same KindCustom escape hatch as
// intents/laya (design spec decision 14) applies here too.
package jev
