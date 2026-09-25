package laya_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ezeike/intents/laya"
)

func TestClient_SystemOne_SendsStateAndQuestionsAndDecodesChoice(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "laya-rl-agent",
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

	c := laya.NewClient(srv.URL)
	resp, err := c.SystemOne(context.Background(), laya.Request{
		State: "the room is warm",
		Questions: map[string]laya.Question{
			"action": {Type: "choice", Instructions: "pick an action", Criteria: laya.OrderedCriteria{
				{Key: "cool", Description: "turn on AC"},
				{Key: "wait", Description: "do nothing"},
			}},
		},
		Model: "typed-decisions",
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v, want nil", err)
	}

	if gotBody["state"] != "the room is warm" {
		t.Fatalf("request state = %v, want %q", gotBody["state"], "the room is warm")
	}
	if gotBody["model"] != "typed-decisions" {
		t.Fatalf("request model = %v, want %q", gotBody["model"], "typed-decisions")
	}

	ans, err := resp.Choice("action")
	if err != nil {
		t.Fatalf("Choice() error = %v, want nil", err)
	}
	if ans.Choice != "wait" || ans.Confidence != 0.61 {
		t.Fatalf("Choice() = %+v, want Choice=wait Confidence=0.61", ans)
	}
	if resp.Usage.InputTokens != 42 {
		t.Fatalf("Usage.InputTokens = %d, want 42", resp.Usage.InputTokens)
	}
}

func TestClient_SystemOne_AuthorizationHeader(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   string
	}{
		{name: "api key set sends bearer", apiKey: "sk-test", want: "Bearer sk-test"},
		{name: "no api key sends no header", apiKey: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{}})
			}))
			defer srv.Close()

			c := laya.NewClient(srv.URL)
			c.APIKey = tt.apiKey
			if _, err := c.SystemOne(context.Background(), laya.Request{}); err != nil {
				t.Fatalf("SystemOne() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("Authorization = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_SystemOne_400StatusWrapsErrBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"detail": "questions must be an object"})
	}))
	defer srv.Close()

	_, err := laya.NewClient(srv.URL).SystemOne(context.Background(), laya.Request{})
	if !errors.Is(err, laya.ErrBadRequest) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrBadRequest", err)
	}
}

func TestClient_SystemOne_422StatusWrapsErrInvalidQuestion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"detail": "unknown question type"})
	}))
	defer srv.Close()

	_, err := laya.NewClient(srv.URL).SystemOne(context.Background(), laya.Request{})
	if !errors.Is(err, laya.ErrInvalidQuestion) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrInvalidQuestion", err)
	}
}

func TestClient_SystemOne_413StatusWrapsErrLimitExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte("payload too large"))
	}))
	defer srv.Close()

	_, err := laya.NewClient(srv.URL).SystemOne(context.Background(), laya.Request{})
	if !errors.Is(err, laya.ErrLimitExceeded) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrLimitExceeded", err)
	}
}

func TestClient_SystemOne_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := laya.NewClient(srv.URL).SystemOne(ctx, laya.Request{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want non-nil on canceled context")
	}
}

func TestResponse_Choice_MissingQuestionWrapsErrMissingAnswer(t *testing.T) {
	resp := laya.Response{Answers: map[string]json.RawMessage{}}

	_, err := resp.Choice("action")
	if !errors.Is(err, laya.ErrMissingAnswer) {
		t.Fatalf("Choice() error = %v, want errors.Is ErrMissingAnswer", err)
	}
}

func TestResponse_Choice_WrongTypeWrapsErrAnswerType(t *testing.T) {
	resp := laya.Response{Answers: map[string]json.RawMessage{
		"q": json.RawMessage(`{"type":"noul","noul":0.5,"confidence":0.5}`),
	}}

	_, err := resp.Choice("q")
	if !errors.Is(err, laya.ErrAnswerType) {
		t.Fatalf("Choice() error = %v, want errors.Is ErrAnswerType", err)
	}
}

func TestResponse_Score_DecodesLegendAndExpectedValue(t *testing.T) {
	resp := laya.Response{Answers: map[string]json.RawMessage{
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
	resp := laya.Response{Answers: map[string]json.RawMessage{
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
	resp := laya.Response{Answers: map[string]json.RawMessage{
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
	if got := laya.NoulConfidence(0.9); got != 0.9 {
		t.Fatalf("NoulConfidence(0.9) = %v, want 0.9", got)
	}
	if got := laya.NoulConfidence(0.1); got != 0.9 {
		t.Fatalf("NoulConfidence(0.1) = %v, want 0.9", got)
	}
}

func TestClient_Healthy_OKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "loaded": []string{"english"}})
	}))
	defer srv.Close()

	if err := laya.NewClient(srv.URL).Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy() error = %v, want nil", err)
	}
}

func TestClient_Healthy_NonOKBodyStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "degraded"})
	}))
	defer srv.Close()

	if err := laya.NewClient(srv.URL).Healthy(context.Background()); err == nil {
		t.Fatal("Healthy() error = nil, want non-nil for status != ok")
	}
}
