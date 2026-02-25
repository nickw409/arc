package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
)

// ReviewOptions configures the code review agent.
type ReviewOptions struct {
	PlanDir     string
	ProjectDir  string
	Diff        string
	PlanMD      string
	Discovery   *DiscoveryResult
	Model       string
	CommandName string
}

// ReviewIssue represents a single code review finding.
type ReviewIssue struct {
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// CodeReviewOutput holds the parsed output from a code review agent.
type CodeReviewOutput struct {
	Issues  []ReviewIssue `json:"issues"`
	Summary string        `json:"summary"`
	Usage   arc.Usage     `json:"-"`
}

// RunCodeReview spawns a code review agent that examines the diff produced
// during orchestration and returns structured feedback.
func RunCodeReview(ctx context.Context, opts ReviewOptions) (*CodeReviewOutput, error) {
	tmplBytes, err := resources.PromptBytes("dev/reviewer.md")
	if err != nil {
		return nil, fmt.Errorf("loading reviewer template: %w", err)
	}

	rendered, err := prompt.RenderString(string(tmplBytes), prompt.TemplateContext{
		PlanMD: opts.PlanMD,
		Params: map[string]string{
			"diff": opts.Diff,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering reviewer template: %w", err)
	}

	spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       rendered,
		AllowedTools: []string{"Read", "Glob", "Grep"},
		MaxTurns:     15,
		Timeout:      180 * time.Second,
		Model:        opts.Model,
		CommandName:  opts.CommandName,
		WorkingDir:   opts.ProjectDir,
	})
	if err != nil {
		return nil, err
	}

	parsed, err := ParseReviewOutput(spawnResult.Output)
	if err != nil {
		return nil, err
	}

	parsed.Usage = spawnResult.Usage
	return parsed, nil
}

// ParseReviewOutput extracts a CodeReviewOutput from raw agent output.
// It finds the last ```json code fence, unmarshals the JSON, and validates
// the structure.
func ParseReviewOutput(output string) (*CodeReviewOutput, error) {
	jsonStart := -1
	jsonEnd := -1

	searchFrom := 0
	for {
		idx := strings.Index(output[searchFrom:], "```json")
		if idx == -1 {
			break
		}
		absStart := searchFrom + idx + len("```json")

		closeIdx := strings.Index(output[absStart:], "```")
		if closeIdx == -1 {
			break
		}

		jsonStart = absStart
		jsonEnd = absStart + closeIdx
		searchFrom = absStart + closeIdx + 3
	}

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON block found in review output")
	}

	jsonStr := strings.TrimSpace(output[jsonStart:jsonEnd])

	var result CodeReviewOutput
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from review output: %w", err)
	}

	if result.Issues == nil {
		result.Issues = []ReviewIssue{}
	}

	return &result, nil
}
