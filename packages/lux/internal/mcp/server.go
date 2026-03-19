package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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

	mcpApp.AddCommand(&command.Command{
		Name: "resource-templates",
		Description: command.Description{
			Short: "List available lux resource templates. Call this first to discover what LSP resources are available, then use resource-read to access them.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
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
			Short: "Read a lux resource by URI. This tool exists because subagents cannot access MCP resources directly (forwarded resource permissions are not yet supported). Call resource-templates to discover available URIs.",
		},
		Annotations: &protocol.ToolAnnotations{
			ReadOnlyHint:    &readOnly,
			DestructiveHint: &notDestructive,
			IdempotentHint:  &idempotent,
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

	toolRegistry := mcpserver.NewToolRegistry()
	mcpApp.RegisterMCPTools(toolRegistry)

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
