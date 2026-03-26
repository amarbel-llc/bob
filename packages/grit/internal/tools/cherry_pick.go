package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/grit/internal/git"
)

func registerCherryPickCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "cherry_pick",
		Title:       "Cherry Pick",
		Description: command.Description{Short: "Apply commits from other branches onto the current branch"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "commits", Type: command.Array, Description: "Commit hashes to cherry-pick (applied in order)", Required: true},
			{Name: "no_commit", Type: command.Bool, Description: "Apply changes without creating commits (--no-commit)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git cherry-pick"}, UseWhen: "cherry-picking commits"},
		},
		Run: handleGitCherryPick,
	})
}

func handleGitCherryPick(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string   `json:"repo_path"`
		Commits  []string `json:"commits"`
		NoCommit bool     `json:"no_commit"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if len(params.Commits) == 0 {
		return command.TextErrorResult("at least one commit hash is required"), nil
	}

	gitArgs := []string{"cherry-pick"}
	if params.NoCommit {
		gitArgs = append(gitArgs, "--no-commit")
	}
	gitArgs = append(gitArgs, params.Commits...)

	_, err := git.Run(ctx, params.RepoPath, gitArgs...)
	if err != nil {
		if strings.Contains(err.Error(), "CONFLICT") || strings.Contains(err.Error(), "could not apply") {
			conflicts := extractConflictFiles(ctx, params.RepoPath)
			return command.JSONResult(struct {
				Status    string   `json:"status"`
				Conflicts []string `json:"conflicts,omitempty"`
			}{
				Status:    "conflict",
				Conflicts: conflicts,
			}), nil
		}
		return command.TextErrorResult(fmt.Sprintf("git cherry-pick: %v", err)), nil
	}

	status := "cherry_picked"
	if params.NoCommit {
		status = "applied"
	}

	return command.JSONResult(git.MutationResult{
		Status: status,
	}), nil
}
