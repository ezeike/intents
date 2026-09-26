package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client calls one hosted Jev endpoint's POST /v1/systemone API.
type Client struct {
	// BaseURL is the endpoint's origin, no trailing slash — the URL your Jev account/provider
	// gives you (e.g. TypeSafe's own endpoint, or a gateway fronting it).
	BaseURL string

	// APIKey is sent as "Authorization: Bearer <APIKey>" on every request. Unlike
	// intents/laya.Client.APIKey (optional — a self-hosted laya-serve has nothing to authenticate
	// against), this is required: Jev has no self-hosted deployment mode.
	APIKey string

	// HTTPClient sends every request. Must be non-nil; NewClient defaults it to
	// http.DefaultClient.
	HTTPClient *http.Client
}

// NewClient constructs a Client against baseURL, authenticating every request with apiKey. Set
// Client.HTTPClient directly afterward for a custom timeout/transport.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{BaseURL: baseURL, APIKey: apiKey, HTTPClient: http.DefaultClient}
}

// Question is one typed decision asked about a Request's State: "choice" (Criteria required,
// picks one key), "score" (Criteria is an ordered list of level descriptions), or "noul"
// (calibrated P(true), Criteria optional). Shape mirrors intents/laya.Question exactly — both
// packages target the same documented /v1/systemone protocol.
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`

	// Criteria is any(OrderedCriteria) for "choice", []string for "score", or a
	// map[string]string keyed only "true"/"false" (or nil) for "noul". Passed through verbatim —
	// this client doesn't constrain question types to a Go enum, since the server, not this
	// client, owns the set of supported types.
	Criteria any `json:"criteria,omitempty"`

	// Labels renames a "noul" question's true/false sides for display (e.g. {"true":"A"}).
	Labels map[string]string `json:"labels,omitempty"`
}

// Request is the POST /v1/systemone request body.
type Request struct {
	// State is the situation to evaluate: a string, a map, or any JSON-marshalable struct.
	State any `json:"state"`

	Questions map[string]Question `json:"questions"`

	// Model selects a checkpoint/model name. What values are valid is provider-specific; passed
	// through verbatim and omitted entirely when empty.
	Model string `json:"model,omitempty"`
}

// Usage carries token accounting for one request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the POST /v1/systemone response body. Answers is decoded lazily per question via
// Choice/Score/Noul, since each question's answer shape depends on its Type.
type Response struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   Usage                      `json:"usage"`
}

// ChoiceAnswer is a "choice" question's resolved answer.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// ScoreAnswer is a "score" question's resolved answer. Score is a probability-weighted
// expectation over level indices, not a level itself — never round it; take the argmax of
// Probabilities for a discrete level (same convention Laya uses, per the laya skill).
type ScoreAnswer struct {
	Score         float64            `json:"score"`
	Legend        map[string]string  `json:"legend"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// NoulAnswer is a "noul" question's resolved answer: Noul is calibrated P(true).
type NoulAnswer struct {
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}

type errorDetail struct {
	Detail string `json:"detail"`
}

// SystemOne calls POST /v1/systemone with req and decodes its response. ctx governs the
// request's lifetime; this call makes no retry attempt of its own — a caller wanting
// retry-on-transient-failure (relevant here more than for a self-hosted server: a hosted endpoint
// can rate-limit, see ErrRateLimited) supplies its own http.RoundTripper via Client.HTTPClient.
//
// A non-2xx response is translated to one of ErrBadRequest (400), ErrUnauthorized (401/403),
// ErrInvalidQuestion (422), ErrLimitExceeded (413), or ErrRateLimited (429), wrapping the server's
// own "detail" text when present, so callers can distinguish them with errors.Is rather than
// matching status codes themselves.
func (c *Client) SystemOne(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("jev: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("jev: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("jev: systemone: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("jev: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("jev: systemone: %w: %s", sentinelForStatus(resp.StatusCode), detailOrBody(data))
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("jev: decode response: %w", err)
	}
	return out, nil
}

func sentinelForStatus(status int) error {
	switch status {
	case http.StatusBadRequest:
		return ErrBadRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusUnprocessableEntity:
		return ErrInvalidQuestion
	case http.StatusRequestEntityTooLarge:
		return ErrLimitExceeded
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("unexpected status %d", status)
	}
}

func detailOrBody(data []byte) string {
	var e errorDetail
	if err := json.Unmarshal(data, &e); err == nil && e.Detail != "" {
		return e.Detail
	}
	return string(data)
}

// Choice decodes qid's answer as a ChoiceAnswer.
func (r Response) Choice(qid string) (ChoiceAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return ChoiceAnswer{}, fmt.Errorf("jev: choice %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		ChoiceAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return ChoiceAnswer{}, fmt.Errorf("jev: decode choice %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "choice" {
		return ChoiceAnswer{}, fmt.Errorf("jev: choice %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	return a.ChoiceAnswer, nil
}

// Score decodes qid's answer as a ScoreAnswer.
func (r Response) Score(qid string) (ScoreAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return ScoreAnswer{}, fmt.Errorf("jev: score %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		ScoreAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return ScoreAnswer{}, fmt.Errorf("jev: decode score %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "score" {
		return ScoreAnswer{}, fmt.Errorf("jev: score %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	return a.ScoreAnswer, nil
}

// Noul decodes qid's answer as a NoulAnswer. If the server omitted Confidence, it's filled in as
// NoulConfidence(Noul) rather than left at its zero value, since a real confidence is always
// >= 0.5 — mirrors intents/laya.Response.Noul's handling of Jev-protocol servers that omit it
// (e.g. CLM, per the laya skill's model-comparison notes).
func (r Response) Noul(qid string) (NoulAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return NoulAnswer{}, fmt.Errorf("jev: noul %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		NoulAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return NoulAnswer{}, fmt.Errorf("jev: decode noul %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "noul" {
		return NoulAnswer{}, fmt.Errorf("jev: noul %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	if a.Confidence == 0 {
		a.Confidence = NoulConfidence(a.Noul)
	}
	return a.NoulAnswer, nil
}

// NoulConfidence derives a noul question's confidence from its raw P(true) as max(p, 1-p) — the
// same formula intents/laya.NoulConfidence reproduces from Laya's own implementation. Kept here
// too (rather than importing intents/laya for it) since intents/jev must not depend on a sibling
// adapter package.
func NoulConfidence(p float64) float64 {
	if p > 1-p {
		return p
	}
	return 1 - p
}
