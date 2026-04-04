# Session Context Prompt Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add layered system prompt append to spinclass sessions --- base
context always, optional issue/PR context via flags, user sweatfile stacking on
top.

**Architecture:** New `internal/prompt` package with embedded Go templates.
`shop.Create()` renders base + optional context into
`.spinclass/system_prompt_append.d/`. `sweatfile.ExecClaude()` globs that
directory at exec time, writes user sweatfile content, concatenates, and passes
as `--append-system-prompt`.

**Tech Stack:** Go `text/template`, `//go:embed`, `os.Glob`, `gh` CLI for
issue/PR/repo metadata.

**Rollback:** N/A --- purely additive. `.spinclass/` is already gitignored.
ExecClaude gate on `.spinclass/` directory is the only behavioral change to
existing code.

--------------------------------------------------------------------------------

### Task 1: Create `internal/prompt` package with embedded templates

**Files:** - Create:
`packages/spinclass/internal/prompt/system_prompt_append.d/0-base.md.tmpl` -
Create:
`packages/spinclass/internal/prompt/system_prompt_append.d/1-issue.md.tmpl` -
Create:
`packages/spinclass/internal/prompt/system_prompt_append.d/1-pr.md.tmpl` -
Create: `packages/spinclass/internal/prompt/prompt.go` - Test:
`packages/spinclass/internal/prompt/prompt_test.go`

**Step 1: Write the failing test**

``` go
// prompt_test.go
package prompt

import (
    "testing"
)

func TestRenderBase(t *testing.T) {
    data := BaseData{
        RepoName:  "bob",
        RemoteURL: "git@github.com:amarbel-llc/bob.git",
        Branch:    "feature-x",
        SessionID: "bob/feature-x",
    }

    got, err := RenderBase(data)
    if err != nil {
        t.Fatalf("RenderBase() error: %v", err)
    }

    if got == "" {
        t.Fatal("RenderBase() returned empty string")
    }

    for _, want := range []string{
        "bob",
        "git@github.com:amarbel-llc/bob.git",
        "feature-x",
        "bob/feature-x",
        "spinclass worktree session",
    } {
        if !contains(got, want) {
            t.Errorf("RenderBase() missing %q in:\n%s", want, got)
        }
    }
}

func TestRenderBaseWithForkInfo(t *testing.T) {
    data := BaseData{
        RepoName:   "bob",
        RemoteURL:  "git@github.com:someone/bob.git",
        Branch:     "fix-bug",
        SessionID:  "bob/fix-bug",
        IsFork:     true,
        OwnerType:  "User",
        OwnerLogin: "someone",
    }

    got, err := RenderBase(data)
    if err != nil {
        t.Fatalf("RenderBase() error: %v", err)
    }

    for _, want := range []string{"Fork:", "User", "someone"} {
        if !contains(got, want) {
            t.Errorf("RenderBase() missing %q in:\n%s", want, got)
        }
    }
}

func TestRenderBaseOmitsForkWhenFalse(t *testing.T) {
    data := BaseData{
        RepoName:  "bob",
        RemoteURL: "git@github.com:amarbel-llc/bob.git",
        Branch:    "feature-x",
        SessionID: "bob/feature-x",
    }

    got, err := RenderBase(data)
    if err != nil {
        t.Fatalf("RenderBase() error: %v", err)
    }

    if contains(got, "Fork:") {
        t.Errorf("RenderBase() should omit Fork when IsFork=false, got:\n%s", got)
    }
}

func TestRenderIssue(t *testing.T) {
    data := IssueData{
        Number: 42,
        Title:  "Fix login bug",
        State:  "OPEN",
        Labels: "bug, auth",
        URL:    "https://github.com/amarbel-llc/bob/issues/42",
        Body:   "Login fails when password contains special chars.",
    }

    got, err := RenderIssue(data)
    if err != nil {
        t.Fatalf("RenderIssue() error: %v", err)
    }

    for _, want := range []string{
        "Issue #42",
        "Fix login bug",
        "OPEN",
        "bug, auth",
        "Login fails",
    } {
        if !contains(got, want) {
            t.Errorf("RenderIssue() missing %q in:\n%s", want, got)
        }
    }
}

func TestRenderIssueOmitsLabelsWhenEmpty(t *testing.T) {
    data := IssueData{
        Number: 10,
        Title:  "No labels",
        State:  "OPEN",
        URL:    "https://github.com/amarbel-llc/bob/issues/10",
        Body:   "Body text.",
    }

    got, err := RenderIssue(data)
    if err != nil {
        t.Fatalf("RenderIssue() error: %v", err)
    }

    if contains(got, "Labels:") {
        t.Errorf("RenderIssue() should omit Labels when empty, got:\n%s", got)
    }
}

func TestRenderPR(t *testing.T) {
    data := PRData{
        Number:  100,
        Title:   "Add feature X",
        State:   "OPEN",
        BaseRef: "master",
        HeadRef: "feature-x",
        Labels:  "enhancement",
        URL:     "https://github.com/amarbel-llc/bob/pull/100",
        Body:    "This PR adds feature X.",
    }

    got, err := RenderPR(data)
    if err != nil {
        t.Fatalf("RenderPR() error: %v", err)
    }

    for _, want := range []string{
        "PR #100",
        "Add feature X",
        "master",
        "feature-x",
        "This PR adds feature X.",
    } {
        if !contains(got, want) {
            t.Errorf("RenderPR() missing %q in:\n%s", want, got)
        }
    }
}

func contains(s, substr string) bool {
    return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
    for i := 0; i <= len(s)-len(sub); i++ {
        if s[i:i+len(sub)] == sub {
            return true
        }
    }
    return false
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestRender ./packages/spinclass/internal/prompt/...`
Expected: FAIL --- package does not exist

**Step 3: Write the templates and render functions**

Create
`packages/spinclass/internal/prompt/system_prompt_append.d/0-base.md.tmpl`:

    # Session Context

    You are working inside a spinclass worktree session.

    ## Repository
    - **Name:** {{.RepoName}}
    - **Remote:** {{.RemoteURL}}
    {{- if .IsFork}}
    - **Fork:** yes
    {{- end}}
    {{- if .OwnerType}}
    - **Owner:** {{.OwnerType}} ({{.OwnerLogin}})
    {{- end}}

    ## Worktree Restrictions

    This session runs in an isolated git worktree. You MUST NOT:
    - Interact with the main worktree or default branch directly
    - Run git commands targeting the parent repository directory
    - Attempt to check out or modify the main/master branch
    - Prefix tool commands with cd into the worktree — you are already there

    Tool uses targeting the main repository will be blocked.

    ## Session
    - **Branch:** {{.Branch}}
    - **Session ID:** {{.SessionID}}

Create
`packages/spinclass/internal/prompt/system_prompt_append.d/1-issue.md.tmpl`:

    # GitHub Issue Context

    This session is working on the following GitHub issue.

    ## Issue #{{.Number}}: {{.Title}}
    - **State:** {{.State}}
    {{- if .Labels}}
    - **Labels:** {{.Labels}}
    {{- end}}
    - **URL:** {{.URL}}

    ## Description

    {{.Body}}

Create `packages/spinclass/internal/prompt/system_prompt_append.d/1-pr.md.tmpl`:

    # Pull Request Context

    This session is working on the following pull request.

    ## PR #{{.Number}}: {{.Title}}
    - **State:** {{.State}}
    - **Base:** {{.BaseRef}} ← **Head:** {{.HeadRef}}
    {{- if .Labels}}
    - **Labels:** {{.Labels}}
    {{- end}}
    - **URL:** {{.URL}}

    ## Description

    {{.Body}}

Create `packages/spinclass/internal/prompt/prompt.go`:

``` go
package prompt

import (
    "bytes"
    "embed"
    "text/template"
)

//go:embed system_prompt_append.d/*.tmpl
var templates embed.FS

type BaseData struct {
    RepoName   string
    RemoteURL  string
    Branch     string
    SessionID  string
    IsFork     bool
    OwnerType  string
    OwnerLogin string
}

type IssueData struct {
    Number int
    Title  string
    State  string
    Labels string
    URL    string
    Body   string
}

type PRData struct {
    Number  int
    Title   string
    State   string
    BaseRef string
    HeadRef string
    Labels  string
    URL     string
    Body    string
}

func render(name string, data any) (string, error) {
    tmpl, err := template.ParseFS(templates, "system_prompt_append.d/"+name)
    if err != nil {
        return "", err
    }

    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", err
    }

    return buf.String(), nil
}

func RenderBase(data BaseData) (string, error) {
    return render("0-base.md.tmpl", data)
}

func RenderIssue(data IssueData) (string, error) {
    return render("1-issue.md.tmpl", data)
}

func RenderPR(data PRData) (string, error) {
    return render("1-pr.md.tmpl", data)
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestRender ./packages/spinclass/internal/prompt/...`
Expected: PASS

**Step 5: Commit**

    git add packages/spinclass/internal/prompt/
    git commit -m "feat(spinclass): add prompt package with embedded templates for session context"

--------------------------------------------------------------------------------

### Task 2: Add `--issue` flag to `sc start`

**Files:** - Modify: `packages/spinclass/cmd/spinclass/main.go:36-44` (add
`startIssue` var) - Modify: `packages/spinclass/cmd/spinclass/main.go:53-141`
(startCmd RunE) - Modify: `packages/spinclass/cmd/spinclass/main.go:554-658`
(init, flag registration)

**Step 1: Write the failing test**

No unit test for CLI flag wiring --- this will be verified by the integration in
Task 4. The mutual exclusivity check is straightforward cobra validation.

**Step 2: Add the flag variable and registration**

In `main.go`, add to the var block (around line 41):

``` go
startIssue string
```

In `init()`, after the `--pr` flag registration (around line 585):

``` go
startCmd.Flags().StringVar(
    &startIssue,
    "issue",
    "",
    "start session with GitHub issue context (number or URL)",
)
startCmd.MarkFlagsMutuallyExclusive("issue", "pr")
```

**Step 3: Commit**

    git add packages/spinclass/cmd/spinclass/main.go
    git commit -m "feat(spinclass): add --issue flag to start (mutually exclusive with --pr)"

--------------------------------------------------------------------------------

### Task 3: Add GitHub metadata fetching for issue and repo info

**Files:** - Create: `packages/spinclass/internal/prompt/github.go` - Test:
`packages/spinclass/internal/prompt/github_test.go`

**Step 1: Write the failing test**

``` go
// github_test.go
package prompt

import (
    "testing"
)

func TestParseRepoInfo(t *testing.T) {
    tests := []struct {
        name      string
        remoteURL string
        wantName  string
        wantURL   string
    }{
        {
            name:      "ssh url",
            remoteURL: "git@github.com:amarbel-llc/bob.git",
            wantName:  "bob",
            wantURL:   "git@github.com:amarbel-llc/bob.git",
        },
        {
            name:      "https url",
            remoteURL: "https://github.com/amarbel-llc/bob.git",
            wantName:  "bob",
            wantURL:   "https://github.com/amarbel-llc/bob.git",
        },
        {
            name:      "no .git suffix",
            remoteURL: "git@github.com:amarbel-llc/bob",
            wantName:  "bob",
            wantURL:   "git@github.com:amarbel-llc/bob",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            info := ParseRepoInfo(tt.remoteURL, "/path/to/bob")
            if info.RemoteURL != tt.wantURL {
                t.Errorf("RemoteURL = %q, want %q", info.RemoteURL, tt.wantURL)
            }
            if info.RepoName != tt.wantName {
                t.Errorf("RepoName = %q, want %q", info.RepoName, tt.wantName)
            }
        })
    }
}

func TestParseRepoInfoFallsBackToDirname(t *testing.T) {
    info := ParseRepoInfo("", "/home/user/repos/my-project")
    if info.RepoName != "my-project" {
        t.Errorf("RepoName = %q, want %q", info.RepoName, "my-project")
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestParseRepo ./packages/spinclass/internal/prompt/...`
Expected: FAIL --- function does not exist

**Step 3: Write implementation**

``` go
// github.go
package prompt

import (
    "encoding/json"
    "fmt"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
)

type RepoInfo struct {
    RepoName   string
    RemoteURL  string
    IsFork     bool
    OwnerType  string
    OwnerLogin string
}

func ParseRepoInfo(remoteURL, repoPath string) RepoInfo {
    info := RepoInfo{
        RemoteURL: remoteURL,
        RepoName:  filepath.Base(repoPath),
    }

    if remoteURL != "" {
        // Extract repo name from URL (strip .git suffix, take last path component)
        name := remoteURL
        name = strings.TrimSuffix(name, ".git")
        if idx := strings.LastIndex(name, "/"); idx >= 0 {
            name = name[idx+1:]
        } else if idx := strings.LastIndex(name, ":"); idx >= 0 {
            name = name[idx+1:]
            if slash := strings.LastIndex(name, "/"); slash >= 0 {
                name = name[slash+1:]
            }
        }
        if name != "" {
            info.RepoName = name
        }
    }

    return info
}

type ghRepoView struct {
    IsFork bool `json:"isFork"`
    Owner  struct {
        Type  string `json:"type"`
        Login string `json:"login"`
    } `json:"owner"`
}

func FetchRepoMetadata(repoPath string) (isFork bool, ownerType, ownerLogin string) {
    slug := repoSlug(repoPath)
    if slug == "" {
        return false, "", ""
    }

    out, err := exec.Command(
        "gh", "repo", "view", slug,
        "--json", "isFork,owner",
    ).Output()
    if err != nil {
        return false, "", ""
    }

    var view ghRepoView
    if json.Unmarshal(out, &view) != nil {
        return false, "", ""
    }

    return view.IsFork, view.Owner.Type, view.Owner.Login
}

func repoSlug(repoPath string) string {
    out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
    if err != nil {
        return ""
    }
    remote := strings.TrimSpace(string(out))

    // SSH: git@github.com:owner/repo.git
    if strings.HasPrefix(remote, "git@") {
        parts := strings.SplitN(remote, ":", 2)
        if len(parts) == 2 {
            return strings.TrimSuffix(parts[1], ".git")
        }
    }
    // HTTPS
    remote = strings.TrimSuffix(remote, ".git")
    if idx := strings.Index(remote, "github.com/"); idx >= 0 {
        return remote[idx+len("github.com/"):]
    }
    return ""
}

type ghIssueView struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    State  string `json:"state"`
    URL    string `json:"url"`
    Body   string `json:"body"`
    Labels []struct {
        Name string `json:"name"`
    } `json:"labels"`
}

func FetchIssue(identifier string, repoPath string) (IssueData, error) {
    slug := repoSlug(repoPath)
    args := []string{
        "issue", "view", identifier,
        "--json", "number,title,state,url,body,labels",
    }
    if slug != "" {
        args = append(args, "--repo", slug)
    }

    out, err := exec.Command("gh", args...).Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return IssueData{}, fmt.Errorf("gh issue view failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
        }
        return IssueData{}, fmt.Errorf("gh issue view: %w", err)
    }

    var view ghIssueView
    if err := json.Unmarshal(out, &view); err != nil {
        return IssueData{}, fmt.Errorf("parsing gh output: %w", err)
    }

    var labels []string
    for _, l := range view.Labels {
        labels = append(labels, l.Name)
    }

    return IssueData{
        Number: view.Number,
        Title:  view.Title,
        State:  view.State,
        URL:    view.URL,
        Body:   view.Body,
        Labels: strings.Join(labels, ", "),
    }, nil
}

type ghPRView struct {
    Number      int    `json:"number"`
    Title       string `json:"title"`
    State       string `json:"state"`
    URL         string `json:"url"`
    Body        string `json:"body"`
    HeadRefName string `json:"headRefName"`
    BaseRefName string `json:"baseRefName"`
    Labels      []struct {
        Name string `json:"name"`
    } `json:"labels"`
}

func FetchPR(identifier string, repoPath string) (PRData, error) {
    slug := repoSlug(repoPath)
    args := []string{
        "pr", "view", identifier,
        "--json", "number,title,state,url,body,headRefName,baseRefName,labels",
    }
    if slug != "" {
        args = append(args, "--repo", slug)
    }

    out, err := exec.Command("gh", args...).Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return PRData{}, fmt.Errorf("gh pr view failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
        }
        return PRData{}, fmt.Errorf("gh pr view: %w", err)
    }

    var view ghPRView
    if err := json.Unmarshal(out, &view); err != nil {
        return PRData{}, fmt.Errorf("parsing gh output: %w", err)
    }

    var labels []string
    for _, l := range view.Labels {
        labels = append(labels, l.Name)
    }

    return PRData{
        Number:  view.Number,
        Title:   view.Title,
        State:   view.State,
        BaseRef: view.BaseRefName,
        HeadRef: view.HeadRefName,
        URL:     view.URL,
        Body:    view.Body,
        Labels:  strings.Join(labels, ", "),
    }, nil
}

func FetchIssueNumber(identifier string) (int, error) {
    n, err := strconv.Atoi(identifier)
    if err != nil {
        return 0, fmt.Errorf("invalid issue identifier %q: must be a number", identifier)
    }
    return n, nil
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestParseRepo ./packages/spinclass/internal/prompt/...`
Expected: PASS

**Step 5: Commit**

    git add packages/spinclass/internal/prompt/github.go packages/spinclass/internal/prompt/github_test.go
    git commit -m "feat(spinclass): add GitHub metadata fetching for session context"

--------------------------------------------------------------------------------

### Task 4: Add `WriteSessionContext` to render and write `.spinclass/system_prompt_append.d/` files

**Files:** - Create: `packages/spinclass/internal/prompt/write.go` - Test:
`packages/spinclass/internal/prompt/write_test.go`

**Step 1: Write the failing test**

``` go
// write_test.go
package prompt

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestWriteBaseContext(t *testing.T) {
    dir := t.TempDir()
    scDir := filepath.Join(dir, ".spinclass")

    opts := WriteOptions{
        WorktreePath: dir,
        RepoPath:     "/home/user/repos/bob",
        RemoteURL:    "git@github.com:amarbel-llc/bob.git",
        Branch:       "feature-x",
        SessionID:    "bob/feature-x",
    }

    if err := WriteSessionContext(opts); err != nil {
        t.Fatalf("WriteSessionContext() error: %v", err)
    }

    basePath := filepath.Join(scDir, "system_prompt_append.d", "0-base.md")
    data, err := os.ReadFile(basePath)
    if err != nil {
        t.Fatalf("reading base file: %v", err)
    }

    content := string(data)
    for _, want := range []string{"bob", "feature-x", "spinclass worktree session"} {
        if !strings.Contains(content, want) {
            t.Errorf("base file missing %q", want)
        }
    }
}

func TestWriteIssueContext(t *testing.T) {
    dir := t.TempDir()
    scDir := filepath.Join(dir, ".spinclass")

    opts := WriteOptions{
        WorktreePath: dir,
        RepoPath:     "/home/user/repos/bob",
        RemoteURL:    "git@github.com:amarbel-llc/bob.git",
        Branch:       "feature-x",
        SessionID:    "bob/feature-x",
        Issue: &IssueData{
            Number: 42,
            Title:  "Fix bug",
            State:  "OPEN",
            URL:    "https://github.com/amarbel-llc/bob/issues/42",
            Body:   "Bug description.",
        },
    }

    if err := WriteSessionContext(opts); err != nil {
        t.Fatalf("WriteSessionContext() error: %v", err)
    }

    issuePath := filepath.Join(scDir, "system_prompt_append.d", "1-issue-42.md")
    data, err := os.ReadFile(issuePath)
    if err != nil {
        t.Fatalf("reading issue file: %v", err)
    }

    if !strings.Contains(string(data), "Issue #42") {
        t.Errorf("issue file missing issue number")
    }
}

func TestWritePRContext(t *testing.T) {
    dir := t.TempDir()
    scDir := filepath.Join(dir, ".spinclass")

    opts := WriteOptions{
        WorktreePath: dir,
        RepoPath:     "/home/user/repos/bob",
        RemoteURL:    "git@github.com:amarbel-llc/bob.git",
        Branch:       "feature-x",
        SessionID:    "bob/feature-x",
        PR: &PRData{
            Number:  100,
            Title:   "Add feature",
            State:   "OPEN",
            BaseRef: "master",
            HeadRef: "feature-x",
            URL:     "https://github.com/amarbel-llc/bob/pull/100",
            Body:    "PR body.",
        },
    }

    if err := WriteSessionContext(opts); err != nil {
        t.Fatalf("WriteSessionContext() error: %v", err)
    }

    prPath := filepath.Join(scDir, "system_prompt_append.d", "1-pr-100.md")
    data, err := os.ReadFile(prPath)
    if err != nil {
        t.Fatalf("reading PR file: %v", err)
    }

    if !strings.Contains(string(data), "PR #100") {
        t.Errorf("PR file missing PR number")
    }

    // Should not have issue file
    issueGlob, _ := filepath.Glob(filepath.Join(scDir, "system_prompt_append.d", "1-issue-*.md"))
    if len(issueGlob) > 0 {
        t.Error("should not have issue file when PR is set")
    }
}

func TestWriteNoOptionalContext(t *testing.T) {
    dir := t.TempDir()
    scDir := filepath.Join(dir, ".spinclass")

    opts := WriteOptions{
        WorktreePath: dir,
        RepoPath:     "/home/user/repos/bob",
        RemoteURL:    "git@github.com:amarbel-llc/bob.git",
        Branch:       "feature-x",
        SessionID:    "bob/feature-x",
    }

    if err := WriteSessionContext(opts); err != nil {
        t.Fatalf("WriteSessionContext() error: %v", err)
    }

    // Only base file should exist
    matches, _ := filepath.Glob(filepath.Join(scDir, "system_prompt_append.d", "1-*.md"))
    if len(matches) > 0 {
        t.Errorf("expected no optional context files, got: %v", matches)
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestWrite ./packages/spinclass/internal/prompt/...`
Expected: FAIL --- `WriteSessionContext` and `WriteOptions` not defined

**Step 3: Write implementation**

``` go
// write.go
package prompt

import (
    "fmt"
    "os"
    "path/filepath"
)

type WriteOptions struct {
    WorktreePath string
    RepoPath     string
    RemoteURL    string
    Branch       string
    SessionID    string
    IsFork       bool
    OwnerType    string
    OwnerLogin   string
    Issue        *IssueData
    PR           *PRData
}

func WriteSessionContext(opts WriteOptions) error {
    dir := filepath.Join(opts.WorktreePath, ".spinclass", "system_prompt_append.d")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return fmt.Errorf("creating system_prompt_append.d: %w", err)
    }

    baseData := BaseData{
        RepoName:   ParseRepoInfo(opts.RemoteURL, opts.RepoPath).RepoName,
        RemoteURL:  opts.RemoteURL,
        Branch:     opts.Branch,
        SessionID:  opts.SessionID,
        IsFork:     opts.IsFork,
        OwnerType:  opts.OwnerType,
        OwnerLogin: opts.OwnerLogin,
    }

    baseContent, err := RenderBase(baseData)
    if err != nil {
        return fmt.Errorf("rendering base template: %w", err)
    }

    if err := os.WriteFile(filepath.Join(dir, "0-base.md"), []byte(baseContent), 0o644); err != nil {
        return fmt.Errorf("writing 0-base.md: %w", err)
    }

    if opts.Issue != nil {
        content, err := RenderIssue(*opts.Issue)
        if err != nil {
            return fmt.Errorf("rendering issue template: %w", err)
        }
        filename := fmt.Sprintf("1-issue-%d.md", opts.Issue.Number)
        if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
            return fmt.Errorf("writing %s: %w", filename, err)
        }
    }

    if opts.PR != nil {
        content, err := RenderPR(*opts.PR)
        if err != nil {
            return fmt.Errorf("rendering PR template: %w", err)
        }
        filename := fmt.Sprintf("1-pr-%d.md", opts.PR.Number)
        if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
            return fmt.Errorf("writing %s: %w", filename, err)
        }
    }

    return nil
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestWrite ./packages/spinclass/internal/prompt/...`
Expected: PASS

**Step 5: Commit**

    git add packages/spinclass/internal/prompt/write.go packages/spinclass/internal/prompt/write_test.go
    git commit -m "feat(spinclass): add WriteSessionContext to render templates into .spinclass/"

--------------------------------------------------------------------------------

### Task 5: Wire `WriteSessionContext` into `worktree.Create()` / `applyWorktreeConfig()`

**Files:** - Modify: `packages/spinclass/internal/worktree/worktree.go:17-24`
(ResolvedPath --- add Issue/PR fields) - Modify:
`packages/spinclass/internal/worktree/worktree.go:122-160` (Create --- pass
through to applyWorktreeConfig) - Modify:
`packages/spinclass/internal/worktree/worktree.go:191-231` (applyWorktreeConfig
--- call WriteSessionContext)

**Step 1: Add fields to ResolvedPath**

``` go
type ResolvedPath struct {
    AbsPath        string
    RepoPath       string
    SessionKey     string
    Branch         string
    Description    string
    ExistingBranch string
    Issue          *prompt.IssueData // optional issue context
    PR             *prompt.PRData    // optional PR context
}
```

**Step 2: Call WriteSessionContext in applyWorktreeConfig**

After the `.tmp` directory creation and before `sweetfile.Merged.Apply()`, add:

``` go
remoteURL := ""
if out, err := git.Run(repoPath, "remote", "get-url", "origin"); err == nil {
    remoteURL = strings.TrimSpace(out)
}

isFork, ownerType, ownerLogin := prompt.FetchRepoMetadata(repoPath)

branch := filepath.Base(worktreePath)
repoDirname := filepath.Base(repoPath)

writeOpts := prompt.WriteOptions{
    WorktreePath: worktreePath,
    RepoPath:     repoPath,
    RemoteURL:    remoteURL,
    Branch:       branch,
    SessionID:    repoDirname + "/" + branch,
    IsFork:       isFork,
    OwnerType:    ownerType,
    OwnerLogin:   ownerLogin,
}

if issue != nil {
    writeOpts.Issue = issue
}
if pr != nil {
    writeOpts.PR = pr
}

if err := prompt.WriteSessionContext(writeOpts); err != nil {
    return fmt.Errorf("writing session context: %w", err)
}
```

Note: `applyWorktreeConfig` signature needs to accept optional issue/PR data.
Thread it through from `Create()` which gets it from `ResolvedPath`.

**Step 3: Update Create and CreateFrom signatures**

`Create()` reads `issue`/`pr` from the caller. `applyWorktreeConfig()` gains two
optional parameters. Both `Create()` and `CreateFrom()` pass them through.

**Step 4: Run existing tests to verify nothing breaks**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: PASS ---
existing tests don't set Issue/PR fields, so WriteSessionContext renders only
the base file.

**Step 5: Commit**

    git add packages/spinclass/internal/worktree/worktree.go
    git commit -m "feat(spinclass): wire WriteSessionContext into worktree creation"

--------------------------------------------------------------------------------

### Task 6: Wire `--issue` flag through `startCmd` to `shop.Attach()`

**Files:** - Modify: `packages/spinclass/cmd/spinclass/main.go:53-141` (startCmd
RunE --- add issue fetching + pass to ResolvedPath)

**Step 1: Add issue fetching logic to startCmd**

In `startCmd.RunE`, after resolving the path but before calling `shop.Attach`,
add:

``` go
if startIssue != "" {
    issueData, err := prompt.FetchIssue(startIssue, repoPath)
    if err != nil {
        return fmt.Errorf("fetching issue: %w", err)
    }
    resolvedPath.Issue = &issueData
}
```

The existing `--pr` path already resolves PR metadata via `pr.Resolve()`. Add PR
context fetching there too:

``` go
if startPR != "" {
    // ... existing PR resolution code ...

    // Fetch PR context for system prompt
    prData, err := prompt.FetchPR(startPR, repoPath)
    if err != nil {
        // Non-fatal: PR context is nice-to-have when using --pr
        // (the branch checkout is the primary purpose)
        log.Warn("could not fetch PR context for system prompt", "err", err)
    } else {
        resolvedPath.PR = &prData
    }
}
```

**Step 2: Run full test suite**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: PASS

**Step 3: Commit**

    git add packages/spinclass/cmd/spinclass/main.go
    git commit -m "feat(spinclass): wire --issue and --pr context through to session creation"

--------------------------------------------------------------------------------

### Task 7: Update `ExecClaude()` to glob `.spinclass/system_prompt_append.d/`

**Files:** - Modify:
`packages/spinclass/internal/sweatfile/sweatfile.go:115-163` (ExecClaude) -
Test: `packages/spinclass/internal/sweatfile/sweatfile_test.go` (add new test)

**Step 1: Write the failing test**

Add to existing test file:

``` go
func TestCollectSystemPromptAppend(t *testing.T) {
    dir := t.TempDir()
    appendDir := filepath.Join(dir, ".spinclass", "system_prompt_append.d")
    if err := os.MkdirAll(appendDir, 0o755); err != nil {
        t.Fatal(err)
    }

    if err := os.WriteFile(
        filepath.Join(appendDir, "0-base.md"),
        []byte("base context"),
        0o644,
    ); err != nil {
        t.Fatal(err)
    }

    if err := os.WriteFile(
        filepath.Join(appendDir, "1-issue-42.md"),
        []byte("issue context"),
        0o644,
    ); err != nil {
        t.Fatal(err)
    }

    got, err := collectSystemPromptAppend(dir)
    if err != nil {
        t.Fatalf("collectSystemPromptAppend() error: %v", err)
    }

    if !strings.Contains(got, "base context") {
        t.Error("missing base context")
    }
    if !strings.Contains(got, "issue context") {
        t.Error("missing issue context")
    }
    // Base should come before issue
    baseIdx := strings.Index(got, "base context")
    issueIdx := strings.Index(got, "issue context")
    if baseIdx > issueIdx {
        t.Error("base context should come before issue context")
    }
}

func TestCollectSystemPromptAppendWithUserContent(t *testing.T) {
    dir := t.TempDir()
    appendDir := filepath.Join(dir, ".spinclass", "system_prompt_append.d")
    if err := os.MkdirAll(appendDir, 0o755); err != nil {
        t.Fatal(err)
    }

    if err := os.WriteFile(
        filepath.Join(appendDir, "0-base.md"),
        []byte("base"),
        0o644,
    ); err != nil {
        t.Fatal(err)
    }

    // Simulate user sweatfile content being written
    if err := os.WriteFile(
        filepath.Join(appendDir, "2-user.md"),
        []byte("user prompt"),
        0o644,
    ); err != nil {
        t.Fatal(err)
    }

    got, err := collectSystemPromptAppend(dir)
    if err != nil {
        t.Fatalf("collectSystemPromptAppend() error: %v", err)
    }

    if !strings.Contains(got, "user prompt") {
        t.Error("missing user prompt")
    }

    baseIdx := strings.Index(got, "base")
    userIdx := strings.Index(got, "user prompt")
    if baseIdx > userIdx {
        t.Error("base should come before user prompt")
    }
}

func TestExecClaudeFailsWithoutSpinclassDir(t *testing.T) {
    dir := t.TempDir()

    // Change to a directory without .spinclass
    oldWd, _ := os.Getwd()
    os.Chdir(dir)
    defer os.Chdir(oldWd)

    sf := Sweatfile{}
    err := sf.ExecClaude()
    if err == nil {
        t.Error("expected error when .spinclass/ not found")
    }
    if !strings.Contains(err.Error(), ".spinclass") {
        t.Errorf("error should mention .spinclass, got: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestCollect ./packages/spinclass/internal/sweatfile/...`
Expected: FAIL --- `collectSystemPromptAppend` not defined

**Step 3: Write implementation**

Add to `sweatfile.go`:

``` go
func collectSystemPromptAppend(cwd string) (string, error) {
    pattern := filepath.Join(cwd, ".spinclass", "system_prompt_append.d", "*.md")
    matches, err := filepath.Glob(pattern)
    if err != nil {
        return "", fmt.Errorf("globbing system_prompt_append.d: %w", err)
    }

    sort.Strings(matches)

    var parts []string
    for _, path := range matches {
        data, err := os.ReadFile(path)
        if err != nil {
            return "", fmt.Errorf("reading %s: %w", filepath.Base(path), err)
        }
        if content := strings.TrimSpace(string(data)); content != "" {
            parts = append(parts, content)
        }
    }

    return strings.Join(parts, "\n\n"), nil
}
```

Update `ExecClaude()`:

``` go
func (sweatfile Sweatfile) ExecClaude(args ...string) error {
    cwd, err := os.Getwd()
    if err != nil {
        return err
    }

    scDir := filepath.Join(cwd, ".spinclass")
    if _, err := os.Stat(scDir); os.IsNotExist(err) {
        return fmt.Errorf(".spinclass directory not found in %s; exec-claude requires a spinclass session", cwd)
    }

    // Write user sweatfile system-prompt-append to the .d/ directory
    if sweatfile.Claude != nil && sweatfile.Claude.SystemPromptAppend != nil {
        userContent := resolvePathOrString(*sweatfile.Claude.SystemPromptAppend)
        userPath := filepath.Join(scDir, "system_prompt_append.d", "2-user.md")
        os.MkdirAll(filepath.Dir(userPath), 0o755)
        if err := os.WriteFile(userPath, []byte(userContent), 0o644); err != nil {
            return fmt.Errorf("writing user system prompt append: %w", err)
        }
    }

    // Collect all system prompt append fragments
    appendContent, err := collectSystemPromptAppend(cwd)
    if err != nil {
        return err
    }

    if appendContent != "" {
        args = append(
            []string{"--append-system-prompt", appendContent},
            args...,
        )
    }

    // system-prompt (non-append) still works as before
    if sweatfile.Claude != nil && sweatfile.Claude.SystemPrompt != nil {
        args = append(
            []string{
                "--system-prompt",
                resolvePathOrString(*sweatfile.Claude.SystemPrompt),
            },
            args...,
        )
    }

    pathGitDirCommon, err := getGitDirCommon()
    if err != nil {
        return err
    }

    pathSweatfileBin := filepath.Join(pathGitDirCommon, "spinclass/bin/")

    envVarPath := filepath.SplitList(os.Getenv("PATH"))
    envVarPath = slices.DeleteFunc(envVarPath, func(value string) bool {
        return filepath.Clean(value) == pathSweatfileBin
    })
    os.Setenv("PATH", strings.Join(envVarPath, string(filepath.ListSeparator)))

    cmdClaude := exec.Command("claude", args...)
    cmdClaude.Stdout = os.Stdout
    cmdClaude.Stderr = os.Stderr
    cmdClaude.Stdin = os.Stdin

    if err := cmdClaude.Run(); err != nil {
        return err
    }

    return nil
}
```

Add `"sort"` to the imports.

**Step 4: Run tests to verify**

Run:
`nix develop --command go test -run "TestCollect|TestExecClaudeFailsWithout" ./packages/spinclass/internal/sweatfile/...`
Expected: PASS

**Step 5: Commit**

    git add packages/spinclass/internal/sweatfile/sweatfile.go packages/spinclass/internal/sweatfile/sweatfile_test.go
    git commit -m "feat(spinclass): ExecClaude globs system_prompt_append.d/ and requires .spinclass/"

--------------------------------------------------------------------------------

### Task 8: Add `.spinclass/system_prompt_append.d/` to default git excludes

**Files:** - Modify:
`packages/spinclass/internal/sweatfile/sweatfile.go:102-113` (GetDefault)

**Step 1: Verify current default excludes**

Current: `[]string{".spinclass/", ".mcp.json"}` --- `.spinclass/` is already
excluded. The entire directory is gitignored, so `system_prompt_append.d/`
inside it is covered.

**Step 2: No change needed**

`.spinclass/` is already in the default git excludes. The
`system_prompt_append.d/` directory lives inside it.

**Step 3: Commit**

No commit needed --- already covered.

--------------------------------------------------------------------------------

### Task 9: Run full test suite and verify

**Step 1: Run all spinclass tests**

Run: `nix develop --command go test ./packages/spinclass/...` Expected: PASS

**Step 2: Run full project build**

Run: `nix build .#spinclass` Expected: SUCCESS

**Step 3: Manual smoke test**

1.  `sc start --issue 80 testing issue context` --- verify
    `.spinclass/system_prompt_append.d/0-base.md` and `1-issue-80.md` exist in
    the worktree
2.  `cat .spinclass/system_prompt_append.d/0-base.md` --- verify repo name,
    remote, branch, restrictions
3.  `cat .spinclass/system_prompt_append.d/1-issue-80.md` --- verify issue
    title, body, labels
4.  `sc exec-claude` --- verify Claude receives the appended context (check
    Claude's behavior references the issue)

**Step 4: Commit any fixes**

If smoke test reveals issues, fix and commit.

--------------------------------------------------------------------------------

### Task 10: Clean up explore recipe from justfile

**Files:** - Modify: `justfile` (remove `explore-issue-prompt` recipe)

**Step 1: Remove the ad-hoc explore recipe**

The `explore-issue-prompt` recipe was for experimentation. Now that the feature
is implemented natively, remove it.

**Step 2: Commit**

    git add justfile
    git commit -m "chore: remove explore-issue-prompt recipe (replaced by --issue flag)"
