# Go Pitfalls for the Arc Rewrite

Concrete issues you'll hit rewriting Arc in Go, how they'll manifest, and patterns to prevent them. Focused on problems specific to this project — state machine orchestration, subprocess management, file parsing, and agent output extraction.

---

## 1. Error Propagation Through the Iteration Pipeline

### The problem

Arc's iteration pipeline is 8 steps deep. In bash, you chain commands and check `$?`. In Go, each step returns an error, and you'll be tempted to write this:

```go
func runIteration(phase *Phase) error {
    if err := checkIntervention(phase); err != nil {
        return fmt.Errorf("intervention check: %w", err)
    }
    if err := checkEscalation(phase); err != nil {
        return fmt.Errorf("escalation check: %w", err)
    }
    if err := checkPreConstraints(phase); err != nil {
        return fmt.Errorf("pre-constraints: %w", err)
    }
    // ... 5 more steps
}
```

This compiles and runs. The problem is that the *caller* needs to make decisions based on which step failed and why. "Intervention needed" is not the same kind of error as "subprocess crashed" or "verdict unparseable." When everything is `error`, the orchestrator has no structured way to decide whether to retry, escalate, request human input, or abort.

After a few weeks you'll have code like this scattered everywhere:

```go
if err != nil {
    if strings.Contains(err.Error(), "intervention") {
        // ...
    } else if strings.Contains(err.Error(), "stuck") {
        // ...
    }
}
```

This is stringly-typed error handling and it's the number one way Go orchestration projects rot.

### How to avoid it

Define a small set of error types that map to orchestrator decisions, not to what went wrong internally. The orchestrator doesn't care *why* something failed — it cares *what to do next*.

```go
type IterationResult struct {
    NextState  string          // empty if no transition
    Verdict    Verdict         // the parsed verdict, if any
    Action     ResultAction    // what the orchestrator should do
    Err        error           // underlying error, if any
}

type ResultAction int

const (
    ActionContinue       ResultAction = iota // advance to next state
    ActionRetry                              // same state, next iteration
    ActionEscalate                           // trigger escalation ladder
    ActionIntervene                          // stop and wait for human
    ActionAbort                              // unrecoverable
)
```

Now `runIteration` returns a *result*, not just an error. The orchestrator switches on `Action`, not on error string contents. Errors still exist for logging and diagnostics, but they don't drive control flow.

---

## 2. Unstructured Verdict Parsing

### The problem

Verdict extraction is where Arc reads sub-agent output and decides what happened: `approved`, `gaps_found`, `concerns`, etc. In bash you're probably grepping for a pattern. In Go, the temptation is to do the same thing with `strings.Contains` or a regex.

The failure mode: an agent outputs something slightly off-format (e.g., `Verdict: Approved` instead of `approved`, or buries it in a markdown code block), and your parser either misses it or extracts the wrong thing. Since verdicts drive state transitions, a misparsed verdict sends the whole phase down the wrong path silently.

### How to avoid it

Make verdict an explicit type with a strict parser and a clear fallback:

```go
type Verdict string

const (
    VerdictApproved  Verdict = "approved"
    VerdictGapsFound Verdict = "gaps_found"
    VerdictConcerns  Verdict = "concerns"
    VerdictUnknown   Verdict = "unknown"
)

func ParseVerdict(raw string) (Verdict, error) {
    // normalize: lowercase, trim, check known patterns
    // if ambiguous, return VerdictUnknown with a descriptive error
    // never silently pick a default
}
```

The key discipline: `VerdictUnknown` must always trigger a specific orchestrator response (retry with a stricter prompt, or escalate). Never treat an unparseable verdict as `approved` or `gaps_found` by default. Claude Code will sometimes do this — generate a fallback value to keep things compiling. Reject that pattern in review.

---

## 3. YAML Schema Versioning Without Sum Types

### The problem

Arc supports workflow schemas V1 through V5, and they're backwards compatible. In Rust, you'd model this with an enum:

```rust
enum StateTransition {
    Linear(String),                    // V1
    Conditional(HashMap<String, String>), // V2+
}
```

In Go, you'll define a struct that has fields for all versions:

```go
type StateConfig struct {
    Next interface{} `yaml:"next"` // string or map[string]string
}
```

This works for deserialization, but now every piece of code that touches `Next` needs a type switch. Miss one, and you get a nil pointer panic at runtime — exactly the kind of bug that only surfaces when someone uses a V1 workflow on the V5 engine.

### How to avoid it

Normalize during loading, not during use. Parse the raw YAML into a version-aware intermediate struct, then convert to a single canonical internal representation:

```go
// Internal representation — always uses the richest format
type StateTransition struct {
    Branches map[Verdict]string // always a map, even for linear
}

// For V1 linear: next: "impl" becomes Branches{"": "impl"}
// For V2+ conditional: next: {approved: impl, gaps_found: qa} loads directly
```

Write the normalization logic once in the loader. Everything downstream sees one type. This also makes it trivial to validate: if a state has verdicts defined but `Branches` only has one key, that's a schema error you can catch at load time.

---

## 4. Goroutine Leaks in Subprocess Management

### The problem

Arc spawns sub-agents (Claude Code CLIs) as subprocesses and waits for output. It also spawns parallel adversary agents (5 at once for plan review) and has a watchdog process. In Go, the natural pattern is:

```go
for _, adversary := range adversaries {
    go func(a Adversary) {
        result, err := runAdversary(ctx, a)
        results <- result
    }(adversary)
}
```

If one adversary hangs (Claude Code stalls, network issue, process zombies), and your context cancellation isn't wired through to the subprocess, the goroutine leaks. Do this a few times and your `arc run` process accumulates zombie children and leaked goroutines until it eventually stalls or OOMs.

### How to avoid it

Always use `exec.CommandContext` so the subprocess is killed when the context is cancelled. And always set a timeout:

```go
func spawnAgent(ctx context.Context, prompt string, timeout time.Duration) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "claude", "--print", "--output-format", "text")
    cmd.Stdin = strings.NewReader(prompt)

    // Important: set process group so child processes are also killed
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    cmd.Cancel = func() error {
        // Kill the entire process group, not just the parent
        return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
    }

    output, err := cmd.Output()
    if ctx.Err() == context.DeadlineExceeded {
        return "", fmt.Errorf("agent timed out after %s", timeout)
    }
    return string(output), err
}
```

The `Setpgid` + process group kill is critical. Without it, killing the `claude` parent process can leave its children (node processes, etc.) running as orphans. Your existing watchdog logic should also be context-aware.

---

## 5. Race Conditions in State File Access

### The problem

`state.json` is the source of truth for each phase. The orchestrator reads it, runs an iteration, then writes it back. If you're running the monitor TUI concurrently (which also reads `state.json`), or if you have parallel phases in V5, you can get:

- Orchestrator reads state → monitor reads state → orchestrator writes state → monitor displays stale data (cosmetic, but confusing)
- Parallel phase A reads state → parallel phase B reads state → both write → one write is lost (data corruption)

Go's type system won't warn you about this. The race detector will, but only if you have test coverage that exercises the concurrent paths.

### How to avoid it

For the simple case (single orchestrator, monitor is read-only), use file-level locking:

```go
type StateFile struct {
    path string
    mu   sync.Mutex
}

func (s *StateFile) Update(fn func(state *PhaseState) error) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    state, err := s.read()
    if err != nil {
        return err
    }
    if err := fn(state); err != nil {
        return err
    }
    return s.write(state)
}
```

For V5 parallel phases, use filesystem-level locking (`flock`) since you may have multiple processes:

```go
func flockUpdate(path string, fn func(state *PhaseState) error) error {
    f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        return err
    }
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    // read, modify, write state.json inside the lock
}
```

Run `go test -race ./...` in CI from day one. Don't defer this.

---

## 6. Template Rendering Failures at Runtime

### The problem

Arc uses Handlebars-style templates for prompt rendering (V3+). Go's `text/template` uses `{{.Field}}` syntax instead. Either way, a missing variable or wrong type in a template produces a runtime error — or worse, silently renders an empty string, and the sub-agent gets a prompt with blank sections.

This is especially dangerous because prompt quality directly determines agent behavior. A silently malformed prompt doesn't crash — it produces subtly wrong agent output that you might not catch until several iterations later.

### How to avoid it

Validate templates at load time, not at render time:

```go
func LoadWorkflow(path string) (*Workflow, error) {
    // ... parse YAML ...

    for _, state := range workflow.States {
        tmpl, err := template.New(state.Name).Parse(state.PromptTemplate)
        if err != nil {
            return nil, fmt.Errorf("state %s: invalid template: %w", state.Name, err)
        }

        // Dry-run render with a dummy context to catch missing fields
        var buf bytes.Buffer
        dummy := buildDummyContext(workflow)
        if err := tmpl.Execute(&buf, dummy); err != nil {
            return nil, fmt.Errorf("state %s: template render check failed: %w", state.Name, err)
        }

        // Optionally: check that the rendered output isn't suspiciously short
        // or contains template delimiters (indicates a partial render)
    }
}
```

Also: set `template.Option("missingkey=error")` so that missing variables produce errors instead of empty strings. This one setting prevents an entire class of silent failures.

---

## 7. The `interface{}` Creep

### The problem

Arc's configuration has heterogeneous fields: `next` can be a string or a map, `params` can be anything, escalation actions carry different payloads. The expedient Go solution is `interface{}` (or `any`). Claude Code will generate this readily because it makes the code compile.

Over time, `interface{}` propagates. Functions that accept `interface{}` return `interface{}`, and every consumer needs type assertions. You lose all compile-time checking, and panics from failed type assertions become your runtime type system.

### How to avoid it

Contain `interface{}` at the deserialization boundary. Define concrete types for everything internal, and use custom `UnmarshalYAML` / `UnmarshalJSON` methods to handle the polymorphism:

```go
type Transition struct {
    Linear      string
    Conditional map[Verdict]string
}

func (t *Transition) UnmarshalYAML(value *yaml.Node) error {
    // Try string first (V1 linear)
    var s string
    if err := value.Decode(&s); err == nil {
        t.Linear = s
        return nil
    }
    // Try map (V2+ conditional)
    var m map[string]string
    if err := value.Decode(&m); err == nil {
        t.Conditional = make(map[Verdict]string)
        for k, v := range m {
            t.Conditional[Verdict(k)] = v
        }
        return nil
    }
    return fmt.Errorf("line %d: 'next' must be a string or verdict map", value.Line)
}
```

This is more code upfront. It saves you from scattered type assertions everywhere else. Make this a rule in your `CLAUDE.md`: no `interface{}` outside of unmarshal methods.

---

## 8. Implicit Nil Behavior

### The problem

Go's zero values are convenient until they're not. A few places in Arc where this bites:

- A `*PhaseState` that was never loaded from disk is `nil`. Calling methods on it panics.
- A `map[string]string` field in a struct that was deserialized from YAML but the key was absent is `nil`. Writing to it panics. Reading from it silently returns zero values.
- A slice of verdicts that's `nil` vs empty behaves the same for `range` but differently for `json.Marshal` (`null` vs `[]`).

None of these are caught by the compiler. They show up as panics in production, usually in edge cases — a phase with no escalation config, a workflow with no constraints defined, a state with no verdicts.

### How to avoid it

Initialize all maps and slices in constructors, not in zero-value structs:

```go
func NewPhaseState(name string) *PhaseState {
    return &PhaseState{
        Phase:             name,
        CurrentState:      "",
        Iteration:         0,
        VerdictHistory:    []Verdict{},      // not nil
        Disputes:          []Dispute{},      // not nil
        EscalationHistory: []string{},       // not nil
    }
}
```

For optional config sections, use pointer fields with explicit nil checks:

```go
type StateConfig struct {
    Escalation *EscalationConfig `yaml:"escalation"` // nil means "no escalation"
    Constraints *ConstraintConfig `yaml:"constraints"` // nil means "no constraints"
}

func (s *StateConfig) ShouldEscalate(iteration int) bool {
    if s.Escalation == nil {
        return false
    }
    // ...
}
```

The discipline: never access a pointer field or map without checking. Add a linter rule (`nilaway` or `nilerr`) to CI.

---

## 9. Uncontrolled Logging in a Multi-Agent System

### The problem

Arc runs multiple agents concurrently — the orchestrator, sub-agents, the watchdog, the monitor. In bash you pipe to files and `tee`. In Go, the default `log` package writes to stderr with no structure. When 5 adversary agents are running in parallel, interleaved log lines become unreadable.

Worse: if you use `log.Fatal` anywhere (which Claude Code loves to generate), it calls `os.Exit(1)` and skips all your deferred cleanup — open file locks, running subprocesses, uncommitted state.

### How to avoid it

Use `log/slog` (stdlib since Go 1.21) with structured fields:

```go
logger := slog.With(
    "plan", planName,
    "phase", phaseName,
    "state", currentState,
    "iteration", iteration,
)
logger.Info("spawning agent", "agent_type", "qa")
logger.Error("verdict parse failed", "raw_output", truncated)
```

Ban `log.Fatal` and `log.Panic` via a linter rule. Use error returns instead. If you need a hard exit, do it in `main()` after cleanup.

For the monitor TUI: write structured JSON logs to a file, and have the TUI tail that file. Don't try to mix TUI rendering and log output on the same terminal.

---

## 10. Claude Code-Specific Pitfalls

These aren't Go issues per se, but patterns Claude Code tends to produce in Go that are particularly dangerous for Arc:

**Silent fallback values.** When a config field might be absent, Claude Code will often write `model := config.Model; if model == "" { model = "sonnet" }` instead of treating a missing model as an error. In Arc, where model selection is part of the escalation ladder, a silent default could mask an escalation failure. Make every default explicit and logged.

**Overeager abstraction.** Claude Code likes to create `Manager`, `Service`, and `Handler` layers. For Arc, the component boundaries are already clear (loader, state machine, runner, spawner, parser). Push back on unnecessary indirection. A function is fine. Not everything needs a struct with methods.

**Test mocking via interfaces.** Claude Code will extract interfaces for everything so it can mock in tests. For Arc, most of the interesting behavior is subprocess interaction and file I/O — you want integration tests that actually spawn processes and read files, not unit tests against mocked interfaces. Structure the code so the core logic (state transitions, verdict resolution, escalation decisions) is pure functions that are trivially testable without mocks.

**Swallowing context cancellation.** Claude Code often writes `if err != nil { return err }` without checking if the error is a context cancellation. In Arc, where you need clean shutdown (save state, kill subprocesses, release locks), context cancellation needs to be handled explicitly, not treated as a generic error.
