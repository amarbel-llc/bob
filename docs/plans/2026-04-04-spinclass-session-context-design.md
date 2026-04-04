# Spinclass Session Context Prompt

## Summary

Add a layered system prompt append mechanism to spinclass sessions. Every
session gets a base context prompt describing the repo and session constraints.
Optional issue or PR context is injected when `--issue N` or `--pr N` is passed
to `sc start`. User sweatfile `system-prompt-append` stacks on top.

## File Layout

### Session (worktree)

    .spinclass/
      system_prompt_append.d/
        0-base.md                  # always — rendered at shop.Create()
        1-issue-15.md              # if --issue 15 — rendered at shop.Create()
        1-pr-300.md                # if --pr 300 — rendered at shop.Create()
        2-user.md                  # from sweatfile — written at ExecClaude() time

Files are globbed in sort order at `ExecClaude()` time, concatenated with
`\n\n`, and passed as a single `--append-system-prompt` argument.

### Embedded templates (source)

    internal/prompt/
      system_prompt_append.d/
        0-base.md.tmpl
        1-issue.md.tmpl
        1-pr.md.tmpl

Embedded via `//go:embed`. `2-user.md` has no template --- it's the raw
sweatfile `system-prompt-append` content written as-is.

## Templates

### `0-base.md.tmpl`

``` markdown
# Session Context

You are working inside a spinclass worktree session.

## Repository
- **Name:** {{.RepoName}}
- **Remote:** {{.RemoteURL}}
{{- if .IsFork}}
- **Fork:** yes
{{- end}}
{{- if .OwnerType}}
- **Owner:** {{.OwnerType}} ({{.OwnerLogin}})
{{- end}}

## Worktree Restrictions

This session runs in an isolated git worktree. You MUST NOT:
- Interact with the main worktree or default branch directly
- Run git commands targeting the parent repository directory
- Attempt to check out or modify the main/master branch
- Prefix tool commands with cd into the worktree — you are already there

Tool uses targeting the main repository will be blocked.

## Session
- **Branch:** {{.Branch}}
- **Session ID:** {{.SessionID}}
```

### `1-issue.md.tmpl`

``` markdown
# GitHub Issue Context

This session is working on the following GitHub issue.

## Issue #{{.Number}}: {{.Title}}
- **State:** {{.State}}
{{- if .Labels}}
- **Labels:** {{.Labels}}
{{- end}}
- **URL:** {{.URL}}

## Description

{{.Body}}
```

### `1-pr.md.tmpl`

``` markdown
# Pull Request Context

This session is working on the following pull request.

## PR #{{.Number}}: {{.Title}}
- **State:** {{.State}}
- **Base:** {{.BaseRef}} ← **Head:** {{.HeadRef}}
{{- if .Labels}}
- **Labels:** {{.Labels}}
{{- end}}
- **URL:** {{.URL}}

## Description

{{.Body}}
```

## Data Sources

### Base template (best-effort, graceful fallback)

- `git remote get-url origin` → `RemoteURL`
- Repo directory basename → `RepoName`
- `gh repo view --json isFork,owner` → `IsFork`, `OwnerType`, `OwnerLogin`
- Session state → `Branch`, `SessionID`

If `gh` is unavailable or auth fails, fork/owner fields are omitted from the
rendered template. This is not an error.

### Issue template (hard failure)

- `gh issue view N --json title,body,number,labels,state,url`

### PR template (hard failure)

- `gh pr view N --json title,body,number,labels,state,url,headRefName,baseRefName`

## Code Changes

### `shop.Create()`

After worktree creation:

1.  `mkdir -p .spinclass/system_prompt_append.d/`
2.  Gather template data (git remote, optional `gh` calls)
3.  Render `0-base.md.tmpl` → `0-base.md`
4.  If `--issue N`: render `1-issue.md.tmpl` → `1-issue-N.md`
5.  If `--pr N`: render `1-pr.md.tmpl` → `1-pr-N.md`

### `sweatfile.ExecClaude()`

1.  Fail if `.spinclass/` directory not found in CWD
2.  If sweatfile has `system-prompt-append`, write content to
    `.spinclass/system_prompt_append.d/2-user.md`
3.  Glob `.spinclass/system_prompt_append.d/*.md`, sort
4.  Concatenate contents with `\n\n` separator
5.  Pass as `--append-system-prompt` to `claude`
6.  `--system-prompt` handling unchanged

### `--issue` flag on `sc start`

- New flag: `--issue <number>` (mutually exclusive with `--pr`)
- Validates mutual exclusivity at flag parsing time
- Passes issue number through to `shop.Create()`

## Related Issues

- [#80](https://github.com/amarbel-llc/bob/issues/80) --- Spinclass MCP tools
  not available in sessions (separate, not blocked by this work)
- [#81](https://github.com/amarbel-llc/bob/issues/81) --- Sweatfile config key
  for repo ownership context (deferred, best-effort `gh` detection for now)
- [#82](https://github.com/amarbel-llc/bob/issues/82) --- Formal session
  metadata with MCP tool access (future work, builds on this)
- [#83](https://github.com/amarbel-llc/bob/issues/83) --- Extract exec-claude
  into session-only binary (future refactor)
