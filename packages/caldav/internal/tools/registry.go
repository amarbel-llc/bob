package tools

import (
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
)

func RegisterAll(provider *resources.Provider) *command.App {
	app := command.NewApp("caldav", "CalDAV MCP server for tasks and calendars")
	app.Version = "0.1.0"
	app.Description.Long = `CalDAV MCP server for managing tasks (VTODO) and events (VEVENT) on a CalDAV
server. Reads are exposed as MCP resources with progressive disclosure
(caldav://calendars, caldav://calendar/{id}, caldav://task/{uid}). Writes use
tools. Compatible with tasks.org VTODO format including subtasks, tags,
recurrence, and reminders.`
	app.EnvVars = []command.EnvVar{
		{Name: "CALDAV_URL", Description: "Base URL of the CalDAV server"},
		{Name: "CALDAV_USERNAME", Description: "HTTP Basic auth username"},
		{Name: "CALDAV_PASSWORD", Description: "HTTP Basic auth password"},
	}

	registerTaskCommands(app, provider)
	registerEventCommands(app, provider)
	registerCalendarCommands(app, provider)

	return app
}
