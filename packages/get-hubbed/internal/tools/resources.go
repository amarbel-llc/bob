package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

// validateQueryParams checks that all query parameters are in the allowed set.
// Returns an error listing the unknown params and the valid ones.
func validateQueryParams(q url.Values, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}

	var unknown []string
	for key := range q {
		if _, ok := allowedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}

	if len(unknown) > 0 {
		return fmt.Errorf(
			"unknown query parameter(s): %s; valid parameters: %s",
			strings.Join(unknown, ", "),
			strings.Join(allowed, ", "),
		)
	}

	return nil
}

type resourceProvider struct {
	registry     *mcpserver.ResourceRegistry
	cwd          string
	defaultRepo  string
	resolveOnce  sync.Once
	resolveError error
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

	registry.RegisterResource(protocol.Resource{
		URI:         "get-hubbed://repo",
		Name:        "Repository",
		Description: "View current repository details (name, description, stars, forks, visibility)",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://repos?owner={owner}&limit={limit}",
		Name:        "Repository List",
		Description: "List repositories for an owner. Required: owner. Optional: limit",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://issues?repo={repo}&state={state}&limit={limit}&labels={labels}",
		Name:        "Issue List",
		Description: "List issues in a repository. Optional: repo (defaults to current), state (open/closed/all), limit, labels (comma-separated)",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://issues?number={number}&repo={repo}",
		Name:        "Issue Detail",
		Description: "View issue details. Required: number. Optional: repo (defaults to current)",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://pulls?repo={repo}&state={state}&limit={limit}",
		Name:        "Pull Request List",
		Description: "List pull requests in a repository. Optional: repo (defaults to current), state (open/closed/merged/all), limit",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://pulls?number={number}&repo={repo}",
		Name:        "Pull Request Detail",
		Description: "View pull request details. Required: number. Optional: repo (defaults to current)",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://contents?path={path}&repo={repo}&ref={ref}&line_offset={line_offset}&line_limit={line_limit}",
		Name:        "File Contents",
		Description: "Read file contents from a repository. Required: path. Optional: repo, ref, line_offset, line_limit. Tip: use get-hubbed://tree first to verify the path exists before reading contents.",
		MimeType:    "text/plain",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://tree?repo={repo}&path={path}&ref={ref}&recursive={recursive}&limit={limit}&offset={offset}",
		Name:        "Directory Tree",
		Description: "List directory contents of a repository. Optional: repo, path, ref, recursive (bool), limit, offset",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://blame?path={path}&repo={repo}&ref={ref}&start_line={start_line}&end_line={end_line}",
		Name:        "File Blame",
		Description: "Show line-by-line authorship of a file. Required: path. Optional: repo, ref, start_line, end_line",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://commits?path={path}&repo={repo}&ref={ref}&per_page={per_page}&page={page}",
		Name:        "File Commits",
		Description: "List commits for a file or directory. Required: path. Optional: repo, ref, per_page, page",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://runs?repo={repo}&branch={branch}&status={status}&workflow={workflow}&event={event}&commit={commit}&user={user}&limit={limit}",
		Name:        "Workflow Run List",
		Description: "List recent workflow runs. Optional: repo, branch, status, workflow, event, commit, user, limit",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://runs?run_id={run_id}&repo={repo}&attempt={attempt}",
		Name:        "Workflow Run",
		Description: "View a workflow run with jobs and steps. Required: run_id. Optional: repo, attempt",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://runs/log?run_id={run_id}&repo={repo}&job_id={job_id}",
		Name:        "Workflow Run Log",
		Description: "Get logs for failed steps in a workflow run. Required: run_id. Optional: repo, job_id",
		MimeType:    "text/plain",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://compare?repo={repo}&base={base}&head={head}&per_page={per_page}&page={page}",
		Name:        "Compare Refs",
		Description: "Compare two refs (branches, tags, or commits) showing commits and file changes. Required: base, head. Optional: repo (defaults to current), per_page, page",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "get-hubbed://gist?id={id}",
		Name:        "Gist",
		Description: "View a gist's metadata and file contents. Required: id",
		MimeType:    "application/json",
	}, nil)

	return p, nil
}

func (p *resourceProvider) resolveRepo(queryRepo string) (string, error) {
	if queryRepo != "" {
		return queryRepo, nil
	}

	p.resolveOnce.Do(func() {
		out, err := gh.Run(context.Background(),
			"repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner",
		)
		if err != nil {
			p.resolveError = fmt.Errorf("detecting current repo: %w", err)
			return
		}
		p.defaultRepo = strings.TrimSpace(out)
	})

	if p.resolveError != nil {
		return "", p.resolveError
	}

	return p.defaultRepo, nil
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

	switch parsed.Host {
	case "repo":
		if parsed.Path == "" || parsed.Path == "/" {
			return p.readRepo(ctx, uri, parsed.Query())
		}
		return nil, fmt.Errorf("unknown resource: %s", uri)
	case "repos":
		// Handle GitHub API-style paths like get-hubbed://repos/owner/name/issues
		if redir, err := p.redirectReposPath(ctx, uri, parsed); redir != nil || err != nil {
			return redir, err
		}
		return p.readRepos(ctx, uri, parsed.Query())
	case "issues":
		number := strings.TrimPrefix(parsed.Path, "/")
		if number == "" {
			number = parsed.Query().Get("number")
		}
		if number != "" {
			return p.readIssueView(ctx, uri, number, parsed.Query())
		}
		return p.readIssueList(ctx, uri, parsed.Query())
	case "pulls":
		number := strings.TrimPrefix(parsed.Path, "/")
		if number == "" {
			number = parsed.Query().Get("number")
		}
		if number != "" {
			return p.readPRView(ctx, uri, number, parsed.Query())
		}
		return p.readPRList(ctx, uri, parsed.Query())
	case "contents":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			path = parsed.Query().Get("path")
		}
		if path == "" {
			return nil, fmt.Errorf("missing path in contents URI. Use get-hubbed://contents/{path} or get-hubbed://contents?path={path}. List files first with get-hubbed://tree")
		}
		return p.readContents(ctx, uri, path, parsed.Query())
	case "tree":
		return p.readTree(ctx, uri, parsed.Query())
	case "blame":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			path = parsed.Query().Get("path")
		}
		if path == "" {
			return nil, fmt.Errorf("missing path in blame URI. Use get-hubbed://blame/{path} or get-hubbed://blame?path={path}")
		}
		return p.readBlame(ctx, uri, path, parsed.Query())
	case "commits":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			path = parsed.Query().Get("path")
		}
		if path == "" {
			return nil, fmt.Errorf("missing path in commits URI. Use get-hubbed://commits/{path} or get-hubbed://commits?path={path}")
		}
		return p.readCommits(ctx, uri, path, parsed.Query())
	case "runs":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			if runID := parsed.Query().Get("run_id"); runID != "" {
				return p.readRunView(ctx, uri, runID, parsed.Query())
			}
			return p.readRunList(ctx, uri, parsed.Query())
		}
		if path == "log" {
			runID := parsed.Query().Get("run_id")
			if runID == "" {
				return nil, fmt.Errorf("missing run_id for runs/log. Use get-hubbed://runs/log?run_id={run_id}")
			}
			return p.readRunLog(ctx, uri, runID, parsed.Query())
		}
		if strings.HasSuffix(path, "/log") {
			runID := strings.TrimSuffix(path, "/log")
			return p.readRunLog(ctx, uri, runID, parsed.Query())
		}
		return p.readRunView(ctx, uri, path, parsed.Query())
	case "compare":
		return p.readCompare(ctx, uri, parsed.Query())
	case "gist":
		id := strings.TrimPrefix(parsed.Path, "/")
		if id == "" {
			id = parsed.Query().Get("id")
		}
		if id == "" {
			return nil, fmt.Errorf("missing id in gist URI. Use get-hubbed://gist?id={id}")
		}
		return p.readGist(ctx, uri, id, parsed.Query())
	default:
		return nil, fmt.Errorf("unknown resource: %s", uri)
	}
}

// redirectReposPath handles GitHub API-style URIs like
// get-hubbed://repos/owner/name/issues by parsing the owner/name and
// dispatching to the correct resource handler. Returns (nil, nil) if
// the path doesn't match a known pattern, allowing fallthrough to readRepos.
func (p *resourceProvider) redirectReposPath(ctx context.Context, uri string, parsed *url.URL) (*protocol.ResourceReadResult, error) {
	path := strings.TrimPrefix(parsed.Path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return nil, nil
	}

	repo := parts[0] + "/" + parts[1]
	q := parsed.Query()
	q.Set("repo", repo)

	if len(parts) == 2 {
		// get-hubbed://repos/owner/name → repo view
		return p.readRepo(ctx, uri, q)
	}

	rest := parts[2]
	switch {
	case rest == "issues":
		return p.readIssueList(ctx, uri, q)
	case rest == "pulls":
		return p.readPRList(ctx, uri, q)
	case rest == "runs":
		return p.readRunList(ctx, uri, q)
	case strings.HasPrefix(rest, "issues/"):
		number := strings.TrimPrefix(rest, "issues/")
		return p.readIssueView(ctx, uri, number, q)
	case strings.HasPrefix(rest, "pulls/"):
		number := strings.TrimPrefix(rest, "pulls/")
		return p.readPRView(ctx, uri, number, q)
	}

	return nil, nil
}

func (p *resourceProvider) readRepo(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	out, err := gh.Run(ctx,
		"repo", "view", repo,
		"--json", "name,owner,description,url,defaultBranchRef,stargazerCount,forkCount,isPrivate,createdAt,updatedAt",
	)
	if err != nil {
		return nil, fmt.Errorf("gh repo view: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readRepos(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"owner", "limit"}); err != nil {
		return nil, err
	}

	owner := q.Get("owner")
	if owner == "" {
		return nil, fmt.Errorf("owner parameter is required for repos resource")
	}

	ghArgs := []string{
		"repo", "list", owner,
		"--json", "name,owner,description,url,isPrivate,stargazerCount,updatedAt",
	}

	if limit := q.Get("limit"); limit != "" {
		ghArgs = append(ghArgs, "--limit", limit)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh repo list: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readIssueList(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "state", "limit", "labels"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"issue", "list",
		"-R", repo,
		"--json", "number,title,state,author,labels,createdAt,updatedAt,url",
	}

	if state := q.Get("state"); state != "" {
		ghArgs = append(ghArgs, "--state", state)
	}

	if limit := q.Get("limit"); limit != "" {
		ghArgs = append(ghArgs, "--limit", limit)
	}

	if labels := q.Get("labels"); labels != "" {
		for _, label := range strings.Split(labels, ",") {
			ghArgs = append(ghArgs, "--label", strings.TrimSpace(label))
		}
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readIssueView(ctx context.Context, uri, number string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "number"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	out, err := gh.Run(ctx,
		"issue", "view", number,
		"-R", repo,
		"--json", "number,title,state,body,author,labels,assignees,comments,createdAt,updatedAt,url",
	)
	if err != nil {
		return nil, fmt.Errorf("gh issue view: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readPRList(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "state", "limit"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"pr", "list",
		"-R", repo,
		"--json", "number,title,state,author,baseRefName,headRefName,createdAt,updatedAt,url",
	}

	if state := q.Get("state"); state != "" {
		ghArgs = append(ghArgs, "--state", state)
	}

	if limit := q.Get("limit"); limit != "" {
		ghArgs = append(ghArgs, "--limit", limit)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readPRView(ctx context.Context, uri, number string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "number"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	out, err := gh.Run(ctx,
		"pr", "view", number,
		"-R", repo,
		"--json", "number,title,state,body,author,baseRefName,headRefName,labels,reviewDecision,commits,comments,createdAt,updatedAt,url",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr view: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readContents(ctx context.Context, uri, path string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "ref", "path", "line_offset", "line_limit"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"api",
		fmt.Sprintf("repos/%s/contents/%s", repo, path),
		"--method", "GET",
	}

	if ref := q.Get("ref"); ref != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("ref=%s", ref))
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "404") || strings.Contains(errMsg, "Not Found") {
			hint := fmt.Sprintf("404 Not Found: %s at path %q", repo, path)
			if ref := q.Get("ref"); ref != "" {
				hint += fmt.Sprintf(
					" (ref %q). The ref may not exist. "+
						"To verify tag names, use api-get(endpoint: \"repos/%s/tags\") "+
						"or api-get(endpoint: \"repos/%s/branches\")",
					ref, repo, repo,
				)
			}
			hint += fmt.Sprintf(
				". Use get-hubbed://tree?repo=%s to list files and verify the path exists before reading",
				repo,
			)
			return nil, fmt.Errorf("%s", hint)
		}
		return nil, fmt.Errorf("gh api contents: %w", err)
	}

	trimmed := strings.TrimSpace(out)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return textResourceResult(uri, fmt.Sprintf("Path '%s' is a directory. Use get-hubbed://tree to list its contents.", path)), nil
	}

	var contentResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Size     int    `json:"size"`
		Name     string `json:"name"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		SHA      string `json:"sha"`
	}

	if err := json.Unmarshal([]byte(out), &contentResp); err != nil {
		return nil, fmt.Errorf("parsing content response: %w", err)
	}

	if contentResp.Type == "dir" {
		return textResourceResult(uri, fmt.Sprintf("Path '%s' is a directory. Use get-hubbed://tree to list its contents.", path)), nil
	}

	if contentResp.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding: %s", contentResp.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(
		strings.ReplaceAll(contentResp.Content, "\n", ""),
	)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 content: %w", err)
	}

	text := string(decoded)
	lines := strings.Split(text, "\n")
	totalLines := len(lines)

	startLine := 1
	if v := q.Get("line_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			startLine = n
		}
	}

	if startLine > totalLines {
		startLine = totalLines
	}

	endLine := totalLines
	if v := q.Get("line_limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && startLine-1+n < endLine {
			endLine = startLine - 1 + n
		}
	}

	selectedLines := lines[startLine-1 : endLine]

	sha := contentResp.SHA
	if len(sha) > 8 {
		sha = sha[:8]
	}

	header := fmt.Sprintf("File: %s (SHA: %s, %d bytes, %d total lines)\n",
		contentResp.Path, sha, contentResp.Size, totalLines)

	if q.Get("line_offset") != "" || q.Get("line_limit") != "" {
		header += fmt.Sprintf("Showing lines %d-%d of %d\n", startLine, endLine, totalLines)
	}

	return textResourceResult(uri, header+"\n"+strings.Join(selectedLines, "\n")), nil
}

func (p *resourceProvider) readTree(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "path", "ref", "recursive", "limit", "offset"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ref := q.Get("ref")
	if ref == "" {
		ref = "HEAD"
	}

	treeSha := ref
	if path := q.Get("path"); path != "" {
		treeSha = ref + ":" + path
	}

	ghArgs := []string{
		"api",
		fmt.Sprintf("repos/%s/git/trees/%s", repo, treeSha),
		"--method", "GET",
	}

	if q.Get("recursive") == "true" {
		ghArgs = append(ghArgs, "-f", "recursive=1")
	}

	ghArgs = append(ghArgs, "--jq", ".tree")

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh api git/trees: %w", err)
	}

	offset := 0
	limit := 0

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	if offset > 0 || limit > 0 {
		var entries []json.RawMessage
		if err := json.Unmarshal([]byte(out), &entries); err != nil {
			return textResourceResult(uri, out), nil
		}

		total := len(entries)
		start := offset
		if start > total {
			start = total
		}

		end := total
		if limit > 0 && start+limit < end {
			end = start + limit
		}

		paginated := entries[start:end]

		result := struct {
			Entries []json.RawMessage `json:"entries"`
			Total   int               `json:"total"`
			Offset  int               `json:"offset"`
			Count   int               `json:"count"`
		}{
			Entries: paginated,
			Total:   total,
			Offset:  start,
			Count:   len(paginated),
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling paginated tree: %w", err)
		}

		return textResourceResult(uri, string(resultJSON)), nil
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readBlame(ctx context.Context, uri, path string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "ref", "path", "start_line", "end_line"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected OWNER/REPO: %s", repo)
	}
	owner, name := parts[0], parts[1]

	ref := q.Get("ref")
	if ref == "" {
		ref = "HEAD"
	}

	query := fmt.Sprintf(`query {
		repository(owner: %q, name: %q) {
			object(expression: %q) {
				... on Commit {
					blame(path: %q) {
						ranges {
							startingLine
							endingLine
							commit {
								oid
								message
								author {
									name
									date
								}
							}
						}
					}
				}
			}
		}
	}`, owner, name, ref, path)

	ghArgs := []string{"api", "graphql", "-f", fmt.Sprintf("query=%s", query)}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql blame: %w", err)
	}

	startLine := 0
	endLine := 0

	if v := q.Get("start_line"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			startLine = n
		}
	}

	if v := q.Get("end_line"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			endLine = n
		}
	}

	if startLine > 0 || endLine > 0 {
		var result struct {
			Data struct {
				Repository struct {
					Object struct {
						Blame struct {
							Ranges []json.RawMessage `json:"ranges"`
						} `json:"blame"`
					} `json:"object"`
				} `json:"repository"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(out), &result); err != nil {
			return textResourceResult(uri, out), nil
		}

		var filtered []json.RawMessage
		for _, r := range result.Data.Repository.Object.Blame.Ranges {
			var rangeInfo struct {
				StartingLine int `json:"startingLine"`
				EndingLine   int `json:"endingLine"`
			}

			if err := json.Unmarshal(r, &rangeInfo); err != nil {
				continue
			}

			startFilter := startLine
			if startFilter == 0 {
				startFilter = 1
			}

			endFilter := endLine
			if endFilter == 0 {
				endFilter = rangeInfo.EndingLine
			}

			if rangeInfo.EndingLine >= startFilter && rangeInfo.StartingLine <= endFilter {
				filtered = append(filtered, r)
			}
		}

		filteredJSON, err := json.MarshalIndent(filtered, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling filtered blame: %w", err)
		}

		return textResourceResult(uri, string(filteredJSON)), nil
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readCommits(ctx context.Context, uri, path string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "ref", "path", "per_page", "page"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("repos/%s/commits", repo)

	ghArgs := []string{"api", endpoint, "--method", "GET", "-f", fmt.Sprintf("path=%s", path)}

	if ref := q.Get("ref"); ref != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("sha=%s", ref))
	}

	if perPage := q.Get("per_page"); perPage != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("per_page=%s", perPage))
	}

	if page := q.Get("page"); page != "" {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("page=%s", page))
	}

	ghArgs = append(ghArgs, "--jq",
		`[.[] | {sha: .sha, message: .commit.message, author: .commit.author.name, date: .commit.author.date, url: .html_url}]`,
	)

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh api commits: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readRunList(ctx context.Context, uri string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "branch", "status", "workflow", "event", "commit", "user", "limit"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"run", "list",
		"-R", repo,
		"--json", "attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,name,number,startedAt,status,updatedAt,url,workflowName",
	}

	if branch := q.Get("branch"); branch != "" {
		ghArgs = append(ghArgs, "--branch", branch)
	}

	if status := q.Get("status"); status != "" {
		ghArgs = append(ghArgs, "--status", status)
	}

	if workflow := q.Get("workflow"); workflow != "" {
		ghArgs = append(ghArgs, "--workflow", workflow)
	}

	if event := q.Get("event"); event != "" {
		ghArgs = append(ghArgs, "--event", event)
	}

	if commit := q.Get("commit"); commit != "" {
		ghArgs = append(ghArgs, "--commit", commit)
	}

	if user := q.Get("user"); user != "" {
		ghArgs = append(ghArgs, "--user", user)
	}

	if limit := q.Get("limit"); limit != "" {
		ghArgs = append(ghArgs, "--limit", limit)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh run list: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readRunView(ctx context.Context, uri, runID string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "run_id", "attempt"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"run", "view", runID,
		"-R", repo,
		"--json", "attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,jobs,name,number,startedAt,status,updatedAt,url,workflowDatabaseId,workflowName",
	}

	if attempt := q.Get("attempt"); attempt != "" {
		ghArgs = append(ghArgs, "--attempt", attempt)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh run view: %w", err)
	}

	return textResourceResult(uri, out), nil
}

func (p *resourceProvider) readRunLog(ctx context.Context, uri, runID string, q url.Values) (*protocol.ResourceReadResult, error) {
	if err := validateQueryParams(q, []string{"repo", "run_id", "job_id"}); err != nil {
		return nil, err
	}

	repo, err := p.resolveRepo(q.Get("repo"))
	if err != nil {
		return nil, err
	}

	ghArgs := []string{
		"run", "view", runID,
		"-R", repo,
		"--log-failed",
	}

	if jobID := q.Get("job_id"); jobID != "" {
		ghArgs = append(ghArgs, "--job", jobID)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh run view log: %w", err)
	}

	if out == "" {
		out = "No failed step logs found for this run."
	}

	return textResourceResult(uri, out), nil
}

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

func textResourceResult(uri, text string) *protocol.ResourceReadResult {
	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      uri,
				MimeType: "text/plain",
				Text:     text,
			},
		},
	}
}
