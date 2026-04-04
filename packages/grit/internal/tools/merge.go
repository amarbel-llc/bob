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

func registerMergeCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "merge",
		Title:       "Merge Branch",
		Description: command.Description{Short: "Merge a branch into the current branch (blocked on main/master for safety)"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "branch", Type: command.String, Description: "Branch to merge", Completer: branchCompleter(true)},
			{Name: "no_ff", Type: command.Bool, Description: "Create a merge commit even if fast-forward is possible (--no-ff)"},
			{Name: "squash", Type: command.Bool, Description: "Squash commits before merging (--squash)"},
			{Name: "abort", Type: command.Bool, Description: "Abort an in-progress merge"},
			{Name: "continue", Type: command.Bool, Description: "Continue merge after resolving conflicts"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git merge"}, UseWhen: "merging a branch"},
		},
		Run: handleGitMerge,
	})
}

func handleGitMerge(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string `json:"repo_path"`
		Branch   string `json:"branch"`
		NoFF     bool   `json:"no_ff"`
		Squash   bool   `json:"squash"`
		Abort    bool   `json:"abort"`
		Continue bool   `json:"continue"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	// Validate mutually exclusive operations
	opCount := 0
	if params.Abort {
		opCount++
	}
	if params.Continue {
		opCount++
	}
	if params.Branch != "" {
		opCount++
	}

	if opCount > 1 {
		return command.TextErrorResult("only one of branch, abort, or continue can be specified"), nil
	}

	if opCount == 0 {
		return command.TextErrorResult("must specify branch (for new merge) or abort/continue (for existing merge)"), nil
	}

	// Handle abort
	if params.Abort {
		if _, err := git.Run(ctx, params.RepoPath, "merge", "--abort"); err != nil {
			return command.TextErrorResult(fmt.Sprintf("git merge --abort: %v", err)), nil
		}

		return command.JSONResult(git.MergeResult{
			Status: "aborted",
		}), nil
	}

	// Handle continue
	if params.Continue {
		out, err := git.Run(ctx, params.RepoPath, "merge", "--continue")
		if err != nil {
			if strings.Contains(err.Error(), "fix conflicts") || strings.Contains(err.Error(), "still have conflicts") {
				conflicts := extractConflictFiles(ctx, params.RepoPath)
				return command.JSONResult(git.MergeResult{
					Status:    "conflict",
					Conflicts: conflicts,
				}), nil
			}

			return command.TextErrorResult(fmt.Sprintf("git merge --continue: %v", err)), nil
		}

		return command.JSONResult(git.MergeResult{
			Status:  "completed",
			Summary: strings.TrimSpace(out),
		}), nil
	}

	// New merge — determine current branch for safety check
	branchOut, err := git.Run(ctx, params.RepoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		currentBranch := strings.TrimSpace(branchOut)
		if currentBranch == "main" || currentBranch == "master" {
			return command.TextErrorResult("merging into main/master is blocked for safety"), nil
		}
	}

	// Check for existing merge state
	state, err := git.DetectInProgressState(ctx, params.RepoPath)
	if err == nil && state != nil && state.Operation == "merge" {
		return command.TextErrorResult("a merge is already in progress; use continue or abort"), nil
	}

	gitArgs := []string{"merge"}

	if params.NoFF {
		gitArgs = append(gitArgs, "--no-ff")
	}

	if params.Squash {
		gitArgs = append(gitArgs, "--squash")
	}

	gitArgs = append(gitArgs, params.Branch)

	out, err := git.Run(ctx, params.RepoPath, gitArgs...)
	if err != nil {
		if strings.Contains(err.Error(), "CONFLICT") || strings.Contains(err.Error(), "Automatic merge failed") {
			conflicts := extractConflictFiles(ctx, params.RepoPath)
			return command.JSONResult(git.MergeResult{
				Status:    "conflict",
				Branch:    params.Branch,
				Conflicts: conflicts,
			}), nil
		}

		return command.TextErrorResult(fmt.Sprintf("git merge: %v", err)), nil
	}

	result := git.MergeResult{
		Status:  "merged",
		Branch:  params.Branch,
		Summary: strings.TrimSpace(out),
	}

	if strings.Contains(out, "Already up to date") {
		result.Status = "already_up_to_date"
		result.Summary = ""
	} else if strings.Contains(out, "Fast-forward") {
		result.Status = "fast_forward"
	}

	if params.Squash {
		result.Status = "squash_staged"
	}

	return command.JSONResult(result), nil
}
