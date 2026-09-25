package laya

import "errors"

// Sentinel errors. Every wrapping call site uses fmt.Errorf("laya: ...: %w", err) so callers use
// errors.Is.
var (
	// ErrBadRequest wraps a 400 response: a structurally malformed request (bad JSON, missing or
	// wrong-shaped "questions").
	ErrBadRequest = errors.New("laya: server rejected a malformed request")

	// ErrInvalidQuestion wraps a 422 response: a well-formed request with a semantically invalid
	// question definition (unknown type, missing instructions, wrong criteria shape). Validation
	// is whole-batch — one invalid question among N valid ones fails the entire request.
	ErrInvalidQuestion = errors.New("laya: server rejected an invalid question definition")

	// ErrLimitExceeded wraps a 413 response: a size guardrail tripped (>64 questions, state
	// >50,000 chars, body >2MiB).
	ErrLimitExceeded = errors.New("laya: request exceeded a server size limit")

	// ErrMissingAnswer is returned by Response.Choice/Score/Noul when the named question has no
	// entry in the response's answers map.
	ErrMissingAnswer = errors.New("laya: response has no answer for the requested question")

	// ErrAnswerType is returned by Response.Choice/Score/Noul when the named question's answer
	// decodes but is shaped for a different question type than requested.
	ErrAnswerType = errors.New("laya: answer is not shaped for the requested question type")
)
