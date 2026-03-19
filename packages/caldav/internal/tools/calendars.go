package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
)

func registerCalendarCommands(app *command.App, provider *resources.Provider) {
	app.AddCommand(&command.Command{
		Name:  "create_calendar",
		Title: "Create Calendar",
		Description: command.Description{
			Short: "Create a new CalDAV calendar collection for VTODO tasks",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "name", Type: command.String, Description: "Display name for the calendar", Required: true},
			{Name: "description", Type: command.String, Description: "Calendar description"},
			{Name: "id", Type: command.String, Description: "Calendar ID/slug (used in the URL path). Defaults to a sanitized version of the name."},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleCreateCalendar(ctx, args, provider)
		},
	})
}

func handleCreateCalendar(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ID          string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	calID := params.ID
	if calID == "" {
		calID = sanitizeID(params.Name)
	}

	href := calID + "/"
	if err := provider.Client().MkCalendar(href, params.Name, params.Description); err != nil {
		return command.TextErrorResult(fmt.Sprintf("creating calendar: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"id":     calID,
		"name":   params.Name,
		"status": "created",
	}), nil
}

func sanitizeID(name string) string {
	var result []byte
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		} else if c == ' ' || c == '_' {
			result = append(result, '-')
		}
	}
	if len(result) == 0 {
		return "calendar"
	}
	return string(result)
}
