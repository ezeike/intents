package jev

import "errors"

// Sentinel errors. Every wrapping call site uses fmt.Errorf("jev: ...: %w", err) so callers use
// errors.Is.
//
// Status-code mapping below mirrors intents/laya's, since Jev is documented as speaking the same
// /v1/systemone protocol Laya was built compatible with — but this mapping has not been confirmed
// against a live Jev response (see doc.go's "What's still genuinely not done"). Treat it as a
// best-effort default, not a verified contract, until it's checked against a real endpoint.
var (
	// ErrBadRequest wraps a 400 response: a structurally malformed request (bad JSON, missing or
	// wrong-shaped "questions").
	ErrBadRequest = errors.New("jev: server rejected a malformed request")

	// ErrInvalidQuestion wraps a 422 response: a well-formed request with a semantically invalid
	// question definition (unknown type, missing instructions, wrong criteria shape).
	ErrInvalidQuestion = errors.New("jev: server rejected an invalid question definition")

	// ErrLimitExceeded wraps a 413 response: a size guardrail tripped.
	ErrLimitExceeded = errors.New("jev: request exceeded a server size limit")

	// ErrUnauthorized wraps a 401/403 response: a missing or rejected API key. Laya has no
	// equivalent sentinel — a self-hosted laya-serve has no auth to reject.
	ErrUnauthorized = errors.New("jev: server rejected the API key")

	// ErrRateLimited wraps a 429 response: TypeSafe's hosted quota was exceeded. Laya has no
	// equivalent — a self-hosted server has no per-account quota.
	ErrRateLimited = errors.New("jev: request was rate limited")

	// ErrMissingAnswer is returned by Response.Choice/Score/Noul when the named question has no
	// entry in the response's answers map.
	ErrMissingAnswer = errors.New("jev: response has no answer for the requested question")

	// ErrAnswerType is returned by Response.Choice/Score/Noul when the named question's answer
	// decodes but is shaped for a different question type than requested.
	ErrAnswerType = errors.New("jev: answer is not shaped for the requested question type")
)
