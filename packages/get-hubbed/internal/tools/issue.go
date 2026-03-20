package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

func registerIssueCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:        "issue-create",
		Title:       "Create Issue",
		Description: command.Description{Short: "Create a new issue"},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{
				Name:        "repo",
				Type:        command.String,
				Description: "Repository in OWNER/REPO format",
				Required:    true,
			},
			{
				Name:        "title",
				Type:        command.String,
				Description: "Issue title",
				Required:    true,
			},
			{Name: "body", Type: command.String, Description: "Issue body"},
			{Name: "labels", Type: command.Array, Description: "Labels to add"},
		},
		MapsTools: []command.ToolMapping{
			{
				Replaces:        "Bash",
				CommandPrefixes: []string{"gh issue create"},
				UseWhen:         "creating issues",
			},
		},
		Run: handleIssueCreate,
	})
}

func handleIssueCreate(
	ctx context.Context,
	args json.RawMessage,
	_ command.Prompter,
) (*command.Result, error) {
	var params struct {
		Repo   string   `json:"repo"`
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("invalid arguments: %v", err),
		), nil
	}

	// Use the REST API directly to avoid gh's fork-to-parent resolution,
	// which silently creates issues on the upstream repo instead of the
	// specified one.
	reqBody := map[string]any{
		"title": params.Title,
	}
	if params.Body != "" {
		reqBody["body"] = params.Body
	}
	if len(params.Labels) > 0 {
		reqBody["labels"] = params.Labels
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("marshaling request body: %v", err),
		), nil
	}

	ghArgs := []string{
		"api",
		fmt.Sprintf("repos/%s/issues", params.Repo),
		"--method", "POST",
		"--input", "-",
	}

	out, err := gh.RunWithInput(ctx, bytes.NewReader(bodyJSON), ghArgs...)
	if err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("gh api issue create: %v", err),
		), nil
	}

	// Extract the URL from the API response for a clean result
	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return command.TextResult(out), nil
	}

	return command.TextResult(fmt.Sprintf("%s\n", result.HTMLURL)), nil
}
