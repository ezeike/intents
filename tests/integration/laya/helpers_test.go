//go:build integration

// Package laya_test exercises the live Laya server's POST /v1/systemone wire
// protocol directly (no client library exists yet — see docs/architecture.md
// and the design spec's "backends are adapters, not core" decision). These
// tests are the ground truth a future intents/laya adapter must match.
//
// Target server defaults to http://p16:8000 (the box these were written
// against — see "Full Laya request spec" in the design conversation, sourced
// from laya/agent.py and laya/serve.py on p16:/mnt/extra/models/apps/laya-github).
// Override with LAYA_TEST_URL. Every test skips (not fails) if the server
// isn't reachable, so `go test -tags=integration ./...` stays safe to run
// without p16 available.
package laya_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func layaBaseURL() string {
	if v := os.Getenv("LAYA_TEST_URL"); v != "" {
		return v
	}
	return "http://p16:8000"
}

// requireLaya skips the test if the live server can't be reached, rather
// than failing — this suite depends on an already-running external system
// (go-testing-tiers: live-system test), not infra the test provisions.
func requireLaya(t *testing.T) string {
	t.Helper()
	base := layaBaseURL()
	resp, err := httpClient.Get(base + "/health")
	if err != nil {
		t.Skipf("laya not reachable at %s: %v", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("laya health check at %s returned %d", base, resp.StatusCode)
	}
	return base
}

// ---- request shapes (POST /v1/systemone body) ----

type systemOneRequest struct {
	State     any                    `json:"state"`
	Questions map[string]questionDef `json:"questions"`
	Model     string                 `json:"model,omitempty"`
}

type questionDef struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
	Labels       any    `json:"labels,omitempty"`
}

// ---- response shapes ----

type systemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   usageInfo                  `json:"usage"`
	Routing *routingInfo               `json:"routing"`
}

type usageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type routingInfo struct {
	Model     string `json:"model"`
	Repo      string `json:"repo"`
	Reason    string `json:"reason"`
	Detection any    `json:"detection"`
	Workflow  any    `json:"workflow"`
}

type choiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

type scoreAnswer struct {
	Type          string             `json:"type"`
	Score         float64            `json:"score"`
	Legend        map[string]string  `json:"legend"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

type noulAnswer struct {
	Type       string  `json:"type"`
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}

type errorResponse struct {
	Detail string `json:"detail"`
}

type healthResponse struct {
	Status string   `json:"status"`
	Loaded []string `json:"loaded"`
	Device string   `json:"device"`
}

// ---- HTTP plumbing ----

// rawPostSystemOne sends body as-is (already-marshaled bytes), for cases
// (malformed JSON, oversized body) that must not go through systemOneRequest.
func rawPostSystemOne(t *testing.T, base string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, data
}

func postSystemOne(t *testing.T, base string, reqBody systemOneRequest) (int, []byte) {
	t.Helper()
	b, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return rawPostSystemOne(t, base, b)
}

// mustOK posts the request and fatals if the server didn't return 200,
// decoding the successful response.
func mustOK(t *testing.T, base string, reqBody systemOneRequest) systemOneResponse {
	t.Helper()
	status, data := postSystemOne(t, base, reqBody)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", status, data)
	}
	var out systemOneResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, data)
	}
	return out
}

func decodeChoice(t *testing.T, resp systemOneResponse, qid string) choiceAnswer {
	t.Helper()
	raw, ok := resp.Answers[qid]
	if !ok {
		t.Fatalf("answers missing question %q: %+v", qid, resp.Answers)
	}
	var a choiceAnswer
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("decode choice answer %q: %v", qid, err)
	}
	return a
}

func decodeScore(t *testing.T, resp systemOneResponse, qid string) scoreAnswer {
	t.Helper()
	raw, ok := resp.Answers[qid]
	if !ok {
		t.Fatalf("answers missing question %q: %+v", qid, resp.Answers)
	}
	var a scoreAnswer
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("decode score answer %q: %v", qid, err)
	}
	return a
}

func decodeNoul(t *testing.T, resp systemOneResponse, qid string) noulAnswer {
	t.Helper()
	raw, ok := resp.Answers[qid]
	if !ok {
		t.Fatalf("answers missing question %q: %+v", qid, resp.Answers)
	}
	var a noulAnswer
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("decode noul answer %q: %v", qid, err)
	}
	return a
}

func decodeError(t *testing.T, data []byte) errorResponse {
	t.Helper()
	var e errorResponse
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("decode error response: %v (body: %s)", err, data)
	}
	return e
}

func sumProbabilities(p map[string]float64) float64 {
	var sum float64
	for _, v := range p {
		sum += v
	}
	return sum
}
