package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
	"github.com/friedenberg/grit/internal/hooks"
	"github.com/friedenberg/grit/internal/tools"
	intTransport "github.com/friedenberg/grit/internal/transport"
)

func main() {
	sseMode := flag.Bool("sse", false, "Use HTTP/SSE transport instead of stdio")
	port := flag.Int("port", 8080, "Port for HTTP/SSE transport")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "grit — an MCP server exposing git operations\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  grit [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Starts an MCP server on stdio (default) or HTTP/SSE.\n")
		fmt.Fprintf(os.Stderr, "Intended to be launched by an MCP client such as Claude Code.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  grit                     # stdio transport\n")
		fmt.Fprintf(os.Stderr, "  grit --sse --port 8080   # HTTP/SSE transport\n")
	}

	flag.Parse()

	app, resProvider := tools.RegisterAll()

	if flag.NArg() >= 1 && flag.Arg(0) == "generate-plugin" {
		if err := app.HandleGeneratePlugin(flag.Args()[1:], os.Stdout); err != nil {
			log.Fatalf("generating plugin: %v", err)
		}
		return
	}

	if flag.NArg() >= 1 && flag.Arg(0) == "hook" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("reading hook input: %v", err)
		}

		handled, err := hooks.HandleResourceHook(input, os.Stdout)
		if err != nil {
			log.Fatalf("handling resource hook: %v", err)
		}

		if !handled {
			if err := app.HandleHook(bytes.NewReader(input), os.Stdout); err != nil {
				log.Fatalf("handling hook: %v", err)
			}
		}

		return
	}

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "grit: unexpected arguments: %v\n", flag.Args())
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var t transport.Transport

	if *sseMode {
		sse := intTransport.NewSSE(fmt.Sprintf(":%d", *port))
		if err := sse.Start(ctx); err != nil {
			log.Fatalf("starting SSE transport: %v", err)
		}
		defer sse.Close()
		log.Printf("SSE transport listening on %s", sse.Addr())
		t = sse
	} else {
		t = transport.NewStdio(os.Stdin, os.Stdout)
	}

	registry := server.NewToolRegistryV1()
	app.RegisterMCPToolsV1(registry)

	opts := server.Options{
		ServerName:    app.Name,
		ServerVersion: app.Version,
		Instructions:  "Git MCP server exposing repository operations. Read-only operations (status, log, show, blame, branches, remotes, tags) are available as MCP resources. Mutation operations (commit, push, rebase, etc.) remain as tools.",
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
