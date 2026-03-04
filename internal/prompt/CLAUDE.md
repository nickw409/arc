# Prompt

Handles the boundary between the pipeline and AI agents: preparing inputs (rendering prompts) and parsing outputs (extracting verdicts).

## File Map

| File | Purpose |
|------|---------|
| `render.go` | Turns Markdown prompt templates into concrete strings. `Render` (from path) and `RenderString` (from bytes) are the entry points. Contains `preprocessHandlebars` which translates Handlebars-like syntax to Go templates. |
| `extract.go` | `ExtractVerdict` — parses agent output for `## Verdict` section, extracts the verdict token. Takes the **last** occurrence (agents may revise mid-output). |

## Key Design Decisions

- **Handlebars shim over Go templates**: Prompt `.md` files use `{{phase}}`, `{{state.current_state}}`, `{{#if params.foo}}` syntax. `preprocessHandlebars` translates this to Go `text/template` syntax via ordered regex replacements. The order matters — pipe-default must run before dot-access.
- **Partial includes**: `{{> path/to/partial.md}}` inlines other embedded prompt files at preprocessing time. Silently dropped if not found.
- **`StateToTemplateMap` flattens everything to strings** — avoids Go template type-switch complexity. All `PhaseState` fields become string map entries accessible as `{{state.field_name}}`.
- **Both capitalized and lowercase keys** in `contextToMap` — `.Phase` for Go template tests, `{{phase}}` for Handlebars prompts.
- **`safeIndex` replaces built-in `index`** — returns error on nil maps instead of panicking.
- **Last-wins verdict scanning** — `ExtractVerdict` records the last `## Verdict` header, not the first. Skips headers inside code fences.

## Template Variables Available in Prompts

```
{{phase}}, {{plan}}, {{iteration}}, {{plan_md}}
{{state.current_state}}, {{state.iteration}}, {{state.tests_passing}}, ...
{{params.key}} — per-state custom params from workflow YAML
{{previous_memory}} — loopback memory from prior run of same state
{{mode}}, {{dispute_count}}, {{dispute_list}}
```
