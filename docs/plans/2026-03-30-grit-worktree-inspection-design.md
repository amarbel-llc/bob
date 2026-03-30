# Grit Worktree Inspection Resource

## Summary

Add a `grit://worktrees` static MCP resource that lists all git worktrees for
the current repository with path, HEAD, branch, and lock/prune state.

## Resource

- **URI:** `grit://worktrees`
- **Type:** Static resource (no parameters beyond optional `repo_path`)
- **Data source:** `git worktree list --porcelain`

## Data Shape

``` go
type WorktreeEntry struct {
    Path       string `json:"path"`
    Head       string `json:"head"`
    Branch     string `json:"branch,omitempty"`
    IsBare     bool   `json:"is_bare,omitempty"`
    IsMain     bool   `json:"is_main"`
    Locked     bool   `json:"locked,omitempty"`
    LockReason string `json:"lock_reason,omitempty"`
    Prunable   bool   `json:"prunable,omitempty"`
}
```

## Files Modified

1.  `internal/git/types.go` --- add `WorktreeEntry`
2.  `internal/git/parse_worktree.go` --- parse `--porcelain` output
3.  `internal/git/parse_worktree_test.go` --- parser tests
4.  `internal/tools/resources.go` --- register resource and handler

## Scope

Inspection only. No mutation tools (add, remove, move, lock/unlock). No hook
changes needed (read-only resource, no Bash commands to intercept).
