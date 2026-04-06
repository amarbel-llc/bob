# `lux fmt-all` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Implement `lux fmt-all` subcommand and switch hook generation from per-edit PostToolUse to single Stop hook, per the approved design doc at `packages/lux/docs/plans/2026-04-06-fmt-all-stop-hook-design.md`.

**Architecture:** New `fmt-all.toml` config loader in `internal/config/`, new `fmt-all` subcommand in `cmd/lux/app.go` that walks the project tree and formats every recognized file via the existing `formatter.Router`. Hook generation in `internal/hooks/generate.go` switches from PostToolUse Edit|Write to Stop, deleting the `format-file` bash script.

**Tech Stack:** Go, BurntSushi/toml, gobwas/glob (already in deps), BATS for integration tests.

**Rollback:** Revert commit, `just build && just install-bob`, restart Claude Code sessions.

---

### Task 1: `fmt-all.toml` config loading

**Files:**
- Create: `packages/lux/internal/config/fmtall.go`
- Create: `packages/lux/internal/config/fmtall_test.go`

**Step 1: Write the failing tests**

```go
// packages/lux/internal/config/fmtall_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFmtAll_DefaultsWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := LoadFmtAll()
	if err != nil {
		t.Fatalf("LoadFmtAll: %v", err)
	}
	if cfg.Walk != "git" {
		t.Errorf("Walk = %q, want %q", cfg.Walk, "git")
	}
	if len(cfg.ExcludeGlobs) != 0 {
		t.Errorf("ExcludeGlobs = %v, want empty", cfg.ExcludeGlobs)
	}
}

func TestLoadFmtAll_LoadsFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	luxDir := filepath.Join(dir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "all"
exclude_globs = ["**/flake.lock", "**/go.sum"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFmtAll()
	if err != nil {
		t.Fatalf("LoadFmtAll: %v", err)
	}
	if cfg.Walk != "all" {
		t.Errorf("Walk = %q, want %q", cfg.Walk, "all")
	}
	if len(cfg.ExcludeGlobs) != 2 {
		t.Errorf("ExcludeGlobs = %v, want 2 entries", cfg.ExcludeGlobs)
	}
}

func TestLoadFmtAll_InvalidWalk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	luxDir := filepath.Join(dir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "invalid"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFmtAll()
	if err == nil {
		t.Fatal("expected error for invalid walk value")
	}
}

func TestLoadMergedFmtAll_LocalOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalDir)
	luxDir := filepath.Join(globalDir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "git"
exclude_globs = ["**/flake.lock"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	localDir := filepath.Join(projectDir, ".lux")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "fmt-all.toml"), []byte(`
walk = "all"
exclude_globs = ["**/Cargo.lock"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMergedFmtAll(projectDir)
	if err != nil {
		t.Fatalf("LoadMergedFmtAll: %v", err)
	}
	if cfg.Walk != "all" {
		t.Errorf("Walk = %q, want %q (local override)", cfg.Walk, "all")
	}
	if len(cfg.ExcludeGlobs) != 1 || cfg.ExcludeGlobs[0] != "**/Cargo.lock" {
		t.Errorf("ExcludeGlobs = %v, want [**/Cargo.lock] (local override)", cfg.ExcludeGlobs)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `nix develop --command go test -run TestLoadFmtAll -v ./packages/lux/internal/config/`
Expected: compilation failure (types not defined)

**Step 3: Write minimal implementation**

```go
// packages/lux/internal/config/fmtall.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type FmtAllConfig struct {
	Walk         string   `toml:"walk"`
	ExcludeGlobs []string `toml:"exclude_globs"`
}

func fmtAllDefaults() *FmtAllConfig {
	return &FmtAllConfig{
		Walk:         "git",
		ExcludeGlobs: nil,
	}
}

func LoadFmtAll() (*FmtAllConfig, error) {
	return loadFmtAllFile(filepath.Join(configDir(), "fmt-all.toml"))
}

func LoadLocalFmtAll(projectRoot string) (*FmtAllConfig, error) {
	return loadFmtAllFile(filepath.Join(projectRoot, ".lux", "fmt-all.toml"))
}

func LoadMergedFmtAll(projectRoot string) (*FmtAllConfig, error) {
	global, err := LoadFmtAll()
	if err != nil {
		return nil, fmt.Errorf("loading global fmt-all config: %w", err)
	}

	if projectRoot == "" {
		return global, nil
	}

	local, err := LoadLocalFmtAll(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("loading local fmt-all config: %w", err)
	}

	return mergeFmtAll(global, local), nil
}

func loadFmtAllFile(path string) (*FmtAllConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmtAllDefaults(), nil
		}
		return nil, fmt.Errorf("reading fmt-all config %s: %w", path, err)
	}

	cfg := fmtAllDefaults()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing fmt-all config %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid fmt-all config %s: %w", path, err)
	}

	return cfg, nil
}

func (c *FmtAllConfig) validate() error {
	switch c.Walk {
	case "git", "all":
		return nil
	default:
		return fmt.Errorf("walk must be %q or %q, got %q", "git", "all", c.Walk)
	}
}

func mergeFmtAll(global, local *FmtAllConfig) *FmtAllConfig {
	merged := *global
	if local.Walk != fmtAllDefaults().Walk {
		merged.Walk = local.Walk
	}
	if local.ExcludeGlobs != nil {
		merged.ExcludeGlobs = local.ExcludeGlobs
	}
	return &merged
}
```

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test -run TestLoadFmtAll -v ./packages/lux/internal/config/`
Expected: all 4 tests PASS

**Step 5: Commit**

```
feat(lux): add fmt-all.toml config loading

Adds FmtAllConfig type with walk strategy and exclude_globs fields.
Supports global (~/.config/lux/fmt-all.toml) and per-project
(.lux/fmt-all.toml) configs with local-overrides-global merge.
```

---

### Task 2: `lux fmt-all` subcommand — file walker

**Files:**
- Create: `packages/lux/cmd/lux/fmtall.go`
- Create: `packages/lux/cmd/lux/fmtall_test.go`

**Step 1: Write the failing tests**

```go
// packages/lux/cmd/lux/fmtall_test.go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitWalk_ReturnsTrackedAndUntrackedFiles(t *testing.T) {
	dir := t.TempDir()

	// Init a git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create tracked file
	os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package main"), 0o644)
	run("add", "tracked.go")
	run("commit", "-m", "init")

	// Create untracked file (not ignored)
	os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package main"), 0o644)

	// Create ignored file
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.go\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.go"), []byte("package main"), 0o644)

	files, err := gitWalk(dir)
	if err != nil {
		t.Fatalf("gitWalk: %v", err)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[filepath.Base(f)] = true
	}

	if !fileSet["tracked.go"] {
		t.Error("missing tracked.go")
	}
	if !fileSet["untracked.go"] {
		t.Error("missing untracked.go")
	}
	if fileSet["ignored.go"] {
		t.Error("ignored.go should not be included")
	}
}

func TestAllWalk_SkipsGitDirAndSymlinks(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "objects", "foo"), []byte("x"), 0o644)
	os.Symlink(filepath.Join(dir, "file.go"), filepath.Join(dir, "link.go"))

	files, err := allWalk(dir)
	if err != nil {
		t.Fatalf("allWalk: %v", err)
	}

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if strings.HasPrefix(rel, ".git") {
			t.Errorf("should skip .git: %s", rel)
		}
		info, _ := os.Lstat(f)
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("should skip symlink: %s", rel)
		}
	}

	found := false
	for _, f := range files {
		if filepath.Base(f) == "file.go" {
			found = true
		}
	}
	if !found {
		t.Error("missing file.go")
	}
}

func TestExcludeGlobs_FiltersMatchingPaths(t *testing.T) {
	files := []string{
		"/repo/flake.lock",
		"/repo/sub/flake.lock",
		"/repo/main.go",
		"/repo/go.sum",
		"/repo/vendor/go.sum",
	}
	globs := []string{"**/flake.lock", "**/go.sum"}

	filtered := applyExcludeGlobs(files, globs, "/repo")
	if len(filtered) != 1 || filepath.Base(filtered[0]) != "main.go" {
		t.Errorf("filtered = %v, want only main.go", filtered)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `nix develop --command go test -run "TestGitWalk|TestAllWalk|TestExcludeGlobs" -v ./packages/lux/cmd/lux/`
Expected: compilation failure

**Step 3: Write the implementation**

```go
// packages/lux/cmd/lux/fmtall.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
	"github.com/amarbel-llc/lux/internal/formatter"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/lux/internal/subprocess"
)

func runFmtAll(ctx context.Context, paths []string) error {
	fmtAllCfg, err := loadFmtAllConfig()
	if err != nil {
		return err
	}

	filetypes, err := filetype.LoadMerged()
	if err != nil {
		return fmt.Errorf("loading filetype configs: %w", err)
	}

	fmtCfg, err := config.LoadMergedFormatters()
	if err != nil {
		return fmt.Errorf("loading formatter config: %w", err)
	}

	if err := fmtCfg.Validate(); err != nil {
		return fmt.Errorf("invalid formatter config: %w", err)
	}

	fmtMap := make(map[string]*config.Formatter)
	for i := range fmtCfg.Formatters {
		f := &fmtCfg.Formatters[i]
		if !f.Disabled {
			fmtMap[f.Name] = f
		}
	}

	router, err := formatter.NewRouter(filetypes, fmtMap)
	if err != nil {
		return fmt.Errorf("creating formatter router: %w", err)
	}

	executor := subprocess.NewNixExecutor()

	var files []string
	if len(paths) == 0 {
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		files, err = walkFiles(root, fmtAllCfg.Walk)
		if err != nil {
			return err
		}
		files = applyExcludeGlobs(files, fmtAllCfg.ExcludeGlobs, root)
	} else {
		for _, p := range paths {
			abs, err := filepath.Abs(p)
			if err != nil {
				fmt.Fprintf(logfile.Writer(), "resolving path %s: %v\n", p, err)
				continue
			}
			info, err := os.Stat(abs)
			if err != nil {
				fmt.Fprintf(logfile.Writer(), "stat %s: %v\n", abs, err)
				continue
			}
			if info.IsDir() {
				dirFiles, err := walkFiles(abs, fmtAllCfg.Walk)
				if err != nil {
					fmt.Fprintf(logfile.Writer(), "walking %s: %v\n", abs, err)
					continue
				}
				dirFiles = applyExcludeGlobs(dirFiles, fmtAllCfg.ExcludeGlobs, abs)
				files = append(files, dirFiles...)
			} else {
				files = append(files, abs)
			}
		}
	}

	for _, f := range files {
		if err := formatFile(ctx, f, router, executor); err != nil {
			fmt.Fprintf(logfile.Writer(), "fmt %s: %v\n", f, err)
		}
	}

	return nil
}

func loadFmtAllConfig() (*config.FmtAllConfig, error) {
	root, err := os.Getwd()
	if err != nil {
		return config.LoadFmtAll()
	}

	projectRoot, err := config.FindProjectRoot(root)
	if err != nil {
		return config.LoadFmtAll()
	}

	return config.LoadMergedFmtAll(projectRoot)
}

func walkFiles(root, strategy string) ([]string, error) {
	switch strategy {
	case "git":
		files, err := gitWalk(root)
		if err != nil {
			// Fall back to all-walk outside git repos
			return allWalk(root)
		}
		return files, nil
	case "all":
		return allWalk(root)
	default:
		return nil, fmt.Errorf("unknown walk strategy: %s", strategy)
	}
}

func gitWalk(root string) ([]string, error) {
	// git ls-files: tracked files
	// git ls-files --others --exclude-standard: untracked, not-ignored files
	tracked, err := gitLsFiles(root, nil)
	if err != nil {
		return nil, err
	}

	untracked, err := gitLsFiles(root, []string{"--others", "--exclude-standard"})
	if err != nil {
		return nil, err
	}

	var files []string
	for _, f := range append(tracked, untracked...) {
		abs := filepath.Join(root, f)
		files = append(files, abs)
	}
	return files, nil
}

func gitLsFiles(dir string, extraArgs []string) ([]string, error) {
	args := []string{"ls-files"}
	args = append(args, extraArgs...)

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", dir, err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func allWalk(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func applyExcludeGlobs(files []string, patterns []string, root string) []string {
	if len(patterns) == 0 {
		return files
	}

	var compiled []glob.Glob
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			fmt.Fprintf(logfile.Writer(), "bad exclude glob %q: %v\n", p, err)
			continue
		}
		compiled = append(compiled, g)
	}

	var result []string
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			result = append(result, f)
			continue
		}
		excluded := false
		for _, g := range compiled {
			if g.Match(rel) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, f)
		}
	}
	return result
}

func formatFile(ctx context.Context, filePath string, router *formatter.Router, executor subprocess.Executor) error {
	match := router.Match(filePath)
	if match == nil {
		return nil // silently skip unrecognized files
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}

	var result *formatter.Result
	switch match.Mode {
	case "chain":
		result, err = formatter.FormatChain(ctx, match.Formatters, filePath, content, executor)
	case "fallback":
		result, err = formatter.FormatFallback(ctx, match.Formatters, filePath, content, executor)
	default:
		return fmt.Errorf("unknown formatter mode: %s", match.Mode)
	}
	if err != nil {
		return err
	}

	if !result.Changed {
		return nil
	}

	return os.WriteFile(filePath, []byte(result.Formatted), 0o644)
}
```

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test -run "TestGitWalk|TestAllWalk|TestExcludeGlobs" -v ./packages/lux/cmd/lux/`
Expected: all 3 tests PASS

**Step 5: Commit**

```
feat(lux): add fmt-all subcommand file walker

Implements gitWalk (git ls-files for tracked + untracked-not-ignored),
allWalk (recursive walk skipping .git/ and symlinks), and
applyExcludeGlobs (gobwas/glob matching against relative paths).
```

---

### Task 3: Wire `fmt-all` command into CLI

**Files:**
- Modify: `packages/lux/cmd/lux/app.go` (add command registration after `fmt`)

**Step 1: Write a test that verifies the command exists**

Add to `packages/lux/cmd/lux/fmtall_test.go`:

```go
func TestFmtAll_FormatsRecognizedFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Create minimal formatter + filetype configs
	luxDir := filepath.Join(dir, "lux")
	os.MkdirAll(filepath.Join(luxDir, "filetype"), 0o755)

	// A trivial "formatter" that uppercases input
	fmtScript := filepath.Join(dir, "upper")
	os.WriteFile(fmtScript, []byte("#!/bin/sh\ntr a-z A-Z"), 0o755)

	os.WriteFile(filepath.Join(luxDir, "formatters.toml"), []byte(fmt.Sprintf(`
[[formatter]]
name = "upper"
path = "%s"
mode = "stdin"
`, fmtScript)), 0o644)

	os.WriteFile(filepath.Join(luxDir, "filetype", "test.toml"), []byte(`
extensions = [".txt"]
formatters = ["upper"]
`), 0o644)

	// Create test files
	workDir := t.TempDir()
	os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(workDir, "b.go"), []byte("world"), 0o644)

	// Run fmt-all
	origDir, _ := os.Getwd()
	os.Chdir(workDir)
	defer os.Chdir(origDir)

	ctx := context.Background()
	err := runFmtAll(ctx, nil)
	if err != nil {
		t.Fatalf("runFmtAll: %v", err)
	}

	// a.txt should be uppercased
	got, _ := os.ReadFile(filepath.Join(workDir, "a.txt"))
	if string(got) != "HELLO" {
		t.Errorf("a.txt = %q, want %q", got, "HELLO")
	}

	// b.go should be untouched (no .go filetype configured)
	got, _ = os.ReadFile(filepath.Join(workDir, "b.go"))
	if string(got) != "world" {
		t.Errorf("b.go = %q, want %q (should be untouched)", got, "world")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestFmtAll_FormatsRecognizedFiles -v ./packages/lux/cmd/lux/`
Expected: FAIL (command not wired up — or pass if `runFmtAll` is already callable directly; adjust test if needed)

**Step 3: Add command registration to app.go**

Add after the `fmt` command block (after line 333 in `cmd/lux/app.go`):

```go
	app.AddCommand(&command.Command{
		Name: "fmt-all",
		Description: command.Description{
			Short: "Format all files in the project",
			Long:  "Walk the project tree and format every recognized file using configured formatters.",
		},
		RunCLI: func(ctx context.Context, args json.RawMessage) error {
			return runFmtAll(ctx, os.Args[2:])
		},
	})
```

**Note:** `os.Args[2:]` captures paths after `lux fmt-all`. The `command.App` framework passes remaining CLI args via `os.Args` since `fmt-all` has no declared `Params` — explicit paths are positional. Verify this matches the framework's behavior in `app.RunCLI`.

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test -run TestFmtAll -v ./packages/lux/cmd/lux/`
Expected: PASS

**Step 5: Commit**

```
feat(lux): wire fmt-all subcommand into CLI

Registers `lux fmt-all [path...]` command that walks the project
tree and formats every recognized file via formatter.Router.
```

---

### Task 4: Switch hook generation from PostToolUse to Stop

**Files:**
- Modify: `packages/lux/internal/hooks/generate.go`
- Modify: `packages/lux/internal/hooks/generate_test.go`
- Modify: `packages/lux/cmd/lux/main.go` (rename function call)

**Step 1: Write the new tests first**

Replace the contents of `packages/lux/internal/hooks/generate_test.go`:

```go
package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateStopHook_CreatesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pluginDir, "hooks", "hooks.json"))
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parsing hooks.json: %v", err)
	}

	hooks, ok := manifest["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks.json missing 'hooks' key")
	}

	stop, ok := hooks["Stop"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'Stop' key")
	}

	if len(stop) != 1 {
		t.Fatalf("expected 1 Stop entry, got %d", len(stop))
	}

	entry := stop[0].(map[string]any)
	innerHooks := entry["hooks"].([]any)
	hook := innerHooks[0].(map[string]any)

	if hook["command"] != "lux fmt-all" {
		t.Errorf("command = %q, want %q", hook["command"], "lux fmt-all")
	}

	timeout, ok := hook["timeout"].(float64)
	if !ok || timeout != 60 {
		t.Errorf("timeout = %v, want 60", hook["timeout"])
	}
}

func TestGenerateStopHook_MergesWithExistingPreToolUse(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existing := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash|Read",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "${CLAUDE_PLUGIN_ROOT}/hooks/pre-tool-use",
							"timeout": 5,
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(result, &manifest); err != nil {
		t.Fatal(err)
	}

	hooks := manifest["hooks"].(map[string]any)

	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse was lost during merge")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop was not added")
	}
	if _, ok := hooks["PostToolUse"]; ok {
		t.Error("PostToolUse should not be present")
	}
}

func TestGenerateStopHook_NoFormatFileScript(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	scriptPath := filepath.Join(pluginDir, "hooks", "format-file")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("format-file script should not exist")
	}
}

func TestGenerateStopHook_RemovesExistingPostToolUse(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Simulate old hooks.json with PostToolUse
	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"matcher": "Edit|Write",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "${CLAUDE_PLUGIN_ROOT}/hooks/format-file",
							"timeout": 30,
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644)

	// Also write old format-file script
	os.WriteFile(filepath.Join(hooksDir, "format-file"), []byte("#!/bin/bash\n"), 0o755)

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	var manifest map[string]any
	json.Unmarshal(result, &manifest)
	hooks := manifest["hooks"].(map[string]any)

	if _, ok := hooks["PostToolUse"]; ok {
		t.Error("PostToolUse should have been removed")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop should have been added")
	}

	if _, err := os.Stat(filepath.Join(hooksDir, "format-file")); !os.IsNotExist(err) {
		t.Error("format-file script should have been deleted")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `nix develop --command go test -run "TestGenerateStop" -v ./packages/lux/internal/hooks/`
Expected: compilation failure (`GenerateStopHook` not defined)

**Step 3: Rewrite generate.go**

```go
// packages/lux/internal/hooks/generate.go
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateStopHook adds a Stop hook entry to the hooks directory under
// pluginDir. If hooks/hooks.json already exists (e.g. from go-mcp's
// PreToolUse generation), the Stop entry is merged in and any
// existing PostToolUse entry is removed. The old format-file script
// is deleted if present.
func GenerateStopHook(pluginDir string) error {
	hooksDir := filepath.Join(pluginDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	hooksJSONPath := filepath.Join(hooksDir, "hooks.json")

	manifest := make(map[string]any)

	data, err := os.ReadFile(hooksJSONPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading existing hooks.json: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("parsing existing hooks.json: %w", err)
		}
	}

	hooks, ok := manifest["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		manifest["hooks"] = hooks
	}

	// Remove old PostToolUse formatter hook
	delete(hooks, "PostToolUse")

	// Add Stop hook
	hooks["Stop"] = []any{
		map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "lux fmt-all",
					"timeout": float64(60),
				},
			},
		},
	}

	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks.json: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(hooksJSONPath, data, 0o644); err != nil {
		return fmt.Errorf("writing hooks.json: %w", err)
	}

	// Clean up old format-file script
	scriptPath := filepath.Join(hooksDir, "format-file")
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing old format-file script: %w", err)
	}

	return nil
}
```

**Step 4: Update `cmd/lux/main.go` to call the new function**

Change line 39 from:
```go
if err := hooks.GeneratePostToolUseHooks(pluginDir); err != nil {
```
to:
```go
if err := hooks.GenerateStopHook(pluginDir); err != nil {
```

And update the error message on line 40 from `"Error generating PostToolUse hooks: %v\n"` to `"Error generating Stop hook: %v\n"`.

**Step 5: Run tests to verify they pass**

Run: `nix develop --command go test -run "TestGenerate" -v ./packages/lux/internal/hooks/`
Expected: all 4 tests PASS

Then verify the full build:
Run: `nix develop --command go build ./packages/lux/cmd/lux/`
Expected: success

**Step 6: Commit**

```
feat(lux): switch hook from PostToolUse Edit|Write to Stop fmt-all

Replace per-edit formatter hook with a single Stop hook that runs
`lux fmt-all`. Eliminates agent edit-loop pathology caused by
mid-turn formatting invalidating old_string values.

Removes the format-file bash script and its jq-based extraction.
Cleans up any existing PostToolUse entries from hooks.json during
generate-plugin.

Relates-to: #88, #87
```

---

### Task 5: BATS integration test for `lux fmt-all`

**Files:**
- Create: `zz-tests_bats/lux_fmt_all.bats`

**Step 1: Write the integration test**

```bash
#!/usr/bin/env bats

# Integration tests for lux fmt-all subcommand.
# Requires: nix build .#lux

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux="$(result_dir)/bin/lux"
  [[ -x "$lux" ]] || skip "lux binary not found at $lux — run: nix build .#lux"

  "$lux" init --default --force

  # Create a test project with git
  export PROJECT_DIR="$BATS_TEST_TMPDIR/project"
  mkdir -p "$PROJECT_DIR"
  cd "$PROJECT_DIR"
  git init --quiet
  git config user.email "test@test.com"
  git config user.name "Test"
}

# @test "fmt-all formats recognized go file" {
  cat > "$PROJECT_DIR/main.go" <<'GO'
package main

func main() {

	println("hi")
}
GO
  git add main.go
  git commit -m "init" --quiet

  run "$lux" fmt-all
  assert_success

  # gofumpt removes the empty line after func main() {
  run cat "$PROJECT_DIR/main.go"
  refute_output --partial $'\nfunc main() {\n\n'
}

# @test "fmt-all skips unrecognized files" {
  echo "some random content" > "$PROJECT_DIR/readme.txt"
  git add readme.txt
  git commit -m "add txt" --quiet

  run "$lux" fmt-all
  assert_success

  # File should be untouched
  run cat "$PROJECT_DIR/readme.txt"
  assert_output "some random content"
}

# @test "fmt-all respects exclude_globs" {
  echo '{"a":1}' > "$PROJECT_DIR/flake.lock"
  git add flake.lock
  git commit -m "add lock" --quiet

  mkdir -p "$XDG_CONFIG_HOME/lux"
  cat > "$XDG_CONFIG_HOME/lux/fmt-all.toml" <<'TOML'
exclude_globs = ["flake.lock", "**/flake.lock"]
TOML

  run "$lux" fmt-all
  assert_success

  # flake.lock should NOT have been reformatted by jq
  run cat "$PROJECT_DIR/flake.lock"
  assert_output '{"a":1}'
}

# @test "fmt-all with explicit path formats only that path" {
  cat > "$PROJECT_DIR/a.go" <<'GO'
package main

func a() {

	println("a")
}
GO
  cat > "$PROJECT_DIR/b.go" <<'GO'
package main

func b() {

	println("b")
}
GO
  git add a.go b.go
  git commit -m "add files" --quiet

  run "$lux" fmt-all a.go
  assert_success

  # a.go should be formatted
  run cat "$PROJECT_DIR/a.go"
  refute_output --partial $'\nfunc a() {\n\n'

  # b.go should still have the empty line (not touched)
  run cat "$PROJECT_DIR/b.go"
  assert_output --partial $'\nfunc b() {\n\n'
}

# @test "fmt-all exits 0 even when one file fails" {
  echo "not valid go" > "$PROJECT_DIR/bad.go"
  echo '{"ok":true}' > "$PROJECT_DIR/good.json"
  git add bad.go good.json
  git commit -m "mixed" --quiet

  run "$lux" fmt-all
  assert_success
}
```

**Step 2: Run the BATS test (requires nix build first)**

Run: `nix build .#lux && nix develop --command bats --tap zz-tests_bats/lux_fmt_all.bats`
Expected: all tests PASS

**Step 3: Commit**

```
test(lux): add BATS integration tests for fmt-all

Covers recognized file formatting, unrecognized file skipping,
exclude_globs filtering, explicit path targeting, and graceful
handling of per-file failures.
```

---

### Task 6: Verify hook generation in generate-plugin output

**Step 1: Build and inspect the hook output**

Run: `nix build .#lux && jq . result/share/purse-first/lux/hooks/hooks.json`

Expected output should contain `"Stop"` with `"lux fmt-all"` and NO `"PostToolUse"` key. Should NOT contain a `format-file` script:

```
ls result/share/purse-first/lux/hooks/format-file
```
Expected: `No such file or directory`

**Step 2: Build the full marketplace**

Run: `nix build`
Expected: success

**Step 3: Commit (if any fixups needed)**

Only commit if changes were required. Otherwise, this task is a verification-only step.

---

### Task 7: Manual verification on the bob worktree

**Step 1: Run `lux fmt-all` in the worktree**

Run from worktree root:
```
lux fmt-all
```

Verify:
- `.go` files are formatted
- `.nix` files are formatted (if nixfmt-rfc-style is in user's formatters.toml)
- `.md` files are skipped (no formatter configured / pandoc commented out)
- `flake.lock` is untouched (if exclude_globs is set in user's fmt-all.toml)
- No errors on stderr

**Step 2: Check for spurious changes**

Run: `git diff --stat`

Only formatting changes should appear. No lockfiles, no markdown churn.

Plan complete and saved to `docs/plans/2026-04-06-fmt-all-implementation.md`. Ready to execute?
