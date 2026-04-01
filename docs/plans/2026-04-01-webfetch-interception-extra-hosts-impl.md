# WebFetch Interception: Extra Hosts Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add specific WebFetch deny messages for api.github.com, raw.githubusercontent.com, and gist.github.com, plus new compare and gist resources.

**Architecture:** Three new match functions in webfetch.go dispatch by host. Two new resources in resources.go (gist, compare). Host-based dispatch replaces the single matchGitHubURL call.

**Tech Stack:** Go, get-hubbed MCP server, gh CLI

**Rollback:** Purely additive --- revert commits and reinstall marketplace.

---

### Task 1: Update github.com compare mapping from tool to resource

The existing `matchGitHubURL` compare case returns a tool reference. Change it to return a resource URI instead.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go:114-117`
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go:183-199`

**Step 1: Update the compare test to expect a resource URI**

In `webfetch_test.go`, change `TestWebFetchHookCompareURL` (line 183) to expect the new resource URI instead of `content-compare`:

```go
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
	if !strings.Contains(out.String(), "get-hubbed://compare?repo=owner/repo&base=main&head=feature") {
		t.Errorf("expected compare resource URI in output, got %q", out.String())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test-get-hubbed`
Expected: FAIL on `TestWebFetchHookCompareURL` --- output contains `content-compare` not `get-hubbed://compare`

**Step 3: Update matchGitHubURL compare case**

In `webfetch.go`, change the `compare` case (line 114-117) from:

```go
	case "compare":
		if len(segments) >= 4 {
			return fmt.Sprintf("content-compare tool with repo=%s, base and head from %s", repoSlug, segments[3]), true
		}
```

To:

```go
	case "compare":
		if len(segments) >= 4 {
			spec := segments[3]
			if parts := strings.SplitN(spec, "...", 2); len(parts) == 2 {
				return fmt.Sprintf("get-hubbed://compare?repo=%s&base=%s&head=%s", repoSlug, parts[0], parts[1]), false
			}
			return fmt.Sprintf("get-hubbed://compare?repo=%s", repoSlug), false
		}
```

**Step 4: Run test to verify it passes**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): change compare URL mapping from tool to resource

The github.com/compare URL mapping now returns a get-hubbed://compare
resource URI instead of a content-compare tool reference. Agents
naturally guess get-hubbed://compare, so matching that reduces
incorrect tool usage.

Part of #69.
```

---

### Task 2: Add host-based dispatch in HandleWebFetchHook

Replace the single `matchGitHubURL` call with a host-based switch. The new match functions don't exist yet, so the three new hosts will still fall through to catch-all --- but the dispatch structure is in place.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go:152-153`

**Step 1: Write a test that verifies dispatch still works for existing domains**

No new test needed --- the existing `TestWebFetchHookAllMappings` and `TestWebFetchHookAllGitHubDomains` cover this. Run them to establish a baseline.

Run: `just test-get-hubbed`
Expected: PASS (all existing tests)

**Step 2: Replace the single matchGitHubURL call with host dispatch**

In `webfetch.go`, replace lines 152-153:

```go
	// Try specific mapping
	resourceURI, isTool := matchGitHubURL(rawURL)
```

With:

```go
	// Try host-specific mapping
	var resourceURI string
	var isTool bool

	switch parsed.Host {
	case "github.com", "www.github.com":
		resourceURI, isTool = matchGitHubURL(rawURL)
	case "api.github.com":
		resourceURI, isTool = matchAPIGitHubURL(parsed)
	case "raw.githubusercontent.com":
		resourceURI, isTool = matchRawGitHubURL(parsed)
	case "gist.github.com":
		resourceURI, isTool = matchGistGitHubURL(parsed)
	}
```

Also add stub functions at the end of the file (before `writeDenyJSON`):

```go
// matchAPIGitHubURL matches api.github.com REST API paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchAPIGitHubURL(parsed *url.URL) (string, bool) {
	return "", false
}

// matchRawGitHubURL matches raw.githubusercontent.com paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchRawGitHubURL(parsed *url.URL) (string, bool) {
	return "", false
}

// matchGistGitHubURL matches gist.github.com paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchGistGitHubURL(parsed *url.URL) (string, bool) {
	return "", false
}
```

Note: these new functions take `*url.URL` not `string`, since `HandleWebFetchHook` already parsed the URL. `matchGitHubURL` keeps its `string` signature since it re-parses internally (changing it would affect its host guard).

**Step 3: Run tests to verify nothing broke**

Run: `just test-get-hubbed`
Expected: PASS (all existing tests still pass, new hosts still hit catch-all)

**Step 4: Commit**

```
refactor(get-hubbed): add host-based dispatch in HandleWebFetchHook

Replace single matchGitHubURL call with a switch on parsed.Host.
Stub functions for api.github.com, raw.githubusercontent.com, and
gist.github.com return no match (catch-all behavior unchanged).

Part of #69, #70, #71.
```

---

### Task 3: Implement matchRawGitHubURL (#70)

Simplest of the three --- single mapping from `/{owner}/{repo}/{ref}/{path...}` to `get-hubbed://contents`.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go` (fill in `matchRawGitHubURL`)
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go`

**Step 1: Write the failing test**

Add to `webfetch_test.go`:

```go
func TestWebFetchHookRawGitHubURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://raw.githubusercontent.com/owner/repo/main/README.md",
			"get-hubbed://contents?path=README.md&repo=owner/repo&ref=main",
		},
		{
			"https://raw.githubusercontent.com/owner/repo/v1.0.0/src/lib/foo.go",
			"get-hubbed://contents?path=src/lib/foo.go&repo=owner/repo&ref=v1.0.0",
		},
		{
			"https://raw.githubusercontent.com/owner/repo/abc1234/file.txt",
			"get-hubbed://contents?path=file.txt&repo=owner/repo&ref=abc1234",
		},
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

func TestWebFetchHookRawGitHubURLTooFewSegments(t *testing.T) {
	// Only 3 segments (owner/repo/ref, no path) --- should catch-all
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://raw.githubusercontent.com/owner/repo/main",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle raw.githubusercontent.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message for too-few segments, got %q", out.String())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test-get-hubbed`
Expected: FAIL on `TestWebFetchHookRawGitHubURLMappings` --- output contains catch-all message, not `get-hubbed://contents`

**Step 3: Implement matchRawGitHubURL**

In `webfetch.go`, replace the stub with:

```go
func matchRawGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Need at least /{owner}/{repo}/{ref}/{path...}
	if len(segments) < 4 || segments[0] == "" {
		return "", false
	}

	owner := segments[0]
	repo := segments[1]
	ref := segments[2]
	filePath := strings.Join(segments[3:], "/")

	return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s/%s&ref=%s", filePath, owner, repo, ref), false
}
```

**Step 4: Run test to verify it passes**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): intercept raw.githubusercontent.com URLs in WebFetch hook

Map raw.githubusercontent.com/{owner}/{repo}/{ref}/{path} to
get-hubbed://contents?path={path}&repo={owner}/{repo}&ref={ref}.

Fixes #70.
```

---

### Task 4: Implement matchAPIGitHubURL (#69)

Most complex of the three --- 10 API path patterns.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go` (fill in `matchAPIGitHubURL`)
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go`

**Step 1: Write the failing test**

Add to `webfetch_test.go`:

```go
func TestWebFetchHookAPIGitHubURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://api.github.com/repos/owner/repo",
			"get-hubbed://repo",
		},
		{
			"https://api.github.com/repos/owner/repo/issues",
			"get-hubbed://issues?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/issues/42",
			"get-hubbed://issues?number=42&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/pulls",
			"get-hubbed://pulls?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/pulls/7",
			"get-hubbed://pulls?number=7&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/contents/src/foo.go",
			"get-hubbed://contents?path=src/foo.go&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/git/trees/main",
			"get-hubbed://tree?repo=owner/repo&ref=main",
		},
		{
			"https://api.github.com/repos/owner/repo/actions/runs",
			"get-hubbed://runs?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/actions/runs/12345",
			"get-hubbed://runs?run_id=12345&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/compare/main...feature",
			"get-hubbed://compare?repo=owner/repo&base=main&head=feature",
		},
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

func TestWebFetchHookAPIGitHubURLCatchAll(t *testing.T) {
	// Unrecognized API path should catch-all
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://api.github.com/repos/owner/repo/stargazers",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle api.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}

func TestWebFetchHookAPIGitHubURLNonRepos(t *testing.T) {
	// Non-/repos/ API paths should catch-all
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://api.github.com/users/owner",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle api.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test-get-hubbed`
Expected: FAIL on `TestWebFetchHookAPIGitHubURLMappings` --- output contains catch-all, not specific URIs

**Step 3: Implement matchAPIGitHubURL**

In `webfetch.go`, replace the stub with:

```go
func matchAPIGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// API paths start with /repos/{owner}/{repo}/...
	if len(segments) < 3 || segments[0] != "repos" || segments[1] == "" {
		return "", false
	}

	owner := segments[1]
	repo := segments[2]
	repoSlug := owner + "/" + repo

	// Exact: /repos/{owner}/{repo}
	if len(segments) == 3 {
		return "get-hubbed://repo", false
	}

	section := segments[3]
	switch section {
	case "issues":
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://issues?repo=%s", repoSlug), false
		}
		if len(segments) == 5 {
			return fmt.Sprintf("get-hubbed://issues?number=%s&repo=%s", segments[4], repoSlug), false
		}

	case "pulls":
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://pulls?repo=%s", repoSlug), false
		}
		if len(segments) == 5 {
			return fmt.Sprintf("get-hubbed://pulls?number=%s&repo=%s", segments[4], repoSlug), false
		}

	case "contents":
		if len(segments) >= 5 {
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s", filePath, repoSlug), false
		}

	case "git":
		if len(segments) >= 6 && segments[4] == "trees" {
			ref := segments[5]
			return fmt.Sprintf("get-hubbed://tree?repo=%s&ref=%s", repoSlug, ref), false
		}

	case "actions":
		if len(segments) == 5 && segments[4] == "runs" {
			return fmt.Sprintf("get-hubbed://runs?repo=%s", repoSlug), false
		}
		if len(segments) == 6 && segments[4] == "runs" {
			return fmt.Sprintf("get-hubbed://runs?run_id=%s&repo=%s", segments[5], repoSlug), false
		}

	case "compare":
		if len(segments) >= 5 {
			spec := segments[4]
			if parts := strings.SplitN(spec, "...", 2); len(parts) == 2 {
				return fmt.Sprintf("get-hubbed://compare?repo=%s&base=%s&head=%s", repoSlug, parts[0], parts[1]), false
			}
			return fmt.Sprintf("get-hubbed://compare?repo=%s", repoSlug), false
		}
	}

	return "", false
}
```

**Step 4: Run test to verify it passes**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): intercept api.github.com URLs in WebFetch hook

Map api.github.com/repos/{owner}/{repo}/... paths to get-hubbed
resource URIs: issues, pulls, contents, tree, runs, and compare.

Fixes #69.
```

---

### Task 5: Implement matchGistGitHubURL (#71) --- hook side only

Add the match function for gist URLs. The gist resource in resources.go comes in Task 7.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go` (fill in `matchGistGitHubURL`)
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go`

**Step 1: Write the failing test**

Add to `webfetch_test.go`:

```go
func TestWebFetchHookGistURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://gist.github.com/owner/abc123",
			"get-hubbed://gist?id=abc123",
		},
		{
			"https://gist.github.com/owner/deadbeef1234567890",
			"get-hubbed://gist?id=deadbeef1234567890",
		},
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

func TestWebFetchHookGistURLTooFewSegments(t *testing.T) {
	// Just /{owner} with no gist_id --- should catch-all
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://gist.github.com/owner",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle gist.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test-get-hubbed`
Expected: FAIL on `TestWebFetchHookGistURLMappings` --- output contains catch-all, not `get-hubbed://gist`

**Step 3: Implement matchGistGitHubURL**

In `webfetch.go`, replace the stub with:

```go
func matchGistGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Need at least /{owner}/{gist_id}
	if len(segments) < 2 || segments[0] == "" {
		return "", false
	}

	gistID := segments[1]
	return fmt.Sprintf("get-hubbed://gist?id=%s", gistID), false
}
```

**Step 4: Run test to verify it passes**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): intercept gist.github.com URLs in WebFetch hook

Map gist.github.com/{owner}/{gist_id} to get-hubbed://gist?id={gist_id}.
The gist resource handler is added in a follow-up commit.

Part of #71.
```

---

### Task 6: Update catch-all deny to include new resources

Add `get-hubbed://gist` and `get-hubbed://compare` to the catch-all deny message so agents know about them.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch.go:184-191`
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go`

**Step 1: Write the failing test**

Add to `webfetch_test.go`:

```go
func TestWebFetchHookCatchAllIncludesNewResources(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/settings",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	HandleWebFetchHook(input, &out)

	output := out.String()
	for _, resource := range []string{"get-hubbed://gist", "get-hubbed://compare"} {
		if !strings.Contains(output, resource) {
			t.Errorf("catch-all deny message should mention %s, got %q", resource, output)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test-get-hubbed`
Expected: FAIL --- catch-all doesn't mention `get-hubbed://gist` or `get-hubbed://compare`

**Step 3: Update writeCatchAllDeny**

In `webfetch.go`, update the `writeCatchAllDeny` function:

```go
func writeCatchAllDeny(w io.Writer) error {
	reason := "DENIED: GitHub URLs are served by get-hubbed. Do not use WebFetch for GitHub.\n" +
		"Use get-hubbed for ALL GitHub interactions \u2014 do not use WebFetch or Bash with gh/curl for GitHub.\n\n" +
		"Resources (read-only): get-hubbed://repo, get-hubbed://issues, get-hubbed://pulls, " +
		"get-hubbed://contents, get-hubbed://tree, get-hubbed://blame, get-hubbed://commits, " +
		"get-hubbed://runs, get-hubbed://compare, get-hubbed://gist\n" +
		"Tools (mutations): issue-create, issue-close, issue-comment, pr-create, " +
		"content-search, content-compare, api-get, graphql-query, graphql-mutation"
	return writeDenyJSON(w, reason)
}
```

**Step 4: Run test to verify it passes**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): add gist and compare to catch-all deny message

Agents hitting unrecognized GitHub URLs now see get-hubbed://compare
and get-hubbed://gist in the list of available resources.
```

---

### Task 7: Add get-hubbed://compare resource

Add a `compare` resource to `resources.go` backed by `gh api repos/{repo}/compare/{base}...{head}`. Uses the same jq filter as the existing `content-compare` tool.

**Files:**
- Modify: `packages/get-hubbed/internal/tools/resources.go`

**Step 1: Register the compare resource template**

In `resources.go`, add after the runs/log template registration (after line 155), before `return p, nil`:

```go
	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://compare?repo={repo}&base={base}&head={head}&per_page={per_page}&page={page}",
		Name:        "Compare Refs",
		Description: "Compare two refs (branches, tags, or commits) showing commits and file changes. Required: base, head. Optional: repo (defaults to current), per_page, page",
		MimeType:    "application/json",
	}, nil)
```

**Step 2: Add the compare case to ReadResource**

In `resources.go`, add a new case before the `default` in the `ReadResource` switch (before line 276):

```go
	case "compare":
		return p.readCompare(ctx, uri, parsed.Query())
```

**Step 3: Add readCompare method**

Add after `readRunLog` (after line 959):

```go
func (p *resourceProvider) readCompare(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "base", "head", "per_page", "page"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	base := q.Get("base")
	head := q.Get("head")
	if base == "" || head == "" {
		return nil, fmt.Errorf("base and head parameters are required. Use get-hubbed://compare?base={base}&head={head}")
	}

	endpoint := fmt.Sprintf("repos/%s/compare/%s...%s", repo, base, head)

	ghArgs := []string{"api", endpoint, "--method", "GET"}

	if perPage := q.Get("per_page"); perPage != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("per_page=%s", perPage))
	}

	if page := q.Get("page"); page != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("page=%s", page))
	}

	ghArgs = append(ghArgs, "--jq",
		`{status, ahead_by, behind_by, total_commits, commits: [.commits[] | {sha: .sha[:8], message: .commit.message, author: .commit.author.name, date: .commit.author.date}], files: [.files[] | {filename, status, additions, deletions, changes}]}`,
	)

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh api compare: %w", err)
	}

	return textResourceResult(uri, out), nil
}
```

**Step 4: Run tests**

Run: `just test-get-hubbed`
Expected: PASS (no runtime test for this resource since it requires gh CLI, but compilation verifies correctness)

**Step 5: Commit**

```
feat(get-hubbed): add compare resource

Add get-hubbed://compare?base={base}&head={head} resource backed by
gh api repos/{repo}/compare/{base}...{head}. Uses same jq filter as
the existing content-compare tool.
```

---

### Task 8: Add get-hubbed://gist resource

Add a `gist` resource to `resources.go` backed by `gh api /gists/{id}`.

**Files:**
- Modify: `packages/get-hubbed/internal/tools/resources.go`

**Step 1: Register the gist resource template**

In `resources.go`, add after the compare template registration:

```go
	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://gist?id={id}",
		Name:        "Gist",
		Description: "View a gist's metadata and file contents. Required: id",
		MimeType:    "application/json",
	}, nil)
```

**Step 2: Add the gist case to ReadResource**

In `resources.go`, add a new case before the `default` in the `ReadResource` switch:

```go
	case "gist":
		id := strings.TrimPrefix(parsed.Path, "/")
		if id == "" {
			id = parsed.Query().Get("id")
		}
		if id == "" {
			return nil, fmt.Errorf("missing id in gist URI. Use get-hubbed://gist?id={id}")
		}
		return p.readGist(ctx, uri, id, parsed.Query())
```

**Step 3: Add readGist method**

Add after `readCompare`:

```go
func (p *resourceProvider) readGist(ctx context.Context, uri, id string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"id"}); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("gists/%s", id)

	ghArgs := []string{
		"api", endpoint, "--method", "GET",
		"--jq", `{id, description, public, created_at, updated_at, owner: .owner.login, files: [.files | to_entries[] | {filename: .key, language: .value.language, size: .value.size, content: .value.content}]}`,
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh api gists: %w", err)
	}

	return textResourceResult(uri, out), nil
}
```

**Step 4: Run tests**

Run: `just test-get-hubbed`
Expected: PASS

**Step 5: Commit**

```
feat(get-hubbed): add gist resource

Add get-hubbed://gist?id={id} resource backed by gh api gists/{id}.
Returns gist metadata and file contents.

Fixes #71.
```

---

### Task 9: Update TestWebFetchHookAllGitHubDomains to verify specific URIs

The existing test at line 97 only checks that all domains are handled (denied). Now that all three extra hosts return specific URIs, update the test to verify the specific URIs are present, not just that handling occurred.

**Files:**
- Modify: `packages/get-hubbed/internal/hooks/webfetch_test.go:97-122`

**Step 1: Update the test**

Replace `TestWebFetchHookAllGitHubDomains`:

```go
func TestWebFetchHookAllGitHubDomains(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{"https://github.com/owner/repo/settings", "GitHub URLs are served by get-hubbed"},
		{"https://www.github.com/owner/repo/settings", "GitHub URLs are served by get-hubbed"},
		{"https://api.github.com/repos/owner/repo", "get-hubbed://repo"},
		{"https://raw.githubusercontent.com/owner/repo/main/README.md", "get-hubbed://contents?path=README.md&repo=owner/repo&ref=main"},
		{"https://gist.github.com/owner/abc123", "get-hubbed://gist?id=abc123"},
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
				t.Fatalf("expected hook to handle %s", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %q in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}
```

**Step 2: Run tests**

Run: `just test-get-hubbed`
Expected: PASS

**Step 3: Commit**

```
test(get-hubbed): verify specific URIs in all-domains test

Update TestWebFetchHookAllGitHubDomains to check specific resource
URIs for api.github.com, raw.githubusercontent.com, and
gist.github.com instead of just checking that they are handled.
```

---

### Task 10: File follow-up issue to deprecate content-compare tool

**Step 1: Create GitHub issue**

Title: `Deprecate content-compare tool in favor of get-hubbed://compare resource`
Body:
```
The new `get-hubbed://compare` resource provides the same functionality as the
`content-compare` tool. Agents naturally guess `get-hubbed://compare` as the
resource URI, making the resource the preferred interface.

## What to do

1. Add a deprecation notice to the content-compare tool description
2. Update the tool's MapsTools to point agents to the resource instead
3. After a promotion period, consider removing the tool entirely

## Context

- Resource added in the WebFetch interception work (#69, #70, #71)
- Both use the same `gh api repos/{repo}/compare/{base}...{head}` endpoint
- Both use the same jq filter for output
```

Labels: `enhancement`

**Step 2: No commit needed** (issue is external)

---

### Task 11: Run full test suite and nix build

**Step 1: Run full get-hubbed tests**

Run: `just test-get-hubbed`
Expected: PASS (all tests)

**Step 2: Run nix build**

Run: `nix build .#get-hubbed`
Expected: Build succeeds

**Step 3: No commit** (verification only)
