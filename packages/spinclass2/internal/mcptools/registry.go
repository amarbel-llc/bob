package mcptools

import "github.com/amarbel-llc/purse-first/libs/go-mcp/command"

func RegisterAll() *command.App {
	app := command.NewApp("spinclass2", "MCP server for git worktree session management")
	app.Version = "0.1.0"

	registerMergeThisSession(app)

	return app
}
