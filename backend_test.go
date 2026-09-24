package intents_test

import (
	"testing"

	"github.com/ezeike/intents"
)

func TestChoiceAnswer_TopTwo_OrdersByProbDescendingThenKeyAscendingOnTies(t *testing.T) {
	a := intents.ChoiceAnswer{
		Distribution: map[string]float64{
			"invite_collaborator": 0.4,
			"rename_document":     0.4,
			"export_document":     0.2,
		},
	}

	top, second := a.TopTwo()

	if top.Key != "invite_collaborator" || top.Prob != 0.4 {
		t.Fatalf("top = %+v, want {Key: invite_collaborator, Prob: 0.4}", top)
	}
	if second.Key != "rename_document" || second.Prob != 0.4 {
		t.Fatalf("second = %+v, want {Key: rename_document, Prob: 0.4}", second)
	}
}

func TestChoiceAnswer_TopTwo_FewerThanTwoEntriesZeroFills(t *testing.T) {
	a := intents.ChoiceAnswer{Distribution: map[string]float64{"only_option": 1.0}}

	top, second := a.TopTwo()

	if top.Key != "only_option" || top.Prob != 1.0 {
		t.Fatalf("top = %+v, want {Key: only_option, Prob: 1.0}", top)
	}
	if second != (intents.Scored{}) {
		t.Fatalf("second = %+v, want zero value", second)
	}
}

func TestChoiceAnswer_TopTwo_EmptyDistributionZeroFillsBoth(t *testing.T) {
	a := intents.ChoiceAnswer{}

	top, second := a.TopTwo()

	if top != (intents.Scored{}) || second != (intents.Scored{}) {
		t.Fatalf("top = %+v, second = %+v, want both zero value", top, second)
	}
}
