package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

func registerContentCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "content-compare",
		Title:       "Compare Refs",
		Description: command.Description{Short: "Compare two refs (branches, tags, or commits) showing commits and file changes"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(true),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "repo", Type: command.String, Description: "Repository in OWNER/REPO format", Required: true},
			{Name: "base", Type: command.String, Description: "Base ref (branch, tag, or SHA)", Required: true},
			{Name: "head", Type: command.String, Description: "Head ref (branch, tag, or SHA)", Required: true},
			{Name: "per_page", Type: command.Int, Description: "Number of file entries per page (max 100, default 30)"},
			{Name: "page", Type: command.Int, Description: "Page number for pagination (default 1)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"gh api repos"}, UseWhen: "comparing two refs in a repository"},
		},
		Run: handleContentCompare,
	})

	app.AddCommand(&command.Command{
		Name:        "content-search",
		Title:       "Search Code",
		Description: command.Description{Short: "Search for code within a repository"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(true),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "repo", Type: command.String, Description: "Repository in OWNER/REPO format", Required: true},
			{Name: "query", Type: command.String, Description: "Search query (code to search for)", Required: true},
			{Name: "path", Type: command.String, Description: "Restrict search to a file path or directory prefix"},
			{Name: "extension", Type: command.String, Description: "Restrict search to a file extension (e.g. 'go', 'py')"},
			{Name: "per_page", Type: command.Int, Description: "Number of results per page (max 100, default 30)"},
			{Name: "page", Type: command.Int, Description: "Page number for pagination (default 1)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"gh api search/code"}, UseWhen: "searching for code in a repository"},
		},
		Run: handleContentSearch,
	})
}

func handleContentCompare(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		Repo    string `json:"repo"`
		Base    string `json:"base"`
		Head    string `json:"head"`
		PerPage int    `json:"per_page"`
		Page    int    `json:"page"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	endpoint := fmt.Sprintf("repos/%s/compare/%s...%s", params.Repo, params.Base, params.Head)

	ghArgs := []string{"api", endpoint, "--method", "GET"}

	if params.PerPage > 0 {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("per_page=%d", params.PerPage))
	}

	if params.Page > 0 {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("page=%d", params.Page))
	}

	ghArgs = append(ghArgs, "--jq",
		`{status, ahead_by, behind_by, total_commits, commits: [.commits[] | {sha: .sha[:8], message: .commit.message, author: .commit.author.name, date: .commit.author.date}], files: [.files[] | {filename, status, additions, deletions, changes}]}`,
	)

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("gh api compare: %v", err)), nil
	}

	return command.TextResult(out), nil
}

func handleContentSearch(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		Repo      string `json:"repo"`
		Query     string `json:"query"`
		Path      string `json:"path"`
		Extension string `json:"extension"`
		PerPage   int    `json:"per_page"`
		Page      int    `json:"page"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	q := fmt.Sprintf("%s repo:%s", params.Query, params.Repo)

	if params.Path != "" {
		q += fmt.Sprintf(" path:%s", params.Path)
	}

	if params.Extension != "" {
		q += fmt.Sprintf(" extension:%s", params.Extension)
	}

	ghArgs := []string{
		"api", "search/code",
		"--method", "GET",
		"-H", "Accept: application/vnd.github.text-match+json",
		"-f", fmt.Sprintf("q=%s", q),
	}

	if params.PerPage > 0 {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("per_page=%d", params.PerPage))
	}

	if params.Page > 0 {
		ghArgs = append(ghArgs, "-f", fmt.Sprintf("page=%d", params.Page))
	}

	ghArgs = append(ghArgs, "--jq",
		`{total_count, items: [.items[] | {name, path, sha, url: .html_url, score, text_matches: [.text_matches[]? | {fragment, matches: .matches}]}]}`,
	)

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("gh api search/code: %v", err)), nil
	}

	return command.TextResult(out), nil
}
