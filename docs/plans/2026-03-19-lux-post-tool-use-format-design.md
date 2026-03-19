# Lux PostToolUse Formatting Hook — Design

**Date:** 2026-03-19

## Problem

When Claude edits files via Edit/Write, they aren't auto-formatted. Lux already
has `lux fmt` which routes files to configured formatters (nixfmt, prettierd,
gofumpt, etc.) but it's never called automatically.

## Solution

Add a PostToolUse hook to lux's `generate-plugin` output that runs `lux fmt` on
edited files after `Edit`/`Write` tool calls.

## Architecture

Go-mcp's `GenerateHooks()` only generates PreToolUse hooks. Rather than
modifying go-mcp (cross-repo), lux post-processes the generated hooks directory
to add PostToolUse entries. This follows grit's pattern of augmenting generated
artifacts locally.

The hook is a thin shell script that extracts `file_path` from the hook JSON
input and calls `lux fmt`. All formatter routing, Nix flake resolution, and
chain/fallback logic is already handled by `lux fmt`.

### Failure modes (all safe)

- **No formatter configured** for file type: `lux fmt` errors to stderr, hook
  swallows via `|| true`
- **Nix build fails** (flake not found, network): same — error to stderr,
  swallowed
- **Formatter process crashes**: same — swallowed
- **No lsps.toml / no config**: `lux fmt` uses formatter config
  (`formatters.toml`), not LSP config — this path is independent of LSP state

The hook never blocks Claude regardless of formatter state.

### What doesn't change

- No cross-repo changes to go-mcp
- No changes to the MCP server, formatter router, or Nix build expression
- `lux fmt` is unchanged

## Rollback

Delete `packages/lux/internal/hooks/` and revert the one-line addition in
`main.go`. Purely additive — no existing behavior modified.
