package jev_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ezeike/intents"
	"github.com/ezeike/intents/jev"
)

func TestClassifier_Classify_NilGroundingSendsBareInputAsState(t *testing.T) {
	var gotState any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State any `json:"state"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotState = body.State
		json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"action": map[string]any{"type": "choice", "choice": "a", "probabilities": map[string]float64{"a": 0.9, "b": 0.1}},
			},
		})
	}))
	defer srv.Close()

	c := jev.NewClassifier(jev.NewClient(srv.URL, "key"), "action", "pick one", jev.OrderedCriteria{{Key: "a"}, {Key: "b"}})
	_, err := c.Classify(context.Background(), "hello there", nil)
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	if gotState != "hello there" {
		t.Fatalf("request state = %v, want bare input %q", gotState, "hello there")
	}
}

func TestClassifier_Classify_NonNilGroundingIsSentVerbatimNotMerged(t *testing.T) {
	var gotState map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State map[string]any `json:"state"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotState = body.State
		json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"action": map[string]any{"type": "choice", "choice": "a", "probabilities": map[string]float64{"a": 1.0}},
			},
		})
	}))
	defer srv.Close()

	type grounding struct {
		Input   string `json:"input"`
		Context string `json:"context"`
	}
	c := jev.NewClassifier(jev.NewClient(srv.URL, "key"), "action", "pick one", jev.OrderedCriteria{{Key: "a"}})
	_, err := c.Classify(context.Background(), "the raw input", grounding{Input: "the raw input", Context: "extra grounding"})
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	if gotState["input"] != "the raw input" || gotState["context"] != "extra grounding" {
		t.Fatalf("request state = %+v, want grounding sent verbatim", gotState)
	}
}

func TestClassifier_Classify_ConfidenceIsTopTwoMarginNotServerConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"action": map[string]any{
					"type":          "choice",
					"choice":        "a",
					"probabilities": map[string]float64{"a": 0.6, "b": 0.4},
					"confidence":    0.9, // deliberately different from the margin, to prove it's ignored
				},
			},
		})
	}))
	defer srv.Close()

	c := jev.NewClassifier(jev.NewClient(srv.URL, "key"), "action", "pick one", jev.OrderedCriteria{{Key: "a"}, {Key: "b"}})
	ans, err := c.Classify(context.Background(), "input", nil)
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	const wantMargin = 0.6 - 0.4
	if diff := ans.Confidence - wantMargin; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("Classify().Confidence = %v, want top-two margin %v", ans.Confidence, wantMargin)
	}
}

func TestClassifier_Classify_ReturnsFullDistribution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"action": map[string]any{
					"type":          "choice",
					"choice":        "a",
					"probabilities": map[string]float64{"a": 0.7, "b": 0.2, "c": 0.1},
				},
			},
		})
	}))
	defer srv.Close()

	c := jev.NewClassifier(jev.NewClient(srv.URL, "key"), "action", "pick one", jev.OrderedCriteria{{Key: "a"}, {Key: "b"}, {Key: "c"}})
	ans, err := c.Classify(context.Background(), "input", nil)
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	if len(ans.Distribution) != 3 || ans.Distribution["c"] != 0.1 {
		t.Fatalf("Classify().Distribution = %+v, want the full 3-entry probability map", ans.Distribution)
	}
}

func TestClassifier_Classify_UnauthorizedIsWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"detail": "bad key"})
	}))
	defer srv.Close()

	c := jev.NewClassifier(jev.NewClient(srv.URL, "bad-key"), "action", "pick one", jev.OrderedCriteria{{Key: "a"}})
	_, err := c.Classify(context.Background(), "input", nil)
	if err == nil {
		t.Fatal("Classify() error = nil, want non-nil on server 401")
	}
}

// TestClassifier_ImplementsIntentsClassifier is a compile-time-shaped runtime check that the
// adapter satisfies the seam the intents engine registers against.
func TestClassifier_ImplementsIntentsClassifier(t *testing.T) {
	var _ intents.Classifier = jev.NewClassifier(jev.NewClient("http://example.invalid", "key"), "q", "", nil)
}
