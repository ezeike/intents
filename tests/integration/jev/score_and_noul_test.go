//go:build integration

package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

func TestScore_ClearlyCritical_ScoresHighAndLegendMatchesCriteria(t *testing.T) {
	c := requireJev(t)

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: "URGENT!! right now, emergency",
		Questions: map[string]jev.Question{
			"urgency": {
				Type:         "score",
				Instructions: "how urgent is this?",
				Criteria:     []string{"not urgent", "soon", "critical"},
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	a, err := resp.Score("urgency")
	if err != nil {
		t.Fatalf("Score() error = %v, want nil", err)
	}
	if a.Legend["2"] != "critical" {
		t.Errorf("legend[2] = %q, want %q", a.Legend["2"], "critical")
	}
	if a.Score < 1.5 {
		t.Errorf("score = %v, want >= 1.5 for an unambiguous emergency (max level is 2)", a.Score)
	}
}

func TestNoul_ClearlyUrgentVsNotUrgent_DirectionsCorrectly(t *testing.T) {
	c := requireJev(t)

	notUrgent, err := c.SystemOne(context.Background(), jev.Request{
		State:     "no rush at all, whenever",
		Questions: map[string]jev.Question{"urgent": {Type: "noul", Instructions: "is this urgent?"}},
		Model:     jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}
	urgent, err := c.SystemOne(context.Background(), jev.Request{
		State:     "URGENT!! right now, emergency",
		Questions: map[string]jev.Question{"urgent": {Type: "noul", Instructions: "is this urgent?"}},
		Model:     jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	notUrgentAns, err := notUrgent.Noul("urgent")
	if err != nil {
		t.Fatalf("Noul() error = %v, want nil", err)
	}
	urgentAns, err := urgent.Noul("urgent")
	if err != nil {
		t.Fatalf("Noul() error = %v, want nil", err)
	}

	if urgentAns.Noul <= notUrgentAns.Noul {
		t.Errorf("P(urgent) = %v for the urgent case, want it greater than %v for the non-urgent case", urgentAns.Noul, notUrgentAns.Noul)
	}
	// Confidence must always be filled (>= 0.5), whether or not the server itself sent it —
	// confirmed live that this route omits it entirely (doc.go's "Confirmed against a live
	// endpoint"), so this also exercises Response.Noul's NoulConfidence fallback for real.
	if urgentAns.Confidence < 0.5 || notUrgentAns.Confidence < 0.5 {
		t.Errorf("Confidence = %v / %v, want both >= 0.5", urgentAns.Confidence, notUrgentAns.Confidence)
	}
}
