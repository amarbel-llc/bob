package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/grit/internal/git"
)

func registerWorktreeCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "worktree-list",
		Title:       "List Worktrees",
		Description: command.Description{Short: "List all git worktrees with path, HEAD, branch, and lock/prune state"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(true),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git worktree list"}, UseWhen: "listing worktrees"},
		},
		Run: handleWorktreeList,
	})

	app.AddCommand(&command.Command{
		Name:        "worktree-remove",
		Title:       "Remove Worktree",
		Description: command.Description{Short: "Remove a git worktree by path"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "path", Type: command.String, Description: "Path of the worktree to remove", Required: true},
			{Name: "force", Type: command.Bool, Description: "Force removal even if worktree has uncommitted changes (--force)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git worktree remove"}, UseWhen: "removing a worktree"},
		},
		Run: handleWorktreeRemove,
	})
}

func handleWorktreeList(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	out, err := git.Run(ctx, params.RepoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("git worktree list: %v", err)), nil
	}

	entries := git.ParseWorktreeList(out)
	return command.JSONResult(entries), nil
}

func handleWorktreeRemove(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
		Path     string `json:"path"`
		Force    bool   `json:"force"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	gitArgs := []string{"worktree", "remove", params.Path}
	if params.Force {
		gitArgs = append(gitArgs, "--force")
	}

	if _, err := git.Run(ctx, params.RepoPath, gitArgs...); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git worktree remove: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "removed",
		Name:   params.Path,
		Force:  params.Force,
	}), nil
}
