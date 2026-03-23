package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
)

func registerEventCommands(app *command.App, provider *resources.Provider) {
	app.AddCommand(&command.Command{
		Name:  "create_event",
		Title: "Create Event",
		Description: command.Description{
			Short: "Create a new VEVENT in a CalDAV calendar",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "calendar_id", Type: command.String, Description: "Calendar collection ID (from caldav://calendars)", Required: true},
			{Name: "summary", Type: command.String, Description: "Event title", Required: true},
			{Name: "dtstart", Type: command.String, Description: "Start datetime (e.g. 20260401T090000Z or YYYY-MM-DD for all-day)", Required: true},
			{Name: "dtend", Type: command.String, Description: "End datetime (mutually exclusive with duration)"},
			{Name: "duration", Type: command.String, Description: "Duration (e.g. PT1H, PT30M; mutually exclusive with dtend)"},
			{Name: "description", Type: command.String, Description: "Event description/notes"},
			{Name: "location", Type: command.String, Description: "Event location"},
			{Name: "status", Type: command.String, Description: "Event status: TENTATIVE, CONFIRMED, CANCELLED"},
			{Name: "categories", Type: command.Array, Description: "Tags/categories"},
			{Name: "rrule", Type: command.String, Description: "Recurrence rule (e.g. FREQ=WEEKLY;BYDAY=MO,WE,FR)"},
			{Name: "transp", Type: command.String, Description: "Time transparency: OPAQUE (busy) or TRANSPARENT (free)"},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleCreateEvent(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "update_event",
		Title: "Update Event",
		Description: command.Description{
			Short: "Update fields on an existing VEVENT by UID",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Event UID to update", Required: true},
			{Name: "summary", Type: command.String, Description: "New event title"},
			{Name: "dtstart", Type: command.String, Description: "New start datetime"},
			{Name: "dtend", Type: command.String, Description: "New end datetime"},
			{Name: "duration", Type: command.String, Description: "New duration"},
			{Name: "description", Type: command.String, Description: "New description"},
			{Name: "location", Type: command.String, Description: "New location"},
			{Name: "status", Type: command.String, Description: "New status"},
			{Name: "categories", Type: command.Array, Description: "New tags (replaces existing)"},
			{Name: "rrule", Type: command.String, Description: "New recurrence rule"},
			{Name: "transp", Type: command.String, Description: "New time transparency"},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleUpdateEvent(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "delete_event",
		Title: "Delete Event",
		Description: command.Description{
			Short: "Delete a VEVENT by UID",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Event UID to delete", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleDeleteEvent(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "move_event",
		Title: "Move Event",
		Description: command.Description{
			Short: "Move an event between calendars",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Event UID to move", Required: true},
			{Name: "target_calendar_id", Type: command.String, Description: "Destination calendar ID", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleMoveEvent(ctx, args, provider)
		},
	})
}

func handleCreateEvent(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		CalendarID  string   `json:"calendar_id"`
		Summary     string   `json:"summary"`
		DtStart     string   `json:"dtstart"`
		DtEnd       string   `json:"dtend"`
		Duration    string   `json:"duration"`
		Description string   `json:"description"`
		Location    string   `json:"location"`
		Status      string   `json:"status"`
		Categories  []string `json:"categories"`
		RRule       string   `json:"rrule"`
		Transp      string   `json:"transp"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	uid := generateUID()
	event := caldav.Event{
		UID:         uid,
		Summary:     params.Summary,
		DtStart:     params.DtStart,
		DtEnd:       params.DtEnd,
		Duration:    params.Duration,
		Description: params.Description,
		Location:    params.Location,
		Status:      params.Status,
		Categories:  params.Categories,
		RRule:       params.RRule,
		Transp:      params.Transp,
	}

	if event.Status == "" {
		event.Status = "CONFIRMED"
	}

	icalData := caldav.EventToIcal(&event)
	href := params.CalendarID + "/" + uid + ".ics"

	if err := provider.Client().PutEvent(href, icalData, ""); err != nil {
		return command.TextErrorResult(fmt.Sprintf("creating event: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":         uid,
		"calendar_id": params.CalendarID,
		"status":      "created",
	}), nil
}

func handleUpdateEvent(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID         string   `json:"uid"`
		Summary     *string  `json:"summary"`
		DtStart     *string  `json:"dtstart"`
		DtEnd       *string  `json:"dtend"`
		Duration    *string  `json:"duration"`
		Description *string  `json:"description"`
		Location    *string  `json:"location"`
		Status      *string  `json:"status"`
		Categories  []string `json:"categories"`
		RRule       *string  `json:"rrule"`
		Transp      *string  `json:"transp"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	em, _, err := provider.Client().FindEventByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding event: %v", err)), nil
	}

	event := em.Event
	if params.Summary != nil {
		event.Summary = *params.Summary
	}
	if params.DtStart != nil {
		event.DtStart = *params.DtStart
	}
	if params.DtEnd != nil {
		event.DtEnd = *params.DtEnd
	}
	if params.Duration != nil {
		event.Duration = *params.Duration
	}
	if params.Description != nil {
		event.Description = *params.Description
	}
	if params.Location != nil {
		event.Location = *params.Location
	}
	if params.Status != nil {
		event.Status = *params.Status
	}
	if params.Categories != nil {
		event.Categories = params.Categories
	}
	if params.RRule != nil {
		event.RRule = *params.RRule
	}
	if params.Transp != nil {
		event.Transp = *params.Transp
	}

	event.Sequence++
	icalData := caldav.EventToIcal(&event)

	if err := provider.Client().PutEvent(event.Href, icalData, event.ETag); err != nil {
		return command.TextErrorResult(fmt.Sprintf("updating event: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":    params.UID,
		"status": "updated",
	}), nil
}

func handleDeleteEvent(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	em, _, err := provider.Client().FindEventByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding event: %v", err)), nil
	}

	if err := provider.Client().DeleteEvent(em.Event.Href, em.Event.ETag); err != nil {
		return command.TextErrorResult(fmt.Sprintf("deleting event: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":    params.UID,
		"status": "deleted",
	}), nil
}

func handleMoveEvent(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID              string `json:"uid"`
		TargetCalendarID string `json:"target_calendar_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	em, _, err := provider.Client().FindEventByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding event: %v", err)), nil
	}

	newHref := params.TargetCalendarID + "/" + params.UID + ".ics"
	icalData := caldav.EventToIcal(&em.Event)

	if err := provider.Client().PutEvent(newHref, icalData, ""); err != nil {
		return command.TextErrorResult(fmt.Sprintf("creating in target: %v", err)), nil
	}

	if err := provider.Client().DeleteEvent(em.Event.Href, em.Event.ETag); err != nil {
		return command.JSONResult(map[string]any{
			"uid":             params.UID,
			"status":          "moved",
			"warning":         "failed to delete from source calendar",
			"target_calendar": params.TargetCalendarID,
		}), nil
	}

	return command.JSONResult(map[string]string{
		"uid":             params.UID,
		"status":          "moved",
		"target_calendar": params.TargetCalendarID,
	}), nil
}
