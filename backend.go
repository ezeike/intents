package intents

import (
	"context"
	"sort"
)

// Classifier is a registered backend seam for KindClassify nodes. grounding is the value
// produced by ClassifySpec.BuildContext (or state.Context verbatim when BuildContext is nil);
// it is opaque here — the adapter owns marshaling it onto the wire.
type Classifier interface {
	Classify(ctx context.Context, input string, grounding any) (ChoiceAnswer, error)
}

// Lookuper is a registered backend seam for KindLookup nodes.
type Lookuper interface {
	Lookup(ctx context.Context, query string, index string, grounding any) (Entity, error)
}

// ChoiceAnswer is a Classifier's raw result: a single choice, its confidence, and the full
// option distribution.
type ChoiceAnswer struct {
	Choice       string
	Confidence   float64
	Distribution map[string]float64
}

// Scored is one entry of a ChoiceAnswer.TopTwo() result.
type Scored struct {
	Key  string
	Prob float64
}

// Entity is a Lookuper's result. Score is a similarity/match score, not a calibrated
// probability — never conflate it with ChoiceAnswer.Confidence.
type Entity struct {
	ID    string
	Label string
	Score float64
}

// TopTwo returns the two highest-probability entries of a.Distribution, sorted by Prob
// descending then Key ascending on ties. If Distribution has fewer than two entries, the
// missing slot(s) are the zero Scored{}.
func (a ChoiceAnswer) TopTwo() (top, second Scored) {
	scored := make([]Scored, 0, len(a.Distribution))
	for k, p := range a.Distribution {
		scored = append(scored, Scored{Key: k, Prob: p})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Prob != scored[j].Prob {
			return scored[i].Prob > scored[j].Prob
		}
		return scored[i].Key < scored[j].Key
	})
	if len(scored) > 0 {
		top = scored[0]
	}
	if len(scored) > 1 {
		second = scored[1]
	}
	return top, second
}
