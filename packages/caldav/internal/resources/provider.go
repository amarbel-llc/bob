package resources

import (
	"fmt"
	"strings"
	"sync"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
)

// Provider manages CalDAV data caching, word indexing, and data access.
type Provider struct {
	client *caldav.Client
	index  *WordIndex

	mu         sync.RWMutex
	taskMap    map[string]*caldav.TaskWithMeta
	eventMap   map[string]*caldav.EventWithMeta
	calHrefs   map[string]string
	eventIndex *WordIndex
}

// NewProvider creates a data provider backed by the given CalDAV client.
func NewProvider(client *caldav.Client) *Provider {
	return &Provider{
		client:     client,
		index:      NewWordIndex(),
		taskMap:    make(map[string]*caldav.TaskWithMeta),
		eventMap:   make(map[string]*caldav.EventWithMeta),
		calHrefs:   make(map[string]string),
		eventIndex: NewWordIndex(),
	}
}

// CalendarInfo is the structured output for a calendar listing.
type CalendarInfo struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"display_name"`
	Color          string   `json:"color,omitempty"`
	Description    string   `json:"description,omitempty"`
	ComponentTypes []string `json:"component_types,omitempty"`
	TaskCount      int      `json:"task_count"`
	EventCount     int      `json:"event_count"`
}

// LoadCalendars fetches all calendars, populates caches and word indexes.
// Returns calendar info, warnings, and any error.
func (p *Provider) LoadCalendars() ([]CalendarInfo, []string, error) {
	calendars, err := p.client.ListCalendars()
	if err != nil {
		return nil, nil, fmt.Errorf("listing calendars: %w", err)
	}

	var infos []CalendarInfo
	var taskItems []IndexItem
	var eventItems []IndexItem
	var warnings []string

	for _, cal := range calendars {
		taskCount := 0
		if len(cal.ComponentTypes) == 0 || hasComponentType(cal.ComponentTypes, "VTODO") {
			result, err := p.client.ListTasks(cal.Href)
			if err != nil {
				warnings = append(warnings,
					fmt.Sprintf("calendar %q: failed to list tasks: %v", cal.DisplayName, err))
			} else {
				taskCount = len(result.Tasks)
				for _, t := range result.Tasks {
					p.cacheTask(t, cal.Href)
					text := t.Task.Summary + " " + t.Task.Description + " " + cal.DisplayName
					if len(t.Task.Categories) > 0 {
						text += " " + strings.Join(t.Task.Categories, " ")
					}
					taskItems = append(taskItems, IndexItem{UID: t.Task.UID, Text: text})
				}
				for _, parseErr := range result.ParseErrors {
					warnings = append(warnings,
						fmt.Sprintf("calendar %q: skipped malformed task: %s", cal.DisplayName, parseErr))
				}
			}
		}

		eventCount := 0
		if hasComponentType(cal.ComponentTypes, "VEVENT") {
			eventResult, err := p.client.ListEvents(cal.Href)
			if err != nil {
				warnings = append(warnings,
					fmt.Sprintf("calendar %q: failed to list events: %v", cal.DisplayName, err))
			} else {
				eventCount = len(eventResult.Events)
				for _, e := range eventResult.Events {
					p.cacheEvent(e, cal.Href)
					text := e.Event.Summary + " " + e.Event.Description + " " + e.Event.Location + " " + cal.DisplayName
					if len(e.Event.Categories) > 0 {
						text += " " + strings.Join(e.Event.Categories, " ")
					}
					eventItems = append(eventItems, IndexItem{UID: e.Event.UID, Text: text})
				}
				for _, parseErr := range eventResult.ParseErrors {
					warnings = append(warnings,
						fmt.Sprintf("calendar %q: skipped malformed event: %s", cal.DisplayName, parseErr))
				}
			}
		}

		infos = append(infos, CalendarInfo{
			ID:             calendarID(cal.Href),
			DisplayName:    cal.DisplayName,
			Color:          cal.Color,
			Description:    cal.Description,
			ComponentTypes: cal.ComponentTypes,
			TaskCount:      taskCount,
			EventCount:     eventCount,
		})
	}

	p.index.BuildFromItems(taskItems)
	p.eventIndex.BuildFromItems(eventItems)

	return infos, warnings, nil
}

// SearchTasks returns task metadata matching the given word.
func (p *Provider) SearchTasks(word string) []caldav.TaskMetadata {
	uids := p.index.Search(word)
	var results []caldav.TaskMetadata
	p.mu.RLock()
	for _, uid := range uids {
		if tm, ok := p.taskMap[uid]; ok {
			results = append(results, tm.Task.ToMetadata())
		}
	}
	p.mu.RUnlock()
	return results
}

// SearchEvents returns event metadata matching the given word.
func (p *Provider) SearchEvents(word string) []caldav.EventMetadata {
	uids := p.eventIndex.Search(word)
	var results []caldav.EventMetadata
	p.mu.RLock()
	for _, uid := range uids {
		if em, ok := p.eventMap[uid]; ok {
			results = append(results, em.Event.ToMetadata())
		}
	}
	p.mu.RUnlock()
	return results
}

// ListCalendarTasks lists task metadata for a specific calendar.
func (p *Provider) ListCalendarTasks(calID string) ([]caldav.TaskMetadata, []string, error) {
	calHref := calendarHrefFromID(calID)
	result, err := p.client.ListTasks(calHref)
	if err != nil {
		return nil, nil, fmt.Errorf("listing tasks: %w", err)
	}

	var metadata []caldav.TaskMetadata
	for _, t := range result.Tasks {
		p.cacheTask(t, calHref)
		metadata = append(metadata, t.Task.ToMetadata())
	}
	resolveSubtasks(result.Tasks)

	return metadata, result.ParseErrors, nil
}

// ListCalendarEvents lists event metadata for a specific calendar.
func (p *Provider) ListCalendarEvents(calID string) ([]caldav.EventMetadata, []string, error) {
	calHref := calendarHrefFromID(calID)
	result, err := p.client.ListEvents(calHref)
	if err != nil {
		return nil, nil, fmt.Errorf("listing events: %w", err)
	}

	var metadata []caldav.EventMetadata
	for _, e := range result.Events {
		p.cacheEvent(e, calHref)
		metadata = append(metadata, e.Event.ToMetadata())
	}

	return metadata, result.ParseErrors, nil
}

// GetRecurringTasks returns metadata for all cached tasks with RRULE.
func (p *Provider) GetRecurringTasks() []caldav.TaskMetadata {
	var results []caldav.TaskMetadata
	p.mu.RLock()
	for _, tm := range p.taskMap {
		if tm.Task.RRule != "" {
			results = append(results, tm.Task.ToMetadata())
		}
	}
	p.mu.RUnlock()
	return results
}

// GetRecurringEvents returns metadata for all cached events with RRULE.
func (p *Provider) GetRecurringEvents() []caldav.EventMetadata {
	var results []caldav.EventMetadata
	p.mu.RLock()
	for _, em := range p.eventMap {
		if em.Event.RRule != "" {
			results = append(results, em.Event.ToMetadata())
		}
	}
	p.mu.RUnlock()
	return results
}

// GetTask returns full task detail with description capped at 4000 chars.
func (p *Provider) GetTask(uid string) (*caldav.Task, error) {
	tm, err := p.getCachedOrFetch(uid)
	if err != nil {
		return nil, err
	}
	task := tm.Task
	task.Description = caldav.CapDescription(task.Description, 4000)
	return &task, nil
}

// GetTaskIcal returns the raw iCalendar text for a task.
func (p *Provider) GetTaskIcal(uid string) (string, error) {
	tm, err := p.getCachedOrFetch(uid)
	if err != nil {
		return "", err
	}
	return tm.Raw, nil
}

// GetEvent returns full event detail with description capped at 4000 chars.
func (p *Provider) GetEvent(uid string) (*caldav.Event, error) {
	p.mu.RLock()
	em, ok := p.eventMap[uid]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("event %s not found in cache — call list_calendars first", uid)
	}
	event := em.Event
	event.Description = caldav.CapDescription(event.Description, 4000)
	return &event, nil
}

// GetEventIcal returns the raw iCalendar text for an event.
func (p *Provider) GetEventIcal(uid string) (string, error) {
	p.mu.RLock()
	em, ok := p.eventMap[uid]
	p.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("event %s not found in cache — call list_calendars first", uid)
	}
	return em.Raw, nil
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

// --- Internal helpers ---

func (p *Provider) cacheTask(t caldav.TaskWithMeta, calHref string) {
	p.mu.Lock()
	p.taskMap[t.Task.UID] = &t
	p.calHrefs[t.Task.UID] = calHref
	p.mu.Unlock()
}

func (p *Provider) cacheEvent(e caldav.EventWithMeta, calHref string) {
	p.mu.Lock()
	p.eventMap[e.Event.UID] = &e
	p.calHrefs[e.Event.UID] = calHref
	p.mu.Unlock()
}

func (p *Provider) getCachedOrFetch(uid string) (*caldav.TaskWithMeta, error) {
	p.mu.RLock()
	if tm, ok := p.taskMap[uid]; ok {
		p.mu.RUnlock()
		return tm, nil
	}
	p.mu.RUnlock()

	tm, calHref, err := p.client.FindTaskByUID(uid)
	if err != nil {
		return nil, fmt.Errorf("finding task %s: %w", uid, err)
	}
	p.cacheTask(*tm, calHref)
	return tm, nil
}

func resolveSubtasks(tasks []caldav.TaskWithMeta) {
	parentMap := make(map[string][]string)
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
	return id + "/"
}

func hasComponentType(types []string, target string) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}
