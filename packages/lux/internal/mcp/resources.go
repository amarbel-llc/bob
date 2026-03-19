package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
	"github.com/amarbel-llc/lux/internal/lsp"
	"github.com/amarbel-llc/lux/internal/subprocess"
	"github.com/amarbel-llc/lux/internal/tools"
	"github.com/amarbel-llc/lux/pkg/filematch"
)

// resourceProvider wraps a ResourceRegistry to handle template-based resources
// that use prefix matching on URIs rather than exact lookup.
type resourceProvider struct {
	registry  *mcpserver.ResourceRegistry
	bridge    *tools.Bridge
	diagStore *DiagnosticsStore
}

func newResourceProvider(registry *mcpserver.ResourceRegistry, bridge *tools.Bridge, diagStore *DiagnosticsStore) *resourceProvider {
	return &resourceProvider{
		registry:  registry,
		bridge:    bridge,
		diagStore: diagStore,
	}
}

func (p *resourceProvider) ListResources(ctx context.Context) ([]protocol.Resource, error) {
	return p.registry.ListResources(ctx)
}

func (p *resourceProvider) ReadResource(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	// LSP operation resources
	if strings.HasPrefix(uri, "lux://lsp/") {
		return p.readLSPResource(ctx, uri)
	}
	// Legacy symbol/diagnostic templates
	if strings.HasPrefix(uri, "lux://symbols/") {
		fileURI := strings.TrimPrefix(uri, "lux://symbols/")
		return readSymbols(ctx, p.bridge, uri, fileURI)
	}
	if strings.HasPrefix(uri, "lux://diagnostics/") {
		encodedURI := strings.TrimPrefix(uri, "lux://diagnostics/")
		return readDiagnostics(p.diagStore, uri, encodedURI)
	}
	return p.registry.ReadResource(ctx, uri)
}

func (p *resourceProvider) ListResourceTemplates(ctx context.Context) ([]protocol.ResourceTemplate, error) {
	return p.registry.ListResourceTemplates(ctx)
}

// readLSPResource dispatches lux://lsp/* resource reads to the bridge.
func (p *resourceProvider) readLSPResource(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid resource URI: %w", err)
	}

	// For lux://lsp/hover?..., url.Parse gives Host="lsp", Path="/hover"
	operation := strings.TrimPrefix(parsed.Path, "/")
	if operation == "" {
		return nil, fmt.Errorf("missing operation in resource URI")
	}
	q := parsed.Query()

	outputFormat := q.Get("format")
	if outputFormat == "" {
		outputFormat = "json"
	}

	getFileURI := func() (lsp.DocumentURI, error) {
		v := q.Get("uri")
		if v == "" {
			return "", fmt.Errorf("missing required parameter 'uri'")
		}
		return lsp.DocumentURI(v), nil
	}

	getPosition := func() (lsp.DocumentURI, int, int, error) {
		fileURI, err := getFileURI()
		if err != nil {
			return "", 0, 0, err
		}
		line, err := strconv.Atoi(q.Get("line"))
		if err != nil {
			return "", 0, 0, fmt.Errorf("invalid or missing 'line' parameter")
		}
		char, err := strconv.Atoi(q.Get("character"))
		if err != nil {
			return "", 0, 0, fmt.Errorf("invalid or missing 'character' parameter")
		}
		return fileURI, line, char, nil
	}

	var text string
	var mimeType string

	switch operation {
	case "hover":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.Hover(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.HoverRaw(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "definition":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.Definition(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.DefinitionRaw(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "references":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		includeDecl := q.Get("include_declaration") != "false"
		if outputFormat == "text" {
			result, err := p.bridge.References(ctx, fileURI, line, char, includeDecl)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.ReferencesRaw(ctx, fileURI, line, char, includeDecl)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "completion":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.Completion(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.CompletionRaw(ctx, fileURI, line, char)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "format":
		fileURI, err := getFileURI()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.Format(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.FormatRaw(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			// FormatRaw returns json.RawMessage, already JSON
			text = string(raw)
			mimeType = "application/json"
		}

	case "document-symbols":
		fileURI, err := getFileURI()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.DocumentSymbols(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.DocumentSymbolsRaw(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "diagnostics":
		fileURI, err := getFileURI()
		if err != nil {
			return nil, err
		}
		if outputFormat == "text" {
			result, err := p.bridge.Diagnostics(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.DiagnosticsRaw(ctx, fileURI)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "code-action":
		fileURI, err := getFileURI()
		if err != nil {
			return nil, err
		}
		startLine, _ := strconv.Atoi(q.Get("start_line"))
		startChar, _ := strconv.Atoi(q.Get("start_character"))
		endLine, _ := strconv.Atoi(q.Get("end_line"))
		endChar, _ := strconv.Atoi(q.Get("end_character"))
		if outputFormat == "text" {
			result, err := p.bridge.CodeAction(ctx, fileURI, startLine, startChar, endLine, endChar)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.CodeActionRaw(ctx, fileURI, startLine, startChar, endLine, endChar)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "rename":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		newName := q.Get("new_name")
		if newName == "" {
			return nil, fmt.Errorf("missing required parameter 'new_name'")
		}
		if outputFormat == "text" {
			result, err := p.bridge.Rename(ctx, fileURI, line, char, newName)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.RenameRaw(ctx, fileURI, line, char, newName)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "workspace-symbols":
		fileURI, err := getFileURI()
		if err != nil {
			return nil, err
		}
		query := q.Get("query")
		if query == "" {
			return nil, fmt.Errorf("missing required parameter 'query'")
		}
		if outputFormat == "text" {
			result, err := p.bridge.WorkspaceSymbols(ctx, fileURI, query)
			if err != nil {
				return nil, err
			}
			if result.IsErr {
				return nil, fmt.Errorf("LSP operation failed: %s", result.Text)
			}
			text = result.Text
			mimeType = "text/plain"
		} else {
			raw, err := p.bridge.WorkspaceSymbolsRaw(ctx, fileURI, query)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return nil, err
			}
			text = string(data)
			mimeType = "application/json"
		}

	case "incoming-calls":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		raw, err := p.bridge.IncomingCallsRaw(ctx, fileURI, line, char)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(raw, "", "  ")
		text = string(data)
		mimeType = "application/json"

	case "outgoing-calls":
		fileURI, line, char, err := getPosition()
		if err != nil {
			return nil, err
		}
		raw, err := p.bridge.OutgoingCallsRaw(ctx, fileURI, line, char)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(raw, "", "  ")
		text = string(data)
		mimeType = "application/json"

	default:
		return nil, fmt.Errorf("unknown LSP operation: %s", operation)
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      uri,
				MimeType: mimeType,
				Text:     text,
			},
		},
	}, nil
}

func registerResources(
	registry *mcpserver.ResourceRegistry,
	pool *subprocess.Pool,
	bridge *tools.Bridge,
	cfg *config.Config,
	ftConfigs []*filetype.Config,
	diagStore *DiagnosticsStore,
) {
	cwd, _ := os.Getwd()

	matcher := filematch.NewMatcherSet()
	for _, ft := range ftConfigs {
		if ft.LSP != "" {
			matcher.Add(ft.Name, ft.Extensions, ft.Patterns, ft.LanguageIDs)
		}
	}

	registry.RegisterResource(
		protocol.Resource{
			URI:         "lux://status",
			Name:        "LSP Status",
			Description: "Current status of configured language servers including which are running",
			MimeType:    "application/json",
		},
		func(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
			return readStatus(pool, cfg, ftConfigs)
		},
	)

	registry.RegisterResource(
		protocol.Resource{
			URI:         "lux://languages",
			Name:        "Supported Languages",
			Description: "Languages supported by lux with their file extensions and patterns",
			MimeType:    "application/json",
		},
		func(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
			return readLanguages(ftConfigs)
		},
	)

	registry.RegisterResource(
		protocol.Resource{
			URI:         "lux://files",
			Name:        "Project Files",
			Description: "Files in the current directory that match configured LSP extensions/patterns",
			MimeType:    "application/json",
		},
		func(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
			return readFiles(cwd, matcher)
		},
	)

	// Template resources for data inspection
	registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "lux://symbols/{uri}",
			Name:        "File Symbols",
			Description: "All symbols (functions, types, constants, etc.) in a file as reported by the LSP. Use file:// URI encoding for the path (e.g., lux://symbols/file:///path/to/file.go)",
			MimeType:    "application/json",
		},
		nil, // Template URIs are handled by the resourceProvider wrapper
	)

	registry.RegisterTemplate(
		protocol.ResourceTemplate{
			URITemplate: "lux://diagnostics/{uri}",
			Name:        "File Diagnostics",
			Description: "Push diagnostics (errors, warnings) for a file as reported by the LSP. Updated in real-time via resource subscriptions.",
			MimeType:    "application/json",
		},
		nil, // Template URIs are handled by the resourceProvider wrapper
	)

	// LSP operation resource templates — accessed via read_resource tool
	lspTemplates := []protocol.ResourceTemplate{
		{
			URITemplate: "lux://lsp/hover?uri={uri}&line={line}&character={character}",
			Name:        "Hover",
			Description: "Get type information, documentation, and signatures for a symbol at a position",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/definition?uri={uri}&line={line}&character={character}",
			Name:        "Go to Definition",
			Description: "Jump to the definition of a symbol at a position using semantic analysis",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/references?uri={uri}&line={line}&character={character}",
			Name:        "Find References",
			Description: "Find all usages of a symbol throughout the codebase. Optional: &include_declaration=false",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/completion?uri={uri}&line={line}&character={character}",
			Name:        "Completion",
			Description: "Get context-aware code completions at a cursor position",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/format?uri={uri}",
			Name:        "Format",
			Description: "Get formatting edits for a document according to language-standard style",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/document-symbols?uri={uri}",
			Name:        "Document Symbols",
			Description: "Get a structured outline of all symbols in a file (functions, types, constants)",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/diagnostics?uri={uri}",
			Name:        "Diagnostics",
			Description: "Get compiler/linter diagnostics (errors, warnings, hints) for a file",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/code-action?uri={uri}&start_line={start_line}&start_character={start_character}&end_line={end_line}&end_character={end_character}",
			Name:        "Code Action",
			Description: "Get suggested fixes, refactorings, and improvements for a code range",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/rename?uri={uri}&line={line}&character={character}&new_name={new_name}",
			Name:        "Rename",
			Description: "Rename a symbol across the entire codebase with semantic accuracy",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/workspace-symbols?uri={uri}&query={query}",
			Name:        "Workspace Symbols",
			Description: "Search for symbols (functions, types, constants) across the workspace by name pattern",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "lux://lsp/incoming-calls?uri={uri}&line={line}&character={character}",
			Name:        "Incoming Calls",
			Description: "Find all callers of a function at a position. Returns one level; walk the graph by passing results back.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "lux://lsp/outgoing-calls?uri={uri}&line={line}&character={character}",
			Name:        "Outgoing Calls",
			Description: "Find all functions called by the function at a position. Returns one level; walk the graph by passing results back.",
			MimeType:    "application/json",
		},
	}

	for _, tmpl := range lspTemplates {
		registry.RegisterTemplate(tmpl, nil)
	}
}

type statusResponse struct {
	ConfiguredLSPs      []lspStatus `json:"configured_lsps"`
	SupportedExtensions []string    `json:"supported_extensions"`
	SupportedLanguages  []string    `json:"supported_languages"`
}

type lspStatus struct {
	Name       string   `json:"name"`
	Flake      string   `json:"flake"`
	Extensions []string `json:"extensions,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`
	State      string   `json:"state"`
}

func readStatus(pool *subprocess.Pool, cfg *config.Config, ftConfigs []*filetype.Config) (*protocol.ResourceReadResult, error) {
	statuses := pool.Status()
	statusMap := make(map[string]string)
	for _, s := range statuses {
		statusMap[s.Name] = s.State
	}

	lspExts := make(map[string][]string)
	lspPatterns := make(map[string][]string)
	var allExts, allLangs []string
	for _, ft := range ftConfigs {
		if ft.LSP != "" {
			lspExts[ft.LSP] = append(lspExts[ft.LSP], ft.Extensions...)
			lspPatterns[ft.LSP] = append(lspPatterns[ft.LSP], ft.Patterns...)
		}
		allExts = append(allExts, ft.Extensions...)
		allLangs = append(allLangs, ft.LanguageIDs...)
	}

	var lsps []lspStatus
	for _, l := range cfg.LSPs {
		state := statusMap[l.Name]
		if state == "" {
			state = "idle"
		}
		lsps = append(lsps, lspStatus{
			Name:       l.Name,
			Flake:      l.Flake,
			Extensions: lspExts[l.Name],
			Patterns:   lspPatterns[l.Name],
			State:      state,
		})
	}

	resp := statusResponse{
		ConfiguredLSPs:      lsps,
		SupportedExtensions: allExts,
		SupportedLanguages:  allLangs,
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      "lux://status",
				MimeType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

type languagesResponse map[string]languageInfo

type languageInfo struct {
	LSP        string   `json:"lsp"`
	Extensions []string `json:"extensions,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`
}

func readLanguages(ftConfigs []*filetype.Config) (*protocol.ResourceReadResult, error) {
	resp := make(languagesResponse)

	for _, ft := range ftConfigs {
		resp[ft.Name] = languageInfo{
			LSP:        ft.LSP,
			Extensions: ft.Extensions,
			Patterns:   ft.Patterns,
		}
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      "lux://languages",
				MimeType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

type filesResponse struct {
	Root  string     `json:"root"`
	Files []string   `json:"files"`
	Stats filesStats `json:"stats"`
}

type filesStats struct {
	TotalFiles  int            `json:"total_files"`
	ByExtension map[string]int `json:"by_extension"`
}

func readFiles(cwd string, matcher *filematch.MatcherSet) (*protocol.ResourceReadResult, error) {
	var files []string
	byExt := make(map[string]int)

	err := filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		relPath, _ := filepath.Rel(cwd, path)

		if matcher.Match(relPath, ext, "") != "" {
			files = append(files, relPath)
			byExt[ext]++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	resp := filesResponse{
		Root:  cwd,
		Files: files,
		Stats: filesStats{
			TotalFiles:  len(files),
			ByExtension: byExt,
		},
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      "lux://files",
				MimeType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

type symbolsResponse struct {
	URI     string         `json:"uri"`
	Symbols []tools.Symbol `json:"symbols"`
}

func readSymbols(ctx context.Context, bridge *tools.Bridge, resourceURI, fileURI string) (*protocol.ResourceReadResult, error) {
	symbols, err := bridge.DocumentSymbolsRaw(ctx, lsp.DocumentURI(fileURI))
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	resp := symbolsResponse{
		URI:     fileURI,
		Symbols: symbols,
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      resourceURI,
				MimeType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func readDiagnostics(diagStore *DiagnosticsStore, resourceURI, encodedFileURI string) (*protocol.ResourceReadResult, error) {
	fileURI, err := url.PathUnescape(encodedFileURI)
	if err != nil {
		return nil, fmt.Errorf("decoding URI: %w", err)
	}

	params, ok := diagStore.Get(lsp.DocumentURI(fileURI))
	if !ok {
		params = lsp.PublishDiagnosticsParams{
			URI:         lsp.DocumentURI(fileURI),
			Diagnostics: []lsp.Diagnostic{},
		}
	}

	data, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return nil, err
	}

	return &protocol.ResourceReadResult{
		Contents: []protocol.ResourceContent{
			{
				URI:      resourceURI,
				MimeType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}
