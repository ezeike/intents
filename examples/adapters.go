package examples

// NoulConfidence derives a confidence value from a raw noul P(true), for models that don't
// return one directly. Laya's own noul confidence is exactly this formula
// (docs/model-comparison.md); CLM's /v1/systemone response omits confidence on noul answers
// entirely (returns only {"type":"noul","noul":p}), confirmed by a live request against both
// models on identical input — this restores an equivalent signal to threshold against.
//
// This does not make the two models agree on WHAT the probability should be for a given input —
// only Laya vs CLM's own live testing surfaced a real case where they diverged sharply on the
// same input (see docs/model-comparison.md). Thresholds tuned against one model's calibration
// still need re-validating against the other; this only restores a missing field.
func NoulConfidence(p float64) float64 {
	if p > 1-p {
		return p
	}
	return 1 - p
}
