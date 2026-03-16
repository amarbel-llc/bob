# get-hubbed clone TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add a `get-hubbed clone [target-dir]` CLI subcommand that lists the authenticated user's GitHub repos, presents a multi-select for uncloned ones, and clones selected repos in parallel with TAP-14 status output.

**Architecture:** New `internal/clone` package with a single `Run(ctx, targetDir)` function. Main dispatches `clone` subcommand before the MCP server block. Uses `internal/gh` for GitHub API calls, `huh` for the multi-select prompt, and tap-dancer's `ConvertExecParallelWithStatus` for parallel cloning.

**Tech Stack:** Go, charmbracelet/huh, tap-dancer (Go library), gh CLI, git CLI

**Rollback:** N/A — purely additive, no existing behavior modified.

---

### Task 1: Add dependencies to go.mod

**Files:**
- Modify: `packages/get-hubbed/go.mod`

**Step 1: Add huh and tap-dancer dependencies**

Run from repo root:

```sh
cd packages/get-hubbed && go get github.com/charmbracelet/huh@v0.8.0 && go get github.com/amarbel-llc/bob/packages/tap-dancer/go@latest && cd ../..
```

**Step 2: Add replace directive for tap-dancer**

Add to `packages/get-hubbed/go.mod` (matching spinclass's pattern):

```
replace github.com/amarbel-llc/bob/packages/tap-dancer/go => ../tap-dancer/go
```

**Step 3: Sync workspace**

Run: `just go-mod-sync`
Expected: Workspace syncs, vendors, and Nix build succeeds.

**Step 4: Commit**

```
chore(get-hubbed): add huh and tap-dancer dependencies
```

---

### Task 2: Create clone package — repo fetching

**Files:**
- Create: `packages/get-hubbed/internal/clone/clone.go`

**Step 1: Write clone.go with repo fetching and clone detection**

```go
package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	tap "github.com/amarbel-llc/bob/packages/tap-dancer/go"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

type repo struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
}

func fetchUserRepos(ctx context.Context) ([]repo, error) {
	out, err := gh.Run(ctx,
		"api", "user/repos",
		"--paginate",
		"--jq", ".[].full_name",
	)
	if err != nil {
		return nil, fmt.Errorf("fetching repos: %w", err)
	}

	var repos []repo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			continue
		}
		repos = append(repos, repo{FullName: line, Name: parts[1]})
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].FullName < repos[j].FullName
	})

	return repos, nil
}

func isCloned(ctx context.Context, targetDir, repoName string) bool {
	dir := filepath.Join(targetDir, repoName)
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func Run(ctx context.Context, targetDir string) error {
	info, err := os.Stat(targetDir)
	if err != nil {
		return fmt.Errorf("target directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target path is not a directory: %s", targetDir)
	}

	repos, err := fetchUserRepos(ctx)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Println("No repos found for authenticated user.")
		return nil
	}

	var uncloned []repo
	for _, r := range repos {
		if !isCloned(ctx, targetDir, r.Name) {
			uncloned = append(uncloned, r)
		}
	}

	if len(uncloned) == 0 {
		fmt.Println("All repos already cloned.")
		return nil
	}

	options := make([]huh.Option[string], len(uncloned))
	for i, r := range uncloned {
		options[i] = huh.NewOption(r.FullName, r.FullName)
	}

	var selected []string
	err = huh.NewMultiSelect[string]().
		Title("Select repos to clone").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		return nil
	}

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolving target directory: %w", err)
	}

	// Build fullName→name lookup for selected repos
	nameByFullName := make(map[string]string, len(uncloned))
	for _, r := range uncloned {
		nameByFullName[r.FullName] = r.Name
	}

	args := make([]string, len(selected))
	for i, fullName := range selected {
		name := nameByFullName[fullName]
		args[i] = fullName + " " + filepath.Join(absTarget, name)
	}

	executor := &tap.GoroutineExecutor{MaxJobs: 4}
	exitCode := tap.ConvertExecParallelWithStatus(
		ctx, executor,
		"gh repo clone {}",
		args,
		os.Stdout,
		false,
		true,
	)

	if exitCode != 0 {
		return fmt.Errorf("some repos failed to clone")
	}

	return nil
}
```

Note: The `json` import is included in case we later switch to JSON parsing but isn't used in the initial implementation. Remove it if the linter flags it.

**Step 2: Verify it compiles**

Run: `nix develop --command go build ./packages/get-hubbed/...`
Expected: Builds without errors.

**Step 3: Commit**

```
feat(get-hubbed): add internal/clone package with repo fetch and clone
```

---

### Task 3: Wire up clone subcommand in main.go

**Files:**
- Modify: `packages/get-hubbed/cmd/get-hubbed/main.go:15-41`

**Step 1: Add clone dispatch**

Add this block after the `hook` handler and before the help flag loop in `main.go`:

```go
	if len(os.Args) >= 2 && os.Args[1] == "clone" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		targetDir := "."
		if len(os.Args) >= 3 {
			targetDir = os.Args[2]
		}

		if err := clone.Run(ctx, targetDir); err != nil {
			log.Fatalf("clone: %v", err)
		}
		return
	}
```

Add the import:

```go
	"github.com/friedenberg/get-hubbed/internal/clone"
```

**Step 2: Update help text**

Update the help output to include the clone subcommand:

```go
		if arg == "-h" || arg == "--help" {
			fmt.Println("get-hubbed - a GitHub MCP server wrapping the gh CLI")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  get-hubbed              Start MCP server (stdio)")
			fmt.Println("  get-hubbed clone [dir]   Clone uncloned repos for authenticated user")
			fmt.Println()
			os.Exit(0)
		}
```

**Step 3: Verify it compiles**

Run: `nix develop --command go build ./packages/get-hubbed/...`
Expected: Builds without errors.

**Step 4: Commit**

```
feat(get-hubbed): wire up clone subcommand in main.go
```

---

### Task 4: Manual integration test

**Step 1: Build get-hubbed**

Run: `nix develop --command go build -o build/get-hubbed ./packages/get-hubbed/cmd/get-hubbed`

**Step 2: Test help**

Run: `./build/get-hubbed --help`
Expected: Shows usage including `clone` subcommand.

**Step 3: Test clone in a temp directory**

```sh
mkdir -p /tmp/test-clone
./build/get-hubbed clone /tmp/test-clone
```

Expected:
1. Fetches repos for authenticated user
2. Shows multi-select with uncloned repos
3. After selection, clones in parallel with TAP-14 status output
4. Re-running shows fewer (or no) uncloned repos

**Step 4: Test already-all-cloned case**

Run: `./build/get-hubbed clone /tmp/test-clone` (again, after cloning everything)
Expected: "All repos already cloned."

**Step 5: Test bad directory**

Run: `./build/get-hubbed clone /nonexistent`
Expected: Error about target directory.

**Step 6: Nix build**

Run: `just go-mod-sync` (if deps changed), then `nix build .#get-hubbed`
Expected: Nix build succeeds.

**Step 7: Clean up**

```sh
rm -rf /tmp/test-clone
```
