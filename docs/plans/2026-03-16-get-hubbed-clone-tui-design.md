# get-hubbed clone TUI

## Command Interface

```
get-hubbed clone [target-dir]
```

- `target-dir` defaults to cwd
- No flags beyond `--help`

## Flow

1. Validate target directory exists
2. Fetch authenticated user's repos via `gh api user/repos --paginate -q '.[].full_name'`
3. For each repo, check if `<target-dir>/<repo-name>/.git` exists via
   `git -C <target-dir>/<repo-name> rev-parse --git-dir`
4. Filter to uncloned repos only
5. If none are uncloned, print a message and exit
6. Present a `huh.NewMultiSelect` with uncloned repo names (sorted alphabetically)
7. If user selects nothing, exit
8. Clone selected repos in parallel via tap-dancer's
   `ConvertExecParallelWithStatus` with template
   `gh repo clone <full-name> <target-dir>/<repo-name>`

## Implementation Structure

**New files:**
- `packages/get-hubbed/internal/clone/clone.go` — single `Run(ctx, targetDir)` function

**Modified files:**
- `packages/get-hubbed/cmd/get-hubbed/main.go` — dispatch `clone` subcommand
- `packages/get-hubbed/go.mod` — add `huh` and `tap-dancer` dependencies

## Repo Fetching & Clone Detection

- Fetch via `gh api user/repos --paginate` using existing `internal/gh` package
- Parse `full_name` (e.g., `friedenberg/dotfiles`) into owner + repo name
- Clone detection: `git -C <target-dir>/<repo-name> rev-parse --git-dir`
  (exit 0 = cloned, non-zero = not cloned)
- Clone target: `<target-dir>/<repo-name>/` (repo name only, not full_name)

## Error Handling

- Not authenticated: surface `gh` error and exit
- Target dir doesn't exist: error before fetching
- Zero repos / all cloned: informational message and exit
- Clone failures: tap-dancer shows `not ok` with stderr diagnostics, exit code 1
- Ctrl-C: huh handles prompt cancellation; context cancellation propagates
  through tap-dancer executor
- No retry, no partial cleanup — re-run detects state via `.git` presence

## Dependencies

- `charmbracelet/huh` — multi-select prompt
- `github.com/amarbel-llc/bob/packages/tap-dancer/go` — parallel exec with TAP status
