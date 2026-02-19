package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/resources"
)

// InstallGitHooks installs pre-commit and commit-msg hooks from embedded resources.
func InstallGitHooks(projectRoot string) error {
	hooksDir := filepath.Join(projectRoot, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}

	hookNames := []string{"pre-commit", "commit-msg"}
	for _, name := range hookNames {
		if err := installOneGitHook(hooksDir, name); err != nil {
			return err
		}
	}
	return nil
}

func installOneGitHook(hooksDir, name string) error {
	hookPath := filepath.Join(hooksDir, name)
	embeddedContent, err := resources.HookBytes(name)
	if err != nil {
		return fmt.Errorf("read embedded hook %s: %w", name, err)
	}

	existing, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing hook %s: %w", name, err)
	}

	if err == nil {
		// File exists
		if strings.Contains(string(existing), "# ARC HOOKS") {
			return nil // already installed
		}
		// Append arc section
		arcSection := "\n# ARC HOOKS\n" + string(embeddedContent) + "\n"
		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("append to hook %s: permission denied", name)
		}
		defer f.Close()
		if _, err := f.WriteString(arcSection); err != nil {
			return fmt.Errorf("write to hook %s: %w", name, err)
		}
		return nil
	}

	// New file
	content := string(embeddedContent)
	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("write hook %s: %w", name, err)
	}
	return nil
}

// InstallClaudeHooks writes Claude Code hook configuration to .claude/settings.json.
func InstallClaudeHooks(projectRoot string) error {
	claudeDir := filepath.Join(projectRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	arcHooks := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Write|Edit",
					"hook":    "arc-enforce-write",
				},
			},
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hook":    "arc-enforce-bash",
				},
			},
		},
	}

	existing, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err == nil {
		// File exists - parse it
		if err := json.Unmarshal(existing, &settings); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
		// Merge arc hooks into existing settings
		mergeClaudeHooks(settings, arcHooks)
	} else {
		// No existing file - use arc hooks as base
		settings = arcHooks
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}

func mergeClaudeHooks(settings, arcHooks map[string]interface{}) {
	arcHooksSection := arcHooks["hooks"].(map[string]interface{})

	existingHooks, ok := settings["hooks"]
	if !ok {
		settings["hooks"] = arcHooksSection
		return
	}

	existingHooksMap, ok := existingHooks.(map[string]interface{})
	if !ok {
		settings["hooks"] = arcHooksSection
		return
	}

	for hookType, arcEntries := range arcHooksSection {
		arcList := arcEntries.([]interface{})
		existingList, ok := existingHooksMap[hookType]
		if !ok {
			existingHooksMap[hookType] = arcList
			continue
		}

		existingSlice, ok := existingList.([]interface{})
		if !ok {
			existingHooksMap[hookType] = arcList
			continue
		}

		// Merge: update existing arc hooks in-place, append new ones
		for _, arcEntry := range arcList {
			arcMap := arcEntry.(map[string]interface{})
			arcHookName := arcMap["hook"].(string)

			found := false
			for i, existingEntry := range existingSlice {
				existingMap, ok := existingEntry.(map[string]interface{})
				if !ok {
					continue
				}
				hookName, _ := existingMap["hook"].(string)
				if strings.HasPrefix(hookName, "arc-") && hookName == arcHookName {
					existingSlice[i] = arcMap
					found = true
					break
				}
			}
			if !found {
				existingSlice = append(existingSlice, arcEntry)
			}
		}
		existingHooksMap[hookType] = existingSlice
	}
}

// InstallSlashCommand writes the /arc-plan command to .claude/commands/arc-plan.md.
func InstallSlashCommand(projectRoot string) error {
	cmdDir := filepath.Join(projectRoot, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}

	content, err := resources.TemplateBytes("plan-command.md")
	if err != nil {
		return fmt.Errorf("read plan-command template: %w", err)
	}

	cmdPath := filepath.Join(cmdDir, "arc-plan.md")
	if err := os.WriteFile(cmdPath, content, 0644); err != nil {
		return fmt.Errorf("write arc-plan.md: %w", err)
	}
	return nil
}
