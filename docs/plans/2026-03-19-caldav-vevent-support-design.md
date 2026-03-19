# CalDAV MCP: VEVENT Support Design

## Problem

The CalDAV MCP only queries VTODO components. VEVENT calendars (fixed,
flexible, reoccurring, Google, TripIt, etc.) show `task_count: 0` even
though they contain events. Agents cannot answer questions like "what
events do I have in April?"

## Approach

Parallel event/task types (Approach A). Add `Event` and `EventMetadata`
structs alongside existing `Task`/`TaskMetadata`. Separate cache,
separate word index, separate resource namespace. No changes to existing
task functionality.

## Data Model

### Event (full detail, Tier 2)

```
UID, Summary, Description, Status, Location, Geo
DtStart, DtEnd, Duration
Organizer, Attendees []string
Categories []string
RRule, RecurrenceID
Sequence, Transp (OPAQUE/TRANSPARENT)
Created, LastModified
Alarms []Alarm  (reuse existing)
Href, ETag
HasDescription, DescriptionTokens
```

### EventMetadata (Tier 1, lightweight)

```
UID, Summary, Status, DtStart, DtEnd, Location
Categories []string, RRule
HasDescription, DescriptionTokens
```

## Client Layer

`ListEvents(calendarHref string) (*EventListResult, error)` --- REPORT
with `<c:comp-filter name="VEVENT" />`. Returns
`EventListResult{Events []EventWithMeta, ParseErrors []string}`. Same
pattern as `ListTasks`.

`ParseVEVENT(raw string) (*Event, error)` --- reuses existing
`unfoldLines`, `parsePropLine`, `propName`, `paramValue`, and VALARM
parsing. Scans for `BEGIN:VEVENT` / `END:VEVENT`.

Read-only for now. No `PutEvent`, `DeleteEvent`, or event mutation
tools.

## Resources

### Modified

`caldav://calendars` --- adds `event_count` field to response. Eagerly
fetches events from VEVENT calendars and tasks from VTODO calendars
based on `ComponentTypes`.

### New

| Tier | URI | Description |
|------|-----|-------------|
| 0 | `caldav://event_index` | Usage hint for event word search |
| 0 | `caldav://event_index/{word}` | Search event summaries, descriptions, locations, categories |
| 0 | `caldav://events/recurring` | All events with RRULE |
| 1 | `caldav://events/{calendar_id}` | Event metadata for a calendar |
| 2 | `caldav://event/{uid}` | Full event detail |
| 3 | `caldav://event/{uid}/ical` | Raw iCalendar |

### Provider Changes

- `eventMap map[string]*EventWithMeta` cache (separate from `taskMap`)
- `eventIndex *WordIndex` (separate from task word index)
- `readCalendars` fetches both VTODO and VEVENT per calendar based on
  `ComponentTypes`
- New routing entries in `ReadResource` for `caldav://event/` and
  `caldav://events/` prefixes

## Future Consideration: Caching

Eager fetching all calendars on every `caldav://calendars` read could be
slow as calendar count grows. Worth investigating:

- ETag-based conditional fetching (skip re-fetch if calendar hasn't
  changed)
- Encouraging users to front their CalDAV server with a caching proxy
- TTL-based in-memory cache with configurable staleness window

## Rollback

Purely additive. No existing resources, tools, or data models change
(except adding `event_count` to the calendars response). Rollback is
reverting the commits.
