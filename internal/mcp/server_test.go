package mcp

import (
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterTools(t *testing.T) {
	s := server.NewMCPServer("arc", "test")
	hctx := &handlerContext{
		projectDir: "/tmp/test",
		arcHome:    "/tmp/arc",
		logger:     slog.Default(),
		jobs:       make(map[string]*runJob),
	}
	hctx.registerTools(s)

	tools := s.ListTools()
	if len(tools) != 17 {
		names := make([]string, 0, len(tools))
		for name := range tools {
			names = append(names, name)
		}
		t.Fatalf("expected 17 tools, got %d: %v", len(tools), names)
	}

	expected := []string{
		"arc_status",
		"arc_plan",
		"arc_run",
		"arc_iterate",
		"arc_review",
		"arc_manage",
		"arc_guide",
		"arc_list_plans",
		"arc_archive",
		"arc_run_status",
		"arc_run_cancel",
		"arc_plan_add_phase",
		"arc_plan_remove_phase",
		"arc_plan_update_phase",
		"arc_plan_update_gate",
		"arc_plan_update_deps",
		"arc_plan_show_spec",
	}

	for _, name := range expected {
		if _, ok := tools[name]; !ok {
			t.Errorf("expected tool %q not registered", name)
		}
	}
}

func TestServerCreation(t *testing.T) {
	s := server.NewMCPServer("arc", "1.0.0")
	if s == nil {
		t.Fatal("NewMCPServer returned nil")
	}

	srv := server.NewStdioServer(s)
	if srv == nil {
		t.Fatal("NewStdioServer returned nil")
	}
}
