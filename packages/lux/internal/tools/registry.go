package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amarbel-llc/lux/internal/lsp"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
)

// RegisterAll adds all MCP tool commands to the given app. When bridge is nil,
// commands are registered with metadata only (for artifact generation); their
// Run handlers return errors if invoked.
func RegisterAll(app *command.App, bridge *Bridge) {
	registerPositionTools(app, bridge)
	registerURITools(app, bridge)
	registerReferencesTool(app, bridge)
	registerCodeActionTool(app, bridge)
	registerRenameTool(app, bridge)
	registerWorkspaceSymbolsTool(app, bridge)
}

// positionParams returns the common (uri, line, character) param set.
func positionParams() []command.Param {
	return []command.Param{
		{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
		{Name: "line", Type: command.Int, Description: "0-indexed line number", Required: true},
		{Name: "character", Type: command.Int, Description: "0-indexed character offset", Required: true},
	}
}

var errNoBridge = fmt.Errorf("no bridge configured (artifact-generation mode)")

func stubHandler(_ context.Context, _ json.RawMessage, _ command.Prompter) (*command.Result, error) {
	return nil, errNoBridge
}

// makePositionHandler creates a Run handler that parses (uri, line, character)
// and delegates to the given bridge method.
func makePositionHandler(
	fn func(ctx context.Context, uri lsp.DocumentURI, line, character int) (*command.Result, error),
) func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	return func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
		var a struct {
			URI       string `json:"uri"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		return fn(ctx, lsp.DocumentURI(a.URI), a.Line, a.Character)
	}
}

// makeURIHandler creates a Run handler that parses (uri) and delegates to the
// given bridge method.
func makeURIHandler(
	fn func(ctx context.Context, uri lsp.DocumentURI) (*command.Result, error),
) func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
	return func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
		var a struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		return fn(ctx, lsp.DocumentURI(a.URI))
	}
}

func registerPositionTools(app *command.App, bridge *Bridge) {
	hoverRun := stubHandler
	definitionRun := stubHandler
	completionRun := stubHandler
	if bridge != nil {
		hoverRun = makePositionHandler(bridge.Hover)
		definitionRun = makePositionHandler(bridge.Definition)
		completionRun = makePositionHandler(bridge.Completion)
	}

	app.AddCommand(&command.Command{
		Name: "hover",
		Description: command.Description{
			Short: "Get type information, documentation, and signatures for a symbol.",
			Long: `Agents MUST use this tool instead of reading source files when you need to
understand what a function/type does, its parameters, return types, or
documentation. Unlike grep/read which show raw text, hover provides
semantically-parsed information from the language server. DO NOT read files
just to check function signatures or types - use this tool instead.`,
		},
		Params: positionParams(),
		Examples: []command.Example{
			{
				Description: "Get type info for a symbol at line 42, column 10",
				Command:     "lux hover --uri file:///path/to/main.go --line 42 --character 10",
			},
		},
		SeeAlso: []string{"lux-definition", "lux-references", "lux-workspace_symbols"},
		Run:     hoverRun,
	})

	app.AddCommand(&command.Command{
		Name: "definition",
		Description: command.Description{
			Short: "Jump to the definition of a symbol (function, type, variable).",
			Long: `Agents MUST use this tool instead of grep/search when you know a symbol name
and need to find its definition or implementation. Uses semantic analysis to
find the actual definition, not just string matches. DO NOT use grep or file
searches to locate function/type definitions - this tool handles cross-file
navigation, interface implementations, and import sources accurately.`,
		},
		Params: positionParams(),
		Examples: []command.Example{
			{
				Description: "Jump to definition of the symbol under the cursor",
				Command:     "lux definition --uri file:///path/to/main.go --line 15 --character 8",
			},
		},
		SeeAlso: []string{"lux-hover", "lux-references", "lux-workspace_symbols"},
		Run:     definitionRun,
	})

	app.AddCommand(&command.Command{
		Name: "completion",
		Description: command.Description{
			Short: "Get context-aware code completions at a cursor position.",
			Long: `Agents should use this tool instead of reading documentation or source files
when exploring available methods on a type, discovering struct fields, finding
imported symbols, or understanding API surfaces. Shows only valid symbols,
methods, and fields actually available in scope - more accurate than guessing
from source.`,
		},
		Params: positionParams(),
		Examples: []command.Example{
			{
				Description: "Get completions after a dot on a struct value",
				Command:     "lux completion --uri file:///path/to/handler.go --line 30 --character 12",
			},
		},
		SeeAlso: []string{"lux-hover", "lux-document_symbols"},
		Run:     completionRun,
	})
}

func registerURITools(app *command.App, bridge *Bridge) {
	formatRun := stubHandler
	docSymbolsRun := stubHandler
	diagnosticsRun := stubHandler
	if bridge != nil {
		formatRun = makeURIHandler(bridge.Format)
		docSymbolsRun = makeURIHandler(bridge.DocumentSymbols)
		diagnosticsRun = makeURIHandler(bridge.Diagnostics)
	}

	app.AddCommand(&command.Command{
		Name: "format",
		Description: command.Description{
			Short: "Get formatting edits for a document according to language-standard style.",
			Long: `Agents should use this tool to get proper formatting instead of manually
adjusting whitespace or running external formatters. Returns text edits needed
to properly format the file. Note: returns edits but does not apply them - use
the Edit tool to apply the returned changes.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "Get formatting edits for a Go file",
				Command:     "lux format --uri file:///path/to/main.go",
			},
		},
		SeeAlso: []string{"lux-fmt", "lux-diagnostics"},
		Run:     formatRun,
	})

	app.AddCommand(&command.Command{
		Name: "document_symbols",
		Description: command.Description{
			Short: "Get a structured outline of all symbols in a file.",
			Long: `Agents MUST use this tool instead of reading entire files when you need to
understand file structure or find what functions/types exist in a file. Returns
hierarchical symbols: function/method names, type definitions, nested
structures, top-level constants. DO NOT read and parse files manually to find
symbol names - this tool is faster and more accurate.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "List all symbols in a file",
				Command:     "lux document_symbols --uri file:///path/to/server.go",
			},
		},
		SeeAlso: []string{"lux-workspace_symbols", "lux-hover"},
		Run:     docSymbolsRun,
	})

	app.AddCommand(&command.Command{
		Name: "diagnostics",
		Description: command.Description{
			Short: "Get compiler/linter diagnostics (errors, warnings, hints) for a file.",
			Long: `Agents should use this tool instead of running build commands when checking for
errors in a specific file. Provides precise error locations and messages. Use to
understand issues before making edits or to verify changes are correct without
running a full build.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "Check a file for errors and warnings",
				Command:     "lux diagnostics --uri file:///path/to/main.go",
			},
		},
		SeeAlso: []string{"lux-code_action", "lux-format"},
		Run:     diagnosticsRun,
	})
}

func registerReferencesTool(app *command.App, bridge *Bridge) {
	run := stubHandler
	if bridge != nil {
		run = func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				URI                string `json:"uri"`
				Line               int    `json:"line"`
				Character          int    `json:"character"`
				IncludeDeclaration *bool  `json:"include_declaration"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			includeDecl := true
			if a.IncludeDeclaration != nil {
				includeDecl = *a.IncludeDeclaration
			}

			return bridge.References(ctx, lsp.DocumentURI(a.URI), a.Line, a.Character, includeDecl)
		}
	}

	app.AddCommand(&command.Command{
		Name: "references",
		Description: command.Description{
			Short: "Find all usages of a symbol throughout the codebase.",
			Long: `Agents MUST use this tool instead of grep/search for finding where
functions/types/variables are used - it understands scope and semantics, finding
actual references not just string matches. DO NOT use grep to find usages of
symbols - grep finds false positives (comments, strings, similar names).
Critical for impact analysis before refactoring, understanding how functions are
called, and tracing data flow.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
			{Name: "line", Type: command.Int, Description: "0-indexed line number", Required: true},
			{Name: "character", Type: command.Int, Description: "0-indexed character offset", Required: true},
			{Name: "include_declaration", Type: command.Bool, Description: "Include the declaration in results", Default: true},
		},
		Examples: []command.Example{
			{
				Description: "Find all callers of a function",
				Command:     "lux references --uri file:///path/to/server.go --line 25 --character 5",
			},
			{
				Description: "Find usages without the declaration itself",
				Command:     "lux references --uri file:///path/to/server.go --line 25 --character 5 --include_declaration=false",
			},
		},
		SeeAlso: []string{"lux-definition", "lux-hover", "lux-rename"},
		Run:     run,
	})
}

func registerCodeActionTool(app *command.App, bridge *Bridge) {
	run := stubHandler
	if bridge != nil {
		run = func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				URI            string `json:"uri"`
				StartLine      int    `json:"start_line"`
				StartCharacter int    `json:"start_character"`
				EndLine        int    `json:"end_line"`
				EndCharacter   int    `json:"end_character"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			return bridge.CodeAction(ctx, lsp.DocumentURI(a.URI), a.StartLine, a.StartCharacter, a.EndLine, a.EndCharacter)
		}
	}

	app.AddCommand(&command.Command{
		Name: "code_action",
		Description: command.Description{
			Short: "Get suggested fixes, refactorings, and improvements for a code range.",
			Long: `Agents should use this tool to get language-server suggested fixes instead of
manually writing fixes for common issues. Provides quick fixes for errors,
refactoring operations (extract function, inline variable), import organization,
and code generation (implement interface). Use after diagnostics to get fixes
for reported issues.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
			{Name: "start_line", Type: command.Int, Description: "0-indexed start line", Required: true},
			{Name: "start_character", Type: command.Int, Description: "0-indexed start character", Required: true},
			{Name: "end_line", Type: command.Int, Description: "0-indexed end line", Required: true},
			{Name: "end_character", Type: command.Int, Description: "0-indexed end character", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "Get fixes for an error on line 10",
				Command:     "lux code_action --uri file:///path/to/main.go --start_line 10 --start_character 0 --end_line 10 --end_character 50",
			},
		},
		SeeAlso: []string{"lux-diagnostics", "lux-format"},
		Run:     run,
	})
}

func registerRenameTool(app *command.App, bridge *Bridge) {
	run := stubHandler
	if bridge != nil {
		run = func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				URI       string `json:"uri"`
				Line      int    `json:"line"`
				Character int    `json:"character"`
				NewName   string `json:"new_name"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			return bridge.Rename(ctx, lsp.DocumentURI(a.URI), a.Line, a.Character, a.NewName)
		}
	}

	app.AddCommand(&command.Command{
		Name: "rename",
		Description: command.Description{
			Short: "Rename a symbol across the entire codebase with semantic accuracy.",
			Long: `Agents MUST use this tool instead of find-and-replace or manual editing when
renaming functions, types, variables, or other symbols. Only renames actual
references (not comments, strings, or similar names), handles scoping correctly,
and updates imports appropriately. DO NOT use grep+edit or find-and-replace for
renaming - it will miss references or change unrelated text.`,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "File URI (e.g., file:///path/to/file.go)", Required: true},
			{Name: "line", Type: command.Int, Description: "0-indexed line number", Required: true},
			{Name: "character", Type: command.Int, Description: "0-indexed character offset", Required: true},
			{Name: "new_name", Type: command.String, Description: "New name for the symbol", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "Rename a function from oldName to newName",
				Command:     "lux rename --uri file:///path/to/main.go --line 15 --character 5 --new_name newName",
			},
		},
		SeeAlso: []string{"lux-references", "lux-definition"},
		Run:     run,
	})
}

func registerWorkspaceSymbolsTool(app *command.App, bridge *Bridge) {
	run := stubHandler
	if bridge != nil {
		run = func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				Query string `json:"query"`
				URI   string `json:"uri"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			return bridge.WorkspaceSymbols(ctx, lsp.DocumentURI(a.URI), a.Query)
		}
	}

	app.AddCommand(&command.Command{
		Name: "workspace_symbols",
		Description: command.Description{
			Short: "Search for symbols across the workspace by name pattern.",
			Long: `Agents MUST use this tool instead of grep/glob when searching for symbol
definitions by name. DO NOT use grep to find function or type definitions -
grep returns all text matches including usages, comments, and strings. This
tool returns only actual symbol definitions with their locations.`,
		},
		Params: []command.Param{
			{Name: "query", Type: command.String, Description: "Symbol name pattern to search for", Required: true},
			{Name: "uri", Type: command.String, Description: "Any file URI in the workspace (used to identify which LSP to query)", Required: true},
		},
		Examples: []command.Example{
			{
				Description: "Find all types and functions matching 'Handler'",
				Command:     "lux workspace_symbols --query Handler --uri file:///path/to/any/file.go",
			},
		},
		SeeAlso: []string{"lux-document_symbols", "lux-definition", "lux-hover"},
		Run:     run,
	})
}
