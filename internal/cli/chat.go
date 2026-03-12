package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/nwiley/arc/internal/resources"
	"github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
	var model string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Launch an interactive Claude session with Arc context",
		Long:  "Launches an interactive Claude session with the Arc guide injected as system context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Find claude binary
			claudeBinary, err := exec.LookPath("claude")
			if err != nil {
				return fmt.Errorf("claude CLI not found in PATH: %w", err)
			}

			// Load arc guide as system context
			guide, err := resources.GuideBytes("guide.md")
			if err != nil {
				return fmt.Errorf("loading arc guide: %w", err)
			}

			// Build claude args
			claudeArgs := []string{"claude", "--append-system-prompt", string(guide)}
			if model != "" {
				claudeArgs = append(claudeArgs, "--model", model)
			}

			// Replace process with claude
			return syscall.Exec(claudeBinary, claudeArgs, os.Environ())
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Model to use for the Claude session")
	return cmd
}
