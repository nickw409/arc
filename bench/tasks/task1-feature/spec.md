# Task 1: Add Label/Tag Support

## Overview

Add a labeling/tagging system to tkit so users can organize tasks with labels.

## Requirements

### Data Model
- Tasks can have zero or more labels (string tags like "work", "urgent", "frontend")
- Labels are stored as a string slice on the Task struct
- Labels are persisted in the JSON file

### CLI Commands

1. **`tkit add`** — Add `--label/-l` flag (repeatable) to assign labels at creation
   ```
   tkit add "Fix login bug" -p high -l bug -l frontend
   ```

2. **`tkit list`** — Add `--label/-l` flag to filter tasks by label
   ```
   tkit list --label bug          # tasks with "bug" label
   tkit list --label bug -s active  # combine with existing filters
   ```

3. **`tkit label`** — New subcommand to manage labels on existing tasks
   ```
   tkit label add <task-id> <label>     # add label to task
   tkit label remove <task-id> <label>  # remove label from task
   tkit label list                       # list all unique labels in use
   ```

4. **`tkit show`** — Display labels in task detail view

### Constraints
- Labels are case-insensitive (stored lowercase)
- No duplicate labels on a single task
- Empty string labels are rejected
- All existing tests must continue to pass
- New functionality must have tests

## Deliverables
- Modified `model/task.go` — Labels field
- Modified `store/store.go` — Label query support
- Modified `filter/filter.go` — Label filtering
- New or modified CLI commands
- Tests for all new functionality
