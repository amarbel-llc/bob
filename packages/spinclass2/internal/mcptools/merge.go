package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/amarbel-llc/spinclass2/internal/executor"
	"github.com/amarbel-llc/spinclass2/internal/merge"
	"github.com/amarbel-llc/spinclass2/internal/worktree"
)

func registerMerge(app *command.App) {
	app.AddCommand(&command.Command{
		Name:  "merge",
		Title: "Merge Worktree",
		Description: command.Description{
			Short: "Merge a worktree branch into the default branch and clean up",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{
				Name:        "target",
				Type:        command.String,
				Description: "Branch or worktree name to merge into the default branch",
				Required:    true,
			},
			{
				Name:        "git_sync",
				Type:        command.Bool,
				Description: "Pull and push after merge (default false)",
			},
		},
		Run: handleMerge,
	})
}

func handleMerge(_ context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		Target  string `json:"target"`
		GitSync bool   `json:"git_sync"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if params.Target == "" {
		return command.TextErrorResult("target is required"), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("could not get working directory: %v", err)), nil
	}

	repoPath, err := worktree.DetectRepo(cwd)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("not in a git repository: %v", err)), nil
	}

	wtPath, branch, err := merge.ResolveWorktree(repoPath, params.Target)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("worktree not found: %v", err)), nil
	}

	defaultBranch, err := merge.ResolveDefaultBranch(repoPath)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("could not determine default branch: %v", err)), nil
	}

	var buf bytes.Buffer
	mergeErr := merge.Resolved(
		executor.ShellExecutor{},
		&buf,
		nil,
		"tap",
		repoPath,
		wtPath,
		branch,
		defaultBranch,
		params.GitSync,
		false,
		true,
	)

	if mergeErr != nil {
		return command.TextErrorResult(buf.String()), nil
	}

	return command.TextResult(buf.String()), nil
}
