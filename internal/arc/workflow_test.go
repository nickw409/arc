package arc

import (
	"encoding/json"
	"testing"
)

func TestStateConfigParamsField(t *testing.T) {
	sc := StateConfig{
		Params: map[string]string{"focus": "security"},
	}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var sc2 StateConfig
	if err := json.Unmarshal(data, &sc2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if sc2.Params == nil {
		t.Fatal("expected non-nil Params after unmarshal")
	}
	if sc2.Params["focus"] != "security" {
		t.Fatalf("expected Params[\"focus\"] == \"security\", got %q", sc2.Params["focus"])
	}
}

func TestStateConfigParamsJSONNil(t *testing.T) {
	sc := StateConfig{Params: nil}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// omitempty should omit the params field entirely
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := m["params"]; ok {
		t.Fatalf("expected params to be omitted for nil, got %s", string(m["params"]))
	}
}

func TestStateConfigParamsJSONEmpty(t *testing.T) {
	sc := StateConfig{Params: map[string]string{}}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Go's json omitempty treats empty maps the same as nil — both are omitted.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := m["params"]; ok {
		t.Fatalf("expected empty map params to be omitted with omitempty, got %s", string(m["params"]))
	}
}
