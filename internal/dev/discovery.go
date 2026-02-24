package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
)

// DiscoveryOptions configures the discovery agent.
type DiscoveryOptions struct {
	TaskDescription string
	Prompt          string // discovery agent system prompt
	Model           string // optional model override
	CommandName     string // agent binary name (empty = "claude")
}

// DiscoveryOutput holds the raw and parsed output from the discovery agent.
type DiscoveryOutput struct {
	Result DiscoveryResult
	Usage  arc.Usage
	Raw    string // full agent output for debugging
}

// RunDiscovery spawns a discovery agent that explores the codebase and
// returns a structured analysis of the task.
// The agent is spawned with Read-only tools (Glob, Grep, Read, LS) and
// a 180-second timeout. Its output is parsed from a ```json code fence.
func RunDiscovery(ctx context.Context, opts DiscoveryOptions) (*DiscoveryOutput, error) {
	prompt := opts.Prompt
	if opts.TaskDescription != "" {
		if prompt != "" {
			prompt += "\n\n" + opts.TaskDescription
		} else {
			prompt = opts.TaskDescription
		}
	}

	spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Glob", "Grep", "LS"},
		Timeout:      180 * time.Second,
		Model:        opts.Model,
		CommandName:  opts.CommandName,
	})
	if err != nil {
		return nil, err
	}

	parsed, err := ParseDiscoveryOutput(spawnResult.Output)
	if err != nil {
		return nil, err
	}

	return &DiscoveryOutput{
		Result: *parsed,
		Usage:  spawnResult.Usage,
		Raw:    spawnResult.Output,
	}, nil
}

// ParseDiscoveryOutput extracts a DiscoveryResult from raw agent output.
// It finds the first ```json code fence, unmarshals the JSON, and validates
// required fields. Returns an error if no JSON block is found or if the
// JSON is malformed.
func ParseDiscoveryOutput(output string) (*DiscoveryResult, error) {
	lines := strings.Split(output, "\n")

	var jsonLines []string
	inBlock := false
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inBlock {
			if trimmed == "```" {
				found = true
				break
			}
			jsonLines = append(jsonLines, line)
		} else if strings.HasPrefix(trimmed, "```") {
			rest := strings.TrimSpace(trimmed[3:])
			if strings.EqualFold(rest, "json") {
				inBlock = true
				jsonLines = nil
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("no JSON block found in agent output")
	}

	jsonStr := strings.Join(jsonLines, "\n")

	// Use a raw struct to control validation order (task_summary before complexity).
	var raw struct {
		TaskSummary     string    `json:"task_summary"`
		Complexity      string    `json:"complexity"`
		Reasoning       string    `json:"reasoning"`
		RelevantFiles   []FileRef `json:"relevant_files"`
		Requirements    []string  `json:"requirements"`
		Approach        string    `json:"approach"`
		WorkflowType    string    `json:"workflow_type"`
		SuggestedPhases []PhaseSpec `json:"suggested_phases"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal discovery output: %w", err)
	}

	if strings.TrimSpace(raw.TaskSummary) == "" {
		return nil, fmt.Errorf("invalid discovery output: task_summary is required")
	}

	complexity := TaskComplexity(raw.Complexity)
	if !complexity.IsValid() {
		return nil, fmt.Errorf("invalid discovery output: complexity %q is not valid", raw.Complexity)
	}

	if strings.TrimSpace(raw.WorkflowType) == "" {
		return nil, fmt.Errorf("invalid discovery output: workflow_type is required")
	}

	result := &DiscoveryResult{
		TaskSummary:     raw.TaskSummary,
		Complexity:      complexity,
		Reasoning:       raw.Reasoning,
		RelevantFiles:   raw.RelevantFiles,
		Requirements:    raw.Requirements,
		Approach:        raw.Approach,
		WorkflowType:    raw.WorkflowType,
		SuggestedPhases: raw.SuggestedPhases,
	}

	if result.RelevantFiles == nil {
		result.RelevantFiles = []FileRef{}
	}
	if result.Requirements == nil {
		result.Requirements = []string{}
	}
	if result.SuggestedPhases == nil {
		result.SuggestedPhases = []PhaseSpec{}
	}

	return result, nil
}
