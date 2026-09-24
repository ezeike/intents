//go:build integration

package laya_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestHealth_LiveServer_ReturnsOKStatus(t *testing.T) {
	base := requireLaya(t)

	resp, err := httpClient.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var h healthResponse
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatalf("decode health response: %v (body: %s)", err, data)
	}

	if h.Status != "ok" {
		t.Errorf("status = %q, want %q", h.Status, "ok")
	}
	if len(h.Loaded) == 0 {
		t.Errorf("loaded = %v, want at least one checkpoint resident", h.Loaded)
	}
	if h.Device == "" {
		t.Errorf("device = %q, want a non-empty value", h.Device)
	}
}
