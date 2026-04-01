# WebFetch Interception: api.github.com, raw.githubusercontent.com, gist.github.com

**Date:** 2026-04-01
**Status:** proposed
**Issues:** #69, #70, #71

## Problem

The WebFetch hook (`internal/hooks/webfetch.go`) correctly identifies all five
GitHub domains and denies WebFetch for all of them. But `matchGitHubURL` only
parses paths for `github.com` and `www.github.com`. URLs on `api.github.com`,
`raw.githubusercontent.com`, and `gist.github.com` hit the generic catch-all
deny instead of returning a specific get-hubbed resource URI.

## Solution

Add three host-specific match functions and two new resources
(`get-hubbed://gist`, `get-hubbed://compare`). Dispatch by host in
`HandleWebFetchHook` instead of calling `matchGitHubURL` for all hosts.

## Decisions

- **Separate functions per host** --- `matchAPIGitHubURL`,
  `matchRawGitHubURL`, `matchGistGitHubURL`. Cleaner than extending
  `matchGitHubURL` with a host switch.
- **`get-hubbed://gist` (singular)** --- agents naturally guess singular form.
  Prioritizing model ergonomics over consistency with `issues`/`pulls`/`runs`.
- **`get-hubbed://compare` resource** --- agents guess `get-hubbed://compare`
  rather than the `content-compare` tool name. Adding a resource that matches
  what models naturally produce reduces incorrect usage. File a follow-up issue
  to deprecate the `content-compare` tool.
- **Update existing compare mapping** --- the `github.com/compare` path in
  `matchGitHubURL` currently points to `content-compare` tool. Update it to
  point to `get-hubbed://compare` resource.

## HandleWebFetchHook dispatch

Replace the single `matchGitHubURL` call with host-based dispatch:

``` go
switch parsed.Host {
case "github.com", "www.github.com":
    resourceURI, isTool = matchGitHubURL(rawURL)
case "api.github.com":
    resourceURI, isTool = matchAPIGitHubURL(rawURL)
case "raw.githubusercontent.com":
    resourceURI, isTool = matchRawGitHubURL(rawURL)
case "gist.github.com":
    resourceURI, isTool = matchGistGitHubURL(rawURL)
}
```

## URL Pattern Mappings

### api.github.com (#69)

All paths start with `/repos/{owner}/{repo}/`. Extract repo slug from segments
1-2 (after `/repos/` prefix).

| API path                                     | get-hubbed resource                                     |
|----------------------------------------------|---------------------------------------------------------|
| `/repos/{o}/{r}`                             | `get-hubbed://repo`                                     |
| `/repos/{o}/{r}/issues`                      | `get-hubbed://issues?repo={o}/{r}`                      |
| `/repos/{o}/{r}/issues/{n}`                  | `get-hubbed://issues?number={n}&repo={o}/{r}`           |
| `/repos/{o}/{r}/pulls`                       | `get-hubbed://pulls?repo={o}/{r}`                       |
| `/repos/{o}/{r}/pulls/{n}`                   | `get-hubbed://pulls?number={n}&repo={o}/{r}`            |
| `/repos/{o}/{r}/contents/{path}`             | `get-hubbed://contents?path={p}&repo={o}/{r}`           |
| `/repos/{o}/{r}/git/trees/{ref}`             | `get-hubbed://tree?repo={o}/{r}&ref={ref}`              |
| `/repos/{o}/{r}/actions/runs`                | `get-hubbed://runs?repo={o}/{r}`                        |
| `/repos/{o}/{r}/actions/runs/{id}`           | `get-hubbed://runs?run_id={id}&repo={o}/{r}`            |
| `/repos/{o}/{r}/compare/{base}...{head}`     | `get-hubbed://compare?repo={o}/{r}&base={b}&head={h}`   |

Unrecognized `/repos/` paths fall through to catch-all.

### raw.githubusercontent.com (#70)

Path structure: `/{owner}/{repo}/{ref}/{path...}` (always at least 4 segments).

| URL path                        | get-hubbed resource                                         |
|---------------------------------|-------------------------------------------------------------|
| `/{o}/{r}/{ref}/{path...}`      | `get-hubbed://contents?path={path}&repo={o}/{r}&ref={ref}`  |

### gist.github.com (#71)

Path structure: `/{owner}/{gist_id}`.

| URL path              | get-hubbed resource              |
|-----------------------|----------------------------------|
| `/{owner}/{gist_id}`  | `get-hubbed://gist?id={gist_id}` |

## New Resources

### get-hubbed://gist

- Template: `get-hubbed://gist?id={id}`
- Required: `id`
- Backed by: `gh api /gists/{id}`
- Returns: gist metadata + file contents

### get-hubbed://compare

- Template: `get-hubbed://compare?repo={repo}&base={base}&head={head}`
- Required: `base`, `head`
- Optional: `repo` (defaults to current)
- Backed by: `gh api repos/{repo}/compare/{base}...{head}`
- Returns: comparison metadata (commits, files changed)

## Files Changed

1. `packages/get-hubbed/internal/hooks/webfetch.go` --- 3 new match functions,
   host-based dispatch, update existing compare mapping from tool to resource
2. `packages/get-hubbed/internal/hooks/webfetch_test.go` --- test tables for
   each new host
3. `packages/get-hubbed/internal/tools/resources.go` --- new `gist` resource
   template + `readGist`, new `compare` resource template + `readCompare`

## Follow-up

- File issue to deprecate `content-compare` tool in favor of
  `get-hubbed://compare` resource

## Rollback

Purely additive. Revert commits and reinstall marketplace.
