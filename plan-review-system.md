# Plan Review System

Two-pass adversarial review for phase plans.

## Architecture

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Plan.md    │────▶│  Pass 1:        │────▶│  Pass 2:        │
│  (1400 lines│     │  Extractor      │     │  Analyzer       │
│   of plan)  │     │  (outputs JSON) │     │  (outputs review│
└─────────────┘     └─────────────────┘     └─────────────────┘
                            │                        │
                            ▼                        ▼
                    extracted.json            review.md
                    (structured data)         (coverage gaps,
                                               verdict)
```

## Why Two Passes

1. **Focus** — Extractor only parses, doesn't judge. Analyzer only judges, doesn't parse. Each does one thing well.

2. **Attention** — 1400-line plans cause drift. Pass 1 compresses to ~200-300 lines of JSON. Pass 2 analyzes the compressed form.

3. **Debuggability** — If the review is wrong, check the JSON. Bad extraction → fix extractor prompt. Bad analysis → fix analyzer prompt.

## Usage

### Pass 1: Extract

**Input:** The raw plan.md file

**Prompt file:** `plan-review-pass1-extractor.md`

**Output:** JSON to stdout (or save to `extracted.json`)

**Invocation example:**
```bash
claude --print "$(cat plan-review-pass1-extractor.md)

---

$(cat plan.md)" > extracted.json
```

### Pass 2: Analyze

**Input:** The JSON from Pass 1 (optionally, the original plan for ambiguous cases)

**Prompt file:** `plan-review-pass2-analyzer.md`

**Output:** Markdown review with verdict

**Invocation example:**
```bash
claude --print "$(cat plan-review-pass2-analyzer.md)

---

## Extracted Plan Data

$(cat extracted.json)" > review.md
```

## Wiring Into Your Orchestrator

### Option A: Sequential Sub-Agents

```
orchestrator
  ├── spawn extractor-agent (mode: extract)
  │     └── outputs: extracted.json
  │
  └── spawn analyzer-agent (mode: review)
        ├── inputs: extracted.json
        └── outputs: review.md with verdict
```

### Option B: Single Agent, Two Calls

```
orchestrator
  ├── call 1: extractor prompt + plan → JSON
  └── call 2: analyzer prompt + JSON → review
```

### Handling the Verdict

```
if verdict == "BLOCKED":
    return review to planner agent
    planner revises plan
    re-run extraction + analysis
    
elif verdict == "CONDITIONAL":
    surface warnings to human
    human decides: revise or proceed
    
elif verdict == "APPROVED":
    proceed to sub-agent execution
```

## Tuning

### If extraction is slow/expensive

The extractor schema is comprehensive. If you don't use certain fields (e.g., `integration_points`), remove them from the schema to reduce output size.

### If analyzer is too harsh

Adjust severity classification in Pass 2. For example, demote "edge case with no test" from Warning to Info.

### If analyzer misses things

The analyzer is only as good as the extraction. If `functions_tested` is often empty because the extractor can't infer it, you have two options:
1. Make test cases in your plans more explicit about what they test
2. Add a heuristic to the extractor (e.g., "if test name contains function name, assume it tests that function")

## Files

| File | Purpose |
|------|---------|
| `plan-review-pass1-extractor.md` | Extractor prompt — parses plan to JSON |
| `plan-review-pass2-analyzer.md` | Analyzer prompt — reviews JSON, outputs verdict |
| `plan-review-system.md` | This file — orchestration docs |
