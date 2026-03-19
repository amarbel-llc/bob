package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
)

// Provider implements the mcpserver.ResourceProvider interface with progressive
// disclosure across four tiers.
type Provider struct {
	registry *mcpserver.ResourceRegistry
	client   *caldav.Client
	index    *WordIndex

	// Cache for UID-to-href and UID-to-raw mapping
	mu       sync.RWMutex
	taskMap  map[string]*caldav.TaskWithMeta // uid → task+raw
	calHrefs map[string]string               // uid → calendar href
}

// NewProvider creates a resource provider for CalDAV progressive disclosure.
func NewProvider(client *caldav.Client) *Provider {
	registry := mcpserver.NewResourceRegistry()
	p := &Provider{
		registry: registry,
		client:   client,
		index:    NewWordIndex(),
		taskMap:  make(map[string]*caldav.TaskWithMeta),
		calHrefs: make(map[string]string),
	}
	p.registerResources()
	return p
}

func (p *Provider) registerResources() {
	// Tier 0: Discovery — list all calendars
	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "caldav://calendars",
			Name:        "Calendars",
			Description: "List all CalDAV calendar collections with display name, color, component types, and task count",
			MimeType:    "application/json",
		},
		p.readCalendars,
	)

	// Tier 0: Discovery — word index listing
	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "caldav://task_index",
			Name:        "Task Index",
			Description: "Word-indexed search across all task summaries, descriptions, and categories. Use caldav://task_index/{word} to search.",
			MimeType:    "application/json",
		},
		p.readTaskIndex,
	)

	// Tier 0: Discovery — word search template
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://task_index/{word}",
			Name:        "Task Search",
			Description: "Search tasks by word. Returns metadata-tier results (UID, summary, status, priority, due) for matching tasks.",
			MimeType:    "application/json",
		},
		nil, // Handled by ReadResource prefix matching
	)

	// Tier 1: Metadata — tasks in a calendar
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://calendar/{calendar_id}",
			Name:        "Calendar Tasks",
			Description: "List tasks in a calendar — metadata only: UID, summary, status, priority, due date, has_description, description_tokens. No description payloads.",
			MimeType:    "application/json",
		},
		nil,
	)

	// Tier 2: Content — full task detail
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://task/{uid}",
			Name:        "Task Detail",
			Description: "Full task detail: all VTODO properties parsed into structured JSON, description capped at 4000 chars, subtask UIDs, alarms.",
			MimeType:    "application/json",
		},
		nil,
	)

	// Tier 3: Original — raw iCalendar
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://task/{uid}/ical",
			Name:        "Task iCalendar",
			Description: "Raw iCalendar VCALENDAR text for a task. Can be very large — delegate to a subagent if context budget is tight.",
			MimeType:    "text/calendar",
		},
		nil,
	)
}

// ListResources returns all static resources.
func (p *Provider) ListResources(ctx context.Context) ([]protocol.Resource, error) {
	return p.registry.ListResources(ctx)
}

// ListResourceTemplates returns all template resources.
func (p *Provider) ListResourceTemplates(ctx context.Context) ([]protocol.ResourceTemplate, error) {
	return p.registry.ListResourceTemplates(ctx)
}

// ReadResource routes URI-based reads to the correct tier handler.
func (p *Provider) ReadResource(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	// Tier 0: word search
	if strings.HasPrefix(uri, "caldav://task_index/") {
		word := strings.TrimPrefix(uri, "caldav://task_index/")
		return p.readTaskIndexWord(ctx, uri, word)
	}

	// Tier 3: raw ical (must check before tier 2)
	if strings.HasPrefix(uri, "caldav://task/") && strings.HasSuffix(uri, "/ical") {
		uid := strings.TrimPrefix(uri, "caldav://task/")
		uid = strings.TrimSuffix(uid, "/ical")
		return p.readTaskIcal(ctx, uri, uid)
	}

	// Tier 2: task detail
	if strings.HasPrefix(uri, "caldav://task/") {
		uid := strings.TrimPrefix(uri, "caldav://task/")
		return p.readTaskDetail(ctx, uri, uid)
	}

	// Tier 1: calendar tasks
	if strings.HasPrefix(uri, "caldav://calendar/") {
		calID := strings.TrimPrefix(uri, "caldav://calendar/")
		return p.readCalendarTasks(ctx, uri, calID)
	}

	// Fall through to registry for static resources
	return p.registry.ReadResource(ctx, uri)
}

// --- Tier 0: Calendars ---

type calendarsResponse struct {
	Calendars []calendarInfo `json:"calendars"`
	Total     int            `json:"total"`
	Warnings  []string       `json:"warnings,omitempty"`
}

type calendarInfo struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"display_name"`
	Color          string   `json:"color,omitempty"`
	Description    string   `json:"description,omitempty"`
	ComponentTypes []string `json:"component_types,omitempty"`
	TaskCount      int      `json:"task_count"`
}

func (p *Provider) readCalendars(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	calendars, err := p.client.ListCalendars()
	if err != nil {
		return nil, fmt.Errorf("listing calendars: %w", err)
	}

	var infos []calendarInfo
	var allTasks []caldav.Task
	var warnings []string

	for _, cal := range calendars {
		result, err := p.client.ListTasks(cal.Href)
		taskCount := 0
		if err != nil {
			warnings = append(warnings,
				fmt.Sprintf("calendar %q: failed to list tasks: %v", cal.DisplayName, err))
		} else {
			taskCount = len(result.Tasks)
			for _, t := range result.Tasks {
				allTasks = append(allTasks, t.Task)
				p.cacheTask(t, cal.Href)
			}
			for _, parseErr := range result.ParseErrors {
				warnings = append(warnings,
					fmt.Sprintf("calendar %q: skipped malformed task: %s", cal.DisplayName, parseErr))
			}
		}
		infos = append(infos, calendarInfo{
			ID:             calendarID(cal.Href),
			DisplayName:    cal.DisplayName,
			Color:          cal.Color,
			Description:    cal.Description,
			ComponentTypes: cal.ComponentTypes,
			TaskCount:      taskCount,
		})
	}

	// Rebuild word index with all discovered tasks
	p.index.Build(allTasks)

	resp := calendarsResponse{Calendars: infos, Total: len(infos), Warnings: warnings}
	return jsonResource(uri, resp)
}

// --- Tier 0: Task Index ---

type taskIndexResponse struct {
	Description string `json:"description"`
	Usage       string `json:"usage"`
}

func (p *Provider) readTaskIndex(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	resp := taskIndexResponse{
		Description: "Word-indexed search across all task summaries, descriptions, and categories",
		Usage:       "Read caldav://task_index/{word} to search. Results are metadata-tier (lightweight). Read caldav://calendars first to populate the index.",
	}
	return jsonResource(uri, resp)
}

func (p *Provider) readTaskIndexWord(ctx context.Context, uri, word string) (*protocol.ResourceReadResult, error) {
	uids := p.index.Search(word)

	var results []caldav.TaskMetadata
	p.mu.RLock()
	for _, uid := range uids {
		if tm, ok := p.taskMap[uid]; ok {
			results = append(results, tm.Task.ToMetadata())
		}
	}
	p.mu.RUnlock()

	resp := struct {
		Query   string                `json:"query"`
		Results []caldav.TaskMetadata `json:"results"`
		Total   int                   `json:"total"`
	}{
		Query:   word,
		Results: results,
		Total:   len(results),
	}
	return jsonResource(uri, resp)
}

// --- Tier 1: Calendar Tasks (metadata only) ---

func (p *Provider) readCalendarTasks(ctx context.Context, uri, calID string) (*protocol.ResourceReadResult, error) {
	// Find the calendar href from the ID
	calHref := calendarHrefFromID(calID)

	result, err := p.client.ListTasks(calHref)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}

	var metadata []caldav.TaskMetadata
	for _, t := range result.Tasks {
		p.cacheTask(t, calHref)
		metadata = append(metadata, t.Task.ToMetadata())
	}

	// Resolve subtask relationships
	resolveSubtasks(result.Tasks)

	resp := struct {
		CalendarID string                `json:"calendar_id"`
		Tasks      []caldav.TaskMetadata `json:"tasks"`
		Total      int                   `json:"total"`
		Warnings   []string              `json:"warnings,omitempty"`
	}{
		CalendarID: calID,
		Tasks:      metadata,
		Total:      len(metadata),
		Warnings:   result.ParseErrors,
	}
	return jsonResource(uri, resp)
}

// --- Tier 2: Task Detail ---

func (p *Provider) readTaskDetail(ctx context.Context, uri, uid string) (*protocol.ResourceReadResult, error) {
	tm, err := p.getCachedOrFetch(uid)
	if err != nil {
		return nil, err
	}

	task := tm.Task
	// Cap description at 4000 chars
	task.Description = caldav.CapDescription(task.Description, 4000)

	return jsonResource(uri, task)
}

// --- Tier 3: Raw iCalendar ---

func (p *Provider) readTaskIcal(ctx context.Context, uri, uid string) (*protocol.ResourceReadResult, error) {
	tm, err := p.getCachedOrFetch(uid)
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{{
			URI:      uri,
			MimeType: "text/calendar",
			Text:     tm.Raw,
		}},
	}, nil
}

// --- Helpers ---

func (p *Provider) cacheTask(t caldav.TaskWithMeta, calHref string) {
	p.mu.Lock()
	p.taskMap[t.Task.UID] = &t
	p.calHrefs[t.Task.UID] = calHref
	p.mu.Unlock()
}

func (p *Provider) getCachedOrFetch(uid string) (*caldav.TaskWithMeta, error) {
	p.mu.RLock()
	if tm, ok := p.taskMap[uid]; ok {
		p.mu.RUnlock()
		return tm, nil
	}
	p.mu.RUnlock()

	// Not cached — search all calendars
	tm, calHref, err := p.client.FindTaskByUID(uid)
	if err != nil {
		return nil, fmt.Errorf("finding task %s: %w", uid, err)
	}
	p.cacheTask(*tm, calHref)
	return tm, nil
}

// Client returns the underlying CalDAV client for use by tools.
func (p *Provider) Client() *caldav.Client {
	return p.client
}

// GetCalendarHref returns the cached calendar href for a task UID.
func (p *Provider) GetCalendarHref(uid string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	href, ok := p.calHrefs[uid]
	return href, ok
}

func resolveSubtasks(tasks []caldav.TaskWithMeta) {
	parentMap := make(map[string][]string) // parent_uid → []child_uid
	for _, t := range tasks {
		if t.Task.ParentUID != "" {
			parentMap[t.Task.ParentUID] = append(parentMap[t.Task.ParentUID], t.Task.UID)
		}
	}
	for i := range tasks {
		if children, ok := parentMap[tasks[i].Task.UID]; ok {
			tasks[i].Task.SubtaskUIDs = children
		}
	}
}

func calendarID(href string) string {
	href = strings.TrimRight(href, "/")
	parts := strings.Split(href, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return href
}

func calendarHrefFromID(id string) string {
	// The ID is the last path segment; return it as-is and let the client resolve
	return id + "/"
}

func jsonResource(uri string, data any) (*protocol.ResourceReadResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{{
			URI:      uri,
			MimeType: "application/json",
			Text:     string(b),
		}},
	}, nil
}
