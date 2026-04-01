---
name: commit
description: Use when the user asks to commit, requests a commit, says "commit this", or when you need to create a git commit as part of completing work
---

# Commit

## Overview

Use grit's `add` and `commit` MCP tools for all commits. Stage files explicitly with `add`, then create the commit with `commit`.

## The Process

### Step 1: Determine What to Commit

Identify which files to stage:

1. **Check status** — read the `grit://status` resource or use your knowledge of what you changed
2. **Draft the commit message** — check recent commits via `grit://log` for style conventions

### Step 2: Stage Files

Call `add` with the file paths to stage:

```
grit add (
  repo_path: "<path>",
  paths: ["file1.go", "file2.go"]
)
```

### Step 3: Create the Commit

Call `commit` with the message:

```
grit commit (
  repo_path: "<path>",
  message: "<commit message>"
)
```

The message should:
- Summarize the "why" not the "what"
- Follow the repo's existing convention (conventional commits, imperative mood, etc.)
- End with `Co-Authored-By: Claude <co-author> <noreply@anthropic.com>` when appropriate

### Step 4: Verify

Read `grit://status` to confirm the commit succeeded and the working tree is in the expected state.

## Quick Reference

| Step | Tool | Purpose |
|------|------|---------|
| Check changes | `grit://status` resource | See what's modified (optional if you already know) |
| Check style | `grit://log` resource | Match commit message convention (optional) |
| Stage | `grit add` | Stage files for commit |
| Commit | `grit commit` | Create the commit |
| Verify | `grit://status` resource | Confirm success |

## Common Mistakes

**Using Bash for git commands**
- **Problem:** Hooks will deny `git add` and `git commit` via Bash
- **Fix:** Use grit's `add` and `commit` tools directly

**Forgetting to stage before committing**
- **Problem:** `commit` only commits staged files
- **Fix:** Always call `add` first to stage the files you want to include
