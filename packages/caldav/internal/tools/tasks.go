package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
)

func registerTaskCommands(app *command.App, provider *resources.Provider) {
	app.AddCommand(&command.Command{
		Name:  "create_task",
		Title: "Create Task",
		Description: command.Description{
			Short: "Create a new VTODO task in a CalDAV calendar",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(false),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "calendar_id", Type: command.String, Description: "Calendar collection ID (from caldav://calendars)", Required: true},
			{Name: "summary", Type: command.String, Description: "Task title", Required: true},
			{Name: "description", Type: command.String, Description: "Task description/notes"},
			{Name: "status", Type: command.String, Description: "Task status: NEEDS-ACTION, IN-PROCESS, COMPLETED, CANCELLED"},
			{Name: "priority", Type: command.Int, Description: "Priority 0-9 (1=highest, 9=lowest, 0=undefined)"},
			{Name: "due", Type: command.String, Description: "Due date (YYYY-MM-DD) or datetime (RFC 3339)"},
			{Name: "dtstart", Type: command.String, Description: "Start/hide-until date"},
			{Name: "categories", Type: command.Array, Description: "Tags/categories"},
			{Name: "parent_uid", Type: command.String, Description: "Parent task UID (creates a subtask via RELATED-TO)"},
			{Name: "rrule", Type: command.String, Description: "Recurrence rule (e.g., FREQ=DAILY;COUNT=5)"},
			{Name: "location", Type: command.String, Description: "Task location"},
			{Name: "sort_order", Type: command.Int, Description: "Manual sort order (X-APPLE-SORT-ORDER)"},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleCreateTask(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "update_task",
		Title: "Update Task",
		Description: command.Description{
			Short: "Update fields on an existing VTODO task by UID",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Task UID to update", Required: true},
			{Name: "summary", Type: command.String, Description: "New task title"},
			{Name: "description", Type: command.String, Description: "New description"},
			{Name: "status", Type: command.String, Description: "New status: NEEDS-ACTION, IN-PROCESS, COMPLETED, CANCELLED"},
			{Name: "priority", Type: command.Int, Description: "New priority 0-9"},
			{Name: "due", Type: command.String, Description: "New due date"},
			{Name: "dtstart", Type: command.String, Description: "New start date"},
			{Name: "categories", Type: command.Array, Description: "New tags (replaces existing)"},
			{Name: "parent_uid", Type: command.String, Description: "New parent UID (empty to remove)"},
			{Name: "rrule", Type: command.String, Description: "New recurrence rule"},
			{Name: "location", Type: command.String, Description: "New location"},
			{Name: "sort_order", Type: command.Int, Description: "New sort order"},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleUpdateTask(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "complete_task",
		Title: "Complete Task",
		Description: command.Description{
			Short: "Mark a task as completed (sets STATUS=COMPLETED and COMPLETED timestamp)",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(false),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Task UID to complete", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleCompleteTask(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "delete_task",
		Title: "Delete Task",
		Description: command.Description{
			Short: "Delete a VTODO task by UID",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Task UID to delete", Required: true},
		},
		MapsTools: []command.ToolMapping{
			{Replaces: "Bash", CommandPrefixes: []string{"curl.*caldav", "curl.*dav"}, UseWhen: "interacting with CalDAV servers"},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleDeleteTask(ctx, args, provider)
		},
	})

	app.AddCommand(&command.Command{
		Name:  "move_task",
		Title: "Move Task",
		Description: command.Description{
			Short: "Move a task between calendars",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    protocol.BoolPtr(false),
			DestructiveHint: protocol.BoolPtr(true),
			IdempotentHint:  protocol.BoolPtr(true),
			OpenWorldHint:   protocol.BoolPtr(true),
		},
		Params: []command.Param{
			{Name: "uid", Type: command.String, Description: "Task UID to move", Required: true},
			{Name: "target_calendar_id", Type: command.String, Description: "Destination calendar ID", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			return handleMoveTask(ctx, args, provider)
		},
	})
}

func handleCreateTask(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		CalendarID  string   `json:"calendar_id"`
		Summary     string   `json:"summary"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Priority    int      `json:"priority"`
		Due         string   `json:"due"`
		DtStart     string   `json:"dtstart"`
		Categories  []string `json:"categories"`
		ParentUID   string   `json:"parent_uid"`
		RRule       string   `json:"rrule"`
		Location    string   `json:"location"`
		SortOrder   int      `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	uid := generateUID()
	task := caldav.Task{
		UID:         uid,
		Summary:     params.Summary,
		Description: params.Description,
		Status:      params.Status,
		Priority:    params.Priority,
		Due:         params.Due,
		DtStart:     params.DtStart,
		Categories:  params.Categories,
		ParentUID:   params.ParentUID,
		RRule:       params.RRule,
		Location:    params.Location,
		SortOrder:   params.SortOrder,
	}

	if task.Status == "" {
		task.Status = "NEEDS-ACTION"
	}

	icalData := caldav.TaskToIcal(&task)
	href := params.CalendarID + "/" + uid + ".ics"

	if err := provider.Client().PutTask(href, icalData, ""); err != nil {
		return command.TextErrorResult(fmt.Sprintf("creating task: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":         uid,
		"calendar_id": params.CalendarID,
		"status":      "created",
	}), nil
}

func handleUpdateTask(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID         string   `json:"uid"`
		Summary     *string  `json:"summary"`
		Description *string  `json:"description"`
		Status      *string  `json:"status"`
		Priority    *int     `json:"priority"`
		Due         *string  `json:"due"`
		DtStart     *string  `json:"dtstart"`
		Categories  []string `json:"categories"`
		ParentUID   *string  `json:"parent_uid"`
		RRule       *string  `json:"rrule"`
		Location    *string  `json:"location"`
		SortOrder   *int     `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	tm, _, err := provider.Client().FindTaskByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding task: %v", err)), nil
	}

	task := tm.Task
	if params.Summary != nil {
		task.Summary = *params.Summary
	}
	if params.Description != nil {
		task.Description = *params.Description
	}
	if params.Status != nil {
		task.Status = *params.Status
	}
	if params.Priority != nil {
		task.Priority = *params.Priority
	}
	if params.Due != nil {
		task.Due = *params.Due
	}
	if params.DtStart != nil {
		task.DtStart = *params.DtStart
	}
	if params.Categories != nil {
		task.Categories = params.Categories
	}
	if params.ParentUID != nil {
		task.ParentUID = *params.ParentUID
	}
	if params.RRule != nil {
		task.RRule = *params.RRule
	}
	if params.Location != nil {
		task.Location = *params.Location
	}
	if params.SortOrder != nil {
		task.SortOrder = *params.SortOrder
	}

	task.Sequence++
	icalData := caldav.TaskToIcal(&task)

	if err := provider.Client().PutTask(task.Href, icalData, task.ETag); err != nil {
		return command.TextErrorResult(fmt.Sprintf("updating task: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":    params.UID,
		"status": "updated",
	}), nil
}

func handleCompleteTask(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	tm, _, err := provider.Client().FindTaskByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding task: %v", err)), nil
	}

	task := tm.Task
	task.Status = "COMPLETED"
	task.Completed = time.Now().UTC().Format("20060102T150405Z")
	task.PercentComplete = 100
	task.Sequence++

	icalData := caldav.TaskToIcal(&task)
	if err := provider.Client().PutTask(task.Href, icalData, task.ETag); err != nil {
		return command.TextErrorResult(fmt.Sprintf("completing task: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":    params.UID,
		"status": "completed",
	}), nil
}

func handleDeleteTask(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	tm, _, err := provider.Client().FindTaskByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding task: %v", err)), nil
	}

	if err := provider.Client().DeleteTask(tm.Task.Href, tm.Task.ETag); err != nil {
		return command.TextErrorResult(fmt.Sprintf("deleting task: %v", err)), nil
	}

	return command.JSONResult(map[string]string{
		"uid":    params.UID,
		"status": "deleted",
	}), nil
}

func handleMoveTask(_ context.Context, args json.RawMessage, provider *resources.Provider) (*command.Result, error) {
	var params struct {
		UID              string `json:"uid"`
		TargetCalendarID string `json:"target_calendar_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	tm, sourceCalHref, err := provider.Client().FindTaskByUID(params.UID)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("finding task: %v", err)), nil
	}

	_ = sourceCalHref

	// Create in target calendar
	newHref := params.TargetCalendarID + "/" + params.UID + ".ics"
	icalData := caldav.TaskToIcal(&tm.Task)

	if err := provider.Client().PutTask(newHref, icalData, ""); err != nil {
		return command.TextErrorResult(fmt.Sprintf("creating in target: %v", err)), nil
	}

	// Delete from source
	if err := provider.Client().DeleteTask(tm.Task.Href, tm.Task.ETag); err != nil {
		// Best effort — task is already in target
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

func generateUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
