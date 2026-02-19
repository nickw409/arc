### Running Tests

**CRITICAL: Do NOT run test commands directly.** Use the phase test runner:

```bash
$ARC_SCRIPTS_DIR/run-phase-tests.sh {{plan_name}} {{phase}}
```

This ensures tests are run with the correct runner plugin and results are tracked in state.json.
