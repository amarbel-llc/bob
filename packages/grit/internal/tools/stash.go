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

func registerStashCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "stash_save",
		Title:       "Stash Changes",
		Description: command.Description{Short: "Save uncommitted changes to a stash"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "message", Type: command.String, Description: "Stash message"},
			{Name: "include_untracked", Type: command.Bool, Description: "Include untracked files (-u)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git stash push", "git stash save", "git stash -"}, UseWhen: "stashing changes"},
		},
		Run: handleGitStashSave,
	})

	app.AddCommand(&command.Command{
		Name:        "stash_apply",
		Title:       "Apply Stash",
		Description: command.Description{Short: "Apply a stash to the working tree without removing it from the stash list"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "stash_ref", Type: command.String, Description: "Stash reference (e.g. stash@{0}). Defaults to stash@{0}"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git stash apply", "git stash pop"}, UseWhen: "applying a stash"},
		},
		Run: handleGitStashApply,
	})

	app.AddCommand(&command.Command{
		Name:        "stash_drop",
		Title:       "Drop Stash",
		Description: command.Description{Short: "Delete a stash entry"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "stash_ref", Type: command.String, Description: "Stash reference to drop (e.g. stash@{0})", Required: true},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git stash drop", "git stash clear"}, UseWhen: "dropping a stash"},
		},
		Run: handleGitStashDrop,
	})
}

func handleGitStashSave(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath         string `json:"repo_path"`
		Message          string `json:"message"`
		IncludeUntracked bool   `json:"include_untracked"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	gitArgs := []string{"stash", "push"}

	if params.IncludeUntracked {
		gitArgs = append(gitArgs, "-u")
	}

	if params.Message != "" {
		gitArgs = append(gitArgs, "-m", params.Message)
	}

	out, err := git.Run(ctx, params.RepoPath, gitArgs...)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("git stash push: %v", err)), nil
	}

	if strings.Contains(out, "No local changes to save") {
		return command.JSONResult(git.MutationResult{
			Status: "no_changes",
		}), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "stashed",
		Name:   params.Message,
	}), nil
}

func handleGitStashApply(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
		StashRef string `json:"stash_ref"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	gitArgs := []string{"stash", "apply"}
	if params.StashRef != "" {
		gitArgs = append(gitArgs, params.StashRef)
	}

	stdout, stderr, err := git.RunBothOutputs(ctx, params.RepoPath, gitArgs...)
	if err != nil {
		combined := stdout + stderr
		if strings.Contains(combined, "CONFLICT") || strings.Contains(combined, "could not apply") {
			conflicts := extractConflictFiles(ctx, params.RepoPath)
			return command.JSONResult(git.MergeResult{
				Status:    "conflict",
				Conflicts: conflicts,
			}), nil
		}

		return command.TextErrorResult(fmt.Sprintf("git stash apply: %v", err)), nil
	}

	ref := params.StashRef
	if ref == "" {
		ref = "stash@{0}"
	}

	return command.JSONResult(git.MutationResult{
		Status: "applied",
		Ref:    ref,
	}), nil
}

func handleGitStashDrop(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
		StashRef string `json:"stash_ref"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if _, err := git.Run(ctx, params.RepoPath, "stash", "drop", params.StashRef); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git stash drop: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "dropped",
		Ref:    params.StashRef,
	}), nil
}
