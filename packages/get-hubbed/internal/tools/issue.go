package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

func registerIssueCommands(app *command.App) {
	app.AddCommand(&command.Command{
		Name:  "issue-close",
		Title: "Close Issue",
		Description: command.Description{
			Short: "Close an issue with optional comment",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
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
				Name:        "number",
				Type:        command.Int,
				Description: "Issue number",
				Required:    true,
			},
			{
				Name:        "comment",
				Type:        command.String,
				Description: "Optional comment to add before closing",
			},
			{
				Name:        "reason",
				Type:        command.String,
				Description: "Close reason: completed (default) or not_planned",
			},
		},
		MapsTools: []command.ToolMapping{
			{
				Replaces:        "Bash",
				CommandPrefixes: []string{"gh issue close"},
				UseWhen:         "closing issues",
			},
		},
		Run: handleIssueClose,
	})

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
				Description: "Repository in OWNER/REPO format. Omit to use the current directory's git remote. Only pass repo when creating an issue in a different repository than the one you're working in.",
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

	if params.Repo == "" {
		repo, err := resolveRepoFromRemote(ctx)
		if err != nil {
			return command.TextErrorResult(
				fmt.Sprintf("repo not specified and could not detect from git remote: %v", err),
			), nil
		}
		params.Repo = repo
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

func handleIssueClose(
	ctx context.Context,
	args json.RawMessage,
	_ command.Prompter,
) (*command.Result, error) {
	var params struct {
		Repo    string `json:"repo"`
		Number  int    `json:"number"`
		Comment string `json:"comment"`
		Reason  string `json:"reason"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("invalid arguments: %v", err),
		), nil
	}

	issueNumber := params.Number

	// Add comment first if provided
	if params.Comment != "" {
		commentBody, err := json.Marshal(
			map[string]string{"body": params.Comment},
		)
		if err != nil {
			return command.TextErrorResult(
				fmt.Sprintf("marshaling comment: %v", err),
			), nil
		}

		_, err = gh.RunWithInput(
			ctx,
			bytes.NewReader(commentBody),
			"api",
			fmt.Sprintf(
				"repos/%s/issues/%d/comments",
				params.Repo,
				issueNumber,
			),
			"--method",
			"POST",
			"--input",
			"-",
		)
		if err != nil {
			return command.TextErrorResult(
				fmt.Sprintf("gh api add comment: %v", err),
			), nil
		}
	}

	// Close the issue via PATCH
	stateReason := "completed"
	if params.Reason == "not_planned" {
		stateReason = "not_planned"
	}

	closeBody, err := json.Marshal(map[string]string{
		"state":        "closed",
		"state_reason": stateReason,
	})
	if err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("marshaling close body: %v", err),
		), nil
	}

	out, err := gh.RunWithInput(ctx, bytes.NewReader(closeBody),
		"api",
		fmt.Sprintf("repos/%s/issues/%d", params.Repo, issueNumber),
		"--method", "PATCH",
		"--input", "-",
	)
	if err != nil {
		return command.TextErrorResult(
			fmt.Sprintf("gh api issue close: %v", err),
		), nil
	}

	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return command.TextResult(out), nil
	}

	return command.TextResult(
		fmt.Sprintf(
			"Closed #%d: %s\n%s\n",
			result.Number,
			result.Title,
			result.HTMLURL,
		),
	), nil
}

// resolveRepoFromRemote detects the current repository's OWNER/REPO from
// the git remote, using `gh repo view`.
func resolveRepoFromRemote(ctx context.Context) (string, error) {
	out, err := gh.Run(ctx,
		"repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner",
	)
	if err != nil {
		return "", fmt.Errorf("detecting current repo: %w", err)
	}
	repo := strings.TrimSpace(out)
	if repo == "" {
		return "", fmt.Errorf("gh repo view returned empty nameWithOwner")
	}
	return repo, nil
}
