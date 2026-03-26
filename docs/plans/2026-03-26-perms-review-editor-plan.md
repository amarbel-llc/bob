# Perms Review Editor Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Replace the one-at-a-time huh TUI in `spinclass perms review` with a
git-rebase-i style `$EDITOR` interface, fix the diff baseline to compare against
global Claude settings + tier files, and add `--dry-run` / `--worktree-dir`
flags.

**Architecture:** New `editor.go` handles format generation, parsing, and
friendly name derivation. The review command in `cmd.go` orchestrates: compute
diff → write temp file → open editor → parse → review loop (accept/edit/abort) →
route decisions. `RouteDecisions` in `review.go` drops snapshot logic.

**Tech Stack:** Go stdlib (`os`, `os/exec`, `strings`, `sort`, `path/filepath`,
`fmt`, `bufio`), `charmbracelet/huh` (for accept/edit/abort prompt only),
`cobra` (CLI flags).

**Rollback:** `git revert` the commit. No dual-architecture needed.

--------------------------------------------------------------------------------

### Task 1: Add Editor Format Generation and Parsing

**Promotion criteria:** N/A

**Files:** - Create: `packages/spinclass/internal/perms/editor.go` - Create:
`packages/spinclass/internal/perms/editor_test.go`

**Step 1: Write the failing tests for `FriendlyName`**

In `packages/spinclass/internal/perms/editor_test.go`:

``` go
package perms

import "testing"

func TestFriendlyName(t *testing.T) {
    tests := []struct {
        rule string
        want string
    }{
        {"mcp__plugin_grit_grit__add", "grit:add"},
        {"mcp__plugin_chix_chix__build", "chix:build"},
        {"mcp__plugin_get-hubbed_get-hubbed__issue-create", "get-hubbed:issue-create"},
        {"mcp__plugin_lux_lux__hover", "lux:hover"},
        {"Bash(go test:*)", ""},
        {"Read", ""},
        {"Glob", ""},
        {"mcp__glean_default__search", "glean:search"},
    }

    for _, tt := range tests {
        t.Run(tt.rule, func(t *testing.T) {
            got := FriendlyName(tt.rule)
            if got != tt.want {
                t.Errorf("FriendlyName(%q) = %q, want %q", tt.rule, got, tt.want)
            }
        })
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestFriendlyName ./packages/spinclass/internal/perms/`
Expected: FAIL --- `FriendlyName` not defined.

**Step 3: Implement `FriendlyName` in `editor.go`**

``` go
package perms

import (
    "strings"
)

// FriendlyName extracts a short "server:tool" name from an MCP permission
// string like "mcp__plugin_grit_grit__add" → "grit:add" or
// "mcp__glean_default__search" → "glean:search". Returns empty string for
// non-MCP rules.
func FriendlyName(rule string) string {
    // Strip trailing arguments like "mcp__foo__bar(args)"
    base := rule
    if idx := strings.Index(base, "("); idx >= 0 {
        base = base[:idx]
    }

    if !strings.HasPrefix(base, "mcp__") {
        return ""
    }

    // Format: mcp__<server>_<server>__<tool> or mcp__<server>__<tool>
    withoutPrefix := base[len("mcp__"):]
    dunderIdx := strings.LastIndex(withoutPrefix, "__")
    if dunderIdx < 0 {
        return ""
    }

    serverPart := withoutPrefix[:dunderIdx]
    tool := withoutPrefix[dunderIdx+2:]

    // serverPart may be "plugin_grit_grit" or "glean_default"
    // For "plugin_X_X", extract X. For others, take first segment.
    if strings.HasPrefix(serverPart, "plugin_") {
        serverPart = serverPart[len("plugin_"):]
        // serverPart is now "grit_grit" or "get-hubbed_get-hubbed"
        // Take everything before the last underscore
        lastUnderscore := strings.LastIndex(serverPart, "_")
        if lastUnderscore > 0 {
            serverPart = serverPart[:lastUnderscore]
        }
    } else {
        // "glean_default" → "glean"
        if idx := strings.Index(serverPart, "_"); idx > 0 {
            serverPart = serverPart[:idx]
        }
    }

    if serverPart == "" || tool == "" {
        return ""
    }

    return serverPart + ":" + tool
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestFriendlyName ./packages/spinclass/internal/perms/`
Expected: PASS

**Step 5: Write the failing tests for `FormatEditorContent` and
`ParseEditorContent`**

Append to `editor_test.go`:

``` go
func TestFormatEditorContent(t *testing.T) {
    rules := []string{
        "mcp__plugin_grit_grit__add",
        "Bash(go test:*)",
        "mcp__plugin_chix_chix__build",
    }
    got := FormatEditorContent(rules, "myrepo")

    // Should be sorted alphabetically
    if !strings.Contains(got, "discard Bash(go test:*)") {
        t.Error("expected Bash rule in output")
    }
    if !strings.Contains(got, "# chix:build") {
        t.Error("expected chix:build friendly name comment")
    }
    if !strings.Contains(got, "# grit:add") {
        t.Error("expected grit:add friendly name comment")
    }
    // Bash rule should NOT have a friendly name comment
    lines := strings.Split(got, "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "discard Bash(go test:*)") {
            if strings.Contains(line, "#") {
                t.Error("non-MCP rule should not have a friendly name comment")
            }
        }
    }
    // Header should contain repo name
    if !strings.Contains(got, "# Repo: myrepo") {
        t.Error("expected repo name in header")
    }
}

func TestParseEditorContent(t *testing.T) {
    input := `# comment line
# another comment

global mcp__plugin_grit_grit__add                    # grit:add
repo   Bash(go test:*)
keep   Bash(nix build:*)
discard mcp__plugin_chix_chix__build                 # chix:build
`
    decisions, err := ParseEditorContent(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(decisions) != 4 {
        t.Fatalf("expected 4 decisions, got %d", len(decisions))
    }

    expected := []ReviewDecision{
        {Rule: "mcp__plugin_grit_grit__add", Action: ReviewPromoteGlobal},
        {Rule: "Bash(go test:*)", Action: ReviewPromoteRepo},
        {Rule: "Bash(nix build:*)", Action: ReviewKeep},
        {Rule: "mcp__plugin_chix_chix__build", Action: ReviewDiscard},
    }

    for i, want := range expected {
        if decisions[i].Rule != want.Rule {
            t.Errorf("decision[%d].Rule = %q, want %q", i, decisions[i].Rule, want.Rule)
        }
        if decisions[i].Action != want.Action {
            t.Errorf("decision[%d].Action = %q, want %q", i, decisions[i].Action, want.Action)
        }
    }
}

func TestParseEditorContentPrefixes(t *testing.T) {
    input := `g Bash(git status)
r Bash(go test:*)
k Edit
d Bash(rm -rf:*)
`
    decisions, err := ParseEditorContent(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(decisions) != 4 {
        t.Fatalf("expected 4 decisions, got %d", len(decisions))
    }

    if decisions[0].Action != ReviewPromoteGlobal {
        t.Errorf("expected global, got %q", decisions[0].Action)
    }
    if decisions[1].Action != ReviewPromoteRepo {
        t.Errorf("expected repo, got %q", decisions[1].Action)
    }
    if decisions[2].Action != ReviewKeep {
        t.Errorf("expected keep, got %q", decisions[2].Action)
    }
    if decisions[3].Action != ReviewDiscard {
        t.Errorf("expected discard, got %q", decisions[3].Action)
    }
}

func TestParseEditorContentBadAction(t *testing.T) {
    input := `xyz Bash(git status)
`
    _, err := ParseEditorContent(input)
    if err == nil {
        t.Fatal("expected error for unknown action")
    }
}

func TestParseEditorContentEmptyLine(t *testing.T) {
    input := `

`
    decisions, err := ParseEditorContent(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(decisions) != 0 {
        t.Fatalf("expected 0 decisions, got %d", len(decisions))
    }
}
```

Add `"strings"` to the import block in the test file.

**Step 6: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestFormatEditorContent|TestParseEditorContent' ./packages/spinclass/internal/perms/`
Expected: FAIL --- functions not defined.

**Step 7: Implement `FormatEditorContent` and `ParseEditorContent`**

Add to `editor.go`:

``` go
import (
    "bufio"
    "fmt"
    "sort"
    "strings"
)

// FormatEditorContent generates the editor buffer content for review.
// Rules are sorted alphabetically and default to "discard". MCP rules
// get an inline "# server:tool" comment.
func FormatEditorContent(rules []string, repoName string) string {
    sorted := make([]string, len(rules))
    copy(sorted, rules)
    sort.Strings(sorted)

    var b strings.Builder

    b.WriteString("# spinclass perms review — change the action word for each permission\n")
    b.WriteString("# Actions: global | repo | keep | discard (unique prefixes OK: g/r/k/d)\n")
    b.WriteString("# Lines starting with # are ignored. Empty lines are ignored.\n")
    fmt.Fprintf(&b, "# Repo: %s\n", repoName)
    b.WriteString("\n")

    for _, rule := range sorted {
        friendly := FriendlyName(rule)
        if friendly != "" {
            fmt.Fprintf(&b, "discard %s  # %s\n", rule, friendly)
        } else {
            fmt.Fprintf(&b, "discard %s\n", rule)
        }
    }

    return b.String()
}

// ParseEditorContent parses the editor buffer back into ReviewDecisions.
// Ignores comment lines (starting with #) and blank lines. Supports
// unique action prefixes (g/r/k/d).
func ParseEditorContent(content string) ([]ReviewDecision, error) {
    var decisions []ReviewDecision

    scanner := bufio.NewScanner(strings.NewReader(content))
    lineNum := 0

    for scanner.Scan() {
        lineNum++
        line := strings.TrimSpace(scanner.Text())

        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }

        // Split on first whitespace: action + rest
        spaceIdx := strings.IndexAny(line, " \t")
        if spaceIdx < 0 {
            return nil, fmt.Errorf("line %d: expected 'action rule', got %q", lineNum, line)
        }

        actionStr := line[:spaceIdx]
        rest := strings.TrimSpace(line[spaceIdx+1:])

        // Strip trailing # comment from the rule
        if commentIdx := strings.LastIndex(rest, "  #"); commentIdx >= 0 {
            rest = strings.TrimSpace(rest[:commentIdx])
        }

        action, err := resolveActionPrefix(actionStr)
        if err != nil {
            return nil, fmt.Errorf("line %d: %w", lineNum, err)
        }

        decisions = append(decisions, ReviewDecision{
            Rule:   rest,
            Action: action,
        })
    }

    return decisions, scanner.Err()
}

// resolveActionPrefix resolves a full action name or unique prefix to the
// canonical action constant. All four actions have unique first letters.
func resolveActionPrefix(prefix string) (string, error) {
    actions := []string{ReviewPromoteGlobal, ReviewPromoteRepo, ReviewKeep, ReviewDiscard}

    var matches []string
    for _, a := range actions {
        if strings.HasPrefix(a, prefix) {
            matches = append(matches, a)
        }
    }

    switch len(matches) {
    case 0:
        return "", fmt.Errorf("unknown action %q (valid: global, repo, keep, discard)", prefix)
    case 1:
        return matches[0], nil
    default:
        return "", fmt.Errorf("ambiguous action prefix %q (matches: %s)", prefix, strings.Join(matches, ", "))
    }
}
```

**Step 8: Run all editor tests to verify they pass**

Run:
`nix develop --command go test -run 'TestFriendly|TestFormat|TestParse' ./packages/spinclass/internal/perms/`
Expected: PASS

**Step 9: Commit**

``` bash
git add packages/spinclass/internal/perms/editor.go packages/spinclass/internal/perms/editor_test.go
git commit -m "feat(spinclass): add editor format generation and parsing for perms review"
```

--------------------------------------------------------------------------------

### Task 2: Add Global Settings Diff and Worktree Rule Filtering

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/perms/settings.go:69-111` -
Modify: `packages/spinclass/internal/perms/settings_test.go` (create if absent)

**Step 1: Write the failing test for `ComputeReviewableRules`**

Create `packages/spinclass/internal/perms/settings_test.go` (or append if
exists):

``` go
package perms

import (
    "os"
    "path/filepath"
    "testing"
)

func TestComputeReviewableRules(t *testing.T) {
    tmpDir := t.TempDir()

    // Worktree settings: mix of rules
    worktreeSettingsPath := filepath.Join(tmpDir, "worktree", ".claude", "settings.local.json")
    worktreeRules := []string{
        "Bash(go test:*)",
        "Bash(nix build:*)",
        "Edit",
        "Glob",
        "Read(/Users/me/.claude/*)",
        "Read(/Users/me/repos/bob/.worktrees/wt/*)",
        "Edit(/Users/me/repos/bob/.worktrees/wt/*)",
        "Write(/Users/me/repos/bob/.worktrees/wt/*)",
        "mcp__plugin_grit_grit__add",
        "WebSearch",
    }
    if err := SaveClaudeSettings(worktreeSettingsPath, worktreeRules); err != nil {
        t.Fatal(err)
    }

    // Global settings: some overlap
    globalSettingsPath := filepath.Join(tmpDir, "global-settings.json")
    if err := SaveClaudeSettings(globalSettingsPath, []string{
        "Glob",
        "WebSearch",
    }); err != nil {
        t.Fatal(err)
    }

    // Tier files: some overlap
    tiersDir := filepath.Join(tmpDir, "tiers")
    os.MkdirAll(filepath.Join(tiersDir, "repos"), 0o755)
    if err := SaveTierFile(filepath.Join(tiersDir, "global.json"), Tier{Allow: []string{"Edit"}}); err != nil {
        t.Fatal(err)
    }

    got, err := ComputeReviewableRules(
        worktreeSettingsPath,
        globalSettingsPath,
        tiersDir,
        "myrepo",
        "/Users/me/repos/bob/.worktrees/wt",
    )
    if err != nil {
        t.Fatal(err)
    }

    // Should contain: Bash(go test:*), Bash(nix build:*), mcp__plugin_grit_grit__add
    // Should NOT contain: Edit (in tier), Glob (global), WebSearch (global),
    // Read/Edit/Write worktree paths, Read(~/.claude/*)
    wantSet := map[string]bool{
        "Bash(go test:*)":              true,
        "Bash(nix build:*)":            true,
        "mcp__plugin_grit_grit__add":   true,
    }

    gotSet := map[string]bool{}
    for _, r := range got {
        gotSet[r] = true
    }

    for want := range wantSet {
        if !gotSet[want] {
            t.Errorf("expected %q in reviewable rules", want)
        }
    }

    for _, r := range got {
        if !wantSet[r] {
            t.Errorf("unexpected rule %q in reviewable rules", r)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestComputeReviewableRules ./packages/spinclass/internal/perms/`
Expected: FAIL --- `ComputeReviewableRules` not defined.

**Step 3: Implement `ComputeReviewableRules`**

Add to `packages/spinclass/internal/perms/settings.go`:

``` go
import "strings"

// ComputeReviewableRules returns worktree rules that are not already covered by
// global Claude settings, curated tier files, or auto-injected worktree-scoped
// rules.
func ComputeReviewableRules(
    worktreeSettingsPath, globalSettingsPath, tiersDir, repo, worktreePath string,
) ([]string, error) {
    worktreeRules, err := LoadClaudeSettings(worktreeSettingsPath)
    if err != nil {
        return nil, err
    }

    globalRules, err := LoadClaudeSettings(globalSettingsPath)
    if err != nil {
        return nil, err
    }

    tierRules := LoadTiers(tiersDir, repo)

    exclude := make(map[string]bool)
    for _, r := range globalRules {
        exclude[r] = true
    }
    for _, r := range tierRules {
        exclude[r] = true
    }

    // Auto-injected worktree-scoped rules
    home, _ := os.UserHomeDir()
    if home != "" {
        exclude[fmt.Sprintf("Read(%s/.claude/*)", home)] = true
    }
    if worktreePath != "" {
        exclude[fmt.Sprintf("Read(%s/*)", worktreePath)] = true
        exclude[fmt.Sprintf("Edit(%s/*)", worktreePath)] = true
        exclude[fmt.Sprintf("Write(%s/*)", worktreePath)] = true
    }

    var result []string
    for _, r := range worktreeRules {
        if !exclude[r] {
            result = append(result, r)
        }
    }

    if result == nil {
        result = []string{}
    }

    return result, nil
}

// GlobalClaudeSettingsPath returns the path to the user-level Claude
// settings.local.json file.
func GlobalClaudeSettingsPath() string {
    home, _ := os.UserHomeDir()
    if home == "" {
        return ""
    }
    return filepath.Join(home, ".claude", "settings.local.json")
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestComputeReviewableRules ./packages/spinclass/internal/perms/`
Expected: PASS

**Step 5: Commit**

``` bash
git add packages/spinclass/internal/perms/settings.go packages/spinclass/internal/perms/settings_test.go
git commit -m "feat(spinclass): add ComputeReviewableRules with global settings diff"
```

--------------------------------------------------------------------------------

### Task 3: Update `RouteDecisions` to Drop Snapshot Logic

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/perms/review.go:19-73` -
Modify: `packages/spinclass/internal/perms/review_test.go`

**Step 1: Update `RouteDecisions` to remove snapshot update**

In `packages/spinclass/internal/perms/review.go`, replace the function body to
remove lines 62-72 (the snapshot update block):

``` go
func RouteDecisions(
    tiersDir, repo, settingsPath string,
    decisions []ReviewDecision,
) error {
    var toRemove []string

    for _, d := range decisions {
        switch d.Action {
        case ReviewPromoteGlobal:
            globalPath := filepath.Join(tiersDir, "global.json")
            if err := AppendToTierFile(globalPath, d.Rule); err != nil {
                return err
            }
            toRemove = append(toRemove, d.Rule)

        case ReviewPromoteRepo:
            repoPath := filepath.Join(tiersDir, "repos", repo+".json")
            if err := AppendToTierFile(repoPath, d.Rule); err != nil {
                return err
            }
            toRemove = append(toRemove, d.Rule)

        case ReviewDiscard:
            toRemove = append(toRemove, d.Rule)

        case ReviewKeep:
            // Leave in settings, nothing to do.
        }
    }

    if len(toRemove) > 0 {
        current, err := LoadClaudeSettings(settingsPath)
        if err != nil {
            return err
        }

        remaining := RemoveRules(current, toRemove)

        if err := SaveClaudeSettings(settingsPath, remaining); err != nil {
            return err
        }
    }

    return nil
}
```

**Step 2: Run existing RouteDecisions tests to verify they still pass**

Run:
`nix develop --command go test -run TestRouteDecisions ./packages/spinclass/internal/perms/`
Expected: PASS (snapshot file may still be created by test setup but is no
longer read by RouteDecisions)

**Step 3: Add `DryRunDecisions` for `--dry-run` output**

Add to `review.go`:

``` go
import (
    "fmt"
    "io"
)

// DryRunDecisions prints what RouteDecisions would do without writing files.
func DryRunDecisions(w io.Writer, tiersDir, repo string, decisions []ReviewDecision) {
    groups := map[string][]string{
        ReviewPromoteGlobal: {},
        ReviewPromoteRepo:   {},
        ReviewDiscard:       {},
        ReviewKeep:          {},
    }

    for _, d := range decisions {
        groups[d.Action] = append(groups[d.Action], d.Rule)
    }

    globalPath := filepath.Join(tiersDir, "global.json")
    repoPath := filepath.Join(tiersDir, "repos", repo+".json")

    if len(groups[ReviewPromoteGlobal]) > 0 {
        fmt.Fprintf(w, "would promote to global tier (%s):\n", globalPath)
        for _, r := range groups[ReviewPromoteGlobal] {
            fmt.Fprintf(w, "  %s\n", r)
        }
    }
    if len(groups[ReviewPromoteRepo]) > 0 {
        fmt.Fprintf(w, "would promote to repo tier (%s):\n", repoPath)
        for _, r := range groups[ReviewPromoteRepo] {
            fmt.Fprintf(w, "  %s\n", r)
        }
    }
    if len(groups[ReviewDiscard]) > 0 {
        fmt.Fprintln(w, "would discard (remove from settings.local.json):")
        for _, r := range groups[ReviewDiscard] {
            fmt.Fprintf(w, "  %s\n", r)
        }
    }
    if len(groups[ReviewKeep]) > 0 {
        fmt.Fprintln(w, "would keep (no change):")
        for _, r := range groups[ReviewKeep] {
            fmt.Fprintf(w, "  %s\n", r)
        }
    }
}
```

**Step 4: Write test for `DryRunDecisions`**

Append to `review_test.go`:

``` go
import "bytes"

func TestDryRunDecisions(t *testing.T) {
    var buf bytes.Buffer
    decisions := []ReviewDecision{
        {Rule: "Bash(go test:*)", Action: ReviewPromoteGlobal},
        {Rule: "Edit", Action: ReviewPromoteRepo},
        {Rule: "Bash(rm -rf:*)", Action: ReviewDiscard},
        {Rule: "Read", Action: ReviewKeep},
    }

    DryRunDecisions(&buf, "/tmp/tiers", "myrepo", decisions)
    out := buf.String()

    if !strings.Contains(out, "would promote to global tier") {
        t.Error("expected global tier output")
    }
    if !strings.Contains(out, "Bash(go test:*)") {
        t.Error("expected promoted rule in output")
    }
    if !strings.Contains(out, "would discard") {
        t.Error("expected discard output")
    }
    if !strings.Contains(out, "would keep") {
        t.Error("expected keep output")
    }
}
```

**Step 5: Run tests**

Run:
`nix develop --command go test -run 'TestRouteDecisions|TestDryRun' ./packages/spinclass/internal/perms/`
Expected: PASS

**Step 6: Commit**

``` bash
git add packages/spinclass/internal/perms/review.go packages/spinclass/internal/perms/review_test.go
git commit -m "feat(spinclass): drop snapshot logic from RouteDecisions, add DryRunDecisions"
```

--------------------------------------------------------------------------------

### Task 4: Replace `RunReviewInteractive` with Editor-Based Flow

**Promotion criteria:** N/A

**Files:** - Modify: `packages/spinclass/internal/perms/cmd.go:41-75`
(newReviewCmd) and `204-250` (RunReviewInteractive)

**Step 1: Rewrite `RunReviewInteractive` to use the editor flow**

Replace `RunReviewInteractive` in `cmd.go` (lines 204-250) with:

``` go
// RunReviewEditor opens $EDITOR with reviewable rules and loops until the user
// accepts, edits again, or aborts.
func RunReviewEditor(worktreePath, repoName string, dryRun bool) error {
    settingsPath := filepath.Join(worktreePath, ".claude", "settings.local.json")
    tiersDir := TiersDir()
    globalSettingsPath := GlobalClaudeSettingsPath()

    rules, err := ComputeReviewableRules(
        settingsPath, globalSettingsPath, tiersDir, repoName, worktreePath,
    )
    if err != nil {
        return err
    }

    if len(rules) == 0 {
        fmt.Println("no new permissions to review")
        return nil
    }

    content := FormatEditorContent(rules, repoName)

    tmpFile, err := os.CreateTemp("", "spinclass-perms-review-*.txt")
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile.Name())

    if _, err := tmpFile.WriteString(content); err != nil {
        tmpFile.Close()
        return err
    }
    tmpFile.Close()

    for {
        if err := openEditor(tmpFile.Name()); err != nil {
            return fmt.Errorf("editor failed: %w", err)
        }

        edited, err := os.ReadFile(tmpFile.Name())
        if err != nil {
            return err
        }

        decisions, err := ParseEditorContent(string(edited))
        if err != nil {
            fmt.Fprintf(os.Stderr, "Parse error: %v\nRe-opening editor.\n", err)
            continue
        }

        if len(decisions) == 0 {
            fmt.Println("no decisions — aborting")
            return nil
        }

        // Print the parsed decisions for review
        fmt.Println()
        for _, d := range decisions {
            friendly := FriendlyName(d.Rule)
            if friendly != "" {
                fmt.Printf("  %-8s %s  # %s\n", d.Action, d.Rule, friendly)
            } else {
                fmt.Printf("  %-8s %s\n", d.Action, d.Rule)
            }
        }
        fmt.Println()

        var choice string
        prompt := huh.NewSelect[string]().
            Title("Review complete").
            Options(
                huh.NewOption("Accept", "accept"),
                huh.NewOption("Edit again", "edit"),
                huh.NewOption("Abort", "abort"),
            ).
            Value(&choice)

        if err := prompt.Run(); err != nil {
            return err
        }

        switch choice {
        case "accept":
            if dryRun {
                DryRunDecisions(os.Stdout, tiersDir, repoName, decisions)
                return nil
            }
            return RouteDecisions(tiersDir, repoName, settingsPath, decisions)
        case "edit":
            continue
        case "abort":
            fmt.Println("aborted")
            return nil
        }
    }
}

func openEditor(path string) error {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vi"
    }

    cmd := exec.Command(editor, path)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    return cmd.Run()
}
```

**Step 2: Update `newReviewCmd` to add flags and call `RunReviewEditor`**

Replace `newReviewCmd` (lines 41-75) with:

``` go
func newReviewCmd() *cobra.Command {
    var worktreeDir string
    var dryRun bool

    cmd := &cobra.Command{
        Use:   "review [worktree-path]",
        Short: "Interactively review new permissions from a session",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            var worktreePath string

            switch {
            case worktreeDir != "":
                worktreePath = worktreeDir
            case len(args) > 0:
                worktreePath = args[0]
            default:
                cwd, err := os.Getwd()
                if err != nil {
                    return err
                }
                worktreePath = cwd
            }

            if !filepath.IsAbs(worktreePath) {
                cwd, err := os.Getwd()
                if err != nil {
                    return err
                }
                worktreePath = filepath.Join(cwd, worktreePath)
            }

            repoPath, err := worktree.DetectRepo(worktreePath)
            if err != nil {
                return fmt.Errorf("could not detect repo: %w", err)
            }
            repoName := filepath.Base(repoPath)

            return RunReviewEditor(worktreePath, repoName, dryRun)
        },
    }

    cmd.Flags().StringVar(&worktreeDir, "worktree-dir", "", "override worktree path")
    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")

    return cmd
}
```

**Step 3: Remove the old `RunReviewInteractive` function and the unused `huh`
import if `huh` is still used in `RunReviewEditor`**

The `huh` import stays since we use it for the accept/edit/abort prompt. Remove
the old `RunReviewInteractive` function entirely (it's been replaced by
`RunReviewEditor`).

**Step 4: Run all perms tests to verify nothing is broken**

Run: `nix develop --command go test ./packages/spinclass/internal/perms/`
Expected: PASS

**Step 5: Commit**

``` bash
git add packages/spinclass/internal/perms/cmd.go
git commit -m "feat(spinclass): replace huh TUI with editor-based perms review"
```

--------------------------------------------------------------------------------

### Task 5: Manual Integration Test

**Promotion criteria:** N/A

**Files:** None (testing only)

**Step 1: Build spinclass**

Run: `nix build .#spinclass`

**Step 2: Test dry run against this worktree**

Run:
`./result/bin/spinclass perms review --worktree-dir /Users/sfriedenberg/eng/repos/bob/.worktrees/mild-hazel --dry-run`

Verify: editor opens with alphabetically sorted rules, MCP tools have
`# server:tool` comments, all default to `discard`.

**Step 3: Test the accept/edit/abort flow**

Change a few actions in the editor, save, verify the review list prints
correctly, select "abort" to avoid modifying real files.

**Step 4: Test with no new rules**

Create a temp directory with a `settings.local.json` containing only rules
already in global settings. Verify "no new permissions to review" is printed.
