# WebFetch Interception for get-hubbed

**Date:** 2026-03-30 **Status:** proposed

## Problem

Claude Code agents use `WebFetch` to scrape GitHub web pages instead of using
get-hubbed's MCP resources and tools. This wastes tokens on HTML-to-markdown
conversion, bypasses the structured data get-hubbed provides, and ignores the
authenticated `gh` CLI that get-hubbed wraps.

## Solution

Add a PreToolUse hook to get-hubbed that intercepts `WebFetch` calls targeting
GitHub domains, denies them, and returns the specific get-hubbed resource URI or
tool that serves the same data.

## Decisions

- **Hard deny** --- block the WebFetch entirely, same as grit blocks all git CLI
- **All GitHub domains** --- `github.com`, `www.github.com`, `api.github.com`,
  `raw.githubusercontent.com`, `gist.github.com`
- **Specific deny messages** --- parse the URL path and construct the exact
  get-hubbed resource URI; fall back to a catch-all listing all resources/tools
  for unrecognized patterns
- **Framework matcher expansion** --- patch hooks.json post-generation to add
  `WebFetch` to the PreToolUse matcher (lux pattern)

## Architecture

### New package: `internal/hooks/`

Single entry point matching grit's pattern:

``` go
func HandleWebFetchHook(input []byte, w io.Writer) (bool, error)
```

Returns `(true, nil)` if denied (GitHub URL matched), `(false, nil)` if not a
WebFetch or not a GitHub URL. Fail-open on parse errors.

### Hook input

WebFetch tool_input structure:

``` json
{
  "tool_name": "WebFetch",
  "tool_input": {
    "url": "https://github.com/owner/repo/issues/42",
    "prompt": "..."
  }
}
```

### main.go changes

Buffer stdin, try `HandleWebFetchHook` first, fall back to `app.HandleHook()`:

``` go
if os.Args[1] == "hook" {
    input, _ := io.ReadAll(os.Stdin)
    handled, _ := hooks.HandleWebFetchHook(input, os.Stdout)
    if !handled {
        app.HandleHook(bytes.NewReader(input), os.Stdout)
    }
}
```

### Hook matcher expansion

After `app.HandleGeneratePlugin()`, post-process hooks.json to change the
PreToolUse matcher from `"Bash"` to `"Bash|WebFetch"`. This follows lux's
pattern for merging PostToolUse hooks into the framework-generated hooks.json.

Implemented as `PatchHooksMatcher()` called from a custom generate-plugin path
in main.go, or as a Nix postInstall step.

### generate-plugin changes

get-hubbed's main.go `generate-plugin` handler will:

1.  Call `app.HandleGeneratePlugin()` (writes hooks.json with `"Bash"` matcher)
2.  Call `hooks.PatchHooksMatcher()` to add `"WebFetch"` to the matcher

This keeps the framework unmodified while cleanly extending the matcher.

## URL Pattern Mappings

  --------------------------------------------------------------------------------------------------------------------------------
  GitHub URL pattern                          get-hubbed equivalent                                     Type
  ------------------------------------------- --------------------------------------------------------- --------------------------
  `/{owner}/{repo}` (exact, 2 segments)       `get-hubbed://repo`                                       resource

  `/{owner}/{repo}/issues`                    `get-hubbed://issues?repo={o}/{r}`                        resource

  `/{owner}/{repo}/issues/{n}`                `get-hubbed://issues?number={n}&repo={o}/{r}`             resource

  `/{owner}/{repo}/pulls`                     `get-hubbed://pulls?repo={o}/{r}`                         resource

  `/{owner}/{repo}/pull/{n}`                  `get-hubbed://pulls?number={n}&repo={o}/{r}`              resource

  `/{owner}/{repo}/blob/{ref}/{path...}`      `get-hubbed://contents?path={p}&repo={o}/{r}&ref={ref}`   resource

  `/{owner}/{repo}/tree/{ref}/{path...}`      `get-hubbed://tree?repo={o}/{r}&path={p}&ref={ref}`       resource

  `/{owner}/{repo}/blame/{ref}/{path...}`     `get-hubbed://blame?path={p}&repo={o}/{r}&ref={ref}`      resource

  `/{owner}/{repo}/commits/{ref}`             `get-hubbed://commits?repo={o}/{r}&ref={ref}`             resource

  `/{owner}/{repo}/actions`                   `get-hubbed://runs?repo={o}/{r}`                          resource

  `/{owner}/{repo}/actions/runs/{id}`         `get-hubbed://runs?run_id={id}&repo={o}/{r}`              resource

  `/{owner}/{repo}/compare/{base}...{head}`   `content-compare` tool                                    tool
  --------------------------------------------------------------------------------------------------------------------------------

### Catch-all

Any GitHub domain URL not matching the above gets a generic deny listing all
resources and tools, plus: "Use get-hubbed for ALL GitHub interactions --- do
not use WebFetch or Bash with gh/curl for GitHub."

### Unmapped domains

URLs on `raw.githubusercontent.com`, `gist.github.com`, and unrecognized
`api.github.com` paths hit the catch-all. File GitHub issues for each domain
that lacks a specific mapping, with enough detail for a clean session to
implement support.

## Deny Message Format

### Specific match

    DENIED: Use get-hubbed://issues?number=42&repo=owner/repo instead.
    Use get-hubbed for ALL GitHub interactions — do not use WebFetch or Bash with
    gh/curl for GitHub.
    Subagents: use mcp__plugin_get-hubbed_get-hubbed__resource-read with uri
    get-hubbed://issues?number=42&repo=owner/repo

### Catch-all

    DENIED: GitHub URLs are served by get-hubbed. Do not use WebFetch for GitHub.
    Use get-hubbed for ALL GitHub interactions — do not use WebFetch or Bash with
    gh/curl for GitHub.

    Resources (read-only): get-hubbed://repo, get-hubbed://issues,
    get-hubbed://pulls, get-hubbed://contents, get-hubbed://tree,
    get-hubbed://blame, get-hubbed://commits, get-hubbed://runs
    Tools (mutations): issue-create, issue-close, issue-comment, pr-create,
    content-search, content-compare, api-get, graphql-query, graphql-mutation
    Discovery: resource-templates, resource-read
    Subagents: mcp__plugin_get-hubbed_get-hubbed__resource-read or
    mcp__plugin_get-hubbed_get-hubbed__<tool_name>

## Files Changed

1.  **New:** `packages/get-hubbed/internal/hooks/webfetch.go` --- URL parsing,
    mapping table, deny logic
2.  **New:** `packages/get-hubbed/internal/hooks/webfetch_test.go` --- unit
    tests
3.  **New:** `packages/get-hubbed/internal/hooks/patch.go` ---
    `PatchHooksMatcher()`
4.  **Modified:** `packages/get-hubbed/cmd/get-hubbed/main.go` --- stdin
    buffering, custom hook dispatch, custom generate-plugin with matcher
    patching

## Testing

Unit tests in `internal/hooks/`: - Each URL pattern maps to the correct resource
URI - Catch-all for unrecognized GitHub URLs on all domains - Non-WebFetch tools
pass through (not handled) - Non-GitHub URLs pass through - Malformed URLs
fail-open - Fragment and query parameter handling

## Rollback

Purely additive --- revert commits and reinstall marketplace. No
dual-architecture needed since no existing behavior is replaced.

## Follow-up Issues

File issues for: - `api.github.com` REST path parsing (similar patterns,
different URL structure) - `raw.githubusercontent.com` content serving via
get-hubbed - `gist.github.com` support - Framework support for
`App.ExtraHookMatchers` in purse-first go-mcp
