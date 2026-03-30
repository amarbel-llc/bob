package tools

import (
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
)

func RegisterAll() (*command.App, *resourceProvider) {
	app := command.NewApp("get-hubbed", "GitHub MCP server wrapping the gh CLI")
	app.Version = "0.1.0"

	registerIssueCommands(app)
	registerPRCommands(app)
	registerContentCommands(app)

	resProvider, err := NewResourceProvider()
	if err != nil {
		resProvider = nil
	}

	return app, resProvider
}

func RegisterAPITools(r *server.ToolRegistryV1) {
	registerAPITools(r)
}
