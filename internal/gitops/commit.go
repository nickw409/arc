package gitops

import "github.com/nwiley/arc/internal/config"

// CommitOptions configures a git commit operation.
type CommitOptions struct {
	Message string
	Config  *config.Config
}

// Commit stages all changes and creates a commit.
func Commit(opts CommitOptions) (string, error) {
	panic("not implemented")
}

// FormatCommitMessage formats a commit message based on config style.
func FormatCommitMessage(style, commitType, scope, description string) string {
	panic("not implemented")
}
