package laya

import (
	"context"
	"fmt"

	"github.com/ezeike/intents"
)

// Classifier implements intents.Classifier for a single KindClassify node, wrapping one shared
// Client with the fixed question shape (Instructions/Criteria/Model) that node always asks.
// Register one Classifier per node under a distinct ClassifySpec.Backend name — Classify takes no
// per-call options, so a node's option set is fixed at construction time, matching
// ClassifySpec.Options declared alongside it.
//
// Classify may be called concurrently from different goroutines during slot_dag wave execution
// (intents.Classifier's contract); Classifier holds no mutable state after construction, so this
// is safe as long as Client.HTTPClient itself is (net/http.Client is).
type Classifier struct {
	Client *Client

	// Question is this node's question id in the /v1/systemone request/response map. Purely a
	// wire-level label; the intents engine never sees it.
	Question string

	Instructions string
	Criteria     OrderedCriteria

	// Model pins the checkpoint ("typed-decisions" is this repo's default for zero-shot
	// classify nodes — see docs/model-comparison.md). Empty auto-routes; see Client.SystemOne's
	// doc comment on the auto-routing hazard for short text.
	Model string
}

// NewClassifier constructs a Classifier asking one fixed "choice" question over criteria.
func NewClassifier(client *Client, question, instructions string, criteria OrderedCriteria) *Classifier {
	return &Classifier{Client: client, Question: question, Instructions: instructions, Criteria: criteria}
}

var _ intents.Classifier = (*Classifier)(nil)

// Classify sends one /v1/systemone request asking c's fixed question about state and returns the
// winning choice.
//
// Wire state: if grounding is nil, state is the bare input string. If grounding is non-nil, state
// is grounding verbatim — c does not merge input into it. A BuildContext that needs the original
// input on the wire alongside situational grounding must include it itself (e.g. a struct with
// its own Input field set from state.Input), matching the reference pattern in
// tests/integration/laya/spec_action_test.go's specActionState. This mirrors ClassifySpec's own
// contract: BuildContext, not this adapter, owns exactly what grounding contains.
//
// The returned ChoiceAnswer.Confidence is the top-two probability margin (top1 - top2), not
// Laya's own "confidence" field — see doc.go's "Confidence vs. margin".
func (c *Classifier) Classify(ctx context.Context, input string, grounding any) (intents.ChoiceAnswer, error) {
	var state any = input
	if grounding != nil {
		state = grounding
	}

	resp, err := c.Client.SystemOne(ctx, Request{
		State: state,
		Questions: map[string]Question{
			c.Question: {Type: "choice", Instructions: c.Instructions, Criteria: c.Criteria},
		},
		Model: c.Model,
	})
	if err != nil {
		return intents.ChoiceAnswer{}, fmt.Errorf("laya: classify: %w", err)
	}

	ans, err := resp.Choice(c.Question)
	if err != nil {
		return intents.ChoiceAnswer{}, fmt.Errorf("laya: classify: %w", err)
	}

	return intents.ChoiceAnswer{
		Choice:       ans.Choice,
		Confidence:   margin(ans.Probabilities),
		Distribution: ans.Probabilities,
	}, nil
}

// margin returns the gap between the two highest values in p (0 if p has fewer than two
// entries), the escalation signal the laya skill's confidence-calibration measurement found more
// reliable than the model's own confidence field.
func margin(p map[string]float64) float64 {
	var top, second float64
	for _, v := range p {
		switch {
		case v > top:
			second = top
			top = v
		case v > second:
			second = v
		}
	}
	return top - second
}
