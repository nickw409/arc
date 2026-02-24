package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
)

// ArchitectOptions configures the architect agent spawning.
type ArchitectOptions struct {
	Discovery   *DiscoveryResult
	Model       string // optional model override
	CommandName string // agent binary name (empty = "claude")
	Interactive bool   // if true, caller handles selection; if false, auto-select pragmatic
}

// ArchitectOutput holds results from all architect agents.
type ArchitectOutput struct {
	Proposals []ArchitectProposal
	Selected  *ArchitectProposal // the chosen proposal
	Usage     arc.Usage
}

// RunArchitects spawns 3 architect agents in parallel (minimal, clean, pragmatic),
// each with a different optimization target. Parses their outputs and returns
// all proposals.
func RunArchitects(ctx context.Context, opts ArchitectOptions) (*ArchitectOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	discoveryJSON, err := json.Marshal(opts.Discovery)
	if err != nil {
		return nil, fmt.Errorf("marshal discovery: %w", err)
	}

	approaches := []string{"minimal", "clean", "pragmatic"}

	type result struct {
		proposal *ArchitectProposal
		usage    arc.Usage
		err      error
	}

	results := make([]result, len(approaches))
	var wg sync.WaitGroup

	for i, approach := range approaches {
		wg.Add(1)
		go func(idx int, appr string) {
			defer wg.Done()

			prompt := fmt.Sprintf("You are an architect agent designing a %s approach.\n\nDiscovery:\n%s\n\nDesign a workflow with the %s optimization target. Output your proposal as a JSON block in ```json fences.", appr, string(discoveryJSON), appr)

			spawnOpts := agent.SpawnOptions{
				Prompt:       prompt,
				AllowedTools: []string{"Read", "Glob", "Grep"},
				MaxTurns:     10,
				CommandName:  opts.CommandName,
			}
			if opts.Model != "" {
				spawnOpts.Model = opts.Model
			}

			spawnResult, err := agent.Spawn(ctx, spawnOpts)
			if err != nil {
				results[idx] = result{err: err}
				return
			}

			proposal, err := ParseArchitectOutput(spawnResult.Output)
			if err != nil {
				results[idx] = result{err: err, usage: spawnResult.Usage}
				return
			}

			results[idx] = result{proposal: proposal, usage: spawnResult.Usage}
		}(i, approach)
	}

	wg.Wait()

	output := &ArchitectOutput{}
	allFailed := true

	for _, r := range results {
		output.Usage.InputTokens += r.usage.InputTokens
		output.Usage.OutputTokens += r.usage.OutputTokens
		output.Usage.CacheCreationInputTokens += r.usage.CacheCreationInputTokens
		output.Usage.CacheReadInputTokens += r.usage.CacheReadInputTokens
		output.Usage.CostUSD += r.usage.CostUSD

		if r.proposal != nil {
			output.Proposals = append(output.Proposals, *r.proposal)
			allFailed = false
		}
	}

	if allFailed {
		return nil, fmt.Errorf("all architect agents failed")
	}

	if !opts.Interactive {
		output.Selected = SelectProposal(output.Proposals)
	}

	return output, nil
}

// ParseArchitectOutput extracts an ArchitectProposal from raw agent output.
func ParseArchitectOutput(output string) (*ArchitectProposal, error) {
	// Find the last ```json block
	jsonStart := -1
	jsonEnd := -1

	searchFrom := 0
	for {
		idx := strings.Index(output[searchFrom:], "```json")
		if idx == -1 {
			break
		}
		absStart := searchFrom + idx + len("```json")

		// Find matching closing ```
		closeIdx := strings.Index(output[absStart:], "```")
		if closeIdx == -1 {
			break
		}

		jsonStart = absStart
		jsonEnd = absStart + closeIdx
		searchFrom = absStart + closeIdx + 3
	}

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON block found in architect output")
	}

	jsonStr := strings.TrimSpace(output[jsonStart:jsonEnd])

	var proposal ArchitectProposal
	if err := json.Unmarshal([]byte(jsonStr), &proposal); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from architect output: %w", err)
	}

	if proposal.Name == "" {
		return nil, fmt.Errorf("invalid proposal: name is empty")
	}
	if len(proposal.SuggestedPhases) == 0 {
		return nil, fmt.Errorf("invalid proposal: SuggestedPhases cannot be empty")
	}
	for _, phase := range proposal.SuggestedPhases {
		if _, ok := proposal.PlanContent[phase.Name]; !ok {
			return nil, fmt.Errorf("invalid proposal: missing plan content for phase %q (PlanContent must have entry for each phase)", phase.Name)
		}
	}

	return &proposal, nil
}

// SelectProposal picks the best proposal from the list.
func SelectProposal(proposals []ArchitectProposal) *ArchitectProposal {
	if len(proposals) == 0 {
		return nil
	}

	for i := range proposals {
		if proposals[i].Name == "pragmatic" {
			return &proposals[i]
		}
	}

	return &proposals[0]
}
