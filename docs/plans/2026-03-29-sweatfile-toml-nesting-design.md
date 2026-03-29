# Sweatfile TOML Nesting Restructure

## Problem

The sweatfile format mixes flat top-level keys (`claude-allow`, `git-excludes`,
`envrc-directives`, `system-prompt`, `system-prompt-append`) with properly
nested TOML tables (`[hooks]`, `[session]`, `[env]`). The flat keys don't
communicate their domain and don't follow TOML table semantics.

## Design

Restructure all top-level keys into domain-specific TOML tables. Clean break ---
old flat key names become parse errors.

### Before

``` toml
system-prompt = "..."
system-prompt-append = "..."
claude-allow = ["Bash(git *)"]
git-excludes = [".spinclass/"]
envrc-directives = ["source_up", "use flake"]

[env]
FOO = "bar"

[hooks]
create = "..."
tool-use-log = true

[session]
start = ["zellij"]
```

### After

``` toml
[claude]
allow = ["Bash(git *)"]
system-prompt = "..."
system-prompt-append = "..."

[git]
excludes = [".spinclass/"]

[direnv]
envrc = ["source_up", "use flake"]

[direnv.dotenv]
FOO = "bar"

[hooks]
create = "..."
stop = "..."
pre-merge = "..."
disallow-main-worktree = true
tool-use-log = true

[session-entry]
start = ["zellij", "-s", "$SPINCLASS_SESSION"]
resume = ["zellij", "attach", "$SPINCLASS_SESSION"]
```

### Key Mapping

  Old                      New
  ------------------------ ---------------------------------
  `claude-allow`           `[claude] allow`
  `system-prompt`          `[claude] system-prompt`
  `system-prompt-append`   `[claude] system-prompt-append`
  `git-excludes`           `[git] excludes`
  `envrc-directives`       `[direnv] envrc`
  `[env]`                  `[direnv.dotenv]`
  `[session]`              `[session-entry]`
  `[hooks]`                `[hooks]` (unchanged)

### New Structs

``` go
type Claude struct {
    Allow              []string `toml:"allow"`
    SystemPrompt       *string  `toml:"system-prompt"`
    SystemPromptAppend *string  `toml:"system-prompt-append"`
}

type Git struct {
    Excludes []string `toml:"excludes"`
}

type Direnv struct {
    Envrc  []string          `toml:"envrc"`
    Dotenv map[string]string `toml:"dotenv"`
}

type SessionEntry struct {
    Start  []string `toml:"start"`
    Resume []string `toml:"resume"`
}

type Hooks struct {
    Create               *string `toml:"create"`
    Stop                 *string `toml:"stop"`
    PreMerge             *string `toml:"pre-merge"`
    DisallowMainWorktree *bool   `toml:"disallow-main-worktree"`
    ToolUseLog           *bool   `toml:"tool-use-log"`
}

type Sweatfile struct {
    Claude       *Claude       `toml:"claude"`
    Git          *Git          `toml:"git"`
    Direnv       *Direnv       `toml:"direnv"`
    Hooks        *Hooks        `toml:"hooks"`
    SessionEntry *SessionEntry `toml:"session-entry"`
}
```

### Merge Semantics

Unchanged from current behavior, applied within each sub-struct:

- **Arrays** (`allow`, `excludes`, `envrc`): nil = inherit, empty = clear,
  non-empty = append
- **Maps** (`dotenv`): merge all keys, deepest wins per key
- **Scalars** (`system-prompt`, hook strings, booleans): deepest wins
- **Tables** (`[claude]`, `[git]`, `[direnv]`, `[hooks]`, `[session-entry]`):
  per-field override, nil = inherit

### Migration

Clean break. Old flat key names are rejected by the tommy decoder as undecoded
keys. No backward compatibility layer.

### Rollback

Spinclass v1 (`sc`) is the rollback --- it uses the old flat format and remains
unchanged.

## Files Affected

1.  `sweatfile.go` --- new sub-structs, update accessor methods and
    `GetDefault()`
2.  `sweatfile_tommy.go` --- regenerate via `tommy generate`
3.  `hierarchy.go` --- update `MergeWith` for nested struct shape
4.  `coding.go` --- update `Parse` nil/empty normalization for new key paths
5.  `apply.go` --- update consumers to read from sub-structs
6.  `apply_test.go` --- update test fixtures
7.  `sweatfile.bats` --- update all sweatfile TOML in test fixtures
