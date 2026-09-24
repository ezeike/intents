package examples

import "context"

// HybridSlot resolves a value that might fall in a small known vocabulary, or might require
// open-ended search — and the caller can't tell which in advance. See docs/architecture.md §9.
type HybridSlot struct {
	Name string

	// ClassifyOptions MUST include an explicit "other" bucket — that's the signal used to
	// fall through to lookup, mirroring the CLINC150 out-of-scope benchmark pattern rather
	// than racing classify and lookup and comparing scores (their confidence scales aren't
	// guaranteed comparable).
	ClassifyOptions map[string]string

	LookupIndex           string
	ClassifyMinConfidence float64
}

// HybridValue is the resolved result plus which mechanism actually answered.
type HybridValue struct {
	Source     string // "laya" | "lookup"
	Result     string
	Confidence float64
}

// ResolveHybrid tries classification first; only pays for the lookup when classification punts
// (low confidence, or the explicit "other" bucket won).
func ResolveHybrid(
	ctx context.Context,
	input string,
	spec HybridSlot,
	classify Classifier,
	search Searcher,
) (HybridValue, error) {
	choice, confidence, err := classify(ctx, input, spec.ClassifyOptions)
	if err != nil {
		return HybridValue{}, err
	}

	if choice != "other" && confidence >= spec.ClassifyMinConfidence {
		return HybridValue{Source: "laya", Result: choice, Confidence: confidence}, nil
	}

	entity, err := search(ctx, input, spec.LookupIndex)
	if err != nil {
		return HybridValue{}, err
	}
	return HybridValue{Source: "lookup", Result: entity}, nil
}
