package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/bob/packages/caldav/internal/tools"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "caldav — an MCP server for CalDAV task management\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  caldav [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Starts an MCP server on stdio.\n")
		fmt.Fprintf(os.Stderr, "Intended to be launched by an MCP client such as Claude Code.\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  CALDAV_URL       Base URL of the CalDAV server (required)\n")
		fmt.Fprintf(os.Stderr, "  CALDAV_USERNAME   HTTP Basic auth username (required)\n")
		fmt.Fprintf(os.Stderr, "  CALDAV_PASSWORD   HTTP Basic auth password (required)\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// generate-plugin and hook work without CalDAV credentials
	if flag.NArg() >= 1 && flag.Arg(0) == "generate-plugin" {
		cfg := &caldav.Config{URL: "http://placeholder", Username: "x", Password: "x"}
		client := caldav.NewClient(cfg)
		provider := resources.NewProvider(client)
		app := tools.RegisterAll(provider)
		if err := app.HandleGeneratePlugin(flag.Args()[1:], os.Stdout); err != nil {
			log.Fatalf("generating plugin: %v", err)
		}
		return
	}

	if flag.NArg() >= 1 && flag.Arg(0) == "hook" {
		cfg := &caldav.Config{URL: "http://placeholder", Username: "x", Password: "x"}
		client := caldav.NewClient(cfg)
		provider := resources.NewProvider(client)
		app := tools.RegisterAll(provider)
		if err := app.HandleHook(os.Stdin, os.Stdout); err != nil {
			log.Fatalf("handling hook: %v", err)
		}
		return
	}

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "caldav: unexpected arguments: %v\n", flag.Args())
		flag.Usage()
		os.Exit(1)
	}

	// Runtime mode — require CalDAV credentials
	cfg, err := caldav.ConfigFromEnv()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	client := caldav.NewClient(cfg)
	provider := resources.NewProvider(client)
	app := tools.RegisterAll(provider)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	t := transport.NewStdio(os.Stdin, os.Stdout)

	registry := server.NewToolRegistryV1()
	app.RegisterMCPToolsV1(registry)

	srv, err := server.New(t, server.Options{
		ServerName:    app.Name,
		ServerVersion: app.Version,
		Instructions: "CalDAV MCP server for managing tasks and calendars. " +
			"Reads are exposed as resources with progressive disclosure " +
			"(caldav://calendars → caldav://calendar/{id} → caldav://task/{uid} → caldav://task/{uid}/ical). " +
			"Writes use tools (create_task, update_task, complete_task, delete_task, move_task, create_calendar). " +
			"Compatible with tasks.org VTODO format including subtasks, tags, recurrence, and reminders.\n\n" +
			"RECURRING TASKS: Tasks.org uses two distinct recurrence patterns. " +
			"(1) RRULE on VTODO — a single task with an RRULE property (e.g. FREQ=DAILY). " +
			"Completing one occurrence does not stop the recurrence; the server generates the next instance. " +
			"(2) Instance-per-occurrence — individual VTODO items created for each recurrence, with no RRULE. " +
			"These appear as separate tasks (e.g. weekly chores like laundry, cleaning). " +
			"When searching for recurring work, check BOTH patterns: " +
			"filter metadata for non-empty rrule to find RRULE-based tasks, " +
			"and look for repeated summaries or categories to identify instance-per-occurrence tasks. " +
			"A task without RRULE may still recur regularly.",
		Tools:         registry,
		Resources:     provider,
	})
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
