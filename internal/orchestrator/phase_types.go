package orchestrator

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/gitops"
	"github.com/nwiley/arc/internal/resources"
)

// stringOrDefault returns s if non-nil, otherwise def.
func stringOrDefault(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

// phaseObjective reads the plan.md to extract a short commit description.
func phaseObjective(opts RunPhaseOptions) string {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)
	data, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	if err != nil {
		return "implement phase"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if len(line) > 72 {
			line = line[:72]
		}
		return strings.ToLower(line)
	}
	return "implement phase"
}

// RunPhaseOptions configures execution of a single phase.
type RunPhaseOptions struct {
	PlanName    string
	PhaseName   string
	PlansDir    string
	ArcHome     string
	ProjectDir  string              // working directory for git commits; empty uses process cwd
	Config      *config.Config
	Logger      *slog.Logger
	UseWorktree bool                // if true, create a per-phase worktree
	WorkingDir  string              // override working directory for agents (set by worktree)
	ChatMode    bool                // if true, block immediately instead of retrying on failure
	Resolver    *resources.Resolver // reserved for future use
	PlanLogger  *PlanLogger         // structured JSONL logger; nil disables structured logging
}

// commitPhase creates a git commit for the phase.
// If dir is provided, it overrides ProjectDir for the commit.
func commitPhase(opts RunPhaseOptions, commitType, description string, dir ...string) (string, error) {
	style := "conventional"
	if opts.Config != nil {
		style = opts.Config.Git.CommitStyle
	}

	commitDir := opts.ProjectDir
	if len(dir) > 0 && dir[0] != "" {
		commitDir = dir[0]
	}

	msg := gitops.FormatCommitMessage(style, commitType, opts.PhaseName, description)
	return gitops.Commit(gitops.CommitOptions{
		Message: msg,
		Dir:     commitDir,
		Config:  opts.Config,
	})
}

// discoverNewTestFiles finds test files in dir that are not in existing.
func discoverNewTestFiles(dir string, existing []string) []string {
	existingSet := make(map[string]bool, len(existing))
	for _, f := range existing {
		existingSet[f] = true
	}

	var newFiles []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		if !existingSet[rel] {
			newFiles = append(newFiles, rel)
		}
		return nil
	})
	return newFiles
}
