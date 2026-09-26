//go:build integration

package jev_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ezeike/intents/jev"
)

func TestSystemOne_InvalidAPIKey_WrapsErrUnauthorized(t *testing.T) {
	requireJev(t) // only to confirm a real key/endpoint is configured to test against at all

	c := jev.NewClient(jevBaseURL(), "sk-definitely-not-a-real-key")
	_, err := c.SystemOne(context.Background(), jev.Request{
		State:     "x",
		Questions: map[string]jev.Question{"q1": {Type: "noul", Instructions: "x"}},
		Model:     jevModel(),
	})
	if !errors.Is(err, jev.ErrUnauthorized) {
		t.Fatalf("SystemOne() error = %v, want errors.Is ErrUnauthorized", err)
	}
}
