package intents

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Every wrapping call site uses fmt.Errorf("intents: ...: %w", err) so
// callers use errors.Is.
var (
	ErrCyclicSlotDAG       = errors.New("intents: slot_dag contains a dependency cycle")
	ErrInvalidSpec         = errors.New("intents: spec failed validation")
	ErrUnregisteredBackend = errors.New("intents: no Classifier/Lookuper registered under this name")
	ErrUnansweredFailure   = errors.New("intents: resume answers do not cover all required failures")
	ErrNotFound            = errors.New("intents: no matching entity")
	ErrInvalidInput        = errors.New("intents: input did not match any expected shape")
	ErrUnknownChoice       = errors.New("intents: classifier returned a choice not in options")
	ErrUnknownAnswerPath   = errors.New("intents: resume answer targets a path with no prior failure")
)

// IncompleteError is returned unwrapped (not via %w) by ResolveState/ResumeState/Resolve[T]
// whenever at least one required Failure remains, alongside the same partial State — so both
// errors.As and a direct type assertion recover the checkpoint.
type IncompleteError struct {
	State State
}

// Error counts and lists every State.Failures entry, not only the required ones that triggered
// this error (State.Failures is always fully populated — optional misses included, decision 5).
// This is deliberate, not an oversight: IncompleteError carries no reference to the spec that
// decided which paths were required, so it cannot filter to required-only without either losing
// its no-spec-reference construction (see errors_test.go's &IncompleteError{State: ...} literal)
// or duplicating requiredFailuresIn's spec-walk inside the error type. Callers that need only the
// required subset re-derive it from State.Failures against their own spec.
func (e *IncompleteError) Error() string {
	var paths []string
	for _, f := range e.State.Failures {
		paths = append(paths, f.Path)
	}
	return fmt.Sprintf("intents: resolution incomplete: %d failure(s): %s", len(paths), strings.Join(paths, ", "))
}
