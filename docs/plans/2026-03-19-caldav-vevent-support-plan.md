# CalDAV VEVENT Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add read-only VEVENT support to the CalDAV MCP with progressive disclosure resources, word search, and recurring event filtering.

**Architecture:** Parallel Event/EventMetadata types alongside existing Task/TaskMetadata. Separate event cache and word index in the provider. Eager fetching of events during `caldav://calendars` read, based on calendar `ComponentTypes`. New `caldav://event*` and `caldav://events/*` resource namespace.

**Tech Stack:** Go, go-mcp framework, iCalendar RFC 5545

**Rollback:** Purely additive. Revert commits to remove. No existing functionality changes except `event_count` field added to calendars response.

---

### Task 1: Generalize WordIndex to accept arbitrary items

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/caldav/internal/resources/index.go:25-49`
- Modify: `packages/caldav/internal/resources/index_test.go`
- Modify: `packages/caldav/internal/resources/provider.go:186,217`

**Step 1: Write the failing test**

Add to `packages/caldav/internal/resources/index_test.go`:

```go
func TestWordIndex_BuildFromItems(t *testing.T) {
	idx := NewWordIndex()

	items := []IndexItem{
		{UID: "e1", Text: "Team standup meeting"},
		{UID: "e2", Text: "Dentist appointment downtown"},
		{UID: "e3", Text: "Team offsite planning"},
	}

	idx.BuildFromItems(items)

	results := idx.Search("team")
	sort.Strings(results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
	if results[0] != "e1" || results[1] != "e3" {
		t.Errorf("expected [e1, e3], got %v", results)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop .#go -c go test -run TestWordIndex_BuildFromItems ./packages/caldav/internal/resources/`
Expected: FAIL — `IndexItem` and `BuildFromItems` not defined

**Step 3: Write minimal implementation**

In `packages/caldav/internal/resources/index.go`, add:

```go
// IndexItem is a generic item for word indexing.
type IndexItem struct {
	UID  string
	Text string
}

// BuildFromItems rebuilds the index from generic items.
func (idx *WordIndex) BuildFromItems(items []IndexItem) {
	newIndex := make(map[string][]string)
	seen := make(map[string]map[string]bool)

	for _, item := range items {
		words := extractWords(item.Text)
		for _, w := range words {
			if seen[w] == nil {
				seen[w] = make(map[string]bool)
			}
			if !seen[w][item.UID] {
				seen[w][item.UID] = true
				newIndex[w] = append(newIndex[w], item.UID)
			}
		}
	}

	idx.mu.Lock()
	idx.index = newIndex
	idx.mu.Unlock()
}
```

Refactor `Build(tasks []caldav.Task)` to delegate:

```go
func (idx *WordIndex) Build(tasks []caldav.Task) {
	items := make([]IndexItem, len(tasks))
	for i, t := range tasks {
		text := t.Summary + " " + t.Description
		if len(t.Categories) > 0 {
			text += " " + strings.Join(t.Categories, " ")
		}
		items[i] = IndexItem{UID: t.UID, Text: text}
	}
	idx.BuildFromItems(items)
}
```

**Step 4: Run tests to verify they pass**

Run: `nix develop .#go -c go test ./packages/caldav/internal/resources/`
Expected: ALL PASS (existing `TestWordIndex_*` + new `TestWordIndex_BuildFromItems`)

**Step 5: Commit**

```
git add packages/caldav/internal/resources/index.go packages/caldav/internal/resources/index_test.go
git commit -m "Generalize WordIndex with BuildFromItems for reuse by event index"
```

---

### Task 2: Add Event and EventMetadata types with ParseVEVENT

**Promotion criteria:** N/A

**Files:**
- Create: `packages/caldav/internal/caldav/event.go`
- Create: `packages/caldav/internal/caldav/event_test.go`

**Step 1: Write the failing test**

Create `packages/caldav/internal/caldav/event_test.go`:

```go
package caldav

import (
	"strings"
	"testing"
)

func TestParseVEVENT(t *testing.T) {
	raw := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
DTSTAMP:20260315T120000Z
UID:event-test-1
CREATED:20260315T100000Z
LAST-MODIFIED:20260315T110000Z
SUMMARY:Team standup
DESCRIPTION:Daily standup meeting
DTSTART;TZID=America/New_York:20260401T093000
DTEND;TZID=America/New_York:20260401T100000
LOCATION:Conference Room A
CATEGORIES:Work,Meetings
STATUS:CONFIRMED
TRANSP:OPAQUE
ORGANIZER:mailto:boss@example.com
ATTENDEE:mailto:alice@example.com
ATTENDEE:mailto:bob@example.com
RRULE:FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR
SEQUENCE:2
BEGIN:VALARM
TRIGGER:-PT15M
ACTION:DISPLAY
DESCRIPTION:Reminder
END:VALARM
END:VEVENT
END:VCALENDAR`

	event, err := ParseVEVENT(raw)
	if err != nil {
		t.Fatalf("ParseVEVENT: %v", err)
	}

	if event.UID != "event-test-1" {
		t.Errorf("UID = %q, want %q", event.UID, "event-test-1")
	}
	if event.Summary != "Team standup" {
		t.Errorf("Summary = %q, want %q", event.Summary, "Team standup")
	}
	if event.Description != "Daily standup meeting" {
		t.Errorf("Description = %q, want %q", event.Description, "Daily standup meeting")
	}
	if event.DtStart != "20260401T093000" {
		t.Errorf("DtStart = %q, want %q", event.DtStart, "20260401T093000")
	}
	if event.DtEnd != "20260401T100000" {
		t.Errorf("DtEnd = %q, want %q", event.DtEnd, "20260401T100000")
	}
	if event.Location != "Conference Room A" {
		t.Errorf("Location = %q, want %q", event.Location, "Conference Room A")
	}
	if event.Status != "CONFIRMED" {
		t.Errorf("Status = %q, want %q", event.Status, "CONFIRMED")
	}
	if event.Transp != "OPAQUE" {
		t.Errorf("Transp = %q, want %q", event.Transp, "OPAQUE")
	}
	if event.Organizer != "mailto:boss@example.com" {
		t.Errorf("Organizer = %q", event.Organizer)
	}
	if len(event.Attendees) != 2 {
		t.Fatalf("len(Attendees) = %d, want 2", len(event.Attendees))
	}
	if event.RRule != "FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR" {
		t.Errorf("RRule = %q", event.RRule)
	}
	if len(event.Categories) != 2 || event.Categories[0] != "Work" {
		t.Errorf("Categories = %v", event.Categories)
	}
	if event.Sequence != 2 {
		t.Errorf("Sequence = %d, want 2", event.Sequence)
	}
	if len(event.Alarms) != 1 {
		t.Fatalf("len(Alarms) = %d, want 1", len(event.Alarms))
	}
	if event.Alarms[0].Trigger != "-PT15M" {
		t.Errorf("Alarm.Trigger = %q", event.Alarms[0].Trigger)
	}
	if !event.HasDescription {
		t.Error("HasDescription = false, want true")
	}
}

func TestParseVEVENT_Minimal(t *testing.T) {
	raw := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:minimal-event
SUMMARY:Quick chat
DTSTART:20260401T140000Z
END:VEVENT
END:VCALENDAR`

	event, err := ParseVEVENT(raw)
	if err != nil {
		t.Fatalf("ParseVEVENT: %v", err)
	}

	if event.UID != "minimal-event" {
		t.Errorf("UID = %q", event.UID)
	}
	if event.Summary != "Quick chat" {
		t.Errorf("Summary = %q", event.Summary)
	}
	if event.HasDescription {
		t.Error("HasDescription = true, want false")
	}
}

func TestParseVEVENT_NoVEVENT(t *testing.T) {
	raw := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VTODO
UID:task1
SUMMARY:A task
END:VTODO
END:VCALENDAR`

	_, err := ParseVEVENT(raw)
	if err == nil {
		t.Error("expected error for missing VEVENT, got nil")
	}
}

func TestParseVEVENT_Duration(t *testing.T) {
	raw := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:duration-event
SUMMARY:Workshop
DTSTART:20260401T090000Z
DURATION:PT2H30M
END:VEVENT
END:VCALENDAR`

	event, err := ParseVEVENT(raw)
	if err != nil {
		t.Fatalf("ParseVEVENT: %v", err)
	}

	if event.Duration != "PT2H30M" {
		t.Errorf("Duration = %q, want %q", event.Duration, "PT2H30M")
	}
}

func TestEventToMetadata(t *testing.T) {
	event := &Event{
		UID:               "uid-1",
		Summary:           "Meeting",
		Status:            "CONFIRMED",
		DtStart:           "20260401T090000Z",
		DtEnd:             "20260401T100000Z",
		Location:          "Room A",
		Categories:        []string{"Work"},
		RRule:             "FREQ=WEEKLY",
		HasDescription:    true,
		DescriptionTokens: 25,
		Description:       "some long description",
		Organizer:         "mailto:boss@example.com",
	}

	meta := event.ToMetadata()
	if meta.UID != "uid-1" {
		t.Errorf("UID = %q", meta.UID)
	}
	if meta.Location != "Room A" {
		t.Errorf("Location = %q", meta.Location)
	}
	if meta.RRule != "FREQ=WEEKLY" {
		t.Errorf("RRule = %q", meta.RRule)
	}
	if !meta.HasDescription {
		t.Error("HasDescription should be true")
	}
}

func TestParseVEVENT_FoldedLines(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fold-test\r\nSUMMARY:This is a\r\n  folded summary\r\nDTSTART:20260401T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR"

	event, err := ParseVEVENT(raw)
	if err != nil {
		t.Fatalf("ParseVEVENT: %v", err)
	}

	if event.Summary != "This is a folded summary" {
		t.Errorf("Summary = %q, want %q", event.Summary, "This is a folded summary")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop .#go -c go test -run TestParseVEVENT ./packages/caldav/internal/caldav/`
Expected: FAIL — `ParseVEVENT`, `Event`, `EventMetadata` not defined

**Step 3: Write minimal implementation**

Create `packages/caldav/internal/caldav/event.go`:

```go
package caldav

import (
	"fmt"
	"strconv"
	"strings"
)

// Event is the structured representation of a VEVENT component.
type Event struct {
	UID          string   `json:"uid"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description,omitempty"`
	Status       string   `json:"status,omitempty"`
	Location     string   `json:"location,omitempty"`
	Geo          string   `json:"geo,omitempty"`
	DtStart      string   `json:"dtstart,omitempty"`
	DtEnd        string   `json:"dtend,omitempty"`
	Duration     string   `json:"duration,omitempty"`
	Organizer    string   `json:"organizer,omitempty"`
	Attendees    []string `json:"attendees,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	RRule        string   `json:"rrule,omitempty"`
	RecurrenceID string   `json:"recurrence_id,omitempty"`
	Sequence     int      `json:"sequence,omitempty"`
	Transp       string   `json:"transp,omitempty"`
	Created      string   `json:"created,omitempty"`
	LastModified string   `json:"last_modified,omitempty"`

	// Derived fields
	Alarms            []Alarm `json:"alarms,omitempty"`
	HasDescription    bool    `json:"has_description"`
	DescriptionTokens int     `json:"description_tokens"`

	// Server metadata
	Href string `json:"href,omitempty"`
	ETag string `json:"etag,omitempty"`
}

// EventMetadata is the lightweight tier-1 view of an event.
type EventMetadata struct {
	UID               string   `json:"uid"`
	Summary           string   `json:"summary"`
	Status            string   `json:"status,omitempty"`
	DtStart           string   `json:"dtstart,omitempty"`
	DtEnd             string   `json:"dtend,omitempty"`
	Location          string   `json:"location,omitempty"`
	Categories        []string `json:"categories,omitempty"`
	RRule             string   `json:"rrule,omitempty"`
	HasDescription    bool     `json:"has_description"`
	DescriptionTokens int      `json:"description_tokens"`
}

// ToMetadata converts a full Event to its lightweight metadata representation.
func (e *Event) ToMetadata() EventMetadata {
	return EventMetadata{
		UID:               e.UID,
		Summary:           e.Summary,
		Status:            e.Status,
		DtStart:           e.DtStart,
		DtEnd:             e.DtEnd,
		Location:          e.Location,
		Categories:        e.Categories,
		RRule:             e.RRule,
		HasDescription:    e.HasDescription,
		DescriptionTokens: e.DescriptionTokens,
	}
}

// EventWithMeta pairs a parsed Event with its raw iCalendar text.
type EventWithMeta struct {
	Event Event
	Raw   string
}

// EventListResult holds the results of listing events, including any parse
// errors for individual events that could not be parsed.
type EventListResult struct {
	Events      []EventWithMeta
	ParseErrors []string
}

// ParseVEVENT parses a raw iCalendar string and extracts the first VEVENT as an Event.
func ParseVEVENT(raw string) (*Event, error) {
	lines := unfoldLines(raw)

	inVEVENT := false
	inVALARM := false
	e := &Event{}
	var currentAlarm *Alarm

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if trimmed == "BEGIN:VEVENT" {
			inVEVENT = true
			continue
		}
		if trimmed == "END:VEVENT" {
			inVEVENT = false
			break
		}
		if !inVEVENT {
			continue
		}

		if trimmed == "BEGIN:VALARM" {
			inVALARM = true
			currentAlarm = &Alarm{}
			continue
		}
		if trimmed == "END:VALARM" {
			inVALARM = false
			if currentAlarm != nil {
				e.Alarms = append(e.Alarms, *currentAlarm)
				currentAlarm = nil
			}
			continue
		}

		name, value := parsePropLine(trimmed)

		if inVALARM && currentAlarm != nil {
			switch propName(name) {
			case "TRIGGER":
				currentAlarm.Trigger = value
			case "ACTION":
				currentAlarm.Action = value
			case "DESCRIPTION":
				currentAlarm.Description = value
			}
			continue
		}

		switch propName(name) {
		case "UID":
			e.UID = value
		case "SUMMARY":
			e.Summary = value
		case "DESCRIPTION":
			e.Description = value
		case "STATUS":
			e.Status = value
		case "LOCATION":
			e.Location = value
		case "GEO":
			e.Geo = value
		case "DTSTART":
			e.DtStart = value
		case "DTEND":
			e.DtEnd = value
		case "DURATION":
			e.Duration = value
		case "ORGANIZER":
			e.Organizer = value
		case "ATTENDEE":
			e.Attendees = append(e.Attendees, value)
		case "CATEGORIES":
			cats := strings.Split(value, ",")
			for i := range cats {
				cats[i] = strings.TrimSpace(cats[i])
			}
			e.Categories = cats
		case "RRULE":
			e.RRule = value
		case "RECURRENCE-ID":
			e.RecurrenceID = value
		case "TRANSP":
			e.Transp = value
		case "CREATED":
			e.Created = value
		case "LAST-MODIFIED":
			e.LastModified = value
		case "SEQUENCE":
			if n, err := strconv.Atoi(value); err == nil {
				e.Sequence = n
			}
		}
	}

	if e.UID == "" {
		return nil, fmt.Errorf("VEVENT missing UID")
	}

	e.HasDescription = e.Description != ""
	e.DescriptionTokens = len(e.Description) / 4

	return e, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `nix develop .#go -c go test ./packages/caldav/internal/caldav/`
Expected: ALL PASS

**Step 5: Commit**

```
git add packages/caldav/internal/caldav/event.go packages/caldav/internal/caldav/event_test.go
git commit -m "Add Event types and ParseVEVENT parser"
```

---

### Task 3: Add ListEvents to client

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/caldav/internal/caldav/client.go`

**Step 1: Write ListEvents**

Add to `packages/caldav/internal/caldav/client.go` after `ListTasks`:

```go
// ListEvents performs a REPORT calendar-query to list all VEVENTs in a calendar.
// Parse errors for individual events are collected in EventListResult.ParseErrors
// rather than causing the entire listing to fail.
func (c *Client) ListEvents(calendarHref string) (*EventListResult, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:getetag />
    <c:calendar-data />
  </d:prop>
  <c:filter>
    <c:comp-filter name="VCALENDAR">
      <c:comp-filter name="VEVENT" />
    </c:comp-filter>
  </c:filter>
</c:calendar-query>`

	url := c.resolveHref(calendarHref)
	resp, err := c.do("REPORT", url, body, 1)
	if err != nil {
		return nil, fmt.Errorf("REPORT: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var ms multistatusResponse
	if err := xml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("parsing multistatus: %w", err)
	}

	result := &EventListResult{}
	for _, r := range ms.Responses {
		for _, ps := range r.PropStat {
			if !strings.Contains(ps.Status, "200") || ps.Prop.CalendarData == "" {
				continue
			}
			event, err := ParseVEVENT(ps.Prop.CalendarData)
			if err != nil {
				result.ParseErrors = append(result.ParseErrors,
					fmt.Sprintf("%s: %v", r.Href, err))
				continue
			}
			event.Href = r.Href
			event.ETag = ps.Prop.GetETag
			result.Events = append(result.Events, EventWithMeta{
				Event: *event,
				Raw:   ps.Prop.CalendarData,
			})
		}
	}
	return result, nil
}
```

**Step 2: Verify it compiles**

Run: `nix develop .#go -c go build ./packages/caldav/...`
Expected: SUCCESS

**Step 3: Commit**

```
git add packages/caldav/internal/caldav/client.go
git commit -m "Add ListEvents to CalDAV client"
```

---

### Task 4: Add event cache, word index, and resources to provider

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/caldav/internal/resources/provider.go`

**Step 1: Add event fields to Provider struct**

Add to `Provider` struct (after `calHrefs`):

```go
	eventMap   map[string]*caldav.EventWithMeta // uid → event+raw
	eventIndex *WordIndex
```

Update `NewProvider` to initialize them:

```go
	eventMap:   make(map[string]*caldav.EventWithMeta),
	eventIndex: NewWordIndex(),
```

**Step 2: Add event_count to calendarInfo**

Add to `calendarInfo` struct:

```go
	EventCount int `json:"event_count"`
```

**Step 3: Update readCalendars to fetch events**

In the `for _, cal := range calendars` loop, after the existing task-fetching
block, add event fetching based on `ComponentTypes`:

```go
		eventCount := 0
		if hasComponentType(cal.ComponentTypes, "VEVENT") {
			eventResult, err := p.client.ListEvents(cal.Href)
			if err != nil {
				warnings = append(warnings,
					fmt.Sprintf("calendar %q: failed to list events: %v", cal.DisplayName, err))
			} else {
				eventCount = len(eventResult.Events)
				for _, e := range eventResult.Events {
					allEvents = append(allEvents, e.Event)
					p.cacheEvent(e, cal.Href)
				}
				for _, parseErr := range eventResult.ParseErrors {
					warnings = append(warnings,
						fmt.Sprintf("calendar %q: skipped malformed event: %s", cal.DisplayName, parseErr))
				}
			}
		}
```

Add `EventCount: eventCount` to `calendarInfo` construction.

Also only fetch tasks when calendar has VTODO:

```go
		taskCount := 0
		if hasComponentType(cal.ComponentTypes, "VTODO") {
			// existing task fetching code...
		}
```

Add after the loop:

```go
	// Rebuild event word index
	var eventItems []IndexItem
	for _, e := range allEvents {
		text := e.Summary + " " + e.Description + " " + e.Location
		if len(e.Categories) > 0 {
			text += " " + strings.Join(e.Categories, " ")
		}
		eventItems = append(eventItems, IndexItem{UID: e.UID, Text: text})
	}
	p.eventIndex.BuildFromItems(eventItems)
```

Add helper:

```go
func hasComponentType(types []string, target string) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}
```

**Step 4: Register event resources**

Add to `registerResources()`:

```go
	// Tier 0: Discovery — event word index
	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "caldav://event_index",
			Name:        "Event Index",
			Description: "Word-indexed search across all event summaries, descriptions, locations, and categories. Use caldav://event_index/{word} to search.",
			MimeType:    "application/json",
		},
		p.readEventIndex,
	)

	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://event_index/{word}",
			Name:        "Event Search",
			Description: "Search events by word. Returns metadata-tier results.",
			MimeType:    "application/json",
		},
		nil,
	)

	// Tier 0: Discovery — recurring events
	p.registry.RegisterResource(
		protocol.Resource{
			URI:         "caldav://events/recurring",
			Name:        "Recurring Events",
			Description: "All events with RRULE recurrence rules across all calendars. Returns metadata-tier results. Read caldav://calendars first to populate the cache.",
			MimeType:    "application/json",
		},
		p.readRecurringEvents,
	)

	// Tier 1: Metadata — events in a calendar
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://events/{calendar_id}",
			Name:        "Calendar Events",
			Description: "List events in a calendar — metadata only: UID, summary, dtstart, dtend, location, status, rrule.",
			MimeType:    "application/json",
		},
		nil,
	)

	// Tier 2: Content — full event detail
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://event/{uid}",
			Name:        "Event Detail",
			Description: "Full event detail: all VEVENT properties parsed into structured JSON, description capped at 4000 chars, attendees, alarms.",
			MimeType:    "application/json",
		},
		nil,
	)

	// Tier 3: Original — raw iCalendar for event
	p.registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "caldav://event/{uid}/ical",
			Name:        "Event iCalendar",
			Description: "Raw iCalendar VCALENDAR text for an event.",
			MimeType:    "text/calendar",
		},
		nil,
	)
```

**Step 5: Add routing in ReadResource**

Add before the `// Fall through to registry` line:

```go
	// Tier 0: event word search
	if strings.HasPrefix(uri, "caldav://event_index/") {
		word := strings.TrimPrefix(uri, "caldav://event_index/")
		return p.readEventIndexWord(ctx, uri, word)
	}

	// Tier 3: raw event ical (must check before tier 2)
	if strings.HasPrefix(uri, "caldav://event/") && strings.HasSuffix(uri, "/ical") {
		uid := strings.TrimPrefix(uri, "caldav://event/")
		uid = strings.TrimSuffix(uid, "/ical")
		return p.readEventIcal(ctx, uri, uid)
	}

	// Tier 2: event detail
	if strings.HasPrefix(uri, "caldav://event/") {
		uid := strings.TrimPrefix(uri, "caldav://event/")
		return p.readEventDetail(ctx, uri, uid)
	}

	// Tier 1: calendar events
	if strings.HasPrefix(uri, "caldav://events/") && !strings.HasPrefix(uri, "caldav://events/recurring") {
		calID := strings.TrimPrefix(uri, "caldav://events/")
		return p.readCalendarEvents(ctx, uri, calID)
	}
```

**Step 6: Add event handler functions**

```go
// --- Event Handlers ---

func (p *Provider) cacheEvent(e caldav.EventWithMeta, calHref string) {
	p.mu.Lock()
	p.eventMap[e.Event.UID] = &e
	p.calHrefs[e.Event.UID] = calHref
	p.mu.Unlock()
}

func (p *Provider) readEventIndex(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	resp := taskIndexResponse{
		Description: "Word-indexed search across all event summaries, descriptions, locations, and categories",
		Usage:       "Read caldav://event_index/{word} to search. Results are metadata-tier (lightweight). Read caldav://calendars first to populate the index.",
	}
	return jsonResource(uri, resp)
}

func (p *Provider) readEventIndexWord(ctx context.Context, uri, word string) (*protocol.ResourceReadResult, error) {
	uids := p.eventIndex.Search(word)

	var results []caldav.EventMetadata
	p.mu.RLock()
	for _, uid := range uids {
		if em, ok := p.eventMap[uid]; ok {
			results = append(results, em.Event.ToMetadata())
		}
	}
	p.mu.RUnlock()

	resp := struct {
		Query   string                 `json:"query"`
		Results []caldav.EventMetadata `json:"results"`
		Total   int                    `json:"total"`
	}{
		Query:   word,
		Results: results,
		Total:   len(results),
	}
	return jsonResource(uri, resp)
}

func (p *Provider) readRecurringEvents(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	var results []caldav.EventMetadata
	p.mu.RLock()
	for _, em := range p.eventMap {
		if em.Event.RRule != "" {
			results = append(results, em.Event.ToMetadata())
		}
	}
	p.mu.RUnlock()

	resp := struct {
		Events []caldav.EventMetadata `json:"events"`
		Total  int                    `json:"total"`
	}{
		Events: results,
		Total:  len(results),
	}
	return jsonResource(uri, resp)
}

func (p *Provider) readCalendarEvents(ctx context.Context, uri, calID string) (*protocol.ResourceReadResult, error) {
	calHref := calendarHrefFromID(calID)

	result, err := p.client.ListEvents(calHref)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	var metadata []caldav.EventMetadata
	for _, e := range result.Events {
		p.cacheEvent(e, calHref)
		metadata = append(metadata, e.Event.ToMetadata())
	}

	resp := struct {
		CalendarID string                 `json:"calendar_id"`
		Events     []caldav.EventMetadata `json:"events"`
		Total      int                    `json:"total"`
		Warnings   []string               `json:"warnings,omitempty"`
	}{
		CalendarID: calID,
		Events:     metadata,
		Total:      len(metadata),
		Warnings:   result.ParseErrors,
	}
	return jsonResource(uri, resp)
}

func (p *Provider) readEventDetail(ctx context.Context, uri, uid string) (*protocol.ResourceReadResult, error) {
	p.mu.RLock()
	em, ok := p.eventMap[uid]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("event %s not found in cache — read caldav://calendars first", uid)
	}

	event := em.Event
	event.Description = caldav.CapDescription(event.Description, 4000)

	return jsonResource(uri, event)
}

func (p *Provider) readEventIcal(ctx context.Context, uri, uid string) (*protocol.ResourceReadResult, error) {
	p.mu.RLock()
	em, ok := p.eventMap[uid]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("event %s not found in cache — read caldav://calendars first", uid)
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{{
			URI:      uri,
			MimeType: "text/calendar",
			Text:     em.Raw,
		}},
	}, nil
}
```

**Step 7: Verify it compiles and tests pass**

Run: `nix develop .#go -c go test ./packages/caldav/...`
Expected: ALL PASS

Run: `nix develop .#go -c go build ./packages/caldav/...`
Expected: SUCCESS

**Step 8: Commit**

```
git add packages/caldav/internal/resources/provider.go
git commit -m "Add event cache, word index, and progressive disclosure resources"
```

---

### Task 5: Update calendars description and build

**Promotion criteria:** N/A

**Files:**
- Modify: `packages/caldav/internal/resources/provider.go:48`

**Step 1: Update the calendars resource description**

Change the description for `caldav://calendars` from:
```
"List all CalDAV calendar collections with display name, color, component types, and task count"
```
to:
```
"List all CalDAV calendar collections with display name, color, component types, task count, and event count"
```

**Step 2: Nix build**

Run: `nix build .#caldav`
Expected: SUCCESS

**Step 3: Commit**

```
git add packages/caldav/internal/resources/provider.go
git commit -m "Update calendars resource description for event count"
```

---

### Task 6: Integration test against real CalDAV server

**Promotion criteria:** N/A

**Files:** None (manual verification)

**Step 1: Restart moxy and read calendars**

Restart moxy to pick up new binary. Read `caldav://calendars` via moxy.
Verify: `event_count` appears on VEVENT calendars, `task_count` appears
on VTODO calendars.

**Step 2: Test event resources**

Read `caldav://events/{calendar_id}` for a VEVENT calendar with events.
Verify: Event metadata returned with dtstart, dtend, location, status.

Read `caldav://event/{uid}` for one of the returned events.
Verify: Full detail with attendees, alarms, organizer.

Read `caldav://event/{uid}/ical` for the same event.
Verify: Raw iCalendar text.

**Step 3: Test event word search**

Read `caldav://event_index/meeting` (or relevant word).
Verify: Returns matching event metadata.

**Step 4: Test recurring events**

Read `caldav://events/recurring`.
Verify: Returns events with RRULE fields.

**Step 5: Verify task resources still work**

Read `caldav://calendar/{vtodo_calendar_id}`.
Verify: Task metadata unchanged.

**Step 6: Commit verification note**

Note what was verified in a final commit message if any fixes were needed.
