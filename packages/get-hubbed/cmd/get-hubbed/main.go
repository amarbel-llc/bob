package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
	"github.com/friedenberg/get-hubbed/internal/clone"
	"github.com/friedenberg/get-hubbed/internal/tools"
)

func main() {
	app, resProvider := tools.RegisterAll()

	if len(os.Args) >= 2 && os.Args[1] == "generate-plugin" {
		if err := app.HandleGeneratePlugin(os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("generating plugin: %v", err)
		}
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "hook" {
		if err := app.HandleHook(os.Stdin, os.Stdout); err != nil {
			log.Fatalf("handling hook: %v", err)
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
		Instructions:  "GitHub MCP server. Read-only operations (repo info, issues, PRs, content, runs) are available as auto-approved resources via get-hubbed:// URIs. Mutation operations (issue/PR creation, API calls) remain as tools.",
		Tools:         registry,
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
