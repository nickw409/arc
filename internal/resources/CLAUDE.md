# Resources

Embedded static assets (workflows, prompts, templates, blocks, guides) via `embed.FS`, plus a `Resolver` for disk-first override lookup.

## File Map

| File | Purpose |
|------|---------|
| `embed.go` | Six `//go:embed` directives for `workflowsFS`, `promptsFS`, `templatesFS`, `enforcementFS`, `guidesFS`, `blocksFS`. Accessor functions: `WorkflowBytes`, `PromptBytes`, `TemplateBytes`, `HookBytes`, `GuideBytes`, `BlockBytes`. List functions: `ListWorkflows`, `ListPrompts`, `ListBlocks`. |
| `resolver.go` | `Resolver` — checks disk directories (`{projectDir}/.arc/` and `{homeDir}/.arc/`) before falling back to embedded FS. Enables custom workflows and blocks without rebuilding the binary. |

## Asset Layout

```
resources/
  prompts/       Agent prompt templates (.md), nested by workflow type
  workflows/     Workflow state machine definitions (.yaml)
  templates/     Plan scaffolding templates (.md)
  blocks/        Reusable workflow blocks (.yaml)
  enforcement/   Hook scripts
  guides/        Agent-facing reference docs (.md)
```

## Key Details

- `Resolver` validates resource names (rejects `..`, `/`, `\`) to prevent path traversal.
- Search order: `{projectDir}/.arc/{type}/{name}` → `{homeDir}/.arc/{type}/{name}` → embedded FS.
- Only workflows and blocks use the resolver; prompts, templates, guides, and hooks always come from embedded FS.
