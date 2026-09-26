package jev

import (
	"context"
	"fmt"

	"github.com/ezeike/intents"
)

// Classifier implements intents.Classifier for a single KindClassify node, wrapping one shared
// Client with the fixed question shape (Instructions/Criteria/Model) that node always asks.
// Register one Classifier per node under a distinct ClassifySpec.Backend name, mirroring
// intents/laya.Classifier exactly — this package exists to let the same ClassifySpec/Options be
// pointed at either backend and compared, so the two adapters deliberately share this shape.
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

	// Model pins the checkpoint/model name. Empty omits the field; what values are accepted is
	// provider-specific — see docs/model-comparison.md.
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
// is grounding verbatim — c does not merge input into it, matching intents/laya.Classifier's
// contract exactly (see that package's doc comment on Classify for the reference pattern a
// BuildContext must follow to get the original input onto the wire alongside grounding).
//
// The returned ChoiceAnswer.Confidence is the top-two probability margin (top1 - top2), not the
// server's own "confidence" field — see doc.go's rationale for keeping this identical to
// intents/laya's behavior despite Jev's advertised (but here unverified) better calibration.
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
		return intents.ChoiceAnswer{}, fmt.Errorf("jev: classify: %w", err)
	}

	ans, err := resp.Choice(c.Question)
	if err != nil {
		return intents.ChoiceAnswer{}, fmt.Errorf("jev: classify: %w", err)
	}

	return intents.ChoiceAnswer{
		Choice:       ans.Choice,
		Confidence:   margin(ans.Probabilities),
		Distribution: ans.Probabilities,
	}, nil
}

// margin returns the gap between the two highest values in p (0 if p has fewer than two
// entries). See doc.go: kept identical to intents/laya's escalation signal so a node's
// MinConfidence behaves the same regardless of which backend it's pointed at.
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
