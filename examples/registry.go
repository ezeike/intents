// Package examples contains illustrative, non-production sketches of the patterns described in
// ../docs/architecture.md. They demonstrate shape and are not wired to a real Laya client, a real
// search index, or a real action implementation — see docs for the reasoning behind each piece.
package examples

import "context"

// Result is the outcome of executing a backend-side action.
type Result struct {
	Message string
	Data    any
}

// Params are the resolved arguments for an action, however they were extracted.
type Params map[string]any

// ActionHandler is one leaf of the action registry: what to run, where, and how confident the
// classifier must be before running it. See docs/architecture.md §3.
type ActionHandler struct {
	Key    string
	Target string // "backend" | "frontend"

	// MinConfidence gates execution. Set higher for irreversible/high-stakes actions
	// (publish, financial transfer, medication change) — never auto-execute those on
	// confidence alone; route to an explicit human confirmation step instead.
	MinConfidence float64

	// Execute runs a backend-side action. Nil for frontend-dispatched actions.
	Execute func(ctx context.Context, p Params) (Result, error)

	// ClientAction names the frontend-local handler to invoke for frontend-side actions.
	// The frontend never sees the classifier's raw action key vocabulary — only this name.
	ClientAction string
}

// ChoiceAnswer is the subset of a Laya choice-question response this package cares about.
type ChoiceAnswer struct {
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}

// TopTwo returns the two highest-probability options, used to detect compound/sequential intent
// (docs/architecture.md §4) rather than relying on argmax alone.
func (a ChoiceAnswer) TopTwo() (top, second struct {
	Key  string
	Prob float64
}) {
	for k, p := range a.Probabilities {
		if p > top.Prob {
			second = top
			top = struct {
				Key  string
				Prob float64
			}{k, p}
		} else if p > second.Prob {
			second = struct {
				Key  string
				Prob float64
			}{k, p}
		}
	}
	return top, second
}

// Decision is what to do with a classified answer.
type Decision struct {
	Kind    string // "single" | "sequence" | "clarify"
	Action  string
	Steps   []string
	Options []string
}

// RouteAction applies the risk-tiered confidence gate and compound-intent detection described in
// docs/architecture.md §3-4.
func RouteAction(ans ChoiceAnswer, registry map[string]ActionHandler) Decision {
	top, second := ans.TopTwo()

	if top.Prob < registry[top.Key].MinConfidence {
		return Decision{Kind: "clarify", Options: []string{top.Key, second.Key}}
	}
	if second.Prob > 0.25 && top.Key != second.Key {
		return Decision{Kind: "sequence", Steps: []string{top.Key, second.Key}}
	}
	return Decision{Kind: "single", Action: top.Key}
}
