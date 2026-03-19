# Grit MCP Resources Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Convert 7 read-only grit tools to native MCP resources with resource-templates/resource-read tool wrappers for subagent access.

**Architecture:** Register resources via `server.ResourceRegistry`, wrap in a `resourceProvider` for URI dispatch, wire into `server.Options{Resources: ...}`. Remove 7 tool registrations, add 2 new tools.

**Tech Stack:** Go, go-mcp (`server.ResourceRegistry`, `protocol.Resource`, `protocol.ResourceTemplate`), grit's existing `git.Run` + parsers.

**Rollback:** One-commit revert — re-add tool registrations, remove resource code.

---

### Task 1: Create resource provider with URI parsing

**Promotion criteria:** N/A

**Files:**
- Create: `packages/grit/internal/tools/resources.go`
- Test: `packages/grit/internal/tools/resources_test.go`

**Step 1: Write the failing test**

Create `packages/grit/internal/tools/resources_test.go`:

```go
package tools

import (
	"net/url"
	"testing"
)

func TestParseResourceURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantHost string
		wantPath string
		wantRepo string
	}{
		{
			name:     "status with repo_path",
			uri:      "grit://status?repo_path=/tmp/repo",
			wantHost: "status",
			wantPath: "",
			wantRepo: "/tmp/repo",
		},
		{
			name:     "status without repo_path",
			uri:      "grit://status",
			wantHost: "status",
			wantPath: "",
			wantRepo: "",
		},
		{
			name:     "commits with ref",
			uri:      "grit://commits/HEAD~3?repo_path=/tmp/repo",
			wantHost: "commits",
			wantPath: "/HEAD~3",
			wantRepo: "/tmp/repo",
		},
		{
			name:     "blame with path",
			uri:      "grit://blame/src/main.go?repo_path=/tmp/repo&ref=HEAD",
			wantHost: "blame",
			wantPath: "/src/main.go",
			wantRepo: "/tmp/repo",
		},
		{
			name:     "log with query params",
			uri:      "grit://log?repo_path=/tmp/repo&ref=main&max_count=5",
			wantHost: "log",
			wantPath: "",
			wantRepo: "/tmp/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.uri)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.uri, err)
			}
			if parsed.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", parsed.Host, tt.wantHost)
			}
			gotPath := parsed.Path
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			gotRepo := parsed.Query().Get("repo_path")
			if gotRepo != tt.wantRepo {
				t.Errorf("repo_path = %q, want %q", gotRepo, tt.wantRepo)
			}
		})
	}
}
```

**Step 2: Run test to verify it passes** (this is a URL parsing test, it should pass immediately — it validates our URI scheme works with Go's url.Parse)

Run: `nix develop .#go -c go test -run TestParseResourceURI ./packages/grit/internal/tools/`
Expected: PASS

**Step 3: Write the resource provider**

Create `packages/grit/internal/tools/resources.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/friedenberg/grit/internal/git"
)

type resourceProvider struct {
	registry *mcpserver.ResourceRegistry
	cwd      string
}

func NewResourceProvider() (*resourceProvider, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	registry := mcpserver.NewResourceRegistry()

	p := &resourceProvider{
		registry: registry,
		cwd:      cwd,
	}

	p.registerResources()

	return p, nil
}

func (p *resourceProvider) registerResources() {
	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "grit://status",
			Name:        "Git Status",
			Description: "Working tree status with branch info and file changes",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "grit://branches",
			Name:        "Git Branches",
			Description: "List local and remote branches with tracking info",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "grit://remotes",
			Name:        "Git Remotes",
			Description: "List remotes with their URLs",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "grit://tags",
			Name:        "Git Tags",
			Description: "List tags with metadata",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "grit://log",
			Name:        "Git Log",
			Description: "Commit history. Optional query params: repo_path, ref, max_count, paths (comma-separated), all (bool)",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "grit://commits/{ref}",
			Name:        "Git Show",
			Description: "Show a commit, tag, or other git object. Optional query params: repo_path, context_lines, max_patch_lines",
			MimeType:    "application/json",
		},
		nil,
	)

	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "grit://blame/{path}",
			Name:        "Git Blame",
			Description: "Line-by-line authorship of a file. Optional query params: repo_path, ref, line_range (e.g. '10,20')",
			MimeType:    "application/json",
		},
		nil,
	)
}

func (p *resourceProvider) ListResources(ctx context.Context) ([]protocol.Resource, error) {
	return p.registry.ListResources(ctx)
}

func (p *resourceProvider) ListResourceTemplates(ctx context.Context) ([]protocol.ResourceTemplate, error) {
	return p.registry.ListResourceTemplates(ctx)
}

func (p *resourceProvider) ReadResource(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid resource URI: %w", err)
	}

	q := parsed.Query()
	repoPath := q.Get("repo_path")
	if repoPath == "" {
		repoPath = p.cwd
	}

	switch parsed.Host {
	case "status":
		return p.readStatus(ctx, uri, repoPath)
	case "branches":
		return p.readBranches(ctx, uri, repoPath, q)
	case "remotes":
		return p.readRemotes(ctx, uri, repoPath)
	case "tags":
		return p.readTags(ctx, uri, repoPath, q)
	case "log":
		return p.readLog(ctx, uri, repoPath, q)
	case "commits":
		ref := strings.TrimPrefix(parsed.Path, "/")
		if ref == "" {
			return nil, fmt.Errorf("missing ref in URI path: %s", uri)
		}
		return p.readShow(ctx, uri, repoPath, ref, q)
	case "blame":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			return nil, fmt.Errorf("missing path in URI path: %s", uri)
		}
		return p.readBlame(ctx, uri, repoPath, path, q)
	default:
		return nil, fmt.Errorf("unknown resource: %s", parsed.Host)
	}
}

func (p *resourceProvider) jsonResult(uri, data string) *protocol.ResourceReadResult {
	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{{
			URI:      uri,
			MimeType: "application/json",
			Text:     data,
		}},
	}
}

func (p *resourceProvider) readStatus(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	out, err := git.Run(ctx, repoPath, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	result := git.ParseStatus(out)

	state, err := git.DetectInProgressState(ctx, repoPath)
	if err == nil && state != nil {
		result.State = state
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readBranches(ctx context.Context, uri, repoPath string, q url.Values) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{
		"branch",
		"--format=%(HEAD)\x1f%(refname:short)\x1f%(objectname:short)\x1f%(subject)\x1f%(upstream:short)\x1f%(upstream:track)\x1e",
	}

	if q.Get("all") == "true" {
		gitArgs = append(gitArgs, "-a")
	} else if q.Get("remote") == "true" {
		gitArgs = append(gitArgs, "-r")
	}

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}

	branches := git.ParseBranchList(out)

	data, err := json.MarshalIndent(branches, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readRemotes(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	out, err := git.Run(ctx, repoPath, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("git remote: %w", err)
	}

	remotes := git.ParseRemoteList(out)

	data, err := json.MarshalIndent(remotes, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readTags(ctx context.Context, uri, repoPath string, q url.Values) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{
		"tag",
		"--list",
		fmt.Sprintf("--format=%s", git.TagListFormat),
	}

	if sort := q.Get("sort"); sort != "" {
		gitArgs = append(gitArgs, fmt.Sprintf("--sort=%s", sort))
	}

	if pattern := q.Get("pattern"); pattern != "" {
		gitArgs = append(gitArgs, pattern)
	}

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}

	tags := git.ParseTagList(out)

	data, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readLog(ctx context.Context, uri, repoPath string, q url.Values) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{"log"}

	maxCount := 10
	if mc := q.Get("max_count"); mc != "" {
		if v, err := strconv.Atoi(mc); err == nil && v > 0 {
			maxCount = v
		}
	}
	gitArgs = append(gitArgs, fmt.Sprintf("--max-count=%d", maxCount))
	gitArgs = append(gitArgs, fmt.Sprintf("--format=%s", git.LogFormat))

	if q.Get("all") == "true" {
		gitArgs = append(gitArgs, "--all")
	}

	if ref := q.Get("ref"); ref != "" {
		gitArgs = append(gitArgs, ref)
	}

	if paths := q.Get("paths"); paths != "" {
		gitArgs = append(gitArgs, "--")
		gitArgs = append(gitArgs, strings.Split(paths, ",")...)
	}

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	entries := git.ParseLog(out)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readShow(ctx context.Context, uri, repoPath, ref string, q url.Values) (*protocol.ResourceReadResult, error) {
	var contextLines *int
	if cl := q.Get("context_lines"); cl != "" {
		if v, err := strconv.Atoi(cl); err == nil {
			contextLines = &v
		}
	}

	var maxPatchLines int
	if mpl := q.Get("max_patch_lines"); mpl != "" {
		if v, err := strconv.Atoi(mpl); err == nil {
			maxPatchLines = v
		}
	}

	metadataOut, err := git.Run(ctx, repoPath, "show", "--no-patch", fmt.Sprintf("--format=%s", git.ShowFormat), ref)
	if err != nil {
		out, fallbackErr := git.Run(ctx, repoPath, "show", ref)
		if fallbackErr != nil {
			return nil, fmt.Errorf("git show: %w", err)
		}
		return &protocol.ResourceReadResult{
			Contents: []protocol.ResourceContent{{
				URI:      uri,
				MimeType: "text/plain",
				Text:     out,
			}},
		}, nil
	}

	numstatOut, err := git.Run(ctx, repoPath, "show", "--numstat", "--format=", ref)
	if err != nil {
		numstatOut = ""
	}

	diffArgs := []string{"diff"}
	if contextLines != nil {
		diffArgs = append(diffArgs, fmt.Sprintf("--unified=%d", *contextLines))
	}
	diffArgs = append(diffArgs, ref+"~1", ref)

	patchOut, err := git.Run(ctx, repoPath, diffArgs...)
	if err != nil {
		patchOut = ""
	}

	result := git.ParseShow(metadataOut, numstatOut, patchOut)

	patch, truncated, truncatedAt := git.TruncatePatch(result.Patch, maxPatchLines)
	result.Patch = patch
	result.Truncated = truncated
	result.TruncatedAtLine = truncatedAt

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}

func (p *resourceProvider) readBlame(ctx context.Context, uri, repoPath, path string, q url.Values) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{"blame", "--porcelain"}

	if lineRange := q.Get("line_range"); lineRange != "" {
		gitArgs = append(gitArgs, fmt.Sprintf("-L%s", lineRange))
	}

	if ref := q.Get("ref"); ref != "" {
		gitArgs = append(gitArgs, ref)
	}

	gitArgs = append(gitArgs, "--", path)

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git blame: %w", err)
	}

	lines := git.ParseBlame(out)

	data, err := json.MarshalIndent(lines, "", "  ")
	if err != nil {
		return nil, err
	}

	return p.jsonResult(uri, string(data)), nil
}
```

**Step 4: Run all grit tests to verify nothing is broken**

Run: `nix develop .#go -c go test ./packages/grit/...`
Expected: PASS

**Step 5: Commit**

```
feat(grit): add resource provider for MCP resources

Register 7 git operations as MCP resources with grit:// URI scheme.
Static resources: status, branches, remotes, tags.
Templates: log, commits/{ref}, blame/{path}.
All default repo_path to cwd when omitted.
```

Files: `packages/grit/internal/tools/resources.go`, `packages/grit/internal/tools/resources_test.go`

---

### Task 2: Add resource-templates and resource-read tools

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/grit/internal/tools/registry.go`

**Step 1: Write resource-templates and resource-read tool registrations**

Add to `registry.go` — a new `registerResourceCommands` function and call it from `RegisterAll`. Also add a `ResourceProvider` return from `RegisterAll` (or a separate constructor). The tools follow the exact same pattern as lux's `server.go:116-192`.

Update `RegisterAll` to accept and wire in the resource provider:

```go
package tools

import (
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
)

func RegisterAll() (*command.App, *resourceProvider) {
	app := command.NewApp("grit", "MCP server exposing git operations")
	app.Version = "0.1.0"

	registerStatusCommands(app)
	registerLogCommands(app)
	registerStagingCommands(app)
	registerCommitCommands(app)
	registerTryCommitCommands(app)
	registerBranchCommands(app)
	registerRemoteCommands(app)
	registerRevParseCommands(app)
	registerRebaseCommands(app)
	registerInteractiveRebaseCommands(app)
	registerHardResetCommands(app)
	registerTagCommands(app)

	resProvider, err := NewResourceProvider()
	if err != nil {
		// Fall back to nil — server will run without resources
		return app, nil
	}

	registerResourceToolCommands(app, resProvider)

	return app, resProvider
}
```

Add `registerResourceToolCommands` in `resources.go`:

```go
func registerResourceToolCommands(app *command.App, resProvider *resourceProvider) {
	readOnly := true
	notDestructive := false
	idempotent := true

	app.AddCommand(&command.Command{
		Name: "resource-templates",
		Description: command.Description{
			Short: "List available grit resource templates. Call this first to discover what git resources are available, then use resource-read to access them.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			templates, err := resProvider.ListResourceTemplates(ctx)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			resources, err := resProvider.ListResources(ctx)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			var sb strings.Builder
			sb.WriteString("Resource templates (fill in {placeholders} and pass to resource-read):\n\n")
			for _, t := range templates {
				fmt.Fprintf(&sb, "- %s: %s\n  %s\n", t.Name, t.URITemplate, t.Description)
			}

			if len(resources) > 0 {
				sb.WriteString("\nStatic resources (pass URI directly to resource-read):\n\n")
				for _, r := range resources {
					fmt.Fprintf(&sb, "- %s: %s\n  %s\n", r.Name, r.URI, r.Description)
				}
			}

			sb.WriteString("\nAll resources accept an optional repo_path query parameter (defaults to current working directory).")

			return command.TextResult(sb.String()), nil
		},
	})

	app.AddCommand(&command.Command{
		Name: "resource-read",
		Description: command.Description{
			Short: "Read a grit resource by URI. This tool exists because subagents cannot access MCP resources directly. Call resource-templates to discover available URIs.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "Resource URI (e.g., grit://status, grit://commits/HEAD~3?repo_path=/path/to/repo)", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			result, err := resProvider.ReadResource(ctx, a.URI)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			var sb strings.Builder
			for i, c := range result.Contents {
				if i > 0 {
					sb.WriteString("\n---\n")
				}
				sb.WriteString(c.Text)
			}

			return command.TextResult(sb.String()), nil
		},
	})
}
```

**Step 2: Run tests to verify compilation**

Run: `nix develop .#go -c go test ./packages/grit/...`
Expected: PASS

**Step 3: Commit**

```
feat(grit): add resource-templates and resource-read tools

Tool wrappers for subagent access to grit MCP resources.
Follows the same pattern as lux's resource tools.
```

Files: `packages/grit/internal/tools/registry.go`, `packages/grit/internal/tools/resources.go`

---

### Task 3: Remove 7 read-only tool registrations

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/grit/internal/tools/status.go` — remove `registerStatusCommands` status tool (keep diff tool and handler functions)
- Modify: `packages/grit/internal/tools/log.go` — remove `registerLogCommands` entirely (all 3 tools become resources), keep handler functions
- Modify: `packages/grit/internal/tools/branch.go` — remove `branch_list` tool from `registerBranchCommands` (keep branch_create, checkout)
- Modify: `packages/grit/internal/tools/remote.go` — remove `remote_list` tool from `registerRemoteCommands` (keep fetch, pull, push)
- Modify: `packages/grit/internal/tools/tag.go` — remove `tag_list` tool from `registerTagCommands` (keep tag_verify)
- Modify: `packages/grit/internal/tools/registry.go` — remove `registerLogCommands(app)` call

**Step 1: Remove tool registrations**

In `status.go`, remove the `status` tool from `registerStatusCommands`. The function still registers `diff`. Keep `handleGitStatus` — it's no longer called from a tool but may be useful for reference. Actually, the resource provider has its own implementation that calls `git.Run` directly, so `handleGitStatus` is now dead code. Remove the tool registration only; the handler can be cleaned up later.

In `log.go`, remove the entire `registerLogCommands` function (all 3 tools — log, show, blame — are now resources). Keep the handler functions for now (they're dead code but removing them is a separate cleanup).

In `branch.go`, remove the `branch_list` tool from `registerBranchCommands`. Keep `branch_create` and `checkout`.

In `remote.go`, remove the `remote_list` tool from `registerRemoteCommands`. Keep `fetch`, `pull`, `push`.

In `tag.go`, remove `tag_list` from `registerTagCommands`. Keep `tag_verify`.

In `registry.go`, remove the `registerLogCommands(app)` call since that function no longer exists.

**Step 2: Run tests**

Run: `nix develop .#go -c go test ./packages/grit/...`
Expected: PASS

**Step 3: Commit**

```
refactor(grit): remove 7 read-only tool registrations

These operations are now exposed as MCP resources:
status, log, show, blame, branch_list, remote_list, tag_list.

Remaining tools: diff, git_rev_parse, tag_verify, add, reset,
commit, try_commit, branch_create, checkout, fetch, pull, push,
rebase, interactive_rebase_plan, interactive_rebase_execute,
hard_reset, resource-templates, resource-read.
```

Files: `status.go`, `log.go`, `branch.go`, `remote.go`, `tag.go`, `registry.go`

---

### Task 4: Wire resource provider into MCP server

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/grit/cmd/grit/main.go`

**Step 1: Update main.go to pass resource provider to server**

Update the `main()` function:

```go
app, resProvider := tools.RegisterAll()
```

Update `server.New` call to include resources:

```go
opts := server.Options{
    ServerName:    app.Name,
    ServerVersion: app.Version,
    Instructions:  "Git MCP server exposing repository operations. Read-only operations (status, log, show, blame, branches, remotes, tags) are available as MCP resources. Mutation operations (commit, push, rebase, etc.) remain as tools.",
    Tools:         registry,
}

if resProvider != nil {
    opts.Resources = resProvider
}

srv, err := server.New(t, opts)
```

**Step 2: Build to verify**

Run: `nix build .#grit`
Expected: builds successfully

**Step 3: Commit**

```
feat(grit): wire resource provider into MCP server

Resources are now advertised in server capabilities. Claude Code
will auto-approve resource reads without permission dialogs.
```

Files: `packages/grit/cmd/grit/main.go`

---

### Task 5: Clean up dead handler code

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/grit/internal/tools/status.go` — remove `handleGitStatus`
- Modify: `packages/grit/internal/tools/log.go` — remove `registerLogCommands`, `handleGitLog`, `handleGitShow`, `handleGitBlame`; remove unused imports
- Modify: `packages/grit/internal/tools/branch.go` — remove `handleGitBranchList`
- Modify: `packages/grit/internal/tools/remote.go` — remove `handleGitRemoteList`
- Modify: `packages/grit/internal/tools/tag.go` — remove `handleGitTagList`

**Step 1: Remove dead handler functions**

Remove all handler functions that are no longer called by any tool registration. The resource provider has its own implementations.

**Step 2: Run tests and build**

Run: `nix develop .#go -c go test ./packages/grit/...`
Run: `nix build .#grit`
Expected: both pass

**Step 3: Commit**

```
refactor(grit): remove dead handler functions for resource-migrated tools

Handler functions for status, log, show, blame, branch_list,
remote_list, and tag_list are replaced by resource provider methods.
```

---

### Task 6: Update server instructions and verify end-to-end

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/grit/CLAUDE.md` — update architecture docs

**Step 1: Full build and test**

Run: `nix build .#grit`
Run: `nix develop .#go -c go test ./packages/grit/...`
Expected: both pass

**Step 2: Verify generate-plugin output**

Run: `nix build .#grit && ./result/bin/grit generate-plugin /tmp/grit-plugin`

Verify:
- `resource-templates` and `resource-read` appear in plugin.json tools
- The 7 removed tools do NOT appear
- Remaining 14 tools still appear

**Step 3: Update CLAUDE.md**

Update `packages/grit/CLAUDE.md` to document:
- Resources section listing the 7 grit:// resources
- Updated tool list (14 tools + 2 resource wrappers)
- Note about resource-templates/resource-read for subagent access

**Step 4: Commit**

```
docs(grit): update CLAUDE.md for MCP resource migration

Document grit:// resources, updated tool surface, and
resource-templates/resource-read pattern for subagent access.
```

Files: `packages/grit/CLAUDE.md`
