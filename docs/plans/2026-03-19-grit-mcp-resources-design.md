# Grit MCP Resources Design

## Problem

All grit read-only tools require permission dialogs in Claude Code because they
are registered as MCP tools. MCP resources are auto-approved without permission
prompts, making them a better fit for pure data retrieval operations.

## Decision

Convert 7 read-only grit tools to native MCP resources. Add `resource-templates`
and `resource-read` tool wrappers for subagent access (subagents cannot use MCP
resources directly). Remove the original 7 tool registrations.

## Resources

| Resource | URI Template | Optional query params |
|----------|-------------|----------------------|
| status | `grit://status` | `repo_path` |
| log | `grit://log` | `repo_path`, `ref`, `max_count`, `paths`, `all` |
| show | `grit://commits/{ref}` | `repo_path`, `context_lines`, `max_patch_lines` |
| blame | `grit://blame/{path}` | `repo_path`, `ref`, `line_range` |
| branches | `grit://branches` | `repo_path`, `all`, `remote` |
| remotes | `grit://remotes` | `repo_path` |
| tags | `grit://tags` | `repo_path`, `pattern`, `sort` |

When `repo_path` is omitted, defaults to the current working directory.

## Tools kept as-is

These remain MCP tools (not converted to resources):

- `diff` --- too many shaping options (staged, ref, paths, context_lines, stat_only)
- `git_rev_parse` --- utility operation, not entity lookup
- `tag_verify` --- verification operation
- `interactive_rebase_plan` --- tied to destructive rebase workflow
- `commit`, `try_commit`, `add`, `reset` --- write operations
- `fetch`, `pull`, `push` --- network/write operations
- `rebase`, `hard_reset`, `interactive_rebase_execute` --- destructive operations
- `branch_create`, `checkout` --- write operations

## New tools

- `resource-templates` --- lists all available grit resources and templates
- `resource-read` --- reads a grit resource by URI (for subagent access)

Both marked `ReadOnlyHint: true`, `DestructiveHint: false`, `IdempotentHint: true`.

## Architecture

### Resource provider

A `resourceProvider` struct wraps `mcpserver.ResourceRegistry` and handles URI
dispatch:

1. Parse URI with `url.Parse`
2. Extract path identity (`ref`, `path`) from the URI path
3. Extract optional params from query string
4. Default `repo_path` to cwd if absent
5. Call existing handler logic (reuse `git.Run` + parsers)

### Registration

```
registerResources(registry, cwd)
├── RegisterResource("grit://status", ...)
├── RegisterResource("grit://branches", ...)
├── RegisterResource("grit://remotes", ...)
├── RegisterResource("grit://tags", ...)
├── RegisterTemplate("grit://log", ...)
├── RegisterTemplate("grit://commits/{ref}", ...)
├── RegisterTemplate("grit://blame/{path}", ...)
```

### Files touched

1. `packages/grit/internal/tools/resources.go` --- new, resource registration + provider
2. `packages/grit/internal/tools/registry.go` --- add resource-templates/resource-read, remove 7 tools
3. `packages/grit/internal/tools/status.go` --- remove tool registration, keep handlers
4. `packages/grit/internal/tools/log.go` --- same
5. `packages/grit/internal/tools/tag.go` --- same
6. `packages/grit/internal/tools/branch.go` --- same
7. `packages/grit/internal/tools/remote.go` --- same
8. `packages/grit/cmd/grit/main.go` --- wire up resource provider to MCP server

## Rollback

No dual-architecture period needed. Resources and tools use the same underlying
handlers. Reverting is a one-commit operation: re-add tool registrations, remove
resource registrations.

## Testing

- Unit: URI parsing, dispatch, default repo_path resolution
- Integration: update BATS tests for resource-templates/resource-read surface
- Build: `nix build .#grit` + `generate-plugin` output verification
- Manual: install, restart Claude Code, confirm auto-approval + subagent access
