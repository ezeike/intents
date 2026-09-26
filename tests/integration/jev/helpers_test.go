//go:build integration

// Package jev_test exercises this package's own Client/Classifier against a live Jev endpoint.
// Unlike the sibling tests/integration/laya suite (written before intents/laya existed, so it
// hand-rolled the wire protocol), this suite dogfoods the real intents/jev adapter — the thing
// under test is "does our Client/Classifier actually work against a live Jev endpoint," not the
// raw protocol.
//
// Default target is Jev via OpenRouter (https://openrouter.ai/api, model "~typesafe/jev-latest")
// — the same base URL/model block-builder/internal/decisionmodel's config defaults to
// (BLOCK_BUILDER_DECISIONMODEL_BASE_URL / _MODEL), since that's the one confirmed real-world route
// to a hosted Jev endpoint in this codebase. Override with JEV_TEST_BASE_URL / JEV_TEST_MODEL. The
// API key comes from JEV_TEST_API_KEY, falling back to OPENROUTER_API_KEY — never hardcoded, per
// this repo's credential convention (`pass show openrouter-key`, loaded into the environment by
// whoever runs this suite, not committed anywhere).
//
// Every test skips (not fails) if no key is configured or a real request to the endpoint fails —
// this is a live-system test (go-testing-tiers' boundary rule: hits an already-running external
// system, not infra the test provisions) against a rate-limited, third-party-hosted, largely
// unverified-by-us endpoint (see intents/jev/doc.go's "What's still genuinely not done"), so
// `go test ./...` must stay safe to run without a key and this suite must not fail the build on a
// transient 429/5xx.
package jev_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ezeike/intents/jev"
)

const (
	defaultBaseURL = "https://openrouter.ai/api"
	defaultModel   = "~typesafe/jev-latest"
)

func jevBaseURL() string {
	if v := os.Getenv("JEV_TEST_BASE_URL"); v != "" {
		return v
	}
	return defaultBaseURL
}

func jevModel() string {
	if v := os.Getenv("JEV_TEST_MODEL"); v != "" {
		return v
	}
	return defaultModel
}

func jevAPIKey() string {
	if v := os.Getenv("JEV_TEST_API_KEY"); v != "" {
		return v
	}
	return os.Getenv("OPENROUTER_API_KEY")
}

// requireJev skips the test if no API key is configured, or if a real ping request to the live
// endpoint fails, rather than failing the suite — see this file's header.
func requireJev(t *testing.T) *jev.Client {
	t.Helper()
	key := jevAPIKey()
	if key == "" {
		t.Skip("no Jev API key configured (set JEV_TEST_API_KEY or OPENROUTER_API_KEY)")
	}

	c := jev.NewClient(jevBaseURL(), key)
	c.HTTPClient = &http.Client{Timeout: 30 * time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.SystemOne(ctx, jev.Request{
		State:     "ping",
		Questions: map[string]jev.Question{"ok": {Type: "noul", Instructions: "is this a connectivity check?"}},
		Model:     jevModel(),
	}); err != nil {
		t.Skipf("jev not reachable at %s (model %s): %v", jevBaseURL(), jevModel(), err)
	}
	return c
}
