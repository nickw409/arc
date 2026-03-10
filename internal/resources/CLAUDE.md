# Resources

Embedded static assets (prompts, templates, enforcement hooks, guides, recipes) via `embed.FS`, plus a `Resolver` for disk-first override lookup.

## File Map

| File | Purpose |
|------|---------|
| `embed.go` | Five `//go:embed` directives for `promptsFS`, `templatesFS`, `enforcementFS`, `guidesFS`, `recipesFS`. Accessor functions: `PromptBytes`, `TemplateBytes`, `HookBytes`, `GuideBytes`, `RecipeBytes`. List functions: `ListPrompts`, `ListBuiltInRecipes`. |
| `resolver.go` | `Resolver` — checks disk directories (`{projectDir}/.arc/` and `{homeDir}/.arc/`) before falling back to embedded FS. Enables custom prompts without rebuilding the binary. |

## Asset Layout

```
resources/
  prompts/       Agent prompt templates (.md), nested by category
  templates/     Plan scaffolding templates (.md)
  enforcement/   Hook scripts
  guides/        Agent-facing reference docs (.md)
  recipes/       Built-in recipe definitions (.yaml)
```

## Key Details

- `Resolver` validates resource names (rejects `..`, `/`, `\`) to prevent path traversal.
- Search order: `{projectDir}/.arc/{type}/{name}` → `{homeDir}/.arc/{type}/{name}` → embedded FS.
- The resolver is used for prompt override lookup; templates, guides, hooks, and recipes always come from embedded FS.
