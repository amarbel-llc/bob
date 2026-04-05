package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/grit/internal/git"
)

func registerStatusCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "diff",
		Title:       "Show Changes",
		Description: command.Description{Short: "Show changes in the working tree or between commits"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(true),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "staged", Type: command.Bool, Description: "Show only staged (true) or only unstaged (false) changes. Omit to show both."},
			{Name: "ref", Type: command.String, Description: "Diff against a specific ref (commit, branch, tag)"},
			{Name: "paths", Type: command.Array, Description: "Limit diff to specific paths"},
			{Name: "stat_only", Type: command.Bool, Description: "Show only diffstat summary"},
			{Name: "context_lines", Type: command.Int, Description: "Number of context lines around each change (git --unified=N, default 3)"},
			{Name: "max_patch_lines", Type: command.Int, Description: "Maximum number of patch output lines. Output is truncated with a truncated flag when exceeded."},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git diff"}, UseWhen: "viewing changes"},
		},
		Run: handleGitDiff,
	})
}

func handleGitDiff(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath      string   `json:"repo_path"`
		Staged        *bool    `json:"staged"`
		Ref           string   `json:"ref"`
		Paths         []string `json:"paths"`
		StatOnly      bool     `json:"stat_only"`
		ContextLines  *int     `json:"context_lines"`
		MaxPatchLines int      `json:"max_patch_lines"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	buildSection := func(cached bool) (*git.DiffSection, error) {
		numstatArgs := []string{"diff", "--numstat"}
		if cached {
			numstatArgs = append(numstatArgs, "--cached")
		}
		if params.Ref != "" {
			numstatArgs = append(numstatArgs, params.Ref)
		}
		if len(params.Paths) > 0 {
			numstatArgs = append(numstatArgs, "--")
			numstatArgs = append(numstatArgs, params.Paths...)
		}

		numstatOut, err := git.Run(ctx, params.RepoPath, numstatArgs...)
		if err != nil {
			return nil, fmt.Errorf("git diff: %v", err)
		}

		stats := git.ParseDiffNumstat(numstatOut)

		var summary git.DiffSummary
		summary.TotalFiles = len(stats)
		for _, s := range stats {
			summary.TotalAdditions += s.Additions
			summary.TotalDeletions += s.Deletions
		}

		section := &git.DiffSection{
			Stats:   stats,
			Summary: summary,
		}

		if !params.StatOnly {
			patchArgs := []string{"diff"}
			if params.ContextLines != nil {
				patchArgs = append(patchArgs, fmt.Sprintf("--unified=%d", *params.ContextLines))
			}
			if cached {
				patchArgs = append(patchArgs, "--cached")
			}
			if params.Ref != "" {
				patchArgs = append(patchArgs, params.Ref)
			}
			if len(params.Paths) > 0 {
				patchArgs = append(patchArgs, "--")
				patchArgs = append(patchArgs, params.Paths...)
			}

			patchOut, err := git.Run(ctx, params.RepoPath, patchArgs...)
			if err != nil {
				return nil, fmt.Errorf("git diff: %v", err)
			}

			patch, truncated, truncatedAt := git.TruncatePatch(patchOut, params.MaxPatchLines)
			section.Patch = patch
			section.Truncated = truncated
			section.TruncatedAtLine = truncatedAt
		}

		return section, nil
	}

	var result git.DiffResult

	switch {
	case params.Staged != nil && *params.Staged:
		section, err := buildSection(true)
		if err != nil {
			return command.TextErrorResult(err.Error()), nil
		}
		result.Staged = section

	case params.Staged != nil && !*params.Staged:
		section, err := buildSection(false)
		if err != nil {
			return command.TextErrorResult(err.Error()), nil
		}
		result.Unstaged = section

	default:
		staged, err := buildSection(true)
		if err != nil {
			return command.TextErrorResult(err.Error()), nil
		}
		result.Staged = staged

		unstaged, err := buildSection(false)
		if err != nil {
			return command.TextErrorResult(err.Error()), nil
		}
		result.Unstaged = unstaged
	}

	return command.JSONResult(result), nil
}
