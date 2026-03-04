package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nwiley/arc/internal/mcp"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "serve",
		Short:  "Start the MCP server (stdio transport)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return err
			}

			arcHome := resolveArcHome()

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			return mcp.Run(ctx, mcp.ServerOptions{
				ProjectDir: projectRoot,
				ArcHome:    arcHome,
				Logger:     logger,
				Version:    Version,
			})
		},
	}
}
