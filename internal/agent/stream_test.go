package agent

import (
	"encoding/json"
	"testing"
)

func TestParseTurnSummary(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","name":""},{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Edit"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}}`
	var msg streamAssistant
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ts := parseTurnSummary(&msg, 1)

	if ts.Turn != 1 {
		t.Fatalf("Turn=%d, want 1", ts.Turn)
	}
	if ts.InputTokens != 100 {
		t.Fatalf("InputTokens=%d, want 100", ts.InputTokens)
	}
	if ts.OutputTokens != 50 {
		t.Fatalf("OutputTokens=%d, want 50", ts.OutputTokens)
	}
	if ts.CacheCreationInputTokens != 10 {
		t.Fatalf("CacheCreationInputTokens=%d, want 10", ts.CacheCreationInputTokens)
	}
	if ts.CacheReadInputTokens != 20 {
		t.Fatalf("CacheReadInputTokens=%d, want 20", ts.CacheReadInputTokens)
	}
	if len(ts.Tools) != 2 {
		t.Fatalf("Tools=%v, want [Read Edit]", ts.Tools)
	}
	if ts.Tools[0] != "Read" || ts.Tools[1] != "Edit" {
		t.Fatalf("Tools=%v, want [Read Edit]", ts.Tools)
	}
}

func TestParseTurnSummaryDedupTools(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Edit"},{"type":"tool_use","name":"Read"}],"usage":{"input_tokens":10,"output_tokens":5}}}`
	var msg streamAssistant
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ts := parseTurnSummary(&msg, 2)

	if len(ts.Tools) != 2 {
		t.Fatalf("Tools=%v, want [Read Edit] (deduped)", ts.Tools)
	}
	if ts.Tools[0] != "Read" || ts.Tools[1] != "Edit" {
		t.Fatalf("Tools=%v, want [Read Edit]", ts.Tools)
	}
}

func TestParseTurnSummaryNoTools(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","name":""}],"usage":{"input_tokens":10,"output_tokens":5}}}`
	var msg streamAssistant
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ts := parseTurnSummary(&msg, 1)

	if len(ts.Tools) != 0 {
		t.Fatalf("Tools=%v, want empty", ts.Tools)
	}
}

func TestParseStreamResult(t *testing.T) {
	line := `{"type":"result","subtype":"success","is_error":false,"result":"task complete","total_cost_usd":0.05,"num_turns":3,"usage":{"input_tokens":500,"output_tokens":200,"cache_creation_input_tokens":30,"cache_read_input_tokens":40}}`
	res, ok := parseStreamResult(line)
	if !ok {
		t.Fatal("parseStreamResult returned false")
	}
	if res.Type != "result" {
		t.Fatalf("Type=%q, want result", res.Type)
	}
	if res.Subtype != "success" {
		t.Fatalf("Subtype=%q, want success", res.Subtype)
	}
	if res.IsError {
		t.Fatal("IsError=true, want false")
	}
	if res.Result != "task complete" {
		t.Fatalf("Result=%q, want 'task complete'", res.Result)
	}
	if res.TotalCost != 0.05 {
		t.Fatalf("TotalCost=%f, want 0.05", res.TotalCost)
	}
	if res.NumTurns != 3 {
		t.Fatalf("NumTurns=%d, want 3", res.NumTurns)
	}
	if res.Usage.InputTokens != 500 {
		t.Fatalf("Usage.InputTokens=%d, want 500", res.Usage.InputTokens)
	}
	if res.Usage.OutputTokens != 200 {
		t.Fatalf("Usage.OutputTokens=%d, want 200", res.Usage.OutputTokens)
	}
}

func TestParseStreamResultError(t *testing.T) {
	line := `{"type":"result","subtype":"error","is_error":true,"result":"API rate limit exceeded","total_cost_usd":0.01,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":10}}`
	res, ok := parseStreamResult(line)
	if !ok {
		t.Fatal("parseStreamResult returned false")
	}
	if !res.IsError {
		t.Fatal("IsError=false, want true")
	}
	if res.Subtype != "error" {
		t.Fatalf("Subtype=%q, want error", res.Subtype)
	}
	if res.Result != "API rate limit exceeded" {
		t.Fatalf("Result=%q, want error message", res.Result)
	}
}

func TestParseTurnSummaryMissingContent(t *testing.T) {
	// message.content is missing entirely
	line := `{"type":"assistant","message":{"usage":{"input_tokens":10,"output_tokens":5}}}`
	var msg streamAssistant
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ts := parseTurnSummary(&msg, 1)

	if len(ts.Tools) != 0 {
		t.Fatalf("Tools=%v, want empty for missing content", ts.Tools)
	}
	if ts.InputTokens != 10 {
		t.Fatalf("InputTokens=%d, want 10", ts.InputTokens)
	}
}

func TestStreamMissingFields(t *testing.T) {
	// Missing usage and result fields
	line := `{"type":"result","subtype":"success"}`
	res, ok := parseStreamResult(line)
	if !ok {
		t.Fatal("parseStreamResult returned false")
	}
	if res.Result != "" {
		t.Fatalf("Result=%q, want empty", res.Result)
	}
	if res.Usage.InputTokens != 0 {
		t.Fatalf("Usage.InputTokens=%d, want 0", res.Usage.InputTokens)
	}
	if res.TotalCost != 0 {
		t.Fatalf("TotalCost=%f, want 0", res.TotalCost)
	}
}

func TestParseStreamAssistantInvalidJSON(t *testing.T) {
	_, ok := parseStreamAssistant("{invalid json")
	if ok {
		t.Fatal("expected false for invalid JSON")
	}
}

func TestParseStreamResultInvalidJSON(t *testing.T) {
	_, ok := parseStreamResult("{invalid json")
	if ok {
		t.Fatal("expected false for invalid JSON")
	}
}
