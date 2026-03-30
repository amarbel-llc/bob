package tools

import "github.com/amarbel-llc/purse-first/libs/go-mcp/command"

func RegisterAll() (*command.App, *resourceProvider) {
	app := command.NewApp("grit", "MCP server exposing git operations")
	app.Version = "0.1.0"

	registerStatusCommands(app)
	registerStagingCommands(app)
	registerRmCommands(app)
	registerCommitCommands(app)
	registerTryCommitCommands(app)
	registerBranchCommands(app)
	registerRemoteCommands(app)
	registerRevParseCommands(app)
	registerRebaseCommands(app)
	registerInteractiveRebaseCommands(app)
	registerCherryPickCommands(app)
	registerHardResetCommands(app)
	registerTagCommands(app)
	registerStashCommands(app)
	registerMergeCommands(app)

	resProvider, err := NewResourceProvider()
	if err != nil {
		resProvider = nil
	}

	if resProvider != nil {
		registerResourceToolCommands(app, resProvider)
	}

	return app, resProvider
}
