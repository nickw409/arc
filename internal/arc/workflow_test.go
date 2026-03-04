package arc

import (
	"encoding/json"
	"testing"
)

func TestStateConfigParamsField(t *testing.T) {
	sc := StateConfig{
		Name:   "test",
		Params: map[string]string{"focus": "security"},
	}

	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var sc2 StateConfig
	if err := json.Unmarshal(data, &sc2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if sc2.Params == nil {
		t.Fatal("expected non-nil Params after unmarshal")
	}
	if sc2.Params["focus"] != "security" {
		t.Errorf("Params[focus] = %q, want %q", sc2.Params["focus"], "security")
	}
}

func TestStateConfigParamsJSONNil(t *testing.T) {
	sc := StateConfig{Name: "test", Params: nil}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// omitempty should omit the field
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}
	if _, ok := m["params"]; ok {
		t.Error("expected params field to be omitted for nil, but it was present")
	}
}

func TestStateConfigParamsJSONEmpty(t *testing.T) {
	sc := StateConfig{Name: "test", Params: map[string]string{}}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Go's json omitempty treats empty maps as "empty", so it is omitted.
	// Verify round-trip still works: unmarshal back and check Params is nil
	// (since the field was omitted).
	var sc2 StateConfig
	if err := json.Unmarshal(data, &sc2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	// The empty map was omitted by omitempty, so after unmarshal Params is nil.
	// This is acceptable behavior — callers should treat nil and empty map equivalently.
	if sc2.Params != nil && len(sc2.Params) != 0 {
		t.Errorf("expected nil or empty Params after round-trip, got %v", sc2.Params)
	}
}
