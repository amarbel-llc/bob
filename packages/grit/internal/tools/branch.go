package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/grit/internal/git"
)

func registerBranchCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "branch_create",
		Title:       "Create Branch",
		Description: command.Description{Short: "Create a new branch"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "name", Type: command.String, Description: "Name for the new branch", Required: true},
			{Name: "start_point", Type: command.String, Description: "Starting point for the new branch (commit, branch, tag)"},
		},
		Run: handleGitBranchCreate,
	})

	app.AddCommand(&command.Command{
		Name:        "branch_delete",
		Title:       "Delete Branch",
		Description: command.Description{Short: "Delete a local branch (blocked on main/master for safety)"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "name", Type: command.String, Description: "Name of the branch to delete", Required: true},
			{Name: "force", Type: command.Bool, Description: "Force delete even if not fully merged (-D instead of -d)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git branch -d", "git branch -D", "git branch --delete"}, UseWhen: "deleting a branch"},
		},
		Run: handleGitBranchDelete,
	})

	app.AddCommand(&command.Command{
		Name:        "checkout",
		Title:       "Switch Branches or Restore Files",
		Description: command.Description{Short: "Switch branches or restore individual files from a ref. Use paths to restore specific files; omit paths to switch branches."},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "ref", Type: command.String, Description: "Branch name or ref to check out or restore files from (defaults to HEAD when used with paths)"},
			{Name: "create", Type: command.Bool, Description: "Create a new branch and check it out (-b)"},
			{Name: "paths", Type: command.Array, Description: "File paths to restore from ref (e.g. [\"src/main.go\", \"README.md\"]). When provided, restores these files instead of switching branches."},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git checkout", "git switch", "git restore"}, UseWhen: "switching branches or restoring files"},
		},
		Run: handleGitCheckout,
	})
}

func handleGitBranchCreate(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath   string `json:"repo_path"`
		Name       string `json:"name"`
		StartPoint string `json:"start_point"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	gitArgs := []string{"branch", params.Name}

	if params.StartPoint != "" {
		gitArgs = append(gitArgs, params.StartPoint)
	}

	if _, err := git.Run(ctx, params.RepoPath, gitArgs...); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git branch create: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status:     "created",
		Name:       params.Name,
		StartPoint: params.StartPoint,
	}), nil
}

func handleGitBranchDelete(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
		Name     string `json:"name"`
		Force    bool   `json:"force"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if params.Name == "main" || params.Name == "master" {
		return command.TextErrorResult("deleting main/master is blocked for safety"), nil
	}

	flag := "-d"
	if params.Force {
		flag = "-D"
	}

	if _, err := git.Run(ctx, params.RepoPath, "branch", flag, params.Name); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git branch delete: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "deleted",
		Name:   params.Name,
		Force:  params.Force,
	}), nil
}

func handleGitCheckout(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string   `json:"repo_path"`
		Ref      string   `json:"ref"`
		Create   bool     `json:"create"`
		Paths    []string `json:"paths"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if len(params.Paths) > 0 {
		// File restore mode: git checkout <ref> -- <paths>
		ref := params.Ref
		if ref == "" {
			ref = "HEAD"
		}

		gitArgs := []string{"checkout", ref, "--"}
		gitArgs = append(gitArgs, params.Paths...)

		if _, err := git.Run(ctx, params.RepoPath, gitArgs...); err != nil {
			return command.TextErrorResult(fmt.Sprintf("git checkout: %v", err)), nil
		}

		return command.JSONResult(git.MutationResult{
			Status: "restored",
			Ref:    ref,
			Paths:  params.Paths,
		}), nil
	}

	// Branch switch mode
	if params.Ref == "" {
		return command.TextErrorResult("ref is required when not restoring specific files"), nil
	}

	gitArgs := []string{"checkout"}

	if params.Create {
		gitArgs = append(gitArgs, "-b")
	}

	gitArgs = append(gitArgs, params.Ref)

	if _, err := git.Run(ctx, params.RepoPath, gitArgs...); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git checkout: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "switched",
		Ref:    params.Ref,
		Create: params.Create,
	}), nil
}
