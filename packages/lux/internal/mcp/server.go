package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/jsonrpc"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/protocol"
	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
	"github.com/amarbel-llc/lux/internal/formatter"
	"github.com/amarbel-llc/lux/internal/lsp"
	"github.com/amarbel-llc/lux/internal/server"
	"github.com/amarbel-llc/lux/internal/subprocess"
	"github.com/amarbel-llc/lux/internal/tools"
	"github.com/amarbel-llc/lux/internal/warmup"
)

type Server struct {
	inner     *mcpserver.Server
	pool      *subprocess.Pool
	docMgr    *DocumentManager
	diagStore *DiagnosticsStore
	transport transport.Transport
}

func New(cfg *config.Config, t transport.Transport) (*Server, error) {
	ftConfigs, err := filetype.LoadMerged()
	if err != nil {
		fmt.Fprintf(logfile.Writer(), "warning: could not load filetype config: %v\n", err)
		ftConfigs = []*filetype.Config{}
	}

	router, err := server.NewRouter(ftConfigs)
	if err != nil {
		return nil, fmt.Errorf("creating router: %w", err)
	}

	executor := subprocess.NewNixExecutor()

	s := &Server{
		transport: t,
		diagStore: NewDiagnosticsStore(),
	}

	s.pool = subprocess.NewPool(executor, func(lspName string) jsonrpc.Handler {
		return s.lspNotificationHandler(lspName)
	})

	for _, l := range cfg.LSPs {
		var capOverrides *subprocess.CapabilityOverride
		if l.Capabilities != nil {
			capOverrides = &subprocess.CapabilityOverride{
				Disable: l.Capabilities.Disable,
				Enable:  l.Capabilities.Enable,
			}
		}
		s.pool.Register(l.Name, l.Flake, l.Binary, l.Args, l.Env, l.InitOptions, l.Settings, l.SettingsWireKey(), capOverrides, l.ShouldWaitForReady(), l.ReadyTimeoutDuration(), l.ActivityTimeoutDuration())
	}

	var fmtRouter *formatter.Router
	fmtCfg, err := config.LoadMergedFormatters()
	if err != nil {
		fmt.Fprintf(logfile.Writer(), "warning: could not load formatter config: %v\n", err)
	} else {
		fmtMap := make(map[string]*config.Formatter)
		for i := range fmtCfg.Formatters {
			f := &fmtCfg.Formatters[i]
			if !f.Disabled {
				fmtMap[f.Name] = f
			}
		}

		fmtRouter, err = formatter.NewRouter(ftConfigs, fmtMap)
		if err != nil {
			fmt.Fprintf(logfile.Writer(), "warning: could not create formatter router: %v\n", err)
			fmtRouter = nil
		}
	}

	bridge := tools.NewBridge(s.pool, router, fmtRouter, executor, func(lspName, message string) {
		notification, err := jsonrpc.NewNotification("notifications/message", map[string]any{
			"level": "info",
			"data":  fmt.Sprintf("%s: %s", lspName, message),
		})
		if err == nil {
			t.Write(notification)
		}
	})
	s.docMgr = NewDocumentManager(s.pool, router, bridge)
	bridge.SetDocumentManager(s.docMgr)

	resourceRegistry := mcpserver.NewResourceRegistry()
	registerResources(resourceRegistry, s.pool, bridge, cfg, ftConfigs, s.diagStore)

	resProvider := newResourceProvider(resourceRegistry, bridge, s.diagStore)

	// Create a minimal MCP app with only the read_resource tool.
	// LSP operations are exposed as resource templates for progressive disclosure:
	// clients list templates to discover capabilities, then read resources through
	// this single tool.
	mcpApp := command.NewApp("lux", "MCP server exposing LSP capabilities as resources")
	mcpApp.Version = "0.1.0"
	mcpApp.MCPArgs = []string{"mcp-stdio"}

	readOnly := true
	notDestructive := false
	idempotent := true
	notOpenWorld := false

	mcpApp.AddCommand(&command.Command{
		Name: "resource-templates",
		Description: command.Description{
			Short: "List available lux resource templates. For subagent use only — the main conversation should use MCP resources directly via ReadMcpResourceTool. Call resource-templates to discover URIs, then use resource-read to access them.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
			OpenWorldHint:   &notOpenWorld,
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			templates, err := resProvider.ListResourceTemplates(ctx)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			resources, err := resProvider.ListResources(ctx)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			var sb strings.Builder
			sb.WriteString("Resource templates (fill in {placeholders} and pass to resource-read):\n\n")
			for _, t := range templates {
				fmt.Fprintf(&sb, "- %s: %s\n  %s\n", t.Name, t.URITemplate, t.Description)
			}

			if len(resources) > 0 {
				sb.WriteString("\nStatic resources (pass URI directly to resource-read):\n\n")
				for _, r := range resources {
					fmt.Fprintf(&sb, "- %s: %s\n  %s\n", r.Name, r.URI, r.Description)
				}
			}

			sb.WriteString("\nFile URIs must be file:// URIs (e.g., file:///path/to/file.go). Line and character are 0-indexed.")

			return command.TextResult(sb.String()), nil
		},
	})

	mcpApp.AddCommand(&command.Command{
		Name: "resource-read",
		Description: command.Description{
			Short: "Read a lux resource by URI. For subagent use only — the main conversation should use MCP resources directly via ReadMcpResourceTool. This tool exists because subagents cannot access MCP resources directly (forwarded resource permissions are not yet supported).",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
			OpenWorldHint:   &notOpenWorld,
		},
		Params: []command.Param{
			{Name: "uri", Type: command.String, Description: "Resource URI (e.g., lux://lsp/hover?uri=file:///path/to/file.go&line=10&character=5)", Required: true},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			result, err := resProvider.ReadResource(ctx, a.URI)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			var sb strings.Builder
			for i, c := range result.Contents {
				if i > 0 {
					sb.WriteString("\n---\n")
				}
				sb.WriteString(c.Text)
			}

			return command.TextResult(sb.String()), nil
		},
	})

	notReadOnly := false
	mcpApp.AddCommand(&command.Command{
		Name: "execute-command",
		Description: command.Description{
			Short: "Execute a workspace/executeCommand on a specific LSP server. Use lux://commands to discover available commands.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &notReadOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
			OpenWorldHint:   &notOpenWorld,
		},
		Params: []command.Param{
			{Name: "lsp", Type: command.String, Description: "LSP server name (e.g. 'gopls'). See lux://status for configured LSPs.", Required: true},
			{Name: "command", Type: command.String, Description: "Command ID to execute (e.g. 'gopls.packages'). See lux://commands for available commands.", Required: true},
			{Name: "arguments", Type: command.String, Description: "JSON-encoded command arguments (optional)", Required: false},
		},
		Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
			var a struct {
				LSP       string `json:"lsp"`
				Command   string `json:"command"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			found := false
			for _, l := range cfg.LSPs {
				if l.Name == a.LSP {
					found = true
					break
				}
			}
			if !found {
				return command.TextErrorResult(fmt.Sprintf("unknown LSP %q — check lux://status for configured LSPs", a.LSP)), nil
			}

			commands := s.pool.Commands()
			if cmds, ok := commands[a.LSP]; ok {
				cmdFound := false
				for _, c := range cmds {
					if c == a.Command {
						cmdFound = true
						break
					}
				}
				if !cmdFound {
					return command.TextErrorResult(fmt.Sprintf("LSP %q does not advertise command %q — check lux://commands", a.LSP, a.Command)), nil
				}
			}

			var arguments json.RawMessage
			if a.Arguments != "" {
				arguments = json.RawMessage(a.Arguments)
			}

			result, err := bridge.ExecuteCommand(ctx, a.LSP, a.Command, arguments)
			if err != nil {
				return command.TextErrorResult(err.Error()), nil
			}

			var pretty bytes.Buffer
			if err := json.Indent(&pretty, result, "", "  "); err != nil {
				return command.TextResult(string(result)), nil
			}
			return command.TextResult(pretty.String()), nil
		},
	})

	toolRegistry := mcpserver.NewToolRegistryV1()
	mcpApp.RegisterMCPToolsV1(toolRegistry)

	promptRegistry := mcpserver.NewPromptRegistry()
	registerPrompts(promptRegistry)

	inner, err := mcpserver.New(t, mcpserver.Options{
		ServerName:    mcpApp.Name,
		ServerVersion: mcpApp.Version,
		Tools:         toolRegistry,
		Resources:     resProvider,
		Prompts:       promptRegistry,
	})
	if err != nil {
		return nil, fmt.Errorf("creating MCP server: %w", err)
	}

	s.inner = inner

	go warmup.PreBuildAll(context.Background(), cfg, executor)

	go func() {
		cwd, _ := os.Getwd()
		initParams := bridge.DefaultInitParams(lsp.DocumentURI("file://" + cwd + "/dummy"))
		scanner := warmup.NewScanner(cfg, ftConfigs)
		warmup.StartRelevantLSPs(context.Background(), s.pool, scanner, []string{cwd}, initParams, cfg)
	}()

	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	defer func() {
		if s.docMgr != nil {
			s.docMgr.CloseAll()
		}
		if s.pool != nil {
			s.pool.StopAll()
		}
	}()
	return s.inner.Run(ctx)
}

func (s *Server) Close() {
	s.inner.Close()
}

func (s *Server) lspNotificationHandler(lspName string) jsonrpc.Handler {
	return func(ctx context.Context, msg *jsonrpc.Message) (*jsonrpc.Message, error) {
		if msg.IsRequest() && msg.Method == lsp.MethodWindowWorkDoneProgressCreate {
			if inst, ok := s.pool.Get(lspName); ok && inst.Progress != nil {
				var params lsp.WorkDoneProgressCreateParams
				if err := json.Unmarshal(msg.Params, &params); err == nil {
					inst.Progress.HandleCreate(params.Token)
				}
			}
			return jsonrpc.NewResponse(*msg.ID, nil)
		}

		if msg.IsNotification() && msg.Method == lsp.MethodProgress {
			if inst, ok := s.pool.Get(lspName); ok && inst.Progress != nil {
				var params lsp.ProgressParams
				if err := json.Unmarshal(msg.Params, &params); err == nil {
					inst.Progress.HandleProgress(params.Token, params.Value)

					active := inst.Progress.ActiveProgress()
					for _, tok := range active {
						logMsg := tok.Title
						if tok.Message != "" {
							logMsg += ": " + tok.Message
						}
						if tok.Pct != nil {
							logMsg += fmt.Sprintf(" (%d%%)", *tok.Pct)
						}
						fmt.Fprintf(logfile.Writer(), "[lux] %s: %s\n", lspName, logMsg)
					}
				}
			}
			return nil, nil
		}

		if msg.Method == "textDocument/publishDiagnostics" && msg.Params != nil {
			var params lsp.PublishDiagnosticsParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return nil, nil
			}

			s.diagStore.Update(params)

			resourceURI := DiagnosticsResourceURI(params.URI)
			notification, err := jsonrpc.NewNotification("notifications/resources/updated", map[string]string{
				"uri": resourceURI,
			})
			if err == nil {
				s.transport.Write(notification)
			}
		}

		return nil, nil
	}
}
