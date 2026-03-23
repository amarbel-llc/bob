# Sweatfile Tommy Codegen Migration --- Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Replace BurntSushi/toml with tommy codegen in spinclass's sweatfile
package, threading `*SweatfileDocument` through the API for comment-preserving
round-trips.

**Architecture:** Add `//go:generate tommy generate` to struct definitions.
Tommy generates `SweatfileDocument` with CST-backed decode/encode. Rewrite
`Parse`/`Load`/`Save` as free functions returning `*SweatfileDocument`. Callers
use `doc.Data()` to access the `Sweatfile` struct. `validate.go` uses
`doc.Undecoded()` instead of BurntSushi's metadata.

**Tech Stack:** tommy codegen (`github.com/amarbel-llc/tommy`), Go 1.24

**Rollback:** `git revert <sha>` --- single commit restores BurntSushi/toml.

--------------------------------------------------------------------------------

### Task 1: Add tommy dependency

**Files:** - Modify: `packages/spinclass/go.mod`

**Step 1: Add tommy dependency**

Run from repo root:

``` sh
nix develop .#go --command bash -c "cd packages/spinclass && go get github.com/amarbel-llc/tommy@latest"
```

**Step 2: Sync workspace and vendor**

``` sh
just go-mod-sync
```

**Step 3: Verify build**

``` sh
nix develop --command go build ./packages/spinclass/...
```

Expected: builds successfully.

**Step 4: Commit**

    feat(spinclass): add tommy dependency for codegen migration

    Ref: #44

--------------------------------------------------------------------------------

### Task 2: Add codegen directives and generate

**Files:** - Modify: `packages/spinclass/internal/sweatfile/sweatfile.go` -
Create: `packages/spinclass/internal/sweatfile/sweatfile_tommy.go` (generated)

**Step 1: Add `//go:generate` directive above Hooks struct**

In `packages/spinclass/internal/sweatfile/sweatfile.go`, add before
`type Hooks struct`:

``` go
//go:generate tommy generate
type Hooks struct {
```

**Step 2: Add `//go:generate` directive above Sweatfile struct**

``` go
//go:generate tommy generate
type Sweatfile struct {
```

**Step 3: Run code generation**

``` sh
nix develop --command go generate ./packages/spinclass/internal/sweatfile/
```

Expected: creates `packages/spinclass/internal/sweatfile/sweatfile_tommy.go`
with: - `HooksDocument` type + `DecodeHooks` + methods - `SweatfileDocument`
type + `DecodeSweatfile` + `Data()` + `Encode()` + `Undecoded()`

**Step 4: Verify generated file compiles**

``` sh
nix develop --command go build ./packages/spinclass/internal/sweatfile/
```

**Step 5: Commit**

    feat(spinclass): add tommy codegen for sweatfile structs

    Generated SweatfileDocument with CST-backed round-trip support.

    Ref: #44

--------------------------------------------------------------------------------

### Task 3: Write round-trip comment preservation test

**Files:** - Modify: `packages/spinclass/internal/sweatfile/sweatfile_test.go`

**Step 1: Write the failing test**

Add to `sweatfile_test.go`:

``` go
func TestRoundTripPreservesComments(t *testing.T) {
    input := `# Global config
system-prompt = "be helpful"
git-excludes = [".claude/", ".direnv/"]
claude-allow = ["Bash(git *)"]
envrc-directives = ["source_up", "use flake"]

[env]
FOO = "bar"
BAZ = "qux"

[hooks]
# install deps on create
create = "npm install"
stop = "just test"
disallow-main-worktree = true
`
    doc, err := Parse([]byte(input))
    if err != nil {
        t.Fatalf("Parse error: %v", err)
    }

    output, err := doc.Encode()
    if err != nil {
        t.Fatalf("Encode error: %v", err)
    }

    if string(output) != input {
        t.Errorf("round-trip mismatch:\n--- want ---\n%s\n--- got ---\n%s", input, string(output))
    }
}
```

**Step 2: Run test to verify it fails**

``` sh
nix develop --command go test -run TestRoundTripPreservesComments ./packages/spinclass/internal/sweatfile/
```

Expected: FAIL --- `Parse` is still a method on `*Sweatfile`, not a free
function.

--------------------------------------------------------------------------------

### Task 4: Rewrite coding.go --- Parse, Load, Save

**Files:** - Modify: `packages/spinclass/internal/sweatfile/coding.go`

**Step 1: Rewrite coding.go**

Replace entire contents of `packages/spinclass/internal/sweatfile/coding.go`:

``` go
package sweatfile

import (
    "errors"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
)

func Parse(data []byte) (*SweatfileDocument, error) {
    return DecodeSweatfile(data)
}

func Load(path string) (*SweatfileDocument, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return DecodeSweatfile(nil)
        }
        return nil, err
    }
    return Parse(data)
}

// resolvePathOrString expands environment variables and ~ in value, then
// tries to read it as a file path. If the file exists, its contents are
// returned (trimmed). Otherwise value is returned as a literal string.
func resolvePathOrString(value string) string {
    expanded := os.ExpandEnv(value)
    if strings.HasPrefix(expanded, "~/") {
        if home, err := os.UserHomeDir(); err == nil {
            expanded = filepath.Join(home, expanded[2:])
        }
    }

    data, err := os.ReadFile(expanded)
    if err != nil {
        return value
    }
    return strings.TrimSpace(string(data))
}

func (doc *SweatfileDocument) Save(path string) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    output, err := doc.Encode()
    if err != nil {
        return err
    }
    return os.WriteFile(path, output, 0o644)
}
```

**Step 2: Run the round-trip test**

``` sh
nix develop --command go test -run TestRoundTripPreservesComments ./packages/spinclass/internal/sweatfile/
```

Expected: PASS (but other tests will fail --- they still use the old
method-based API).

**Step 3: Commit**

    feat(spinclass): rewrite Parse/Load/Save for tommy codegen

    Parse and Load are now free functions returning *SweatfileDocument.
    Save is a method on *SweatfileDocument that preserves comments.

    Ref: #44

--------------------------------------------------------------------------------

### Task 5: Update sweatfile_test.go

**Files:** - Modify: `packages/spinclass/internal/sweatfile/sweatfile_test.go`

All `var sf Sweatfile; sf.Parse(data)` patterns become
`doc, err := Parse(data); sf := doc.Data()`. All
`var sf Sweatfile; sf.Load(path)` patterns become
`doc, err := Load(path); sf := doc.Data()`. `TestSaveRoundTrip` uses
`doc.Save(path)`.

**Step 1: Update Parse call sites**

Each test that does:

``` go
var sf Sweatfile
err := sf.Parse([]byte(input))
```

becomes:

``` go
doc, err := Parse([]byte(input))
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
sf := doc.Data()
```

Tests to update (19 sites): - `TestParseMinimal` (line 13-14) - `TestParseEmpty`
(line 24-25) - `TestParseClaudeAllow` (line 125-126) - `TestParseHooksCreate`
(line 370-371) - `TestParseHooksStop` (line 386-387) - `TestParseHooksBoth`
(line 403-404) - `TestParseHooksAbsent` (line 420-421) -
`TestParseHooksPreMerge` (line 522-523) - `TestParseSystemPrompt` (line
630-631) - `TestParseSystemPromptEmpty` (line 641-642) -
`TestParseSystemPromptAbsent` (line 656-657) - `TestParseSystemPromptAppend`
(line 668-669) - `TestParseSystemPromptAppendAbsent` (line 680-681) -
`TestParseEnvrcDirectives` (line 783-784) - `TestParseEnvrcDirectivesAbsent`
(line 798-799) - `TestParseEnv` (line 847-848) - `TestParseEnvAbsent` (line
861-862) - `TestParseHooksDisallowMainWorktree` (line 928-929) -
`TestParseHooksDisallowMainWorktreeAbsent` (line 939-940)

**Step 2: Update Load call sites**

`TestLoadFromPath` (line 39-40):

``` go
doc, err := Load(path)
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
sf := doc.Data()
```

`TestLoadMissing` (line 50-51):

``` go
doc, err := Load("/nonexistent/sweatfile")
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
sf := doc.Data()
```

**Step 3: Update TestSaveRoundTrip** (line 98-119)

``` go
func TestSaveRoundTrip(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "sweatfile")

    input := "git-excludes = [\".claude/\"]\n"
    doc, err := Parse([]byte(input))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    err = doc.Save(path)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    loaded, err := Load(path)
    if err != nil {
        t.Fatalf("unexpected error loading: %v", err)
    }
    sf := loaded.Data()
    if len(sf.GitSkipIndex) != 1 || sf.GitSkipIndex[0] != ".claude/" {
        t.Errorf("git-excludes roundtrip: got %v", sf.GitSkipIndex)
    }
}
```

**Step 4: Run all sweatfile tests**

``` sh
nix develop --command go test -v ./packages/spinclass/internal/sweatfile/
```

Expected: all PASS.

**Step 5: Commit**

    refactor(spinclass): update sweatfile tests for document-based API

    Mechanical update: Parse/Load return *SweatfileDocument, use doc.Data()
    for struct access.

    Ref: #44

--------------------------------------------------------------------------------

### Task 6: Update hierarchy.go

**Files:** - Modify: `packages/spinclass/internal/sweatfile/hierarchy.go`

**Step 1: Update loadAndMerge closure** (line 43-57)

``` go
    loadAndMerge := func(path string) error {
        doc, err := Load(path)
        if err != nil {
            return err
        }
        _, found := fileExists(path)
        sources = append(
            sources,
            LoadSource{Path: path, Found: found, File: *doc.Data()},
        )
        if found {
            merged = merged.MergeWith(*doc.Data())
        }
        return nil
    }
```

**Step 2: Update LoadWorktreeHierarchy** (line 105-117)

``` go
    worktreePath := filepath.Join(filepath.Clean(worktreeDir), "sweatfile")
    doc, err := Load(worktreePath)
    if err != nil {
        return Hierarchy{}, err
    }

    _, found := fileExists(worktreePath)
    hierarchy.Sources = append(hierarchy.Sources, LoadSource{
        Path: worktreePath, Found: found, File: *doc.Data(),
    })
    if found {
        hierarchy.Merged = hierarchy.Merged.MergeWith(*doc.Data())
    }
```

**Step 3: Run all spinclass tests**

``` sh
nix develop --command go test -v ./packages/spinclass/...
```

Expected: all PASS.

**Step 4: Commit**

    refactor(spinclass): update hierarchy.go for document-based Load

    Ref: #44

--------------------------------------------------------------------------------

### Task 7: Update validate.go --- use doc.Undecoded()

**Files:** - Modify: `packages/spinclass/internal/validate/validate.go`

**Step 1: Update CheckUnknownFields** (line 152-168)

``` go
func CheckUnknownFields(data []byte) []Issue {
    doc, err := sweatfile.Parse(data)
    if err != nil {
        return nil
    }

    var issues []Issue
    for _, key := range doc.Undecoded() {
        issues = append(issues, Issue{
            Message:  fmt.Sprintf("unknown field %q", key),
            Severity: SeverityError,
            Field:    key,
        })
    }
    return issues
}
```

**Step 2: Update Run function parse call** (line 202-203)

``` go
        doc, parseErr := sweatfile.Parse(data)
        if parseErr != nil {
            sub.NotOk("valid TOML", map[string]string{
                "severity": SeverityError,
                "message":  parseErr.Error(),
            })
            sub.Plan()
            tw.EndSubtest(src.Path, sub)
            continue
        }
        sub.Ok("valid TOML")
```

Note: the `src.File` references on lines 227-228 and 248-249 use `src.File` from
the hierarchy (already a `Sweatfile` struct from Task 6), so they don't need
changes.

**Step 3: Remove BurntSushi/toml import**

Remove `"github.com/BurntSushi/toml"` from the import block.

**Step 4: Run validate tests**

``` sh
nix develop --command go test -v ./packages/spinclass/internal/validate/...
```

Expected: all PASS.

**Step 5: Commit**

    refactor(spinclass): use tommy Undecoded() in validate.go

    Replaces BurntSushi/toml metadata-based unknown field detection.

    Ref: #44

--------------------------------------------------------------------------------

### Task 8: Remove BurntSushi/toml dependency

**Files:** - Modify: `packages/spinclass/go.mod`

**Step 1: Remove the dependency**

``` sh
nix develop .#go --command bash -c "cd packages/spinclass && go mod tidy"
```

**Step 2: Sync workspace and vendor**

``` sh
just go-mod-sync
```

**Step 3: Verify full build**

``` sh
nix develop --command go test ./packages/spinclass/...
```

Expected: all tests pass, `github.com/BurntSushi/toml` no longer in `go.mod`.

**Step 4: Verify nix build**

``` sh
nix build .#spinclass
```

**Step 5: Commit**

    chore(spinclass): remove BurntSushi/toml dependency

    Tommy codegen fully replaces BurntSushi/toml for TOML parsing and
    encoding. Comment-preserving round-trips now work via CST.

    Closes #44
