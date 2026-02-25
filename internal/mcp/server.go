package mcp

import (
	"context"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

// ServerOptions configures the MCP server.
type ServerOptions struct {
	ProjectDir string
	ArcHome    string
	Logger     *slog.Logger
	Version    string
}

// Run starts the MCP server on stdio, blocking until ctx is cancelled or stdin closes.
// On shutdown, it cancels running jobs and waits for them to clean up child processes.
func Run(ctx context.Context, opts ServerOptions) error {
	s := server.NewMCPServer("arc", opts.Version)

	// Create a cancellable context for all background jobs. This ensures jobs
	// are cancelled when the server shuts down (stdin EOF), not just on SIGINT/SIGTERM.
	jobsCtx, jobsCancel := context.WithCancel(ctx)
	defer jobsCancel()

	hctx := &handlerContext{
		projectDir: opts.ProjectDir,
		arcHome:    opts.ArcHome,
		logger:     opts.Logger,
		jobs:       make(map[string]*runJob),
		jobsCtx:    jobsCtx,
	}
	hctx.registerTools(s)

	srv := server.NewStdioServer(s)
	err := srv.Listen(ctx, os.Stdin, os.Stdout)

	// Listen returned (stdin EOF or ctx cancelled). Cancel all running jobs
	// and wait for them to clean up child processes before the process exits.
	jobsCancel()
	hctx.drainJobs(opts.Logger)

	return err
}
