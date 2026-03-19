package tools

import (
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
)

func RegisterAll(provider *resources.Provider) *command.App {
	app := command.NewApp("caldav", "CalDAV MCP server for tasks and calendars")
	app.Version = "0.1.0"

	registerTaskCommands(app, provider)
	registerCalendarCommands(app, provider)

	return app
}
