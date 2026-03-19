# Resource-Aware Hook Mappings — grit Prototype

## Summary

Extend grit's PreToolUse hook to deny Bash calls that should use MCP resources
instead of shelling out to git. Currently, tool mappings only redirect to MCP
tools; read-only operations migrated to MCP resources have no hook protection.

## Problem

grit migrated 7 read-only git operations from MCP tools to MCP resources (auto-
approved, no permission dialog). But the hook system only knows about tool
mappings — it cannot point Claude to a resource. Claude can still `Bash` out to
`git status`, `git log`, etc. without being intercepted.

## Approach

Prototype resource-aware hook denial directly in grit, bypassing the purse-first
`HandleHook` framework. This avoids cross-repo changes and lets us learn what
the eventual purse-first API should look like.

## Design

### Resource Mappings

| Command prefix | Resource URI | Description |
|---|---|---|
| `git status` | `grit://status` | Working tree status |
| `git branch` | `grit://branches` | Branch listing |
| `git remote` | `grit://remotes` | Remote listing |
| `git tag` | `grit://tags` | Tag listing |
| `git log` | `grit://log` | Commit history |
| `git show` | `grit://commits/{ref}` | Commit detail |
| `git blame` | `grit://blame/{path}` | Line authorship |

### Integration

In `cmd/grit/main.go`, the hook handler changes from:

```go
app.HandleHook(os.Stdin, os.Stdout)
```

To: buffer stdin, try resource hook first, fall through to `app.HandleHook()`:

```go
input, _ := io.ReadAll(os.Stdin)
if handled, err := hooks.HandleResourceHook(input, os.Stdout); !handled {
    app.HandleHook(bytes.NewReader(input), os.Stdout)
}
```

### New Package: `internal/hooks/`

**`resource_mappings.go`**:

- `ResourceMapping` struct: `CommandPrefix`, `ResourceURI`, `Description`
- Table of 7 mappings
- `HandleResourceHook(input []byte, w io.Writer) (bool, error)`:
  - Parses hook JSON (`tool_name`, `tool_input`)
  - Returns `false, nil` if `tool_name != "Bash"`
  - Extracts and normalizes commands (shell parsing via `mvdan.cc/sh`,
    git option stripping — duplicated from go-mcp's unexported functions)
  - Checks each command against prefix table
  - On match: writes deny JSON, returns `true, nil`
  - On no match: returns `false, nil`

### Deny Message Format

Since the hook input has no field to distinguish agents from subagents, the
denial message includes both instructions:

```
Read the grit://status resource instead (working tree status).
Subagents: use mcp__plugin_grit_grit__resource-read with uri grit://status
```

### Command Normalization

Duplicates ~30 lines from go-mcp's unexported functions:

- **Shell parsing**: `mvdan.cc/sh/v3/syntax` to split compound commands
  (`git status && git log`)
- **Git normalization**: strip global git options (`-C`, `-c`, `--git-dir`,
  `--work-tree`, `--bare`) so `git -C /path status` matches prefix `git status`

### Testing

`internal/hooks/resource_mappings_test.go`:

1. Basic match — `git status` denies with `grit://status`
2. Normalized match — `git -C /path status` same result
3. Compound command — `git status && git log` denies on first match
4. No match — `git commit` returns `false`
5. Non-Bash tool — `tool_name: "Read"` returns `false`
6. Message format — both resource URI and `resource-read` wrapper present

### Rollback

Purely additive. Revert the `main.go` buffering change to restore direct
`app.HandleHook()` delegation. No dual-architecture period needed.

## Follow-up

- GitHub issue: hook input should include context to distinguish agent vs
  subagent, so denial messages can be targeted
- Generalize to get-hubbed and chix after validating the grit prototype
- Eventually upstream into purse-first's `ToolMapping` / `HandleHook` system
