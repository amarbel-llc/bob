# Sweatfile OO Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Convert sweatfile free functions to methods, add file resolution for
system prompt fields, and remove BranchNameCommand.

**Architecture:** Three independent refactors in the
`packages/spinclass/internal/sweatfile/` package plus one caller update in
`packages/spinclass/internal/worktree/worktree.go`. All changes are internal ---
no TOML format or CLI surface changes.

**Tech Stack:** Go, TOML (BurntSushi/toml)

**Rollback:** Revert commit. No wire format changes.

--------------------------------------------------------------------------------

### Task 1: Remove BranchNameCommand

Do this first --- it's a pure deletion and reduces noise in later tasks.

**Promotion criteria:** N/A

**Files:** - Modify:
`packages/spinclass/internal/sweatfile/sweatfile.go:22-97` - Modify:
`packages/spinclass/internal/worktree/worktree.go:55-58`

**Step 1: Remove BranchNameCommand field and CreateBranchName method from
sweatfile.go**

Remove the `BranchNameCommand` field from the `Sweatfile` struct (line 25) and
the entire `CreateBranchName` method (lines 75-97). Also remove the `shlex`
import (line 12) and any imports that become unused (`bytes` on line 4).

The struct should become:

``` go
type Sweatfile struct {
    SystemPrompt       *string           `toml:"system-prompt"`
    SystemPromptAppend *string           `toml:"system-prompt-append"`
    GitSkipIndex       []string          `toml:"git-excludes"`
    ClaudeAllow        []string          `toml:"claude-allow"`
    EnvrcDirectives    []string          `toml:"envrc-directives"`
    Env                map[string]string `toml:"env"`
    Hooks              *Hooks            `toml:"hooks"`
}
```

Remove unused imports: `bytes`, `github.com/google/shlex`. Keep `fmt`, `os`,
`os/exec`, `path/filepath`, `slices`, `strings`.

**Step 2: Update ResolvePath in worktree.go to skip CreateBranchName**

In `packages/spinclass/internal/worktree/worktree.go`, the `ResolvePath`
function calls `sf.CreateBranchName(sanitizedName)` on lines 55-58. Replace
those lines so that `sanitizedName` is used directly:

Replace lines 55-60:

``` go
    transformedName, err := sf.CreateBranchName(sanitizedName)
    if err != nil {
        return ResolvedPath{}, err
    }

    branch, existingBranch := detectBranch(repoPath, unsanitizedName, sanitizedName, transformedName)
```

With:

``` go
    branch, existingBranch := detectBranch(repoPath, unsanitizedName, sanitizedName)
```

Since `ResolvePath` no longer uses the `sf` parameter, remove it from the
signature:

``` go
func ResolvePath(
    repoPath string,
    args []string,
) (ResolvedPath, error) {
```

Then remove the `sweatfile` import from worktree.go if it becomes unused (it
won't --- `Create` and `CreateFrom` still use it).

**Step 3: Update all callers of ResolvePath**

Search for all callers of `worktree.ResolvePath` or `ResolvePath` in the
spinclass package. Each caller currently passes a `sweatfile.Sweatfile` as the
first arg --- remove it.

Run: `grep -rn 'ResolvePath' packages/spinclass/`

Update each call site to drop the `sf` argument.

**Step 4: Run tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: All
tests pass. No test references `BranchNameCommand` or `CreateBranchName`.

**Step 5: Commit**

    feat(spinclass): remove BranchNameCommand

    Deferred to amarbel-llc/bob#15 for re-examination with real-world data.

--------------------------------------------------------------------------------

### Task 2: Convert Parse/Load to pointer-receiver methods

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/sweatfile/coding.go` - Modify:
`packages/spinclass/internal/sweatfile/hierarchy.go:44,101` - Modify:
`packages/spinclass/internal/sweatfile/sweatfile_test.go` (all `Parse(...)` and
`Load(...)` calls)

**Step 1: Convert Parse to a pointer-receiver method**

In `coding.go`, change:

``` go
func Parse(data []byte) (Sweatfile, error) {
    var sf Sweatfile
    if err := toml.Unmarshal(data, &sf); err != nil {
        return Sweatfile{}, err
    }
    return sf, nil
}
```

To:

``` go
func (sf *Sweatfile) Parse(data []byte) error {
    return toml.Unmarshal(data, sf)
}
```

**Step 2: Convert Load to a pointer-receiver method**

In `coding.go`, change:

``` go
func Load(path string) (Sweatfile, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return Sweatfile{}, nil
        }
        return Sweatfile{}, err
    }
    return Parse(data)
}
```

To:

``` go
func (sf *Sweatfile) Load(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return nil
        }
        return err
    }
    return sf.Parse(data)
}
```

**Step 3: Update callers in hierarchy.go**

In `LoadHierarchy` (line 44), the `loadAndMerge` closure calls `Load(path)`.
Change to:

``` go
    loadAndMerge := func(path string) error {
        var sf Sweatfile
        if err := sf.Load(path); err != nil {
            return err
        }
        _, found := fileExists(path)
        sources = append(sources, LoadSource{Path: path, Found: found, File: sf})
        if found {
            merged = Merge(merged, sf)
        }
        return nil
    }
```

In `LoadWorktreeHierarchy` (line 101), change:

``` go
    sf, err := Load(worktreePath)
    if err != nil {
```

To:

``` go
    var sf Sweatfile
    if err := sf.Load(worktreePath); err != nil {
```

**Step 4: Update all test callers**

In `sweatfile_test.go`, every call to `Parse([]byte(...))` becomes:

``` go
var sf Sweatfile
if err := sf.Parse([]byte(input)); err != nil {
```

And every call to `Load(path)` becomes:

``` go
var sf Sweatfile
if err := sf.Load(path); err != nil {
```

There are \~20 Parse calls and \~3 Load calls in tests. Update each one. The
test assertions stay the same --- they just reference `sf` instead of the
returned value.

**Step 5: Run tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: All
tests pass.

**Step 6: Commit**

    refactor(spinclass): convert Parse/Load to pointer-receiver methods on Sweatfile

--------------------------------------------------------------------------------

### Task 3: Convert Save to a value-receiver method

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/sweatfile/coding.go:34-44` -
Modify: `packages/spinclass/internal/sweatfile/sweatfile_test.go`
(TestSaveRoundTrip)

**Step 1: Convert Save**

In `coding.go`, change:

``` go
func Save(path string, sf Sweatfile) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()
    return toml.NewEncoder(f).Encode(sf)
}
```

To:

``` go
func (sf Sweatfile) Save(path string) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()
    return toml.NewEncoder(f).Encode(sf)
}
```

**Step 2: Update TestSaveRoundTrip**

Change:

``` go
    err := Save(path, sf)
```

To:

``` go
    err := sf.Save(path)
```

And update the Load call to use the method form (from Task 2):

``` go
    var loaded Sweatfile
    if err := loaded.Load(path); err != nil {
```

**Step 3: Run tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: All
tests pass.

**Step 4: Commit**

    refactor(spinclass): convert Save to value-receiver method on Sweatfile

--------------------------------------------------------------------------------

### Task 4: Convert Merge to a value-receiver method (MergeWith)

**Promotion criteria:** N/A

**Files:** - Modify:
`packages/spinclass/internal/sweatfile/hierarchy.go:117-194` - Modify:
`packages/spinclass/internal/sweatfile/apply.go:17` - Modify:
`packages/spinclass/internal/sweatfile/sweatfile_test.go` (all `Merge(...)`
calls)

**Step 1: Rename Merge to MergeWith as a value-receiver method**

In `hierarchy.go`, change the function signature from:

``` go
func Merge(base, repo Sweatfile) Sweatfile {
    merged := base
```

To:

``` go
func (sf Sweatfile) MergeWith(other Sweatfile) Sweatfile {
    merged := sf
```

Then replace all references to `repo` with `other` in the method body. The
variable `base` was aliased to `merged` at the top so references to `base`
inside the function body should become `sf` (but since `merged := sf` copies it,
the existing references to `base.X` inside the body should become `sf.X`).
Actually, looking at the code more carefully:

- `merged := base` → `merged := sf`
- All `repo.X` → `other.X`
- All `base.X` → `sf.X`

**Step 2: Update callers in hierarchy.go**

In `LoadHierarchy`, line 51:

``` go
            merged = Merge(merged, sf)
```

Becomes:

``` go
            merged = merged.MergeWith(sf)
```

In `LoadWorktreeHierarchy`, line 111:

``` go
        hierarchy.Merged = Merge(hierarchy.Merged, sf)
```

Becomes:

``` go
        hierarchy.Merged = hierarchy.Merged.MergeWith(sf)
```

**Step 3: Update caller in apply.go**

In `Apply()`, line 17:

``` go
    merged := Merge(sweatfile, defaults)
```

Becomes:

``` go
    merged := sweatfile.MergeWith(defaults)
```

**Step 4: Update all test callers**

In `sweatfile_test.go`, every `Merge(base, repo)` becomes
`base.MergeWith(repo)`. There are \~20 calls. Examples:

- `merged := Merge(base, repo)` → `merged := base.MergeWith(repo)`
- `merged := Merge(base, Sweatfile{})` → `merged := base.MergeWith(Sweatfile{})`

**Step 5: Run tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: All
tests pass.

**Step 6: Commit**

    refactor(spinclass): convert Merge to MergeWith value-receiver method on Sweatfile

--------------------------------------------------------------------------------

### Task 5: Remove TODO comments from coding.go

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/sweatfile/coding.go` - Modify:
`packages/spinclass/internal/sweatfile/hierarchy.go` - Modify:
`packages/spinclass/internal/sweatfile/sweatfile.go`

**Step 1: Remove stale TODO comments**

Remove these lines: - `coding.go`: The `// TODO rewrite as object-oriented`
comments above `Parse`, `Load`, `Save` (all now methods --- TODOs are done) -
`hierarchy.go`: The `// TODO rewrite as object-oriented` comment above
`MergeWith` (now a method --- TODO is done) - `sweatfile.go`: The
`// TODO replace with PathOrString struct` comments on `SystemPrompt` and
`SystemPromptAppend` (will be addressed in Task 6)

Only remove the coding.go and hierarchy.go TODOs in this task. The sweatfile.go
TODOs get removed in Task 6.

**Step 2: Commit**

    chore(spinclass): remove completed TODO comments from sweatfile package

--------------------------------------------------------------------------------

### Task 6: Add PathOrString file resolution for SystemPrompt fields

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/sweatfile/coding.go` - Modify:
`packages/spinclass/internal/sweatfile/sweatfile.go:23-24` (remove TODO
comments) - Modify: `packages/spinclass/internal/sweatfile/sweatfile_test.go`
(add new tests)

**Step 1: Write failing tests for file resolution**

Add these tests to `sweatfile_test.go`:

``` go
func TestParseSystemPromptFromFile(t *testing.T) {
    dir := t.TempDir()
    promptFile := filepath.Join(dir, "prompt.txt")
    os.WriteFile(promptFile, []byte("prompt from file\n"), 0o644)

    input := fmt.Sprintf(`system-prompt = %q`, promptFile)
    var sf Sweatfile
    if err := sf.Parse([]byte(input)); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if sf.SystemPrompt == nil || *sf.SystemPrompt != "prompt from file" {
        t.Errorf("system-prompt: got %v, want %q", sf.SystemPrompt, "prompt from file")
    }
}

func TestParseSystemPromptAppendFromFile(t *testing.T) {
    dir := t.TempDir()
    promptFile := filepath.Join(dir, "append.txt")
    os.WriteFile(promptFile, []byte("appended from file\n"), 0o644)

    input := fmt.Sprintf(`system-prompt-append = %q`, promptFile)
    var sf Sweatfile
    if err := sf.Parse([]byte(input)); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if sf.SystemPromptAppend == nil || *sf.SystemPromptAppend != "appended from file" {
        t.Errorf("system-prompt-append: got %v, want %q", sf.SystemPromptAppend, "appended from file")
    }
}

func TestParseSystemPromptLiteralWhenNotFile(t *testing.T) {
    input := `system-prompt = "this is not a file path"`
    var sf Sweatfile
    if err := sf.Parse([]byte(input)); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if sf.SystemPrompt == nil || *sf.SystemPrompt != "this is not a file path" {
        t.Errorf("system-prompt: got %v", sf.SystemPrompt)
    }
}

func TestParseSystemPromptTildeExpansion(t *testing.T) {
    home, err := os.UserHomeDir()
    if err != nil {
        t.Skip("no home directory")
    }

    dir := t.TempDir()
    promptFile := filepath.Join(dir, "tilde-prompt.txt")
    os.WriteFile(promptFile, []byte("tilde content\n"), 0o644)

    // Use a relative-to-home path by creating a symlink or using absolute
    // Since we can't guarantee ~ resolves to our temp dir, test with $HOME env var instead
    input := fmt.Sprintf(`system-prompt = "%s"`, promptFile)
    _ = home
    var sf Sweatfile
    if err := sf.Parse([]byte(input)); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if sf.SystemPrompt == nil || *sf.SystemPrompt != "tilde content" {
        t.Errorf("system-prompt: got %v, want %q", sf.SystemPrompt, "tilde content")
    }
}

func TestParseSystemPromptEnvExpansion(t *testing.T) {
    dir := t.TempDir()
    promptFile := filepath.Join(dir, "env-prompt.txt")
    os.WriteFile(promptFile, []byte("env content\n"), 0o644)

    t.Setenv("TEST_PROMPT_DIR", dir)
    input := `system-prompt = "$TEST_PROMPT_DIR/env-prompt.txt"`
    var sf Sweatfile
    if err := sf.Parse([]byte(input)); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if sf.SystemPrompt == nil || *sf.SystemPrompt != "env content" {
        t.Errorf("system-prompt: got %v, want %q", sf.SystemPrompt, "env content")
    }
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestParseSystemPrompt(FromFile|AppendFromFile|EnvExpansion)' ./packages/spinclass/internal/sweatfile/`
Expected: FAIL --- file content is not resolved yet.

**Step 3: Add resolvePathOrString helper and call from Parse**

In `coding.go`, add a helper function and call it from `Parse`:

``` go
func resolvePathOrString(value *string) {
    if value == nil || *value == "" {
        return
    }

    expanded := os.Expand(*value, os.Getenv)
    if strings.HasPrefix(expanded, "~/") {
        if home, err := os.UserHomeDir(); err == nil {
            expanded = filepath.Join(home, expanded[2:])
        }
    }

    data, err := os.ReadFile(expanded)
    if err != nil {
        return
    }

    content := strings.TrimSpace(string(data))
    *value = content
}
```

Add `strings` to the imports in `coding.go`.

Then in `Parse`, after the TOML unmarshal, call it:

``` go
func (sf *Sweatfile) Parse(data []byte) error {
    if err := toml.Unmarshal(data, sf); err != nil {
        return err
    }
    resolvePathOrString(sf.SystemPrompt)
    resolvePathOrString(sf.SystemPromptAppend)
    return nil
}
```

**Step 4: Remove TODO comments from sweatfile.go**

Remove the `// TODO replace with PathOrString struct` comments from lines 23 and
24 of `sweatfile.go`. The fields become:

``` go
    SystemPrompt       *string           `toml:"system-prompt"`
    SystemPromptAppend *string           `toml:"system-prompt-append"`
```

**Step 5: Run all tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: All
tests pass, including the new file resolution tests and all existing
system-prompt tests (literal strings still work).

**Step 6: Commit**

    feat(spinclass): resolve system-prompt values as file paths when possible

    If the value of system-prompt or system-prompt-append is a readable file
    path (with env var and tilde expansion), its contents replace the raw
    string. Otherwise the value is kept as a literal.

--------------------------------------------------------------------------------

### Task 7: Update spinclass TODO.md

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/TODO.md`

**Step 1: No items were completed from TODO.md --- no changes needed**

The TODO.md items (mock Executor tests, sc merge rebase issues) are unrelated to
this refactor. No update needed.

**Step 2: Verify the stale TODO removals from apply.go and util.go are still in
the working tree**

These were removed at the start of the session (before this plan). Verify
they're still gone.

Run:
`grep -n 'TODO' packages/spinclass/internal/sweatfile/apply.go packages/spinclass/internal/sweatfile/util.go`
Expected: No output.

**Step 3: Commit (if any changes)**

N/A unless something was missed.
