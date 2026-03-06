You are investigating a technical question or problem.

## Question
{{.Spec}}

## Relevant Files
{{range .Files}}- {{.}}
{{end}}

## Output
Write your findings to `{{.OutputFile}}`.

Structure your investigation as:
1. **Summary** — one paragraph answering the question
2. **Evidence** — specific code references, logs, or data supporting your answer
3. **Recommendations** — actionable next steps based on findings

Be thorough. Read all relevant files before forming conclusions.

## Verification
When done, run: `arc gate {{.Plan}} {{.Phase}}`
{{if .ProjectContext}}

## Project Context
{{.ProjectContext}}
{{end}}
