package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/grit/internal/git"
)

func registerRmCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "rm",
		Title:       "Remove Files",
		Description: command.Description{Short: "Delete files and stage the deletion (git rm)"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(false),
		},
		Params: []command.Param{
			{Name: "repo_path", Type: command.String, Description: "Path to the git repository (defaults to current working directory — almost never needed)"},
			{Name: "paths", Type: command.Array, Description: "Array of file path strings to remove, e.g. [\"old_file.go\", \"removed.txt\"] (relative to repo root)", Required: true},
			{Name: "force", Type: command.Bool, Description: "Force removal of files even if they have local modifications (git rm -f)"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"git rm"}, UseWhen: "removing files from the repository"},
		},
		Run: handleGitRm,
	})
}

func handleGitRm(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		RepoPath string   `json:"repo_path"`
		Paths    []string `json:"paths"`
		Force    bool     `json:"force"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	if len(params.Paths) == 0 {
		params.Paths = coerceStringToArray(args, "paths")
	}

	if len(params.Paths) == 0 {
		return command.TextErrorResult("paths is required and must be an array of strings"), nil
	}

	gitArgs := []string{"rm"}
	if params.Force {
		gitArgs = append(gitArgs, "-f")
	}
	gitArgs = append(gitArgs, "--")
	gitArgs = append(gitArgs, params.Paths...)

	if _, err := git.Run(ctx, params.RepoPath, gitArgs...); err != nil {
		return command.TextErrorResult(fmt.Sprintf("git rm: %v", err)), nil
	}

	return command.JSONResult(git.MutationResult{
		Status: "removed",
		Paths:  params.Paths,
	}), nil
}
