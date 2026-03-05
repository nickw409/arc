package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Worktree represents an isolated git worktree for running agents.
type Worktree struct {
	Branch     string // e.g., "arc/plan-name/phase-name"
	Dir        string // absolute path to worktree directory
	ProjectDir string // original project root
}

// sanitizeRe matches characters that are not allowed in git branch names.
var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9/_.-]`)

// sanitizeBranch converts a string into a valid git branch name.
func sanitizeBranch(s string) string {
	s = sanitizeRe.ReplaceAllString(s, "-")
	// Collapse consecutive dashes/dots
	s = regexp.MustCompile(`[-]{2,}`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-./")
	return s
}

// defaultMinDiskBytes is the minimum free disk space required before creating
// a new worktree (1 GiB).
const defaultMinDiskBytes int64 = 1 << 30

// Create creates a new worktree branch and checks it out in a temp directory.
// Branch is created from current HEAD. If the branch already exists from a
// previous run, Create detects and reuses the existing worktree (preserving
// agent work) or creates a new worktree on the existing branch.
func Create(projectDir, planName, phaseName string) (*Worktree, error) {
	if err := CheckDiskSpace(defaultMinDiskBytes); err != nil {
		return nil, err
	}
	branch := "arc/" + sanitizeBranch(planName)
	if phaseName != "" {
		branch += "/" + sanitizeBranch(phaseName)
	}

	dir, err := os.MkdirTemp("", "arc-worktree-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	cmd := exec.Command("git", "worktree", "add", "-b", branch, dir)
	cmd.Dir = projectDir
	if _, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)

		// Branch already exists — check if a worktree is still attached
		if existingDir := findWorktreeDir(projectDir, branch); existingDir != "" {
			// Scenario A: worktree still exists, reuse it
			return &Worktree{
				Branch:     branch,
				Dir:        existingDir,
				ProjectDir: projectDir,
			}, nil
		}

		// Scenario B: branch exists but worktree was removed — create
		// worktree on the existing branch (without -b)
		dir2, err2 := os.MkdirTemp("", "arc-worktree-*")
		if err2 != nil {
			return nil, fmt.Errorf("creating temp dir: %w", err2)
		}
		cmd2 := exec.Command("git", "worktree", "add", dir2, branch)
		cmd2.Dir = projectDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			os.RemoveAll(dir2)
			return nil, fmt.Errorf("git worktree add (existing branch): %s: %w", strings.TrimSpace(string(out2)), err2)
		}
		return &Worktree{
			Branch:     branch,
			Dir:        dir2,
			ProjectDir: projectDir,
		}, nil
	}

	return &Worktree{
		Branch:     branch,
		Dir:        dir,
		ProjectDir: projectDir,
	}, nil
}

// findWorktreeDir returns the directory of an existing worktree checked out
// on the given branch, or "" if none is found.
func findWorktreeDir(projectDir, branch string) string {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	var currentDir string
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentDir = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			if strings.TrimPrefix(line, "branch refs/heads/") == branch && currentDir != "" {
				return currentDir
			}
		case line == "":
			currentDir = ""
		}
	}
	return ""
}

// Remove cleans up the worktree directory and prunes git worktree metadata.
func Remove(wt *Worktree) error {
	var removeErr error

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", "--force", wt.Dir)
	cmd.Dir = wt.ProjectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		removeErr = fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
		// Fall back to manual cleanup
		if fsErr := os.RemoveAll(wt.Dir); fsErr != nil && removeErr != nil {
			removeErr = fmt.Errorf("git worktree remove and os.RemoveAll both failed: git: %w; fs: %v", removeErr, fsErr)
		} else if fsErr == nil {
			removeErr = nil
		}
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = wt.ProjectDir
		pruneCmd.Run()
	}

	// Delete the branch (best-effort; branch may already be deleted)
	branchCmd := exec.Command("git", "branch", "-D", wt.Branch)
	branchCmd.Dir = wt.ProjectDir
	branchCmd.Run()

	return removeErr
}

// CleanupPlan removes all worktrees associated with a plan.
// Matches branches with prefix "arc/<planName>" (shared) and "arc/<planName>/" (per-phase).
func CleanupPlan(projectDir, planName string) int {
	prefix := "arc/" + sanitizeBranch(planName)

	// Parse `git worktree list --porcelain` to find matching worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	var removed int
	var currentDir, currentBranch string
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentDir = strings.TrimPrefix(line, "worktree ")
			currentBranch = ""
		case strings.HasPrefix(line, "branch refs/heads/"):
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "": // end of entry
			if currentBranch == prefix || strings.HasPrefix(currentBranch, prefix+"/") {
				Remove(&Worktree{
					Branch:     currentBranch,
					Dir:        currentDir,
					ProjectDir: projectDir,
				})
				removed++
			}
			currentDir = ""
			currentBranch = ""
		}
	}
	return removed
}

// MergeBack merges the worktree branch into the current branch.
// Returns the merge commit hash or error if conflicts exist.
func MergeBack(wt *Worktree) (string, error) {
	cmd := exec.Command("git", "merge", "--no-ff", wt.Branch, "-m", fmt.Sprintf("Merge %s", wt.Branch))
	cmd.Dir = wt.ProjectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Abort the merge on conflict
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = wt.ProjectDir
		abortCmd.Run()
		return "", fmt.Errorf("merge conflict: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Get the merge commit hash
	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = wt.ProjectDir
	out, err := hashCmd.Output()
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// OrphanedWorktree describes a git worktree that matches arc branch/path
// patterns but does not correspond to any active plan.
type OrphanedWorktree struct {
	// Dir is the absolute path to the worktree directory.
	Dir string
	// Branch is the git branch checked out in the worktree.
	Branch string
	// Age is how long ago the worktree directory was last modified.
	Age time.Duration
}

// ListOrphaned scans git worktree list for worktrees whose branch matches the
// arc naming convention ("arc/…") but whose directory path does NOT match any
// of the active plan names provided in activePlans. Worktrees under
// ".arc/worktrees/" are also considered arc-managed.
//
// activePlans should be a slice of sanitized plan-name prefixes
// (e.g. "arc/my-plan"). Any worktree whose branch does not start with one of
// those prefixes is treated as orphaned.
func ListOrphaned(projectDir string, activePlans []string) ([]OrphanedWorktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var orphans []OrphanedWorktree
	var currentDir, currentBranch string

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentDir = strings.TrimPrefix(line, "worktree ")
			currentBranch = ""
		case strings.HasPrefix(line, "branch refs/heads/"):
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "": // end of entry
			if currentDir == "" {
				break
			}
			// Only consider arc-managed worktrees.
			isArc := strings.HasPrefix(currentBranch, "arc/") ||
				strings.Contains(currentDir, ".arc/worktrees/")
			if !isArc {
				currentDir = ""
				currentBranch = ""
				break
			}

			// Check if it matches any active plan.
			matched := false
			for _, prefix := range activePlans {
				if currentBranch == prefix || strings.HasPrefix(currentBranch, prefix+"/") {
					matched = true
					break
				}
			}

			if !matched {
				age := worktreeAge(currentDir)
				orphans = append(orphans, OrphanedWorktree{
					Dir:    currentDir,
					Branch: currentBranch,
					Age:    age,
				})
			}

			currentDir = ""
			currentBranch = ""
		}
	}

	return orphans, nil
}

// worktreeAge returns how long ago the worktree directory was last modified.
// Returns 0 on error (e.g. directory no longer exists).
func worktreeAge(dir string) time.Duration {
	info, err := os.Stat(dir)
	if err != nil {
		return 0
	}
	return time.Since(info.ModTime())
}

// CheckDiskSpace checks the available disk space on the filesystem where
// worktrees will be created (os.TempDir). Returns an error if the free space
// is below minBytes.
func CheckDiskSpace(minBytes int64) error {
	var stat syscall.Statfs_t
	dir := os.TempDir()
	if err := syscall.Statfs(dir, &stat); err != nil {
		return fmt.Errorf("statfs %s: %w", dir, err)
	}

	// Bavail is the number of free blocks available to unprivileged users.
	available := int64(stat.Bavail) * stat.Bsize //nolint:unconvert
	if available < minBytes {
		return fmt.Errorf("insufficient disk space: %d bytes available, %d required", available, minBytes)
	}
	return nil
}

// WorktreeMetadata is written to <worktree-dir>/.arc/worktree-meta.json when
// a phase fails, tagging the worktree with the failure reason for later
// inspection (e.g., by arc cleanup).
type WorktreeMetadata struct {
	Plan          string `json:"plan"`
	Phase         string `json:"phase"`
	FailureReason string `json:"failure_reason"`
	Timestamp     string `json:"timestamp"`
}

// WriteMetadata writes a WorktreeMetadata file into <wt.Dir>/.arc/worktree-meta.json.
// The .arc directory is created if it does not exist.
func WriteMetadata(wt *Worktree, reason, phaseName string) error {
	arcDir := filepath.Join(wt.Dir, ".arc")
	if err := os.MkdirAll(arcDir, 0755); err != nil {
		return fmt.Errorf("creating .arc dir in worktree: %w", err)
	}

	// Extract plan name from the branch (arc/<plan>[/<phase>]).
	planName := ""
	parts := strings.SplitN(strings.TrimPrefix(wt.Branch, "arc/"), "/", 2)
	if len(parts) > 0 {
		planName = parts[0]
	}

	meta := WorktreeMetadata{
		Plan:          planName,
		Phase:         phaseName,
		FailureReason: reason,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling worktree metadata: %w", err)
	}

	metaPath := filepath.Join(arcDir, "worktree-meta.json")
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("writing worktree-meta.json: %w", err)
	}
	return nil
}

// ReadMetadata reads the WorktreeMetadata from <dir>/.arc/worktree-meta.json.
// Returns nil, nil if the file does not exist.
func ReadMetadata(dir string) (*WorktreeMetadata, error) {
	metaPath := filepath.Join(dir, ".arc", "worktree-meta.json")
	data, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading worktree-meta.json: %w", err)
	}

	var meta WorktreeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing worktree-meta.json: %w", err)
	}
	return &meta, nil
}
