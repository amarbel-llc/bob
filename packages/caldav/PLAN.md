# CalDAV MCP Package — Design Plan

## Package Identity

- **Name**: `caldav` (binary: `caldav`, URI scheme: `caldav://`)
- **Module path**: `github.com/amarbel-llc/bob/packages/caldav`
- **Type**: MCP + Resource package (tools for writes, resources for reads)
- **Go workspace member**: `./packages/caldav` added to `go.work`

## Directory Layout

```
packages/caldav/
├── cmd/caldav/
│   └── main.go              # Three-mode entry (generate-plugin | hook | server)
├── internal/
│   ├── caldav/
│   │   ├── client.go        # CalDAV HTTP client (PROPFIND, REPORT, PUT, DELETE)
│   │   ├── auth.go          # Auth from env vars (CALDAV_URL, CALDAV_USERNAME, CALDAV_PASSWORD)
│   │   └── parser.go        # iCalendar VTODO parsing/serialization
│   ├── tools/
│   │   ├── registry.go      # RegisterAll() → command.App
│   │   ├── tasks.go         # VTODO CRUD tools
│   │   └── calendars.go     # Calendar/collection management tools
│   └── resources/
│       ├── provider.go       # resourceProvider (progressive disclosure)
│       └── index.go          # Word index for search discovery
├── go.mod
└── go.sum
```

## Authentication

Environment variables (read once at startup, validated before server starts):

| Variable | Required | Description |
|----------|----------|-------------|
| `CALDAV_URL` | yes | Base URL of the CalDAV server (e.g., `https://dav.example.com/dav.php`) |
| `CALDAV_USERNAME` | yes | HTTP Basic auth username |
| `CALDAV_PASSWORD` | yes | HTTP Basic auth password |

No OAuth/token flows — keep it simple. Works with Nextcloud, Radicale, Baikal, sabre/dav, and any RFC 4791 server.

## Resources (Progressive Disclosure — 3 tiers, following nebulous pattern)

### Tier 0: Discovery

| Resource URI | Description |
|---|---|
| `caldav://calendars` | List all calendar collections with display name, color, component types, task count |
| `caldav://task_index` | Word-indexed search over task summaries/descriptions |
| `caldav://task_index/{word}` | Template: tasks matching a search word (returns metadata tier only) |

### Tier 1: Metadata (~200 bytes per task)

| Resource URI | Description |
|---|---|
| `caldav://calendar/{calendar_id}` | List tasks in a calendar — metadata only: UID, summary, status, priority, due date, `has_description`, `description_tokens` |

### Tier 2: Content (capped at 4000 chars)

| Resource URI | Description |
|---|---|
| `caldav://task/{uid}` | Full task detail: all VTODO properties parsed into structured JSON, description capped at 4000 chars, subtask UIDs listed |

### Tier 3: Original (full weight, recommend subagent)

| Resource URI | Description |
|---|---|
| `caldav://task/{uid}/ical` | Raw iCalendar VCALENDAR text for the task (for debugging or full-fidelity access) |

### Design Rationale

- An agent starts at `caldav://calendars` to discover what exists
- Drills into `caldav://calendar/{id}` to see task summaries without description payloads
- Reads `caldav://task/{uid}` only for tasks it needs detail on
- Uses `caldav://task_index/{word}` to search across all calendars without reading everything
- Each tier includes token estimates so the agent can budget context

## Tools (Write Operations)

All tools follow the grit pattern: `command.Command` with `command.Param`, `MapsTools`, and `Annotations`.

### Task Tools

| Tool | Description | Annotations |
|---|---|---|
| `create_task` | Create a VTODO in a calendar | destructive=false, idempotent=false |
| `update_task` | Update fields on an existing VTODO by UID | destructive=false, idempotent=true |
| `complete_task` | Set STATUS=COMPLETED and COMPLETED timestamp | destructive=false, idempotent=true |
| `delete_task` | Delete a VTODO by UID | destructive=true, idempotent=true |
| `move_task` | Move a VTODO between calendars | destructive=false, idempotent=true |

### Calendar Tools

| Tool | Description | Annotations |
|---|---|---|
| `create_calendar` | Create a new calendar collection (MKCALENDAR) | destructive=false, idempotent=false |

## VTODO Field Mapping (tasks.org compatible)

### Create/Update Input Schema

```json
{
  "calendar_id": "string (required for create)",
  "uid": "string (required for update)",
  "summary": "string",
  "description": "string",
  "status": "enum: NEEDS-ACTION | IN-PROCESS | COMPLETED | CANCELLED",
  "priority": "int 0-9 (1=highest, 9=lowest, 0=undefined)",
  "due": "string (RFC 3339 datetime or YYYY-MM-DD date)",
  "dtstart": "string (RFC 3339 or date)",
  "categories": ["string array — maps to tasks.org tags"],
  "percent_complete": "int 0-100",
  "parent_uid": "string — sets RELATED-TO;RELTYPE=PARENT (subtask support)",
  "rrule": "string — recurrence rule (e.g., FREQ=DAILY;COUNT=5)",
  "location": "string",
  "geo": "string (lat;lon)",
  "sort_order": "int — written as X-APPLE-SORT-ORDER"
}
```

### Parsed Output Schema (for resources)

All of the above plus:
- `uid`, `created`, `last_modified`, `dtstamp`
- `completed` (timestamp)
- `subtask_uids` (derived from reverse RELATED-TO lookup)
- `alarms` (array of `{trigger, action, description}` from VALARM)
- `has_description` and `description_tokens` (for metadata tier)
- `etag` (for concurrency)

## CalDAV Client Implementation

### HTTP Operations

| Method | CalDAV Operation | Use |
|--------|-----------------|-----|
| `PROPFIND Depth:1` | Discover calendars at principal URL | `caldav://calendars` |
| `REPORT calendar-query` | List VTODOs in a calendar (metadata) | `caldav://calendar/{id}` |
| `REPORT calendar-multiget` | Fetch specific VTODOs by href | `caldav://task/{uid}` |
| `PUT` | Create or update a VTODO | `create_task`, `update_task` |
| `DELETE` | Remove a VTODO | `delete_task` |
| `MKCALENDAR` | Create a new calendar | `create_calendar` |

### iCalendar Parsing

Use a Go iCalendar library (e.g., `github.com/emersion/go-ical` which handles VCALENDAR/VTODO/VALARM parsing and serialization). This avoids hand-rolling RFC 5545 parsing.

The parser layer will:
1. Parse raw iCal text into structured Go types
2. Extract tasks.org-specific X-properties (`X-APPLE-SORT-ORDER`)
3. Resolve RELATED-TO for subtask hierarchies
4. Serialize Go types back to valid iCal for PUT operations

## Hook (PreToolUse)

Map built-in tools when CalDAV MCP tools should be preferred:

```go
MapsTools: []command.ToolMapping{
    {Replaces: "Bash", CommandPrefixes: []string{"curl.*caldav", "curl.*dav"}, UseWhen: "interacting with CalDAV servers"},
}
```

## Nix Build Expression

`lib/packages/caldav.nix`:

```nix
{ pkgs, goWorkspaceSrc, goVendorHash }:
let
  mkGoModule = import ../mkGoWorkspaceModule.nix {
    inherit pkgs goWorkspaceSrc goVendorHash;
  };
in
mkGoModule {
  pname = "caldav";
  subPackages = [ "packages/caldav/cmd/caldav" ];

  postInstall = ''
    $out/bin/caldav generate-plugin $out
  '';

  meta = with pkgs.lib; {
    description = "CalDAV MCP server — tasks, calendars, and VTODO management";
    homepage = "https://github.com/amarbel-llc/bob";
    license = licenses.mit;
  };
}
```

## Integration Points

### flake.nix additions

1. Add `caldavPkg` build:
   ```nix
   caldavPkg = import ./lib/packages/caldav.nix {
     inherit pkgs goWorkspaceSrc goVendorHash;
   };
   ```

2. Add to `plugins` list for marketplace:
   ```nix
   pkgs.caldavPkg
   ```

3. Add to `packages` output:
   ```nix
   caldav = localPkgs.caldavPkg;
   ```

4. Add to `mcp-all` symlinkJoin paths.

### go.work addition

```
./packages/caldav
```

### marketplace-config.json addition

```json
"caldav": {
  "description": "CalDAV MCP server — manage tasks, calendars, and VTODO items with progressive disclosure",
  "version": "0.1.0",
  "homepage": "https://github.com/amarbel-llc/bob",
  "category": "productivity",
  "tags": ["caldav", "tasks", "calendar", "vtodo", "mcp"]
}
```

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/amarbel-llc/purse-first/libs/go-mcp` | MCP framework (command, server, transport, protocol) |
| `github.com/emersion/go-ical` | iCalendar VTODO parsing and serialization |

No other external dependencies needed. The CalDAV HTTP client is hand-rolled using `net/http` since CalDAV is just HTTP with WebDAV/XML extensions.

## Implementation Order

1. **Scaffold**: `go.mod`, `cmd/caldav/main.go` (three-mode), empty registry
2. **CalDAV client**: auth, PROPFIND, REPORT, PUT, DELETE
3. **iCal parser**: VTODO ↔ Go struct with tasks.org fields
4. **Resources**: Progressive disclosure tiers (calendars → metadata → content → ical)
5. **Tools**: CRUD operations (create, update, complete, delete, move)
6. **Word index**: For `caldav://task_index/{word}` search
7. **Hook**: PreToolUse mapping
8. **Nix**: Build expression, flake.nix integration, marketplace-config.json
9. **Tests**: Unit tests for parser, client (with mock server), resource provider
