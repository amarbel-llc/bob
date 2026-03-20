# Sweatfile OO Refactor + PathOrString Resolution

## Summary

Three changes to `packages/spinclass/internal/sweatfile/`:

1.  Convert `Parse`, `Load`, `Save`, `Merge` from free functions to methods on
    `Sweatfile`
2.  Add file resolution for `SystemPrompt` and `SystemPromptAppend` --- if the
    value is a readable file path, replace with file contents
3.  Remove `BranchNameCommand` / `CreateBranchName` (deferred to
    amarbel-llc/bob#15)

## Item 1: Methods on Sweatfile

  --------------------------------------------------------------------------------------------------
  Before                                     After
  ------------------------------------------ -------------------------------------------------------
  `Parse(data []byte) (Sweatfile, error)`    `(sf *Sweatfile) Parse(data []byte) error`

  `Load(path string) (Sweatfile, error)`     `(sf *Sweatfile) Load(path string) error`

  `Save(path string, sf Sweatfile) error`    `(sf Sweatfile) Save(path string) error`

  `Merge(base, repo Sweatfile) Sweatfile`    `(sf Sweatfile) MergeWith(other Sweatfile) Sweatfile`
  --------------------------------------------------------------------------------------------------

`Parse`/`Load` use pointer receivers (mutate in place on zero-value). `Save`/
`MergeWith` use value receivers.

Callers to update: - `hierarchy.go`: `LoadHierarchy`, `LoadWorktreeHierarchy`
--- use `var sf Sweatfile; sf.Load(path)` and `merged.MergeWith(sf)` -
`apply.go`: `Apply` --- `sf.MergeWith(defaults)` instead of
`Merge(sf, defaults)` - All tests calling `Parse(...)`, `Load(...)`,
`Save(...)`, `Merge(...)`

## Item 2: PathOrString Resolution

After TOML unmarshaling in `Parse`, resolve `SystemPrompt` and
`SystemPromptAppend`:

1.  If nil, skip
2.  Expand env vars via `os.Expand` with `os.Getenv`
3.  Expand `~` prefix to `$HOME`
4.  Attempt `os.ReadFile` on the expanded path
5.  If file exists, replace value with file contents (trimmed)
6.  If not a file, keep the original raw string

No new type --- the fields stay `*string`. Resolution is behavior in `Parse`.

## Item 3: Remove BranchNameCommand

Delete: - `BranchNameCommand` field from `Sweatfile` struct - `CreateBranchName`
method - `shlex` import (only used by `CreateBranchName`) - Caller in
`worktree.ResolvePath()` --- pass `sanitizedName` directly

GitHub issue for re-examination: amarbel-llc/bob#15

## Rollback

All changes are internal refactors. TOML field `branch-name-command` becomes
silently ignored (TOML unmarshaling skips unknown keys). No wire format change
for other fields. Rollback = revert commit.
