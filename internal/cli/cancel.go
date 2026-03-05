package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <plan-name>",
		Short: "Stop a running orchestrator for a plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			planDir := filepath.Join(".plans", "active", planName)

			pidPath := filepath.Join(planDir, "orchestrator.pid")
			pidData, err := os.ReadFile(pidPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no orchestrator PID file found for plan %q — is it running?", planName)
				}
				return fmt.Errorf("reading PID file: %w", err)
			}

			pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
			if err != nil {
				return fmt.Errorf("invalid PID in file: %w", err)
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				os.Remove(pidPath)
				return fmt.Errorf("process %d not found: %w", pid, err)
			}

			// Check if the process is still alive before sending signals.
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				os.Remove(pidPath)
				return fmt.Errorf("orchestrator (PID %d) is not running", pid)
			}

			// Send SIGTERM and give the process a moment to shut down gracefully.
			fmt.Printf("Sending SIGTERM to orchestrator (PID %d)...\n", pid)
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
			}

			// Poll for up to 5 seconds for graceful exit.
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(200 * time.Millisecond)
				if err := proc.Signal(syscall.Signal(0)); err != nil {
					// Process exited.
					os.Remove(pidPath)
					fmt.Println("Orchestrator stopped.")
					return nil
				}
			}

			// Still alive — send SIGKILL.
			fmt.Printf("Orchestrator did not stop in time; sending SIGKILL to PID %d...\n", pid)
			if err := proc.Signal(syscall.SIGKILL); err != nil {
				return fmt.Errorf("sending SIGKILL to PID %d: %w", pid, err)
			}

			os.Remove(pidPath)
			fmt.Println("Orchestrator killed.")
			return nil
		},
	}
}
