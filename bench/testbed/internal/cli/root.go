package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/tkit/internal/store"
	"github.com/spf13/cobra"
)

var taskStore *store.Store

func storePath() string {
	if p := os.Getenv("TKIT_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tkit.json"
	}
	return filepath.Join(home, ".tkit.json")
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tkit",
		Short: "A simple task tracker CLI",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			taskStore = store.New(storePath())
		},
	}

	root.AddCommand(newAddCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newCompleteCmd())
	root.AddCommand(newDeleteCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newStatsCmd())

	return root
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
