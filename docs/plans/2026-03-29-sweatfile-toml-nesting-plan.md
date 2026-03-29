# Sweatfile TOML Nesting Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Restructure sweatfile flat top-level keys into domain-specific TOML
tables (`[claude]`, `[git]`, `[direnv]`, `[session-entry]`).

**Architecture:** Replace flat `Sweatfile` struct fields with nested sub-structs
(`Claude`, `Git`, `Direnv`, `SessionEntry`). Regenerate the tommy decoder.
Update all consumers (merge, apply, validate, shop) to access fields through the
new sub-structs.

**Tech Stack:** Go, TOML (tommy codegen), BATS

**Rollback:** Spinclass v1 (`sc`) uses the old flat format and remains
unchanged.

--------------------------------------------------------------------------------

### Task 1: Restructure Sweatfile Struct

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/sweatfile.go`

**Step 1: Replace flat fields with sub-structs**

Replace the `Sweatfile` struct and add new sub-structs. Keep `Hooks` unchanged.
Rename `Session` to `SessionEntry`.

``` go
type Claude struct {
    SystemPrompt       *string  `toml:"system-prompt"`
    SystemPromptAppend *string  `toml:"system-prompt-append"`
    Allow              []string `toml:"allow"`
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

//go:generate tommy generate
type Sweatfile struct {
    Claude       *Claude       `toml:"claude"`
    Git          *Git          `toml:"git"`
    Direnv       *Direnv       `toml:"direnv"`
    Hooks        *Hooks        `toml:"hooks"`
    SessionEntry *SessionEntry `toml:"session-entry"`
}
```

Delete the old `Session` struct.

**Step 2: Update accessor methods**

Update all methods on `Sweatfile` that read the old flat fields to read from the
new sub-structs:

- `SessionStart()` --- read from `sf.SessionEntry` instead of `sf.Session`

- `SessionResume()` --- read from `sf.SessionEntry` instead of `sf.Session`

- `StopHookCommand()`, `CreateHookCommand()`, `PreMergeHookCommand()` ---
  unchanged (hooks struct unchanged)

- `DisallowMainWorktreeEnabled()`, `ToolUseLogEnabled()` --- unchanged

- `GetDefault()` --- construct using new sub-structs:

  ``` go
  func GetDefault() Sweatfile {
    sf := Sweatfile{
        Git: &Git{Excludes: []string{".spinclass/"}},
    }
    if home, err := os.UserHomeDir(); err == nil && home != "" {
        claudeDir := filepath.Join(home, ".claude")
        sf.Claude = &Claude{Allow: []string{fmt.Sprintf("Read(%s/*)", claudeDir)}}
    }
    return sf
  }
  ```

- `ExecClaude()` --- read `sf.Claude.SystemPrompt` and
  `sf.Claude.SystemPromptAppend` instead of `sf.SystemPrompt` /
  `sf.SystemPromptAppend`. Guard with nil check on `sf.Claude`.

**Step 3: Commit**

    refactor(spinclass2): restructure sweatfile struct with nested sub-types

--------------------------------------------------------------------------------

### Task 2: Update MergeWith

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/hierarchy.go`

**Step 1: Rewrite MergeWith for nested structs**

Replace the flat-field merge logic with sub-struct-aware merging. The pattern
for each sub-struct is:

- If `other.SubStruct` is nil, inherit (no change)
- If non-nil, create merged sub-struct and merge each field using existing
  semantics

``` go
func (sf Sweatfile) MergeWith(other Sweatfile) Sweatfile {
    merged := sf

    // [claude]
    if other.Claude != nil {
        if merged.Claude == nil {
            merged.Claude = &Claude{}
        }
        // system-prompt: deepest wins (with concatenation for non-empty)
        if other.Claude.SystemPrompt != nil {
            if *other.Claude.SystemPrompt == "" {
                merged.Claude.SystemPrompt = other.Claude.SystemPrompt
            } else if merged.Claude.SystemPrompt != nil && *merged.Claude.SystemPrompt != "" {
                joined := *merged.Claude.SystemPrompt + " " + *other.Claude.SystemPrompt
                merged.Claude.SystemPrompt = &joined
            } else {
                merged.Claude.SystemPrompt = other.Claude.SystemPrompt
            }
        }
        // system-prompt-append: same as system-prompt
        if other.Claude.SystemPromptAppend != nil {
            if *other.Claude.SystemPromptAppend == "" {
                merged.Claude.SystemPromptAppend = other.Claude.SystemPromptAppend
            } else if merged.Claude.SystemPromptAppend != nil && *merged.Claude.SystemPromptAppend != "" {
                joined := *merged.Claude.SystemPromptAppend + " " + *other.Claude.SystemPromptAppend
                merged.Claude.SystemPromptAppend = &joined
            } else {
                merged.Claude.SystemPromptAppend = other.Claude.SystemPromptAppend
            }
        }
        // allow: nil=inherit, empty=clear, non-empty=append
        if other.Claude.Allow != nil {
            if len(other.Claude.Allow) == 0 {
                merged.Claude.Allow = []string{}
            } else {
                merged.Claude.Allow = append(merged.Claude.Allow, other.Claude.Allow...)
            }
        }
    }

    // [git]
    if other.Git != nil {
        if merged.Git == nil {
            merged.Git = &Git{}
        }
        if other.Git.Excludes != nil {
            if len(other.Git.Excludes) == 0 {
                merged.Git.Excludes = []string{}
            } else {
                merged.Git.Excludes = append(merged.Git.Excludes, other.Git.Excludes...)
            }
        }
    }

    // [direnv]
    if other.Direnv != nil {
        if merged.Direnv == nil {
            merged.Direnv = &Direnv{}
        }
        if other.Direnv.Envrc != nil {
            if len(other.Direnv.Envrc) == 0 {
                merged.Direnv.Envrc = []string{}
            } else {
                merged.Direnv.Envrc = append(merged.Direnv.Envrc, other.Direnv.Envrc...)
            }
        }
        if other.Direnv.Dotenv != nil {
            if merged.Direnv.Dotenv == nil {
                merged.Direnv.Dotenv = make(map[string]string)
            }
            for k, v := range other.Direnv.Dotenv {
                merged.Direnv.Dotenv[k] = v
            }
        }
    }

    // [hooks] — unchanged from current implementation
    if other.Hooks != nil {
        // ... (keep existing hooks merge logic)
    }

    // [session-entry]
    if other.SessionEntry != nil {
        if merged.SessionEntry == nil {
            merged.SessionEntry = &SessionEntry{}
        }
        if len(other.SessionEntry.Start) > 0 {
            merged.SessionEntry.Start = other.SessionEntry.Start
        }
        if len(other.SessionEntry.Resume) > 0 {
            merged.SessionEntry.Resume = other.SessionEntry.Resume
        }
    }

    return merged
}
```

**Step 2: Commit**

    refactor(spinclass2): update MergeWith for nested sweatfile structs

--------------------------------------------------------------------------------

### Task 3: Update Parse and Coding

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/coding.go`

**Step 1: Update nil/empty normalization in Parse**

Change consumed-key checks from flat keys to nested paths:

``` go
func Parse(data []byte) (*SweatfileDocument, error) {
    doc, err := DecodeSweatfile(data)
    if err != nil {
        return nil, err
    }
    if doc.consumed["claude.allow"] && doc.data.Claude != nil && doc.data.Claude.Allow == nil {
        doc.data.Claude.Allow = []string{}
    }
    if doc.consumed["git.excludes"] && doc.data.Git != nil && doc.data.Git.Excludes == nil {
        doc.data.Git.Excludes = []string{}
    }
    if doc.consumed["direnv.envrc"] && doc.data.Direnv != nil && doc.data.Direnv.Envrc == nil {
        doc.data.Direnv.Envrc = []string{}
    }
    return doc, nil
}
```

**Step 2: Commit**

    refactor(spinclass2): update Parse for nested sweatfile keys

--------------------------------------------------------------------------------

### Task 4: Regenerate Tommy Decoder

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/sweatfile_tommy.go`
(auto-generated)

**Step 1: Run tommy generate**

``` sh
cd packages/spinclass2 && nix develop --command go generate ./internal/sweatfile/
```

**Step 2: Verify the generated code compiles**

``` sh
nix develop --command go build ./...
```

**Step 3: Commit**

    chore(spinclass2): regenerate tommy decoder for nested sweatfile

--------------------------------------------------------------------------------

### Task 5: Update Apply (Consumers)

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/apply.go`

**Step 1: Update Apply method**

The `Apply` method calls `GetDefault()` and merges, then passes to consumers.
`GetDefault()` already returns the new shape (Task 1). Update consumers:

- `ApplyClaudeSettings`: read `sweatfile.Claude.Allow` instead of
  `sweatfile.ClaudeAllow`. Guard with nil check:
  `if sweatfile.Claude != nil { allRules = append(allRules, sweatfile.Claude.Allow...) }`.
  Same for `StopHookCommand()` and `ToolUseLogEnabled()` --- these are unchanged
  (Hooks struct unchanged).

- `writeEnvrc`: read `sf.Direnv.Envrc` instead of `sf.EnvrcDirectives`. Read
  `sf.Direnv.Dotenv` instead of `sf.Env` for the dotenv directive check. Guard
  with nil: if `sf.Direnv == nil`, use default directives.

- `writeSpinclassEnv`: read `sf.Direnv.Dotenv` instead of `sf.Env`.

- `ExecClaude` (in sweatfile.go): read from `sf.Claude`.

**Step 2: Commit**

    refactor(spinclass2): update apply consumers for nested sweatfile

--------------------------------------------------------------------------------

### Task 6: Update External Consumers

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/internal/validate/validate.go` -
Modify: `packages/spinclass2/internal/shop/shop.go`

**Step 1: Update validate.go**

Change all references: - `sf.ClaudeAllow` → `sf.Claude.Allow` (with nil guard on
`sf.Claude`) - `sf.GitSkipIndex` → `sf.Git.Excludes` (with nil guard on
`sf.Git`) - `src.File.ClaudeAllow` → `src.File.Claude` nil check then `.Allow` -
`src.File.GitSkipIndex` → `src.File.Git` nil check then `.Excludes`

For the `CheckGitExcludes` call:

``` go
// Old: sweatfile.Sweatfile{GitSkipIndex: merged.GitSkipIndex}
// New:
sweatfile.Sweatfile{Git: &sweatfile.Git{Excludes: merged.Git.Excludes}}
```

**Step 2: Update shop.go**

Change log lines: - `src.File.GitSkipIndex` → access via `src.File.Git` -
`src.File.ClaudeAllow` → access via `src.File.Claude` - `merged.GitSkipIndex` →
`merged.Git` (nil-safe) - `merged.ClaudeAllow` → `merged.Claude` (nil-safe)

**Step 3: Commit**

    refactor(spinclass2): update validate and shop for nested sweatfile

--------------------------------------------------------------------------------

### Task 7: Update Unit Tests

**Promotion criteria:** N/A

**Files:** - Modify:
`packages/spinclass2/internal/sweatfile/sweatfile_test.go` - Modify:
`packages/spinclass2/internal/sweatfile/apply_test.go`

**Step 1: Update sweatfile_test.go**

All TOML literals in tests change from flat to nested format:

    # Old:
    git-excludes = [".claude/"]
    claude-allow = ["Read", "Bash(git *)"]

    # New:
    [git]
    excludes = [".claude/"]

    [claude]
    allow = ["Read", "Bash(git *)"]

All struct field accesses change: - `sf.GitSkipIndex` → `sf.Git.Excludes` (with
nil guard) - `sf.ClaudeAllow` → `sf.Claude.Allow` (with nil guard) - `sf.Hooks`
--- unchanged - `Sweatfile{GitSkipIndex: ...}` →
`Sweatfile{Git: &Git{Excludes: ...}}` - `Sweatfile{ClaudeAllow: ...}` →
`Sweatfile{Claude: &Claude{Allow: ...}}`

For nil checks (e.g. `TestParseEmpty`): - `sf.GitSkipIndex != nil` →
`sf.Git != nil` (the whole table is absent)

For merge tests, construct using new sub-structs.

**Step 2: Update apply_test.go**

- `Sweatfile{ClaudeAllow: rules}` → `Sweatfile{Claude: &Claude{Allow: rules}}`
- `Sweatfile{Env: map[string]string{...}}` →
  `Sweatfile{Direnv: &Direnv{Dotenv: map[string]string{...}}}`
- `Sweatfile{EnvrcDirectives: ...}` → `Sweatfile{Direnv: &Direnv{Envrc: ...}}`
- `Sweatfile{Hooks: &Hooks{Stop: &cmd}}` --- unchanged

**Step 3: Run tests**

``` sh
nix develop --command go test ./packages/spinclass2/internal/sweatfile/... -v
```

**Step 4: Commit**

    test(spinclass2): update unit tests for nested sweatfile structs

--------------------------------------------------------------------------------

### Task 8: Update BATS Integration Tests

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass2/zz-tests_bats/sweatfile.bats`

**Step 1: Update sweatfile TOML in test fixtures**

``` bash
# Old:
claude-allow = ["Bash(git *)"]

# New:
[claude]
allow = ["Bash(git *)"]
```

``` bash
# Old:
[session]
start = ["echo", "$SPINCLASS_SESSION", "$SPINCLASS_BRANCH"]

# New:
[session-entry]
start = ["echo", "$SPINCLASS_SESSION", "$SPINCLASS_BRANCH"]
```

**Step 2: Run BATS tests**

``` sh
nix develop --command bats --tap packages/spinclass2/zz-tests_bats/sweatfile.bats
```

**Step 3: Commit**

    test(spinclass2): update BATS tests for nested sweatfile syntax

--------------------------------------------------------------------------------

### Task 9: Run Full Test Suite

**Promotion criteria:** N/A

**Step 1: Run all spinclass2 tests**

``` sh
just test-spinclass2
```

**Step 2: Build the package**

``` sh
nix build .#spinclass2
```

**Step 3: Fix any failures, commit fixes**
