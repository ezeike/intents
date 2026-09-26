package jev_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ezeike/intents/jev"
)

func TestClient_SystemOne_SendsAuthStateAndQuestionsAndDecodesChoice(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1",
			"answers": map[string]any{
				"action": map[string]any{
					"type":          "choice",
					"choice":        "wait",
					"probabilities": map[string]float64{"cool": 0.26, "wait": 0.74},
					"confidence":    0.61,
				},
			},
			"usage": map[string]any{"input_tokens": 42, "output_tokens": 0},
		})
	}))
	defer srv.Close()

	c := jev.NewClient(srv.URL, "secret-key")
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: "the room is warm",
		Questions: map[string]jev.Question{
			"action": {Type: "choice", Instructions: "pick an action", Criteria: jev.OrderedCriteria{
				{Key: "cool", Description: "turn on AC"},
				{Key: "wait", Description: "do nothing"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	if gotAuth != "Bearer secret-key" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotBody["state"] != "the room is warm" {
		t.Fatalf("request state = %v, want %q", gotBody["state"], "the room is warm")
	}

	ans, err := resp.Choice("action")
	if err != nil {
		t.Fatalf("Choice() error = %v, want nil", err)
	}
	if ans.Choice != "wait" || ans.Confidence != 0.61 {
		t.Fatalf("Choice() = %+v, want Choice=wait Confidence=0.61", ans)
	}
}

func TestClient_SystemOne_400StatusWrapsErrBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"detail": "questions must be an object"})
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrBadRequest) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrBadRequest", err)
	}
}

func TestClient_SystemOne_401StatusWrapsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"detail": "invalid API key"})
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "bad-key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrUnauthorized) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrUnauthorized", err)
	}
}

func TestClient_SystemOne_403StatusWrapsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrUnauthorized) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrUnauthorized", err)
	}
}

func TestClient_SystemOne_422StatusWrapsErrInvalidQuestion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"detail": "unknown question type"})
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrInvalidQuestion) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrInvalidQuestion", err)
	}
}

func TestClient_SystemOne_413StatusWrapsErrLimitExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte("payload too large"))
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrLimitExceeded) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrLimitExceeded", err)
	}
}

func TestClient_SystemOne_429StatusWrapsErrRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(context.Background(), jev.Request{})
	if !errors.Is(err, jev.ErrRateLimited) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrRateLimited", err)
	}
}

func TestClient_SystemOne_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := jev.NewClient(srv.URL, "key").SystemOne(ctx, jev.Request{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want non-nil on canceled context")
	}
}

func TestResponse_Choice_MissingQuestionWrapsErrMissingAnswer(t *testing.T) {
	resp := jev.Response{Answers: map[string]json.RawMessage{}}

	_, err := resp.Choice("action")
	if !errors.Is(err, jev.ErrMissingAnswer) {
		t.Fatalf("Choice() error = %v, want errors.Is ErrMissingAnswer", err)
	}
}

func TestResponse_Choice_WrongTypeWrapsErrAnswerType(t *testing.T) {
	resp := jev.Response{Answers: map[string]json.RawMessage{
		"q": json.RawMessage(`{"type":"noul","noul":0.5,"confidence":0.5}`),
	}}

	_, err := resp.Choice("q")
	if !errors.Is(err, jev.ErrAnswerType) {
		t.Fatalf("Choice() error = %v, want errors.Is ErrAnswerType", err)
	}
}

func TestResponse_Score_DecodesLegendAndExpectedValue(t *testing.T) {
	resp := jev.Response{Answers: map[string]json.RawMessage{
		"urgency": json.RawMessage(`{"type":"score","score":1.4,"legend":{"0":"low","1":"medium","2":"high"},"probabilities":{"0":0.1,"1":0.4,"2":0.5},"confidence":0.3}`),
	}}

	ans, err := resp.Score("urgency")
	if err != nil {
		t.Fatalf("Score() error = %v, want nil", err)
	}
	if ans.Score != 1.4 || ans.Legend["2"] != "high" {
		t.Fatalf("Score() = %+v, want Score=1.4 Legend[2]=high", ans)
	}
}

func TestResponse_Noul_MissingConfidenceIsFilledFromNoul(t *testing.T) {
	resp := jev.Response{Answers: map[string]json.RawMessage{
		"is_urgent": json.RawMessage(`{"type":"noul","noul":0.2}`),
	}}

	ans, err := resp.Noul("is_urgent")
	if err != nil {
		t.Fatalf("Noul() error = %v, want nil", err)
	}
	if ans.Confidence != 0.8 {
		t.Fatalf("Noul().Confidence = %v, want 0.8 (max(0.2, 1-0.2))", ans.Confidence)
	}
}

func TestResponse_Noul_PresentConfidenceIsNotOverwritten(t *testing.T) {
	resp := jev.Response{Answers: map[string]json.RawMessage{
		"is_urgent": json.RawMessage(`{"type":"noul","noul":0.2,"confidence":0.95}`),
	}}

	ans, err := resp.Noul("is_urgent")
	if err != nil {
		t.Fatalf("Noul() error = %v, want nil", err)
	}
	if ans.Confidence != 0.95 {
		t.Fatalf("Noul().Confidence = %v, want 0.95 (server value kept)", ans.Confidence)
	}
}

func TestNoulConfidence_ReturnsMaxOfPAndComplement(t *testing.T) {
	if got := jev.NoulConfidence(0.9); got != 0.9 {
		t.Fatalf("NoulConfidence(0.9) = %v, want 0.9", got)
	}
	if got := jev.NoulConfidence(0.1); got != 0.9 {
		t.Fatalf("NoulConfidence(0.1) = %v, want 0.9", got)
	}
}
