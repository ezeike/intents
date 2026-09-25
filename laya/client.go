package laya

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client calls one Laya (or Jev-protocol-compatible) server's POST /v1/systemone endpoint.
type Client struct {
	// BaseURL is the server's origin, no trailing slash, e.g. "http://p16:8000".
	BaseURL string

	// HTTPClient sends every request. Must be non-nil; NewClient defaults it to
	// http.DefaultClient.
	HTTPClient *http.Client
}

// NewClient constructs a Client against baseURL with http.DefaultClient. Set Client.HTTPClient
// directly afterward for a custom timeout/transport.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTPClient: http.DefaultClient}
}

// Question is one typed decision asked about a Request's State: "choice" (Criteria required,
// picks one key), "score" (Criteria is an ordered list of level descriptions), or "noul"
// (calibrated P(true), Criteria optional).
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`

	// Criteria is any(OrderedCriteria) for "choice", []string for "score", or a
	// map[string]string keyed only "true"/"false" (or nil) for "noul". Passed through verbatim —
	// this client doesn't constrain question types to a Go enum, since the server, not this
	// client, owns the set of supported types.
	Criteria any `json:"criteria,omitempty"`

	// Labels renames a "noul" question's true/false sides for display (e.g. {"true":"A"}).
	// noul-only: the server 422s a "choice"/"score" question that sets this.
	Labels map[string]string `json:"labels,omitempty"`
}

// Request is the POST /v1/systemone request body.
type Request struct {
	// State is the situation to evaluate: a string, a map, or any JSON-marshalable struct.
	State any `json:"state"`

	Questions map[string]Question `json:"questions"`

	// Model selects "english" | "multilingual" | "typed-decisions", or a published HF checkpoint
	// id. Omitted (empty) auto-routes by language/script detection — pin this for any input short
	// enough for auto-routing's documented misdetection hazard (laya skill, "Routing hazards").
	Model string `json:"model,omitempty"`
}

// Routing carries the server's language/model auto-routing decision for one request.
type Routing struct {
	Model     string `json:"model"`
	Repo      string `json:"repo"`
	Reason    string `json:"reason"`
	Detection any    `json:"detection"`
	Workflow  any    `json:"workflow"`
}

// Usage carries token accounting for one request. OutputTokens is always 0 — Laya never
// generates text.
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
	Routing *Routing                   `json:"routing"`
}

// ChoiceAnswer is a "choice" question's resolved answer.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// ScoreAnswer is a "score" question's resolved answer. Score is a probability-weighted
// expectation over level indices, not a level itself — never round it; take the argmax of
// Probabilities for a discrete level (laya skill, "score returns an expected value").
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
// retry-on-transient-failure supplies its own http.RoundTripper via Client.HTTPClient.
//
// A non-2xx response is translated to one of ErrBadRequest (400), ErrInvalidQuestion (422), or
// ErrLimitExceeded (413), wrapping the server's own "detail" text when present, so callers can
// distinguish them with errors.Is rather than matching status codes themselves.
func (c *Client) SystemOne(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("laya: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("laya: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("laya: systemone: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("laya: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("laya: systemone: %w: %s", sentinelForStatus(resp.StatusCode), detailOrBody(data))
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("laya: decode response: %w", err)
	}
	return out, nil
}

func sentinelForStatus(status int) error {
	switch status {
	case http.StatusBadRequest:
		return ErrBadRequest
	case http.StatusUnprocessableEntity:
		return ErrInvalidQuestion
	case http.StatusRequestEntityTooLarge:
		return ErrLimitExceeded
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

// Healthy reports whether GET /health returns 200 with status "ok". Per the laya skill,
// /health can say ok while every /v1/systemone call 500s (a missing JIT compiler on the serving
// image) — this is a reachability check, not a full liveness probe. Never called from a request
// path.
func (c *Client) Healthy(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("laya: build health request: %w", err)
	}
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("laya: health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("laya: health check: unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("laya: decode health response: %w", err)
	}
	if out.Status != "ok" {
		return fmt.Errorf("laya: health check: status %q", out.Status)
	}
	return nil
}

// Choice decodes qid's answer as a ChoiceAnswer.
func (r Response) Choice(qid string) (ChoiceAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return ChoiceAnswer{}, fmt.Errorf("laya: choice %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		ChoiceAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return ChoiceAnswer{}, fmt.Errorf("laya: decode choice %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "choice" {
		return ChoiceAnswer{}, fmt.Errorf("laya: choice %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	return a.ChoiceAnswer, nil
}

// Score decodes qid's answer as a ScoreAnswer.
func (r Response) Score(qid string) (ScoreAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return ScoreAnswer{}, fmt.Errorf("laya: score %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		ScoreAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return ScoreAnswer{}, fmt.Errorf("laya: decode score %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "score" {
		return ScoreAnswer{}, fmt.Errorf("laya: score %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	return a.ScoreAnswer, nil
}

// Noul decodes qid's answer as a NoulAnswer. If the server omitted Confidence (Laya always sends
// it; some Jev-protocol-compatible servers, e.g. CLM, don't — laya skill, "model-comparison"),
// it's filled in as NoulConfidence(Noul) rather than left at its zero value, since a real
// confidence is always >= 0.5.
func (r Response) Noul(qid string) (NoulAnswer, error) {
	raw, ok := r.Answers[qid]
	if !ok {
		return NoulAnswer{}, fmt.Errorf("laya: noul %q: %w", qid, ErrMissingAnswer)
	}
	var a struct {
		Type string `json:"type"`
		NoulAnswer
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return NoulAnswer{}, fmt.Errorf("laya: decode noul %q: %w", qid, err)
	}
	if a.Type != "" && a.Type != "noul" {
		return NoulAnswer{}, fmt.Errorf("laya: noul %q: got type %q: %w", qid, a.Type, ErrAnswerType)
	}
	if a.Confidence == 0 {
		a.Confidence = NoulConfidence(a.Noul)
	}
	return a.NoulAnswer, nil
}

// NoulConfidence derives a noul question's confidence from its raw P(true), reproducing Laya's
// own exact formula (laya skill: "confidence = max(noul, 1-noul), exactly").
func NoulConfidence(p float64) float64 {
	if p > 1-p {
		return p
	}
	return 1 - p
}
