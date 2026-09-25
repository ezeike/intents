package laya_test

import (
	"encoding/json"
	"testing"

	"github.com/ezeike/intents/laya"
)

func TestOrderedCriteria_MarshalJSON_PreservesDeclaredOrder(t *testing.T) {
	c := laya.OrderedCriteria{
		{Key: "zebra", Description: "last alphabetically, first declared"},
		{Key: "apple", Description: "first alphabetically, second declared"},
	}

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	want := `{"zebra":"last alphabetically, first declared","apple":"first alphabetically, second declared"}`
	if string(b) != want {
		t.Fatalf("Marshal() = %s, want %s", b, want)
	}
}

func TestOrderedCriteria_MarshalJSON_EscapesStrings(t *testing.T) {
	c := laya.OrderedCriteria{{Key: "quote", Description: `has "quotes" and \backslash`}}

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("round-trip Unmarshal() error = %v, want nil", err)
	}
	if decoded["quote"] != `has "quotes" and \backslash` {
		t.Fatalf("decoded[quote] = %q, want the original description unescaped", decoded["quote"])
	}
}

func TestOrderedCriteria_Keys_ReturnsDeclaredOrder(t *testing.T) {
	c := laya.OrderedCriteria{{Key: "b"}, {Key: "a"}, {Key: "c"}}

	got := c.Keys()
	want := []string{"b", "a", "c"}
	for i, k := range want {
		if got[i] != k {
			t.Fatalf("Keys()[%d] = %q, want %q", i, got[i], k)
		}
	}
}

func TestOrderedCriteria_MarshalJSON_Empty(t *testing.T) {
	b, err := json.Marshal(laya.OrderedCriteria{})
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	if string(b) != "{}" {
		t.Fatalf("Marshal() = %s, want {}", b)
	}
}
