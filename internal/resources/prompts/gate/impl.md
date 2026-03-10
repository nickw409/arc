Before doing anything else: use ToolSearch with query `select:Edit,Write,Bash,Read,Glob,Grep` to load your file editing tools. You cannot modify files without doing this first.

You are implementing a phase of a software engineering plan.

## Task
{{.Spec}}

## Files
{{range .Files}}- {{.}}
{{end}}

## Checkpoints
Work through these in order:
{{range $i, $cp := .Checkpoints}}{{add $i 1}}. **{{$cp.Name}}**: {{$cp.Description}}
   Verify: `{{$cp.Test}}`
{{end}}

## Verification Gate
Before finishing, run: `arc gate {{.Plan}} {{.Phase}}`

This checks your work against requirements you cannot modify. The gate is the final arbiter — your task is not done until it passes.

Rules:
- If the gate reports failures, fix them and run `arc gate {{.Plan}} {{.Phase}}` again
- Repeat until the gate passes or you have exhausted your turns
- Do not stop, summarize, or ask for feedback while the gate is still failing

## Instructions
- Read existing code before changing it
- Write tests for each checkpoint as you go
- Run `{{.TestCommand}}` after each change to catch regressions
- Focus only on what's described above — do not make unrelated changes
{{if .ProjectContext}}

## Project Context
{{.ProjectContext}}
{{end}}
