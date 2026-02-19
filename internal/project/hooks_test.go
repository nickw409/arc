package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallGitHooksNew(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	if err := InstallGitHooks(dir); err != nil {
		t.Fatalf("InstallGitHooks error: %v", err)
	}

	// pre-commit should exist
	preCommitPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	info, err := os.Stat(preCommitPath)
	if os.IsNotExist(err) {
		t.Fatal("pre-commit hook should be created")
	}
	if err != nil {
		t.Fatalf("Stat pre-commit error: %v", err)
	}

	// Check executable
	if info.Mode()&0111 == 0 {
		t.Fatal("pre-commit hook should be executable")
	}

	// Verify content contains arc marker
	data, err := os.ReadFile(preCommitPath)
	if err != nil {
		t.Fatalf("ReadFile pre-commit error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pre-commit hook should not be empty")
	}
}

func TestInstallGitHooksExisting(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Write existing non-arc pre-commit
	existingContent := "#!/bin/bash\necho 'my custom hook'\n"
	writeFile(t, filepath.Join(hooksDir, "pre-commit"), existingContent)
	if err := os.Chmod(filepath.Join(hooksDir, "pre-commit"), 0755); err != nil {
		t.Fatalf("chmod error: %v", err)
	}

	if err := InstallGitHooks(dir); err != nil {
		t.Fatalf("InstallGitHooks error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(hooksDir, "pre-commit"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	content := string(data)

	// Original content preserved
	if !strings.Contains(content, "my custom hook") {
		t.Fatal("original hook content should be preserved")
	}
	// Arc section appended
	if !strings.Contains(content, "# ARC HOOKS") {
		t.Fatal("arc section should be appended")
	}
}

func TestInstallGitHooksAlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Write pre-commit with ARC HOOKS marker
	content := "#!/bin/bash\n# ARC HOOKS\necho 'arc stuff'\n"
	writeFile(t, filepath.Join(hooksDir, "pre-commit"), content)
	if err := os.Chmod(filepath.Join(hooksDir, "pre-commit"), 0755); err != nil {
		t.Fatalf("chmod error: %v", err)
	}

	if err := InstallGitHooks(dir); err != nil {
		t.Fatalf("InstallGitHooks error: %v", err)
	}

	// Content should be unchanged (idempotent)
	data, err := os.ReadFile(filepath.Join(hooksDir, "pre-commit"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != content {
		t.Fatal("pre-commit should not be modified when ARC HOOKS already present")
	}
}

func TestInstallGitHooksDirMissing(t *testing.T) {
	dir := t.TempDir()
	// .git exists but .git/hooks does not
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	if err := InstallGitHooks(dir); err != nil {
		t.Fatalf("InstallGitHooks error: %v", err)
	}

	// hooks dir should have been created
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); os.IsNotExist(err) {
		t.Fatal("pre-commit hook should be created even when .git/hooks/ didn't exist")
	}
}

func TestInstallGitHooksCommitMsg(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	if err := InstallGitHooks(dir); err != nil {
		t.Fatalf("InstallGitHooks error: %v", err)
	}

	// commit-msg should also be created
	commitMsgPath := filepath.Join(dir, ".git", "hooks", "commit-msg")
	info, err := os.Stat(commitMsgPath)
	if os.IsNotExist(err) {
		t.Fatal("commit-msg hook should be created")
	}
	if err != nil {
		t.Fatalf("Stat commit-msg error: %v", err)
	}

	// Check executable
	if info.Mode()&0111 == 0 {
		t.Fatal("commit-msg hook should be executable (0755)")
	}
}

func TestInstallGitHooksPermissionDenied(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Create read-only pre-commit (non-arc)
	preCommitPath := filepath.Join(hooksDir, "pre-commit")
	writeFile(t, preCommitPath, "#!/bin/bash\necho 'existing'\n")
	if err := os.Chmod(preCommitPath, 0444); err != nil {
		t.Fatalf("chmod error: %v", err)
	}
	// Restore permissions on cleanup so TempDir can be removed
	t.Cleanup(func() {
		os.Chmod(preCommitPath, 0644)
	})

	err := InstallGitHooks(dir)
	if err == nil {
		t.Fatal("expected error for read-only hook file, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "permission")
	}
}

func TestInstallClaudeHooksNew(t *testing.T) {
	dir := t.TempDir()

	if err := InstallClaudeHooks(dir); err != nil {
		t.Fatalf("InstallClaudeHooks error: %v", err)
	}

	path := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile settings.json error: %v", err)
	}

	// Verify valid JSON
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Should have hooks key
	if _, ok := settings["hooks"]; !ok {
		t.Fatal("settings.json should contain 'hooks' key")
	}
}

func TestInstallClaudeHooksMalformedJson(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), "{invalid json content")

	err := InstallClaudeHooks(dir)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestInstallClaudeHooksMerge(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Existing settings with non-arc hooks
	existing := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Write",
					"hook":    "my-custom-hook",
				},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), string(data))

	if err := InstallClaudeHooks(dir); err != nil {
		t.Fatalf("InstallClaudeHooks error: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(result, &settings); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Existing non-arc hooks should be preserved
	content := string(result)
	if !strings.Contains(content, "my-custom-hook") {
		t.Fatal("existing non-arc hooks should be preserved after merge")
	}
}

func TestInstallClaudeHooksEmptyObject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), "{}")

	if err := InstallClaudeHooks(dir); err != nil {
		t.Fatalf("InstallClaudeHooks error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if _, ok := settings["hooks"]; !ok {
		t.Fatal("settings.json should have hooks section after merge into empty object")
	}
}

func TestInstallClaudeHooksExistingArcHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Existing settings with arc hooks already present AND a non-arc hook
	existing := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Write",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "my-custom-hook",
						},
					},
				},
				map[string]interface{}{
					"matcher": "Write|Edit|Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "$ARC_HOME/hooks/arc-block-orchestrator-writes.sh",
						},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), string(data))

	if err := InstallClaudeHooks(dir); err != nil {
		t.Fatalf("InstallClaudeHooks error: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	content := string(result)
	// Non-arc hooks should be preserved
	if !strings.Contains(content, "my-custom-hook") {
		t.Fatal("non-arc hooks should be preserved")
	}

	// Count arc-block-orchestrator-writes occurrences — should be exactly 1 (overwritten, not duplicated)
	count := strings.Count(content, "arc-block-orchestrator-writes")
	if count != 1 {
		t.Fatalf("arc-block-orchestrator-writes appears %d times, want 1 (should be overwritten not duplicated)", count)
	}
}

func TestInstallSlashCommand(t *testing.T) {
	dir := t.TempDir()

	if err := InstallSlashCommand(dir); err != nil {
		t.Fatalf("InstallSlashCommand error: %v", err)
	}

	path := filepath.Join(dir, ".claude", "commands", "arc-plan.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile arc-plan.md error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("arc-plan.md should not be empty")
	}
}

func TestInstallSlashCommandOverwrite(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	writeFile(t, filepath.Join(cmdDir, "arc-plan.md"), "old content that should be replaced")

	if err := InstallSlashCommand(dir); err != nil {
		t.Fatalf("InstallSlashCommand error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cmdDir, "arc-plan.md"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) == "old content that should be replaced" {
		t.Fatal("slash command should be overwritten, not preserved")
	}
}
