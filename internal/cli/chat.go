package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/nwiley/arc/internal/resources"
	"github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
	var model string
	var noRegister bool

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Launch an interactive Claude session with Arc tools",
		Long:  "Registers the Arc MCP server with Claude Code and launches an interactive session with Arc tools available.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve arc binary path
			arcBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving arc binary: %w", err)
			}
			arcBinary, err = filepath.EvalSymlinks(arcBinary)
			if err != nil {
				return fmt.Errorf("resolving arc symlink: %w", err)
			}

			// Find claude binary
			claudeBinary, err := exec.LookPath("claude")
			if err != nil {
				return fmt.Errorf("claude CLI not found in PATH: %w", err)
			}

			// Register MCP server if not already present (unless --no-register)
			if !noRegister {
				if err := exec.Command(claudeBinary, "mcp", "get", "arc").Run(); err != nil {
					register := exec.Command(claudeBinary, "mcp", "add", "--scope", "local", "arc", "--", arcBinary, "serve")
					register.Stderr = os.Stderr
					if err := register.Run(); err != nil {
						return fmt.Errorf("registering MCP server: %w", err)
					}
				}
			}

			// Load agent instructions
			instructions, err := resources.GuideBytes("chat-agent.md")
			if err != nil {
				return fmt.Errorf("loading chat agent guide: %w", err)
			}

			// Build claude args
			claudeArgs := []string{"claude", "--append-system-prompt", string(instructions)}
			if model != "" {
				claudeArgs = append(claudeArgs, "--model", model)
			}

			// Replace process with claude
			return syscall.Exec(claudeBinary, claudeArgs, os.Environ())
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Model to use for the Claude session")
	cmd.Flags().BoolVar(&noRegister, "no-register", false, "Skip MCP server registration (if already registered)")
	return cmd
}
