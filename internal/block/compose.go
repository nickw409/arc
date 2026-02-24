package block

import (
	"fmt"
	"strconv"

	"github.com/nwiley/arc/internal/arc"
)

// PipelineStep represents a single step in a workflow pipeline.
type PipelineStep struct {
	Block    string            `yaml:"block"`
	Params   map[string]string `yaml:"params"`
	Parallel *ParallelStep     `yaml:"parallel"`
}

// ParallelStep represents parallel block execution within a pipeline.
type ParallelStep struct {
	Strategy string              `yaml:"strategy"` // "all" or "any"
	Blocks   []ParallelBlockRef  `yaml:"blocks"`
}

// ParallelBlockRef references a block instance within a parallel group.
type ParallelBlockRef struct {
	Name   string            `yaml:"name"`   // instance name
	Block  string            `yaml:"block"`  // block type name
	Params map[string]string `yaml:"params"`
}

// ParallelGroup describes a set of blocks to run concurrently at runtime.
type ParallelGroup struct {
	ForkState string          // synthetic state name triggering parallel execution
	JoinState string          // synthetic state name after parallel completes
	Strategy  string          // "all" or "any"
	Blocks    []ResolvedBlock // blocks to run in parallel
}

// ComposeSequential takes a list of resolved blocks and produces a flat
// workflow. State names are namespaced with the block name (e.g.,
// "adversary-loop.adversary"). Exit points wire to the next block's entry.
func ComposeSequential(blocks []ResolvedBlock) (*arc.Workflow, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks to compose")
	}

	var allStates []arc.StateConfig
	firstEntry := ""

	for i, rb := range blocks {
		resolved := rb.Block
		prefix := rb.Name

		// Determine what the block's exit points should map to
		var nextEntry string
		if i < len(blocks)-1 {
			nextEntry = blocks[i+1].Name + "." + blocks[i+1].Block.Entry
		} else {
			nextEntry = "complete"
		}

		for _, bs := range resolved.States {
			sc := blockStateToConfig(bs, prefix)

			// Wire exit references ($exit_name) to next block or terminal
			if sc.Transition.Branches != nil {
				for verdict, target := range sc.Transition.Branches {
					if isExitRef(target) {
						sc.Transition.Branches[verdict] = nextEntry
					} else {
						// Namespace internal references
						sc.Transition.Branches[verdict] = prefix + "." + target
					}
				}
			}

			allStates = append(allStates, sc)
		}

		if i == 0 {
			firstEntry = prefix + "." + resolved.Entry
		}
	}

	// Add terminal states
	allStates = append(allStates,
		arc.StateConfig{Name: "complete", Description: "Phase completed successfully", Prompt: "prompts/common/complete.md"},
		arc.StateConfig{Name: "blocked", Description: "Phase blocked", Prompt: "prompts/common/blocked.md"},
	)

	wf := &arc.Workflow{
		Name:           blocks[0].Name,
		Version:        1,
		EntryState:     firstEntry,
		TerminalStates: []string{"complete", "blocked"},
		States:         allStates,
	}

	return wf, nil
}

// ComposePipeline handles both sequential and parallel steps.
// Returns the flat workflow for sequential states, plus any parallel groups
// that need runtime handling by the orchestrator.
func ComposePipeline(steps []PipelineStep, blockDefs map[string]*Block) (*arc.Workflow, []ParallelGroup, error) {
	if len(steps) == 0 {
		return nil, nil, fmt.Errorf("empty pipeline")
	}

	var allStates []arc.StateConfig
	var parallelGroups []ParallelGroup
	var firstEntry string
	parallelIdx := 0

	// Pre-resolve all sequential blocks and parallel fork/join points
	type resolvedStep struct {
		// For sequential blocks
		block *ResolvedBlock
		// For parallel groups
		parallel  *ParallelGroup
		forkState string
		joinState string
	}

	var resolved []resolvedStep
	for _, step := range steps {
		if step.Parallel != nil {
			forkName := fmt.Sprintf("_fork_%d", parallelIdx)
			joinName := fmt.Sprintf("_join_%d", parallelIdx)

			var rblocks []ResolvedBlock
			for _, pbr := range step.Parallel.Blocks {
				def, ok := blockDefs[pbr.Block]
				if !ok {
					return nil, nil, fmt.Errorf("parallel block %q not found", pbr.Block)
				}
				resolvedBlock, err := ResolveParams(def, pbr.Params)
				if err != nil {
					return nil, nil, fmt.Errorf("resolving parallel block %q: %w", pbr.Name, err)
				}
				rblocks = append(rblocks, ResolvedBlock{
					Name:   pbr.Name,
					Block:  resolvedBlock,
					Params: pbr.Params,
				})
			}

			pg := ParallelGroup{
				ForkState: forkName,
				JoinState: joinName,
				Strategy:  step.Parallel.Strategy,
				Blocks:    rblocks,
			}

			resolved = append(resolved, resolvedStep{
				parallel:  &pg,
				forkState: forkName,
				joinState: joinName,
			})
			parallelIdx++
		} else {
			def, ok := blockDefs[step.Block]
			if !ok {
				return nil, nil, fmt.Errorf("block %q not found", step.Block)
			}
			resolvedBlock, err := ResolveParams(def, step.Params)
			if err != nil {
				return nil, nil, fmt.Errorf("resolving block %q: %w", step.Block, err)
			}
			name := step.Block
			resolved = append(resolved, resolvedStep{
				block: &ResolvedBlock{
					Name:   name,
					Block:  resolvedBlock,
					Params: step.Params,
				},
			})
		}
	}

	// Wire everything together
	for i, rs := range resolved {
		// Determine next entry point
		var nextEntry string
		if i < len(resolved)-1 {
			next := resolved[i+1]
			if next.block != nil {
				nextEntry = next.block.Name + "." + next.block.Block.Entry
			} else {
				nextEntry = next.forkState
			}
		} else {
			nextEntry = "complete"
		}

		if rs.block != nil {
			// Sequential block
			rb := rs.block
			prefix := rb.Name

			for _, bs := range rb.Block.States {
				sc := blockStateToConfig(bs, prefix)
				if sc.Transition.Branches != nil {
					for verdict, target := range sc.Transition.Branches {
						if isExitRef(target) {
							sc.Transition.Branches[verdict] = nextEntry
						} else {
							sc.Transition.Branches[verdict] = prefix + "." + target
						}
					}
				}
				allStates = append(allStates, sc)
			}

			if firstEntry == "" {
				firstEntry = prefix + "." + rb.Block.Entry
			}
		} else {
			// Parallel group — add synthetic fork/join states
			pg := rs.parallel
			pg.ForkState = rs.forkState
			pg.JoinState = rs.joinState

			// Fork state: the orchestrator recognizes this and starts parallel execution
			allStates = append(allStates, arc.StateConfig{
				Name:        rs.forkState,
				Description: "Fork into parallel blocks",
				Prompt:      "prompts/common/complete.md", // placeholder, never rendered
				Transition:  arc.Transition{Branches: map[arc.Verdict]string{"": rs.joinState}},
			})

			// Join state: transitions to next entry
			allStates = append(allStates, arc.StateConfig{
				Name:        rs.joinState,
				Description: "Join parallel blocks",
				Prompt:      "prompts/common/complete.md",
				Transition:  arc.Transition{Branches: map[arc.Verdict]string{"": nextEntry}},
			})

			parallelGroups = append(parallelGroups, *pg)

			if firstEntry == "" {
				firstEntry = rs.forkState
			}
		}
	}

	// Add terminal states
	allStates = append(allStates,
		arc.StateConfig{Name: "complete", Description: "Phase completed successfully", Prompt: "prompts/common/complete.md"},
		arc.StateConfig{Name: "blocked", Description: "Phase blocked", Prompt: "prompts/common/blocked.md"},
	)

	wf := &arc.Workflow{
		Version:        1,
		EntryState:     firstEntry,
		TerminalStates: []string{"complete", "blocked"},
		States:         allStates,
	}

	return wf, parallelGroups, nil
}

// blockStateToConfig converts a BlockState to an arc.StateConfig with namespaced name.
func blockStateToConfig(bs BlockState, prefix string) arc.StateConfig {
	sc := arc.StateConfig{
		Name:        prefix + "." + bs.Name,
		Description: bs.Description,
		Prompt:      bs.Prompt,
		Verdicts:    bs.Verdicts,
		Agent:       bs.Agent,
	}

	if bs.Constraints != nil {
		maxIter := parseInt(bs.Constraints.MaxIterations)
		if maxIter > 0 {
			sc.Constraints = &arc.ConstraintConfig{MaxIterations: maxIter}
		}
	}

	if bs.Next != nil {
		branches := make(map[arc.Verdict]string, len(bs.Next))
		for k, v := range bs.Next {
			branches[arc.Verdict(k)] = v
		}
		sc.Transition = arc.Transition{Branches: branches}
	}

	return sc
}

// isExitRef returns true if a target is an exit reference (starts with "$").
func isExitRef(target string) bool {
	return len(target) > 0 && target[0] == '$'
}

// ValidateComposition checks that a composed workflow has no wiring errors.
func ValidateComposition(wf *arc.Workflow, blocks []ResolvedBlock) []error {
	var errs []error

	stateNames := make(map[string]bool, len(wf.States))
	for _, s := range wf.States {
		stateNames[s.Name] = true
	}

	// Check all transitions point to valid states
	for _, s := range wf.States {
		if s.Transition.Branches != nil {
			for _, target := range s.Transition.Branches {
				if !stateNames[target] {
					errs = append(errs, fmt.Errorf("state %q references unknown state %q", s.Name, target))
				}
			}
		}
	}

	// Check every non-terminal state can reach a terminal state (reverse BFS)
	terminalSet := make(map[string]bool, len(wf.TerminalStates))
	for _, ts := range wf.TerminalStates {
		terminalSet[ts] = true
	}

	// Build reverse graph
	reverseEdges := make(map[string][]string)
	for _, s := range wf.States {
		if s.Transition.Branches != nil {
			for _, target := range s.Transition.Branches {
				reverseEdges[target] = append(reverseEdges[target], s.Name)
			}
		}
	}

	// BFS backward from terminal states
	canReachTerminal := make(map[string]bool)
	queue := make([]string, 0, len(wf.TerminalStates))
	for _, ts := range wf.TerminalStates {
		canReachTerminal[ts] = true
		queue = append(queue, ts)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, pred := range reverseEdges[current] {
			if !canReachTerminal[pred] {
				canReachTerminal[pred] = true
				queue = append(queue, pred)
			}
		}
	}

	for _, s := range wf.States {
		if !canReachTerminal[s.Name] && !terminalSet[s.Name] {
			errs = append(errs, fmt.Errorf("state %q cannot reach any terminal state", s.Name))
		}
	}

	return errs
}

// intFromString converts a string to int, ignoring errors.
func intFromString(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
