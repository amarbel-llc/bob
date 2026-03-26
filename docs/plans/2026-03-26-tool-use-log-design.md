# Tool-Use Log Design

## Problem

There is no audit trail of tool invocations during a spinclass session. Users
cannot see what tools were used, with what arguments, or in what order.

## Design

### Sweatfile Config

New boolean field in `[hooks]`:

``` toml
[hooks]
tool-use-log = true
```

Follows scalar override semantics: `nil` = inherit from parent, `true` = enable,
`false` = disable.

### Registration

In `apply.go`, when `tool-use-log` is enabled, add `PostToolUse` to the hooks
map:

``` json
{
  "PostToolUse": [
    {
      "matcher": "*",
      "hooks": [{ "type": "command", "command": "spinclass hooks" }]
    }
  ]
}
```

### Hook Handler

In `hooks.go`, add `"PostToolUse"` case to `Run()`. The handler:

1.  Walks up from `input.CWD` looking for `.claude/` directory to find the
    worktree root
2.  Opens `<worktree>/.claude/tool-use.log` in append mode
3.  Writes the raw JSON payload as a single JSONL line
4.  Returns nil (no stdout output --- PostToolUse hooks are fire-and-forget)
5.  Fails silently on any error

### Log Format

JSONL --- one raw JSON object per line containing all hook payload fields:

``` jsonl
{"hook_event_name":"PostToolUse","session_id":"abc","tool_name":"Edit","tool_input":{"file_path":"/path"},"cwd":"/worktree"}
{"hook_event_name":"PostToolUse","session_id":"abc","tool_name":"Bash","tool_input":{"command":"go test ./..."},"cwd":"/worktree"}
```

### Log Location

`<worktree>/.claude/tool-use.log` --- cleaned up naturally with the worktree.

### Rollback

Revert the commit. Purely additive --- no existing behavior changes.

## Files Changed

- `packages/spinclass/internal/sweatfile/sweatfile.go` --- add
  `ToolUseLog *bool` to `Hooks`, accessor method
- `packages/spinclass/internal/sweatfile/sweatfile_tommy.go` --- re-run
  `tommy generate`
- `packages/spinclass/internal/sweatfile/hierarchy.go` --- add merge logic for
  `ToolUseLog`
- `packages/spinclass/internal/sweatfile/apply.go` --- conditionally register
  `PostToolUse`
- `packages/spinclass/internal/hooks/hooks.go` --- add `PostToolUse` case and
  log writer
- `packages/spinclass/internal/hooks/hooks_test.go` --- test log writing
