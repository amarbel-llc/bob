# WebFetch Interception Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Intercept WebFetch calls to GitHub URLs in get-hubbed's PreToolUse
hook, deny them, and return specific get-hubbed resource URIs.

**Architecture:** New `internal/hooks/` package in get-hubbed with
URL-to-resource mapping table. `main.go` buffers stdin and tries custom hook
before framework fallback. Post-process hooks.json to add `WebFetch` to the
PreToolUse matcher.

**Tech Stack:** Go, `net/url` for URL parsing, `encoding/json` for hook I/O. No
new dependencies.

**Rollback:** Purely additive. Revert commits and reinstall marketplace.

--------------------------------------------------------------------------------

### Task 1: WebFetch URL matching and deny logic

**Promotion criteria:** N/A

**Files:** - Create: `packages/get-hubbed/internal/hooks/webfetch.go` - Create:
`packages/get-hubbed/internal/hooks/webfetch_test.go`

**Step 1: Write the failing tests**

Create `packages/get-hubbed/internal/hooks/webfetch_test.go`:

``` go
package hooks

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"
)

func makeHookInput(toolName string, toolInput map[string]any) []byte {
    input := map[string]any{
        "tool_name":  toolName,
        "tool_input": toolInput,
    }
    data, _ := json.Marshal(input)
    return data
}

func TestWebFetchHookIssueDetail(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/issues/42",
        "prompt": "get the issue",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !handled {
        t.Fatal("expected hook to handle GitHub issue URL")
    }
    if !strings.Contains(out.String(), "get-hubbed://issues?number=42&repo=owner/repo") {
        t.Errorf("expected specific resource URI in output, got %q", out.String())
    }
}

func TestWebFetchHookNonWebFetchTool(t *testing.T) {
    input := makeHookInput("Bash", map[string]any{"command": "ls"})
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if handled {
        t.Fatal("expected hook to not handle non-WebFetch tool")
    }
}

func TestWebFetchHookNonGitHubURL(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://example.com/page",
        "prompt": "get the page",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if handled {
        t.Fatal("expected hook to not handle non-GitHub URL")
    }
}

func TestWebFetchHookMalformedURL(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "not-a-url",
        "prompt": "get something",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if handled {
        t.Fatal("expected hook to fail-open on malformed URL")
    }
}

func TestWebFetchHookCatchAllGitHub(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/settings",
        "prompt": "get the settings",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !handled {
        t.Fatal("expected hook to catch-all GitHub URL")
    }
    if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
        t.Errorf("expected catch-all message, got %q", out.String())
    }
}

func TestWebFetchHookAllGitHubDomains(t *testing.T) {
    domains := []string{
        "https://github.com/owner/repo/settings",
        "https://www.github.com/owner/repo/settings",
        "https://api.github.com/repos/owner/repo",
        "https://raw.githubusercontent.com/owner/repo/main/README.md",
        "https://gist.github.com/owner/abc123",
    }

    for _, url := range domains {
        t.Run(url, func(t *testing.T) {
            input := makeHookInput("WebFetch", map[string]any{
                "url":    url,
                "prompt": "fetch",
            })
            var out bytes.Buffer
            handled, err := HandleWebFetchHook(input, &out)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !handled {
                t.Fatalf("expected hook to handle %s", url)
            }
        })
    }
}

func TestWebFetchHookDenyMessageFormat(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/issues/42",
        "prompt": "fetch",
    })
    var out bytes.Buffer
    HandleWebFetchHook(input, &out)

    output := out.String()
    if !strings.Contains(output, "deny") {
        t.Error("deny message should contain deny decision")
    }
    if !strings.Contains(output, "resource-read") {
        t.Error("deny message should contain resource-read for subagents")
    }
    if !strings.Contains(output, "get-hubbed for ALL GitHub interactions") {
        t.Error("deny message should instruct exclusive get-hubbed usage")
    }
}

func TestWebFetchHookAllMappings(t *testing.T) {
    tests := []struct {
        url         string
        resourceURI string
    }{
        {"https://github.com/owner/repo", "get-hubbed://repo"},
        {"https://github.com/owner/repo/issues", "get-hubbed://issues?repo=owner/repo"},
        {"https://github.com/owner/repo/issues/42", "get-hubbed://issues?number=42&repo=owner/repo"},
        {"https://github.com/owner/repo/pulls", "get-hubbed://pulls?repo=owner/repo"},
        {"https://github.com/owner/repo/pull/7", "get-hubbed://pulls?number=7&repo=owner/repo"},
        {"https://github.com/owner/repo/blob/main/src/foo.go", "get-hubbed://contents?path=src/foo.go&repo=owner/repo&ref=main"},
        {"https://github.com/owner/repo/tree/main/src", "get-hubbed://tree?path=src&repo=owner/repo&ref=main"},
        {"https://github.com/owner/repo/blame/main/src/foo.go", "get-hubbed://blame?path=src/foo.go&repo=owner/repo&ref=main"},
        {"https://github.com/owner/repo/commits/main", "get-hubbed://commits?repo=owner/repo&ref=main"},
        {"https://github.com/owner/repo/actions", "get-hubbed://runs?repo=owner/repo"},
        {"https://github.com/owner/repo/actions/runs/12345", "get-hubbed://runs?run_id=12345&repo=owner/repo"},
    }

    for _, tt := range tests {
        t.Run(tt.url, func(t *testing.T) {
            input := makeHookInput("WebFetch", map[string]any{
                "url":    tt.url,
                "prompt": "fetch",
            })
            var out bytes.Buffer
            handled, err := HandleWebFetchHook(input, &out)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !handled {
                t.Fatalf("expected hook to handle %q", tt.url)
            }
            if !strings.Contains(out.String(), tt.resourceURI) {
                t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
            }
        })
    }
}

func TestWebFetchHookCompareURL(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/compare/main...feature",
        "prompt": "compare branches",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !handled {
        t.Fatal("expected hook to handle compare URL")
    }
    if !strings.Contains(out.String(), "content-compare") {
        t.Errorf("expected content-compare tool in output, got %q", out.String())
    }
}

func TestWebFetchHookURLWithFragment(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/issues/42#issuecomment-123",
        "prompt": "fetch",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !handled {
        t.Fatal("expected hook to handle URL with fragment")
    }
    if !strings.Contains(out.String(), "get-hubbed://issues?number=42&repo=owner/repo") {
        t.Errorf("expected issue resource URI, got %q", out.String())
    }
}

func TestWebFetchHookURLWithQueryParams(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "https://github.com/owner/repo/issues?q=is:open+label:bug",
        "prompt": "fetch",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !handled {
        t.Fatal("expected hook to handle URL with query params")
    }
    if !strings.Contains(out.String(), "get-hubbed://issues?repo=owner/repo") {
        t.Errorf("expected issues resource URI, got %q", out.String())
    }
}

func TestWebFetchHookInvalidJSON(t *testing.T) {
    var out bytes.Buffer
    handled, err := HandleWebFetchHook([]byte("not json"), &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if handled {
        t.Fatal("expected hook to fail-open on invalid JSON")
    }
}

func TestWebFetchHookEmptyURL(t *testing.T) {
    input := makeHookInput("WebFetch", map[string]any{
        "url":    "",
        "prompt": "fetch",
    })
    var out bytes.Buffer
    handled, err := HandleWebFetchHook(input, &out)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if handled {
        t.Fatal("expected hook to not handle empty URL")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `nix develop --command go test ./packages/get-hubbed/internal/hooks/...`
Expected: FAIL --- package doesn't exist yet.

**Step 3: Write the implementation**

Create `packages/get-hubbed/internal/hooks/webfetch.go`:

``` go
package hooks

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/url"
    "strings"
)

type hookInput struct {
    ToolName  string         `json:"tool_name"`
    ToolInput map[string]any `json:"tool_input"`
}

var githubDomains = map[string]bool{
    "github.com":                true,
    "www.github.com":            true,
    "api.github.com":            true,
    "raw.githubusercontent.com": true,
    "gist.github.com":           true,
}

type urlMapping struct {
    // match returns the get-hubbed resource URI if the path matches,
    // or empty string if no match. segments is the cleaned URL path
    // split by "/", with leading empty string removed.
    match func(segments []string) string
    // description for the deny message (only used for tool mappings)
    description string
}

// matchGitHubURL attempts to match a github.com URL path to a get-hubbed
// resource URI or tool name. Returns (resourceURI, isToolNotResource) or
// ("", false) if no match.
func matchGitHubURL(rawURL string) (string, bool) {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return "", false
    }

    // Only match github.com paths (not API or raw)
    if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
        return "", false
    }

    path := strings.TrimSuffix(parsed.Path, "/")
    segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

    if len(segments) < 2 || segments[0] == "" {
        return "", false
    }

    owner := segments[0]
    repo := segments[1]
    repoSlug := owner + "/" + repo

    // Exact: /{owner}/{repo}
    if len(segments) == 2 {
        return "get-hubbed://repo", false
    }

    section := segments[2]
    switch section {
    case "issues":
        if len(segments) == 3 {
            return fmt.Sprintf("get-hubbed://issues?repo=%s", repoSlug), false
        }
        if len(segments) == 4 {
            return fmt.Sprintf("get-hubbed://issues?number=%s&repo=%s", segments[3], repoSlug), false
        }

    case "pulls":
        if len(segments) == 3 {
            return fmt.Sprintf("get-hubbed://pulls?repo=%s", repoSlug), false
        }

    case "pull":
        if len(segments) >= 4 {
            return fmt.Sprintf("get-hubbed://pulls?number=%s&repo=%s", segments[3], repoSlug), false
        }

    case "blob":
        if len(segments) >= 5 {
            ref := segments[3]
            filePath := strings.Join(segments[4:], "/")
            return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s&ref=%s", filePath, repoSlug, ref), false
        }

    case "tree":
        if len(segments) >= 5 {
            ref := segments[3]
            dirPath := strings.Join(segments[4:], "/")
            return fmt.Sprintf("get-hubbed://tree?path=%s&repo=%s&ref=%s", dirPath, repoSlug, ref), false
        }
        if len(segments) == 4 {
            ref := segments[3]
            return fmt.Sprintf("get-hubbed://tree?repo=%s&ref=%s", repoSlug, ref), false
        }

    case "blame":
        if len(segments) >= 5 {
            ref := segments[3]
            filePath := strings.Join(segments[4:], "/")
            return fmt.Sprintf("get-hubbed://blame?path=%s&repo=%s&ref=%s", filePath, repoSlug, ref), false
        }

    case "commits":
        if len(segments) >= 4 {
            ref := segments[3]
            return fmt.Sprintf("get-hubbed://commits?repo=%s&ref=%s", repoSlug, ref), false
        }

    case "actions":
        if len(segments) == 3 {
            return fmt.Sprintf("get-hubbed://runs?repo=%s", repoSlug), false
        }
        if len(segments) >= 5 && segments[3] == "runs" {
            return fmt.Sprintf("get-hubbed://runs?run_id=%s&repo=%s", segments[4], repoSlug), false
        }

    case "compare":
        if len(segments) >= 4 {
            return fmt.Sprintf("content-compare tool with repo=%s, base and head from %s", repoSlug, segments[3]), true
        }
    }

    return "", false
}

// HandleWebFetchHook checks whether a hook input is a WebFetch targeting a
// GitHub URL. Returns (true, nil) if denied, (false, nil) if no match.
// Follows fail-open: parse errors return (false, nil).
func HandleWebFetchHook(input []byte, w io.Writer) (bool, error) {
    var hi hookInput
    if err := json.Unmarshal(input, &hi); err != nil {
        log.Printf("webfetch hook: ignoring decode error (fail-open): %v", err)
        return false, nil
    }

    if hi.ToolName != "WebFetch" {
        return false, nil
    }

    rawURL, _ := hi.ToolInput["url"].(string)
    if rawURL == "" {
        return false, nil
    }

    parsed, err := url.Parse(rawURL)
    if err != nil {
        log.Printf("webfetch hook: ignoring URL parse error (fail-open): %v", err)
        return false, nil
    }

    if !githubDomains[parsed.Host] {
        return false, nil
    }

    // Try specific mapping
    resourceURI, isTool := matchGitHubURL(rawURL)
    if resourceURI != "" {
        if isTool {
            return true, writeToolDeny(w, resourceURI)
        }
        return true, writeResourceDeny(w, resourceURI)
    }

    // Catch-all for any GitHub domain
    return true, writeCatchAllDeny(w)
}

func writeResourceDeny(w io.Writer, resourceURI string) error {
    reason := fmt.Sprintf(
        "DENIED: Use %s instead.\n"+
            "Use get-hubbed for ALL GitHub interactions — do not use WebFetch or Bash with gh/curl for GitHub.\n"+
            "Subagents: use mcp__plugin_get-hubbed_get-hubbed__resource-read with uri %s",
        resourceURI, resourceURI,
    )
    return writeDenyJSON(w, reason)
}

func writeToolDeny(w io.Writer, toolDescription string) error {
    reason := fmt.Sprintf(
        "DENIED: Use the get-hubbed %s instead.\n"+
            "Use get-hubbed for ALL GitHub interactions — do not use WebFetch or Bash with gh/curl for GitHub.\n"+
            "Subagents: use mcp__plugin_get-hubbed_get-hubbed__content-compare",
        toolDescription,
    )
    return writeDenyJSON(w, reason)
}

func writeCatchAllDeny(w io.Writer) error {
    reason := "DENIED: GitHub URLs are served by get-hubbed. Do not use WebFetch for GitHub.\n" +
        "Use get-hubbed for ALL GitHub interactions — do not use WebFetch or Bash with gh/curl for GitHub.\n\n" +
        "Resources (read-only): get-hubbed://repo, get-hubbed://issues, get-hubbed://pulls, " +
        "get-hubbed://contents, get-hubbed://tree, get-hubbed://blame, get-hubbed://commits, get-hubbed://runs\n" +
        "Tools (mutations): issue-create, issue-close, issue-comment, pr-create, " +
        "content-search, content-compare, api-get, graphql-query, graphql-mutation\n" +
        "Discovery: resource-templates, resource-read\n" +
        "Subagents: mcp__plugin_get-hubbed_get-hubbed__resource-read or mcp__plugin_get-hubbed_get-hubbed__<tool_name>"
    return writeDenyJSON(w, reason)
}

func writeDenyJSON(w io.Writer, reason string) error {
    output := map[string]any{
        "hookSpecificOutput": map[string]any{
            "hookEventName":            "PreToolUse",
            "permissionDecision":       "deny",
            "permissionDecisionReason": reason,
        },
    }
    return json.NewEncoder(w).Encode(output)
}
```

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test -v ./packages/get-hubbed/internal/hooks/...`
Expected: All PASS.

**Step 5: Commit**

    git add packages/get-hubbed/internal/hooks/webfetch.go packages/get-hubbed/internal/hooks/webfetch_test.go
    git commit -m "feat(get-hubbed): add WebFetch GitHub URL interception hook

    Parse GitHub URLs from WebFetch tool_input and deny with specific
    get-hubbed resource URIs. Falls back to catch-all for unrecognized
    GitHub URL patterns."

--------------------------------------------------------------------------------

### Task 2: Hook matcher patching

**Promotion criteria:** N/A

**Files:** - Create: `packages/get-hubbed/internal/hooks/patch.go` - Create:
`packages/get-hubbed/internal/hooks/patch_test.go`

**Step 1: Write the failing tests**

Create `packages/get-hubbed/internal/hooks/patch_test.go`:

``` go
package hooks

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestPatchHooksMatcherAddWebFetch(t *testing.T) {
    dir := t.TempDir()
    hooksDir := filepath.Join(dir, "hooks")
    if err := os.MkdirAll(hooksDir, 0o755); err != nil {
        t.Fatal(err)
    }

    // Write initial hooks.json with "Bash" matcher (as framework generates)
    initial := map[string]any{
        "hooks": map[string]any{
            "PreToolUse": []any{
                map[string]any{
                    "matcher": "Bash",
                    "hooks": []any{
                        map[string]any{
                            "type":    "command",
                            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/pre-tool-use",
                            "timeout": float64(5),
                        },
                    },
                },
            },
        },
    }
    data, _ := json.MarshalIndent(initial, "", "  ")
    if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644); err != nil {
        t.Fatal(err)
    }

    if err := PatchHooksMatcher(dir, "WebFetch"); err != nil {
        t.Fatalf("unexpected error: %v", err)
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
    entries := hooks["PreToolUse"].([]any)
    entry := entries[0].(map[string]any)
    matcher := entry["matcher"].(string)

    if matcher != "Bash|WebFetch" {
        t.Errorf("expected matcher 'Bash|WebFetch', got %q", matcher)
    }
}

func TestPatchHooksMatcherAlreadyPresent(t *testing.T) {
    dir := t.TempDir()
    hooksDir := filepath.Join(dir, "hooks")
    if err := os.MkdirAll(hooksDir, 0o755); err != nil {
        t.Fatal(err)
    }

    initial := map[string]any{
        "hooks": map[string]any{
            "PreToolUse": []any{
                map[string]any{
                    "matcher": "Bash|WebFetch",
                    "hooks":   []any{},
                },
            },
        },
    }
    data, _ := json.MarshalIndent(initial, "", "  ")
    if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644); err != nil {
        t.Fatal(err)
    }

    if err := PatchHooksMatcher(dir, "WebFetch"); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    result, _ := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
    var manifest map[string]any
    json.Unmarshal(result, &manifest)
    hooks := manifest["hooks"].(map[string]any)
    entries := hooks["PreToolUse"].([]any)
    entry := entries[0].(map[string]any)
    matcher := entry["matcher"].(string)

    if matcher != "Bash|WebFetch" {
        t.Errorf("expected matcher unchanged 'Bash|WebFetch', got %q", matcher)
    }
}

func TestPatchHooksMatcherNoFile(t *testing.T) {
    dir := t.TempDir()
    err := PatchHooksMatcher(dir, "WebFetch")
    if err == nil {
        t.Fatal("expected error when hooks.json missing")
    }
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -v -run TestPatchHooks ./packages/get-hubbed/internal/hooks/...`
Expected: FAIL --- `PatchHooksMatcher` undefined.

**Step 3: Write the implementation**

Create `packages/get-hubbed/internal/hooks/patch.go`:

``` go
package hooks

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// PatchHooksMatcher reads hooks/hooks.json under dir, appends extraMatcher to
// each PreToolUse entry's matcher (if not already present), and writes it back.
// This is called after the framework's GenerateHooks to extend the matcher
// beyond what tool mappings produce (e.g., adding "WebFetch").
func PatchHooksMatcher(dir string, extraMatcher string) error {
    hooksJSONPath := filepath.Join(dir, "hooks", "hooks.json")

    data, err := os.ReadFile(hooksJSONPath)
    if err != nil {
        return fmt.Errorf("reading hooks.json: %w", err)
    }

    var manifest map[string]any
    if err := json.Unmarshal(data, &manifest); err != nil {
        return fmt.Errorf("parsing hooks.json: %w", err)
    }

    hooks, ok := manifest["hooks"].(map[string]any)
    if !ok {
        return fmt.Errorf("hooks.json missing 'hooks' key")
    }

    preToolUse, ok := hooks["PreToolUse"].([]any)
    if !ok {
        return fmt.Errorf("hooks.json missing 'PreToolUse' key")
    }

    for _, entry := range preToolUse {
        entryMap, ok := entry.(map[string]any)
        if !ok {
            continue
        }
        matcher, _ := entryMap["matcher"].(string)
        if matcher == "" {
            continue
        }

        // Check if extraMatcher is already in the pipe-separated list
        parts := strings.Split(matcher, "|")
        found := false
        for _, p := range parts {
            if p == extraMatcher {
                found = true
                break
            }
        }
        if !found {
            entryMap["matcher"] = matcher + "|" + extraMatcher
        }
    }

    data, err = json.MarshalIndent(manifest, "", "  ")
    if err != nil {
        return fmt.Errorf("marshaling hooks.json: %w", err)
    }
    data = append(data, '\n')

    return os.WriteFile(hooksJSONPath, data, 0o644)
}
```

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -v -run TestPatchHooks ./packages/get-hubbed/internal/hooks/...`
Expected: All PASS.

**Step 5: Commit**

    git add packages/get-hubbed/internal/hooks/patch.go packages/get-hubbed/internal/hooks/patch_test.go
    git commit -m "feat(get-hubbed): add PatchHooksMatcher for extending PreToolUse matcher

    Post-processes hooks.json to add extra matchers (e.g. WebFetch) beyond
    what the framework's tool mappings produce."

--------------------------------------------------------------------------------

### Task 3: Wire hooks into main.go

**Promotion criteria:** N/A

**Files:** - Modify: `packages/get-hubbed/cmd/get-hubbed/main.go`

**Step 1: Modify main.go**

Update the `hook` and `generate-plugin` handlers in
`packages/get-hubbed/cmd/get-hubbed/main.go`:

1.  Add imports: `"bytes"`, `"io"`, `"path/filepath"`, and
    `"github.com/friedenberg/get-hubbed/internal/hooks"`
2.  Replace the `generate-plugin` block to call `hooks.PatchHooksMatcher` after
    `app.HandleGeneratePlugin`
3.  Replace the `hook` block to buffer stdin, try `hooks.HandleWebFetchHook`
    first, fall back to `app.HandleHook`

The full updated main.go:

``` go
package main

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "log"
    "os"
    "os/signal"
    "path/filepath"

    "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
    "github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
    "github.com/friedenberg/get-hubbed/internal/clone"
    "github.com/friedenberg/get-hubbed/internal/hooks"
    "github.com/friedenberg/get-hubbed/internal/tools"
)

func main() {
    app, resProvider := tools.RegisterAll()

    if len(os.Args) >= 2 && os.Args[1] == "generate-plugin" {
        if err := app.HandleGeneratePlugin(os.Args[2:], os.Stdout); err != nil {
            log.Fatalf("generating plugin: %v", err)
        }

        // Patch hooks.json to also match WebFetch tool uses
        pluginDir := resolvePluginDir(os.Args[2:])
        if err := hooks.PatchHooksMatcher(pluginDir, "WebFetch"); err != nil {
            log.Fatalf("patching hooks matcher: %v", err)
        }

        return
    }

    if len(os.Args) >= 2 && os.Args[1] == "hook" {
        input, err := io.ReadAll(os.Stdin)
        if err != nil {
            log.Fatalf("reading hook input: %v", err)
        }

        handled, err := hooks.HandleWebFetchHook(input, os.Stdout)
        if err != nil {
            log.Fatalf("handling webfetch hook: %v", err)
        }

        if !handled {
            if err := app.HandleHook(bytes.NewReader(input), os.Stdout); err != nil {
                log.Fatalf("handling hook: %v", err)
            }
        }

        return
    }

    if len(os.Args) >= 2 && os.Args[1] == "clone" {
        if len(os.Args) >= 3 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
            fmt.Println("Usage: get-hubbed clone [dir]")
            fmt.Println()
            fmt.Println("Clone uncloned repos for the authenticated GitHub user.")
            fmt.Println("Defaults to current directory if dir is omitted.")
            os.Exit(0)
        }

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

    for _, arg := range os.Args[1:] {
        if arg == "-h" || arg == "--help" {
            fmt.Println("get-hubbed - a GitHub MCP server wrapping the gh CLI")
            fmt.Println()
            fmt.Println("Usage:")
            fmt.Println("  get-hubbed              Start MCP server (stdio)")
            fmt.Println("  get-hubbed clone [dir]   Clone uncloned repos for authenticated user")
            fmt.Println()
            os.Exit(0)
        }
    }

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    t := transport.NewStdio(os.Stdin, os.Stdout)

    registry := server.NewToolRegistryV1()
    app.RegisterMCPToolsV1(registry)
    tools.RegisterAPITools(registry)

    opts := server.Options{
        ServerName:    app.Name,
        ServerVersion: app.Version,
        Instructions: "GitHub MCP server. Read-only operations (repo info, issues, PRs, content, runs) are available as auto-approved resources via get-hubbed:// URIs. Mutation operations (issue/PR creation, comments, API calls) remain as tools." +
            "\n\nIMPORTANT: There are no tools named content_read, content_tree, content_commits, or repo_view. Use resource-read with get-hubbed:// URIs instead." +
            " All resource URIs use query parameters (e.g. get-hubbed://contents?path=README.md, get-hubbed://issues?number=42). Call resource-templates to see all available URIs.",
        Tools: registry,
    }

    if resProvider != nil {
        opts.Resources = resProvider
    }

    srv, err := server.New(t, opts)
    if err != nil {
        log.Fatalf("creating server: %v", err)
    }

    if err := srv.Run(ctx); err != nil {
        log.Fatalf("server error: %v", err)
    }
}

// resolvePluginDir determines where generate-plugin wrote its output.
// Mirrors the HandleGeneratePlugin dispatch: 0 args = ".", 1 arg = that dir.
// Returns the share/purse-first/get-hubbed subdirectory where hooks.json lives.
func resolvePluginDir(args []string) string {
    base := "."
    for _, a := range args {
        if a != "-" && !strings.HasPrefix(a, "-") {
            base = a
            break
        }
    }
    return filepath.Join(base, "share", "purse-first", "get-hubbed")
}
```

Note: add `"strings"` to the imports for the `resolvePluginDir` function.

**Step 2: Run tests to verify nothing is broken**

Run: `nix develop --command go test ./packages/get-hubbed/...` Expected: All
PASS (existing tests + new hook tests).

**Step 3: Build and verify the hooks.json output**

Run: `nix build .#get-hubbed` (use the chix build MCP tool) Then inspect:
`jq . result/share/purse-first/get-hubbed/hooks/hooks.json` Expected: matcher is
`"Bash|WebFetch"`

**Step 4: Commit**

    git add packages/get-hubbed/cmd/get-hubbed/main.go
    git commit -m "feat(get-hubbed): wire WebFetch hook into main.go

    Buffer stdin in hook handler to try WebFetch interception before
    framework fallback. Patch hooks.json matcher during generate-plugin
    to include WebFetch."

--------------------------------------------------------------------------------

### Task 4: File follow-up issues

**Promotion criteria:** N/A

**Files:** None (GitHub issues only)

**Step 1: File issues for unmapped domains**

Use `/file-issue` skill to create issues for: 1. `api.github.com` REST path
parsing --- similar URL patterns but different path structure (e.g.,
`/repos/{owner}/{repo}/issues/{n}`) 2. `raw.githubusercontent.com` content
serving --- map to `get-hubbed://contents` 3. `gist.github.com` support ---
needs new resource or tool 4. Framework `App.ExtraHookMatchers` in purse-first
go-mcp --- avoid hooks.json post-processing

**Step 2: Commit (no code changes, just verification)**

Run full test suite: `nix develop --command go test ./packages/get-hubbed/...`
Build: `nix build .#get-hubbed` Expected: All pass, hooks.json has
`"Bash|WebFetch"` matcher.
