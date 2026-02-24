package plan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// SummaryOptions configures summary generation.
type SummaryOptions struct {
	PlanDir     string
	PlanName    string
	Meta        *arc.PlanMeta
	PhaseStates map[string]*arc.PhaseState
	ProjectDir  string // for git log to find changed files
}

// GenerateSummary creates a SUMMARY.md artifact in the plan directory.
func GenerateSummary(opts SummaryOptions) (string, error) {
	if opts.Meta == nil {
		return "", fmt.Errorf("meta is required")
	}
	if opts.PhaseStates == nil {
		return "", fmt.Errorf("phase states are required")
	}

	var b strings.Builder

	// Header
	b.WriteString("# Summary: ")
	b.WriteString(opts.PlanName)
	b.WriteString("\n\n")
	b.WriteString("Generated: ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\n\n")

	// Objective from first phase's plan.md
	objective := extractObjective(opts.PlanDir, opts.Meta)
	b.WriteString("## Objective\n\n")
	b.WriteString(objective)
	b.WriteString("\n\n")

	// Completion status
	complete, blocked, deferred := 0, 0, 0
	for _, phase := range opts.Meta.Phases {
		ps := opts.PhaseStates[phase]
		if ps == nil {
			continue
		}
		switch ps.PhaseStatus {
		case "complete":
			complete++
		case "blocked":
			blocked++
		case "deferred":
			deferred++
		}
	}
	total := len(opts.Meta.Phases)

	b.WriteString("## Status\n\n")
	b.WriteString(fmt.Sprintf("- **Phases:** %d/%d complete\n", complete, total))
	if blocked > 0 {
		b.WriteString(fmt.Sprintf("- **Blocked:** %d\n", blocked))
	}
	if deferred > 0 {
		b.WriteString(fmt.Sprintf("- **Deferred:** %d\n", deferred))
	}
	b.WriteString("\n")

	// Per-phase details
	b.WriteString("## Phase Details\n\n")
	var totalTests, totalPassing int
	var totalCost float64

	for _, phase := range opts.Meta.Phases {
		ps := opts.PhaseStates[phase]
		if ps == nil {
			b.WriteString(fmt.Sprintf("### %s\n\n- Status: unknown\n\n", phase))
			continue
		}

		b.WriteString(fmt.Sprintf("### %s\n\n", phase))
		b.WriteString(fmt.Sprintf("- Status: %s\n", ps.PhaseStatus))
		b.WriteString(fmt.Sprintf("- Iterations: %d\n", ps.Iteration.Current))
		b.WriteString(fmt.Sprintf("- Tests: %d/%d\n", ps.TestsPassing, ps.TestsTotal))

		if ps.LastCommit != "" {
			short := ps.LastCommit
			if len(short) > 7 {
				short = short[:7]
			}
			b.WriteString(fmt.Sprintf("- Commit: %s\n", short))
		}

		b.WriteString(fmt.Sprintf("- Cost: $%.4f\n", ps.Usage.CostUSD))

		if ps.PhaseStatus == "blocked" && ps.Blocked.Reason != nil && *ps.Blocked.Reason != "" {
			b.WriteString(fmt.Sprintf("- Reason: %s\n", *ps.Blocked.Reason))
		}

		b.WriteString("\n")

		totalTests += ps.TestsTotal
		totalPassing += ps.TestsPassing
		totalCost += ps.Usage.CostUSD
	}

	// Files changed
	var commits []string
	for _, phase := range opts.Meta.Phases {
		ps := opts.PhaseStates[phase]
		if ps != nil && ps.LastCommit != "" {
			commits = append(commits, ps.LastCommit)
		}
	}

	b.WriteString("## Files Changed\n\n")
	if len(commits) == 0 {
		b.WriteString("No commits recorded\n")
	} else {
		files, _ := CollectChangedFiles(opts.ProjectDir, commits)
		if len(files) == 0 {
			b.WriteString("No commits recorded\n")
		} else {
			for _, f := range files {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
		}
	}
	b.WriteString("\n")

	// Cost summary
	b.WriteString("## Cost\n\n")
	b.WriteString(fmt.Sprintf("- Total: $%.4f\n", totalCost))
	b.WriteString(fmt.Sprintf("- Tests: %d/%d\n", totalPassing, totalTests))
	b.WriteString("\n")

	// Next steps
	if blocked > 0 || deferred > 0 {
		b.WriteString("## Next Steps\n\n")
		for _, phase := range opts.Meta.Phases {
			ps := opts.PhaseStates[phase]
			if ps == nil {
				continue
			}
			if ps.PhaseStatus == "blocked" {
				reason := "unknown reason"
				if ps.Blocked.Reason != nil && *ps.Blocked.Reason != "" {
					reason = *ps.Blocked.Reason
				}
				b.WriteString(fmt.Sprintf("- **%s** (blocked): %s\n", phase, reason))
			}
			if ps.PhaseStatus == "deferred" {
				b.WriteString(fmt.Sprintf("- **%s** (deferred)\n", phase))
			}
		}
		b.WriteString("\n")
	}

	content := b.String()

	// Write to file
	summaryPath := filepath.Join(opts.PlanDir, "SUMMARY.md")
	if err := os.WriteFile(summaryPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing SUMMARY.md: %w", err)
	}

	return content, nil
}

func extractObjective(planDir string, meta *arc.PlanMeta) string {
	if len(meta.Phases) == 0 {
		return "(No objective found)"
	}

	planMDPath := filepath.Join(planDir, "phases", meta.Phases[0], "plan.md")
	data, err := os.ReadFile(planMDPath)
	if err != nil {
		return "(No objective found)"
	}

	content := string(data)
	idx := strings.Index(content, "## Objective")
	if idx == -1 {
		return "(No objective found)"
	}

	after := content[idx+len("## Objective"):]
	after = strings.TrimLeft(after, "\n\r ")

	// Find next heading or end
	nextHeading := strings.Index(after, "\n## ")
	if nextHeading == -1 {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(after[:nextHeading])
}

// CollectChangedFiles uses git log --name-only to find files changed in given commits.
func CollectChangedFiles(projectDir string, commits []string) ([]string, error) {
	if len(commits) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]bool)
	var files []string

	for _, hash := range commits {
		if hash == "" {
			continue
		}

		cmd := exec.Command("git", "log", "--name-only", "--format=", hash, "-n", "1")
		cmd.Dir = projectDir
		out, err := cmd.Output()
		if err != nil {
			continue // skip invalid commits
		}

		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !seen[line] {
				seen[line] = true
				files = append(files, line)
			}
		}
	}

	sort.Strings(files)
	return files, nil
}
