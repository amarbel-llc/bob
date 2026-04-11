package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
)

func registerReadCommands(app *command.App, provider *resources.Provider) {
	app.AddCommand(&command.Command{
		Name:  "list_calendars",
		Title: "List Calendars",
		Description: command.Description{
			Short: "List all CalDAV calendar collections",
			Long: `Lists all calendar collections on the CalDAV server with display name, color,
component types, task count, and event count. This is the bootstrap call —
it populates the internal cache and word index used by search_tasks,
search_events, list_recurring_tasks, and list_recurring_events. Call this
before using those tools.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Examples: []command.Example{
			{Description: "List all calendars", Command: "caldav list_calendars"},
		},
		SeeAlso: []string{"caldav-list_tasks", "caldav-search_tasks"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleListCalendars(ctx, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "search_tasks",
		Title: "Search Tasks",
		Description: command.Description{
			Short: "Search tasks by keyword",
			Long: `Searches the word index across all task summaries, descriptions, categories,
and calendar names. Returns metadata-tier results (UID, summary, status,
priority, due). Call list_calendars first to populate the index.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "query", Type: command.String, Description: "Search word", Required: true},
		},
		Examples: []command.Example{
			{Description: "Search for grocery tasks", Command: "caldav search_tasks --query grocery"},
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_task"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleSearchTasks(args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "list_tasks",
		Title: "List Tasks",
		Description: command.Description{
			Short: "List tasks in a calendar",
			Long: `Lists all tasks in a specific calendar collection. Returns metadata only:
UID, summary, status, priority, due date, has_description, description_tokens.
No description payloads — use get_task for full detail.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "calendar_id", Type: command.String, Description: "Calendar collection ID (from list_calendars)", Required: true},
		},
		Examples: []command.Example{
			{Description: "List tasks in inbox", Command: "caldav list_tasks --calendar_id inbox"},
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_task"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleListTasks(args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "list_recurring_tasks",
		Title: "List Recurring Tasks",
		Description: command.Description{
			Short: "List all tasks with recurrence rules",
			Long: `Returns metadata for all cached tasks that have an RRULE recurrence rule.
Call list_calendars first to populate the cache.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_task"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleListRecurringTasks(provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "get_task",
		Title: "Get Task",
		Description: command.Description{
			Short: "Get full task detail or raw iCalendar by UID",
			Long: `Returns the full task detail for a given UID. By default returns structured
JSON with all VTODO properties, description capped at 4000 chars, subtask UIDs,
and alarms. Use --format ical for the raw iCalendar VCALENDAR text.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Task UID", Required: true},
			{Name: "format", Type: command.String, Description: "Output format: json (default) or ical", Default: "json"},
		},
		Examples: []command.Example{
			{Description: "Get task detail", Command: "caldav get_task --uid abc123"},
			{Description: "Get raw iCal", Command: "caldav get_task --uid abc123 --format ical"},
		},
		SeeAlso: []string{"caldav-list_tasks", "caldav-search_tasks"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleGetTask(args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "search_events",
		Title: "Search Events",
		Description: command.Description{
			Short: "Search events by keyword",
			Long: `Searches the word index across all event summaries, descriptions, locations,
categories, and calendar names. Returns metadata-tier results.
Call list_calendars first to populate the index.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "query", Type: command.String, Description: "Search word", Required: true},
		},
		Examples: []command.Example{
			{Description: "Search for meeting events", Command: "caldav search_events --query meeting"},
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_event"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleSearchEvents(args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "list_events",
		Title: "List Events",
		Description: command.Description{
			Short: "List events in a calendar",
			Long: `Lists all events in a specific calendar collection. Returns metadata only:
UID, summary, dtstart, dtend, location, status, rrule.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "calendar_id", Type: command.String, Description: "Calendar collection ID (from list_calendars)", Required: true},
		},
		Examples: []command.Example{
			{Description: "List events in personal calendar", Command: "caldav list_events --calendar_id personal"},
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_event"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleListEvents(args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "list_recurring_events",
		Title: "List Recurring Events",
		Description: command.Description{
			Short: "List all events with recurrence rules",
			Long: `Returns metadata for all cached events that have an RRULE recurrence rule.
Call list_calendars first to populate the cache.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		SeeAlso: []string{"caldav-list_calendars", "caldav-get_event"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleListRecurringEvents(provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "get_event",
		Title: "Get Event",
		Description: command.Description{
			Short: "Get full event detail or raw iCalendar by UID",
			Long: `Returns the full event detail for a given UID. By default returns structured
JSON with all VEVENT properties, description capped at 4000 chars, attendees,
and alarms. Use --format ical for the raw iCalendar VCALENDAR text.`,
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:  protocol.BoolPtr(true),
			OpenWorldHint: protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Event UID", Required: true},
			{Name: "format", Type: command.String, Description: "Output format: json (default) or ical", Default: "json"},
		},
		Examples: []command.Example{
			{Description: "Get event detail", Command: "caldav get_event --uid abc123"},
			{Description: "Get raw iCal", Command: "caldav get_event --uid abc123 --format ical"},
		},
		SeeAlso: []string{"caldav-list_events", "caldav-search_events"},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleGetEvent(args, provider)
		},
	})
}

// --- Handlers ---

func handleListCalendars(_ context.Context, provider *resources.Provider) (*command.Result, error) {
	infos, warnings, err := provider.LoadCalendars()
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("listing calendars: %v", err)), nil
	}
	return command.JSONResult(map[string]any{
		"calendars": infos,
		"total":     len(infos),
		"warnings":  warnings,
	}), nil
}

func handleSearchTasks(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	results := provider.SearchTasks(params.Query)
	return command.JSONResult(map[string]any{
		"query":   params.Query,
		"results": results,
		"total":   len(results),
	}), nil
}

func handleListTasks(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		CalendarID string `json:"calendar_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	metadata, warnings, err := provider.ListCalendarTasks(params.CalendarID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("listing tasks: %v", err)), nil
	}
	return command.JSONResult(map[string]any{
		"calendar_id": params.CalendarID,
		"tasks":       metadata,
		"total":       len(metadata),
		"warnings":    warnings,
	}), nil
}

func handleListRecurringTasks(provider *resources.Provider) (*command.Result, error) {
	results := provider.GetRecurringTasks()
	return command.JSONResult(map[string]any{
		"tasks": results,
		"total": len(results),
	}), nil
}

func handleGetTask(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID    string `json:"uid"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Format == "" {
		params.Format = "json"
	}

	if params.Format == "ical" {
		raw, err := provider.GetTaskIcal(params.UID)
		if err != nil {
			return command.TextErrorResult(fmt.Sprintf("getting task iCal: %v", err)), nil
		}
		return command.TextResult(raw), nil
	}

	task, err := provider.GetTask(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("getting task: %v", err)), nil
	}
	return command.JSONResult(task), nil
}

func handleSearchEvents(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	results := provider.SearchEvents(params.Query)
	return command.JSONResult(map[string]any{
		"query":   params.Query,
		"results": results,
		"total":   len(results),
	}), nil
}

func handleListEvents(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		CalendarID string `json:"calendar_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	metadata, warnings, err := provider.ListCalendarEvents(params.CalendarID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("listing events: %v", err)), nil
	}
	return command.JSONResult(map[string]any{
		"calendar_id": params.CalendarID,
		"events":      metadata,
		"total":       len(metadata),
		"warnings":    warnings,
	}), nil
}

func handleListRecurringEvents(provider *resources.Provider) (*command.Result, error) {
	results := provider.GetRecurringEvents()
	return command.JSONResult(map[string]any{
		"events": results,
		"total":  len(results),
	}), nil
}

func handleGetEvent(args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID    string `json:"uid"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Format == "" {
		params.Format = "json"
	}

	if params.Format == "ical" {
		raw, err := provider.GetEventIcal(params.UID)
		if err != nil {
			return command.TextErrorResult(fmt.Sprintf("getting event iCal: %v", err)), nil
		}
		return command.TextResult(raw), nil
	}

	event, err := provider.GetEvent(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("getting event: %v", err)), nil
	}
	return command.JSONResult(event), nil
}
