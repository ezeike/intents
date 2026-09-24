package intents_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ezeike/intents"
)

func TestIncompleteError_Error_FormatsRequiredFailurePaths(t *testing.T) {
	err := &intents.IncompleteError{
		State: intents.State{
			Failures: []intents.Failure{
				{Path: "transfer.amount", Reason: intents.ReasonLowConfidence},
				{Path: "transfer.recipient", Reason: intents.ReasonNotFound},
			},
		},
	}

	got := err.Error()

	if !strings.HasPrefix(got, "intents: resolution incomplete: 2 required failure(s): ") {
		t.Fatalf("Error() = %q, want prefix %q", got, "intents: resolution incomplete: 2 required failure(s): ")
	}
	if !strings.Contains(got, "transfer.amount") || !strings.Contains(got, "transfer.recipient") {
		t.Fatalf("Error() = %q, want it to contain both failure paths", got)
	}
}

func TestIncompleteError_As_RecoversState(t *testing.T) {
	original := &intents.IncompleteError{State: intents.State{Terminal: "invite_collaborator"}}
	var err error = original

	var target *intents.IncompleteError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(err, &target) = false, want true")
	}
	if target.State.Terminal != "invite_collaborator" {
		t.Fatalf("target.State.Terminal = %q, want %q", target.State.Terminal, "invite_collaborator")
	}
}
