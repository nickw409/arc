# Block

Composable workflow blocks — reusable, parameterized workflow fragments loaded from YAML and wired together into flat `arc.Workflow` state machines.

**Start here:** `block.go` for types and parameter substitution, `compose.go` for wiring blocks into workflows.

## File Map

| File | Purpose |
|------|---------|
| `block.go` | YAML parsing, data types (`Block`, `BlockState`, `ResolvedBlock`), and `ResolveParams` for `${param}` substitution. |
| `compose.go` | `ComposePipeline` (full pipeline composition with routing and parallelism) and `ComposeSequential` (simpler flat list). Also `ValidateComposition` for post-composition graph validation. |

## Key Concepts

- **Block**: A reusable workflow fragment with an `entry` state, named `exits`, parameterized states, and `${param}` placeholders.
- **Pipeline**: An ordered list of `PipelineStep`s, each referencing a block (with params and routing) or a parallel group.
- **Exit references**: In a block's `next` map, targets starting with `$` (e.g., `$bugs_found`) are cross-block exits wired externally. Bare names are intra-block state references that get namespaced.
- **Linear transitions**: A string `next: foo` becomes `{"": "foo"}` — the empty-string key uniformly means "unconditional transition" throughout the system.

## Key Design Decisions

- **Two-pass composition in `ComposePipeline`**: Pass 1 builds a `stepEntry` map (step name → entry state), needed by Pass 2 to resolve `route` targets.
- **`RunOnce` applied only to the block's entry state** — re-entry detection fires at the first state only.
- **Param substitution happens before composition** — by the time transitions are wired, all `${...}` placeholders are resolved.
- **`rawBlockState` uses `yaml.Node`** for the `next` field because it can be either a scalar string or a map — `gopkg.in/yaml.v3` can't unmarshal that polymorphism into a native Go type.

## External Caller

The only external caller is `internal/workflow/loader.go`. When a workflow YAML contains a `pipeline:` key, the loader calls `LoadBlock` → `ComposePipeline` → `ValidateComposition`.

Block YAML files live in `internal/resources/blocks/*.yaml` (embedded). The `resources.Resolver` adds disk-first lookup (`.arc/blocks/`) for custom blocks.
