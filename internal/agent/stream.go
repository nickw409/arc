package agent

import "encoding/json"

// streamMessage is the top-level envelope for stream-json output.
type streamMessage struct {
	Type    string `json:"type"`    // "system", "assistant", "result"
	Subtype string `json:"subtype"` // "init", "success", etc.
}

// streamAssistant is the "assistant" message with per-turn content and usage.
type streamAssistant struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type string `json:"type"` // "text", "tool_use", "tool_result"
			Name string `json:"name"` // tool name (only for tool_use)
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			OutputTokens             int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// streamResult is the final "result" message with total usage and output.
type streamResult struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"` // "success" or "error"
	IsError   bool    `json:"is_error"`
	Result    string  `json:"result"`
	TotalCost float64 `json:"total_cost_usd"`
	NumTurns  int     `json:"num_turns"`
	Usage     struct {
		InputTokens              int `json:"input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
}

// TurnSummary is extracted from a streamAssistant for logging.
type TurnSummary struct {
	Turn                     int
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	Tools                    []string
}

// parseTurnSummary extracts a TurnSummary from a streamAssistant message.
func parseTurnSummary(msg *streamAssistant, turnNum int) TurnSummary {
	ts := TurnSummary{
		Turn:                     turnNum,
		InputTokens:              msg.Message.Usage.InputTokens,
		OutputTokens:             msg.Message.Usage.OutputTokens,
		CacheCreationInputTokens: msg.Message.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     msg.Message.Usage.CacheReadInputTokens,
	}
	seen := map[string]bool{}
	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && c.Name != "" && !seen[c.Name] {
			ts.Tools = append(ts.Tools, c.Name)
			seen[c.Name] = true
		}
	}
	return ts
}

// parseStreamAssistant attempts to parse a JSON line as a streamAssistant.
func parseStreamAssistant(line string) (*streamAssistant, bool) {
	var msg streamAssistant
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, false
	}
	if msg.Type != "assistant" {
		return nil, false
	}
	return &msg, true
}

// parseStreamResult attempts to parse a JSON line as a streamResult.
func parseStreamResult(line string) (*streamResult, bool) {
	var msg streamResult
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, false
	}
	if msg.Type != "result" {
		return nil, false
	}
	return &msg, true
}
