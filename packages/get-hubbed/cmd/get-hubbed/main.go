package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
	"github.com/friedenberg/get-hubbed/internal/clone"
	"github.com/friedenberg/get-hubbed/internal/hooks"
	"github.com/friedenberg/get-hubbed/internal/tools"
)

func main() {
	app, resProvider := tools.RegisterAll()

	if len(os.Args) >= 2 && os.Args[1] == "generate-plugin" {
		if err := app.HandleGeneratePlugin(os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("generating plugin: %v", err)
		}

		// Patch hooks.json to also match WebFetch tool uses
		pluginDir := resolvePluginDir(os.Args[2:])
		if err := hooks.PatchHooksMatcher(pluginDir, "WebFetch"); err != nil {
			log.Fatalf("patching hooks matcher: %v", err)
		}

		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "hook" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("reading hook input: %v", err)
		}

		handled, err := hooks.HandleWebFetchHook(input, os.Stdout)
		if err != nil {
			log.Fatalf("handling webfetch hook: %v", err)
		}

		if !handled {
			if err := app.HandleHook(bytes.NewReader(input), os.Stdout); err != nil {
				log.Fatalf("handling hook: %v", err)
			}
		}

		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "clone" {
		if len(os.Args) >= 3 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
			fmt.Println("Usage: get-hubbed clone [dir]")
			fmt.Println()
			fmt.Println("Clone uncloned repos for the authenticated GitHub user.")
			fmt.Println("Defaults to current directory if dir is omitted.")
			os.Exit(0)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		targetDir := "."
		if len(os.Args) >= 3 {
			targetDir = os.Args[2]
		}

		if err := clone.Run(ctx, targetDir); err != nil {
			log.Fatalf("clone: %v", err)
		}
		return
	}

	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			fmt.Println("get-hubbed - a GitHub MCP server wrapping the gh CLI")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  get-hubbed              Start MCP server (stdio)")
			fmt.Println("  get-hubbed clone [dir]   Clone uncloned repos for authenticated user")
			fmt.Println()
			os.Exit(0)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	t := transport.NewStdio(os.Stdin, os.Stdout)

	registry := server.NewToolRegistryV1()
	app.RegisterMCPToolsV1(registry)
	tools.RegisterAPITools(registry)

	opts := server.Options{
		ServerName:    app.Name,
		ServerVersion: app.Version,
		Instructions: "GitHub MCP server. Read-only operations (repo info, issues, PRs, content, runs) are available as auto-approved resources via get-hubbed:// URIs. Mutation operations (issue/PR creation, comments, API calls) remain as tools." +
			"\n\nIMPORTANT: There are no tools named content_read, content_tree, content_commits, or repo_view. Use resource-read with get-hubbed:// URIs instead." +
			" All resource URIs use query parameters (e.g. get-hubbed://contents?path=README.md, get-hubbed://issues?number=42). Call resource-templates to see all available URIs.",
		Tools: registry,
	}

	if resProvider != nil {
		opts.Resources = resProvider
	}

	srv, err := server.New(t, opts)
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// resolvePluginDir determines where generate-plugin wrote its output.
// Mirrors the HandleGeneratePlugin dispatch: 0 args = ".", 1 arg = that dir.
// Returns the share/purse-first/get-hubbed subdirectory where hooks.json lives.
func resolvePluginDir(args []string) string {
	base := "."
	for _, a := range args {
		if a != "-" && !strings.HasPrefix(a, "-") {
			base = a
			break
		}
	}
	return filepath.Join(base, "share", "purse-first", "get-hubbed")
}
