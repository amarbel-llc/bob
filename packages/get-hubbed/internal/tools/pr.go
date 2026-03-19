package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

func registerPRCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "pr-create",
		Title:       "Create Pull Request",
		Description: command.Description{Short: "Create a new pull request"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "repo", Type: command.String, Description: "Repository in OWNER/REPO format", Required: true},
			{Name: "title", Type: command.String, Description: "Pull request title", Required: true},
			{Name: "body", Type: command.String, Description: "Pull request body"},
			{Name: "base", Type: command.String, Description: "Base branch (the branch into which you want your code merged)"},
			{Name: "head", Type: command.String, Description: "Head branch (the branch that contains commits for your pull request). Usually necessary because gh refuses to create PRs when the working tree has untracked files unless --head is explicit. Format: OWNER:BRANCH for cross-repo PRs"},
			{Name: "draft", Type: command.Bool, Description: "Create as draft pull request"},
			{Name: "labels", Type: command.Array, Description: "Labels to add"},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"gh pr create"}, UseWhen: "creating pull requests"},
		},
		Run: handlePRCreate,
	})
}

func handlePRCreate(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	var params struct {
		Repo   string   `json:"repo"`
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Base   string   `json:"base"`
		Head   string   `json:"head"`
		Draft  bool     `json:"draft"`
		Labels []string `json:"labels"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	ghArgs := []string{
		"pr", "create",
		"-R", params.Repo,
		"--title", params.Title,
	}

	if params.Body != "" {
		ghArgs = append(ghArgs, "--body", params.Body)
	}

	if params.Base != "" {
		ghArgs = append(ghArgs, "--base", params.Base)
	}

	if params.Head != "" {
		ghArgs = append(ghArgs, "--head", params.Head)
	}

	if params.Draft {
		ghArgs = append(ghArgs, "--draft")
	}

	for _, label := range params.Labels {
		ghArgs = append(ghArgs, "--label", label)
	}

	out, err := gh.Run(ctx, ghArgs...)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("gh pr create: %v", err)), nil
	}

	return command.TextResult(out), nil
}
