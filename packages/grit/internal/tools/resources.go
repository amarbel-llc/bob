package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
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

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://status",
		Name:        "Repository Status",
		Description: "Working tree status with branch info and in-progress state",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://branches",
		Name:        "Branch List",
		Description: "Local and remote branches with tracking info",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://remotes",
		Name:        "Remote List",
		Description: "Configured remotes with their URLs",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://tags",
		Name:        "Tag List",
		Description: "Tags with metadata (name, hash, type, date, tagger)",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://stashes",
		Name:        "Stash List",
		Description: "Stash entries with index and message",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterResource(protocol.Resource{
		URI:         "grit://worktrees",
		Name:        "Worktree List",
		Description: "Git worktrees with path, HEAD, branch, and lock/prune state",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "grit://log?repo_path={repo_path}&max_count={max_count}&ref={ref}&paths={paths}&all={all}",
		Name:        "Commit Log",
		Description: "Commit history. All params optional: max_count (default 10), ref, paths (comma-separated), all (bool). repo_path defaults to cwd — almost never needed.",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "grit://commits/{ref}?repo_path={repo_path}&context_lines={context_lines}&max_patch_lines={max_patch_lines}",
		Name:        "Commit Detail",
		Description: "Show a commit with metadata and patch. Required: ref. Optional: context_lines, max_patch_lines. repo_path defaults to cwd — almost never needed.",
		MimeType:    "application/json",
	}, nil)

	registry.RegisterTemplate(protocol.ResourceTemplate{
		URITemplate: "grit://blame/{path}?repo_path={repo_path}&ref={ref}&line_range={line_range}",
		Name:        "Line Authorship",
		Description: "Line-by-line authorship of a file. Required: path. Optional: ref, line_range (START,END). repo_path defaults to cwd — almost never needed.",
		MimeType:    "application/json",
	}, nil)

	return p, nil
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

	repoPath := parsed.Query().Get("repo_path")
	if repoPath == "" {
		repoPath = p.cwd
	}

	switch parsed.Host {
	case "status":
		return p.readStatus(ctx, uri, repoPath)
	case "branches":
		return p.readBranches(ctx, uri, repoPath)
	case "remotes":
		return p.readRemotes(ctx, uri, repoPath)
	case "tags":
		return p.readTags(ctx, uri, repoPath)
	case "stashes":
		return p.readStashes(ctx, uri, repoPath)
	case "worktrees":
		return p.readWorktrees(ctx, uri, repoPath)
	case "log":
		return p.readLog(ctx, uri, repoPath, parsed.Query())
	case "commits":
		ref := strings.TrimPrefix(parsed.Path, "/")
		if ref == "" {
			return nil, fmt.Errorf("missing ref in commits URI")
		}
		return p.readShow(ctx, uri, repoPath, ref, parsed.Query())
	case "blame":
		path := strings.TrimPrefix(parsed.Path, "/")
		if path == "" {
			return nil, fmt.Errorf("missing path in blame URI")
		}
		return p.readBlame(ctx, uri, repoPath, path, parsed.Query())
	default:
		return nil, fmt.Errorf("unknown resource: %s", uri)
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

	return marshalResourceResult(uri, result)
}

func (p *resourceProvider) readBranches(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{
		"branch",
		"--format=%(HEAD)\x1f%(refname:short)\x1f%(objectname:short)\x1f%(subject)\x1f%(upstream:short)\x1f%(upstream:track)\x1e",
		"-a",
	}

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}

	branches := git.ParseBranchList(out)
	return marshalResourceResult(uri, branches)
}

func (p *resourceProvider) readRemotes(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	out, err := git.Run(ctx, repoPath, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("git remote: %w", err)
	}

	remotes := git.ParseRemoteList(out)
	return marshalResourceResult(uri, remotes)
}

func (p *resourceProvider) readTags(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{
		"tag",
		"--list",
		fmt.Sprintf("--format=%s", git.TagListFormat),
	}

	out, err := git.Run(ctx, repoPath, gitArgs...)
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}

	tags := git.ParseTagList(out)
	return marshalResourceResult(uri, tags)
}

func (p *resourceProvider) readStashes(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	out, err := git.Run(ctx, repoPath, "stash", "list", fmt.Sprintf("--format=%s", git.StashListFormat))
	if err != nil {
		// No stash ref means empty stash list, not an error
		if strings.Contains(err.Error(), "unknown revision") {
			return marshalResourceResult(uri, []git.StashEntry{})
		}
		return nil, fmt.Errorf("git stash list: %w", err)
	}

	entries := git.ParseStashList(out)
	return marshalResourceResult(uri, entries)
}

func (p *resourceProvider) readWorktrees(ctx context.Context, uri, repoPath string) (*protocol.ResourceReadResult, error) {
	out, err := git.Run(ctx, repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	entries := git.ParseWorktreeList(out)
	return marshalResourceResult(uri, entries)
}

func (p *resourceProvider) readLog(ctx context.Context, uri, repoPath string, q url.Values) (*protocol.ResourceReadResult, error) {
	gitArgs := []string{"log"}

	maxCount := 10
	if v := q.Get("max_count"); v != "" {
		fmt.Sscanf(v, "%d", &maxCount)
		if maxCount <= 0 {
			maxCount = 10
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
	return marshalResourceResult(uri, entries)
}

func (p *resourceProvider) readShow(ctx context.Context, uri, repoPath, ref string, q url.Values) (*protocol.ResourceReadResult, error) {
	metadataOut, err := git.Run(ctx, repoPath, "show", "--no-patch", fmt.Sprintf("--format=%s", git.ShowFormat), ref)
	if err != nil {
		// Fall back to raw output for non-commit objects
		out, fallbackErr := git.Run(ctx, repoPath, "show", ref)
		if fallbackErr != nil {
			return nil, fmt.Errorf("git show: %w", err)
		}
		return &protocol.ResourceReadResult{
			Contents: []protocol.ResourceContent{
				{URI: uri, MimeType: "text/plain", Text: out},
			},
		}, nil
	}

	numstatOut, err := git.Run(ctx, repoPath, "show", "--numstat", "--format=", ref)
	if err != nil {
		numstatOut = ""
	}

	diffArgs := []string{"diff"}
	if v := q.Get("context_lines"); v != "" {
		diffArgs = append(diffArgs, fmt.Sprintf("--unified=%s", v))
	}
	diffArgs = append(diffArgs, ref+"~1", ref)

	patchOut, err := git.Run(ctx, repoPath, diffArgs...)
	if err != nil {
		patchOut = ""
	}

	result := git.ParseShow(metadataOut, numstatOut, patchOut)

	if v := q.Get("max_patch_lines"); v != "" {
		var maxLines int
		fmt.Sscanf(v, "%d", &maxLines)
		if maxLines > 0 {
			patch, truncated, truncatedAt := git.TruncatePatch(result.Patch, maxLines)
			result.Patch = patch
			result.Truncated = truncated
			result.TruncatedAtLine = truncatedAt
		}
	}

	return marshalResourceResult(uri, result)
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
	return marshalResourceResult(uri, lines)
}

func registerResourceToolCommands(app *command.App, resProvider *resourceProvider) {
	readOnly := true
	notDestructive := false
	idempotent := true
	notOpenWorld := false

	app.AddCommand(&command.Command{
		Name: "resource-templates",
		Description: command.Description{
			Short: "List available grit resource templates. Call this first to discover what git resources are available, then use resource-read to access them.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
			OpenWorldHint:   &notOpenWorld,
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
			OpenWorldHint:   &notOpenWorld,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "Resource URI (e.g., grit://status, grit://log?max_count=5)", Required: true},
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

func marshalResourceResult(uri string, data any) (*protocol.ResourceReadResult, error) {
	text, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(text),
			},
		},
	}, nil
}
