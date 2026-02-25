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
func Run(ctx context.Context, opts ServerOptions) error {
	s := server.NewMCPServer("arc", opts.Version)

	hctx := &handlerContext{
		projectDir: opts.ProjectDir,
		arcHome:    opts.ArcHome,
		logger:     opts.Logger,
		jobs:       make(map[string]*runJob),
	}
	hctx.registerTools(s)

	srv := server.NewStdioServer(s)
	return srv.Listen(ctx, os.Stdin, os.Stdout)
}
