package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
	"github.com/amarbel-llc/bob/packages/caldav/internal/clownplugin"
	"github.com/amarbel-llc/bob/packages/caldav/internal/resources"
	"github.com/amarbel-llc/bob/packages/caldav/internal/tools"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/transport"
)

// Populated at link time via `-X main.version` / `-X main.commit` by
// the amarbel-llc/nixpkgs fork's buildGoApplication. Single source of
// truth for `version` lives in flake.nix as `caldavVersion`; commit is
// derived from the flake's self.shortRev / self.dirtyShortRev.
var (
	version = "dev"
	commit  = "unknown"
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

	if flag.NArg() >= 1 && flag.Arg(0) == "version" {
		fmt.Printf("%s+%s\n", version, commit)
		return
	}

	// generate-plugin and hook work without CalDAV credentials or logging
	if flag.NArg() >= 1 && flag.Arg(0) == "generate-plugin" {
		cfg := &caldav.Config{URL: "http://placeholder", Username: "x", Password: "x"}
		logger := log.New(os.Stderr, "", log.LstdFlags)
		client := caldav.NewClient(cfg, logger)
		provider := resources.NewProvider(client)
		app := tools.RegisterAll(provider)
		generateArgs := flag.Args()[1:]
		if err := app.HandleGeneratePlugin(generateArgs, os.Stdout); err != nil {
			log.Fatalf("generating plugin: %v", err)
		}
		// Stdout-only mode ("-") emits no on-disk artifacts; nothing to extend.
		if len(generateArgs) == 1 && generateArgs[0] == "-" {
			return
		}
		dir := "."
		if len(generateArgs) == 1 {
			dir = generateArgs[0]
		}
		pluginRoot := filepath.Join(dir, "share", "purse-first", "caldav")
		if err := clownplugin.Write(pluginRoot, "caldav"); err != nil {
			log.Fatalf("generating clown plugin manifest: %v", err)
		}
		return
	}

	if flag.NArg() >= 1 && flag.Arg(0) == "hook" {
		cfg := &caldav.Config{URL: "http://placeholder", Username: "x", Password: "x"}
		logger := log.New(os.Stderr, "", log.LstdFlags)
		client := caldav.NewClient(cfg, logger)
		provider := resources.NewProvider(client)
		app := tools.RegisterAll(provider)
		if err := app.HandleHook(os.Stdin, os.Stdout); err != nil {
			log.Fatalf("handling hook: %v", err)
		}
		return
	}

	// Runtime mode — require CalDAV credentials, set up file logging
	logger, closeLog := caldav.InitLogging()
	defer closeLog()

	cfg, err := caldav.ConfigFromEnv()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	client := caldav.NewClient(cfg, logger)
	provider := resources.NewProvider(client)
	app := tools.RegisterAll(provider)

	if flag.NArg() > 0 {
		ctx := context.Background()
		if err := app.RunCLI(ctx, flag.Args(), nil); err != nil {
			fmt.Fprintf(os.Stderr, "caldav: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	t := transport.NewStdio(os.Stdin, os.Stdout)

	registry := server.NewToolRegistryV1()
	app.RegisterMCPToolsV1(registry)

	srv, err := server.New(t, server.Options{
		ServerName:    app.Name,
		ServerVersion: app.Version,
		Instructions: "CalDAV MCP server for managing tasks and calendars. " +
			"All operations are tools. Start with list_calendars to discover calendars and " +
			"populate the search index, then use list_tasks, get_task, search_tasks, etc. " +
			"Write tools: create_task, update_task, complete_task, delete_task, move_task, " +
			"create_event, update_event, delete_event, move_event, create_calendar. " +
			"Compatible with tasks.org VTODO format including subtasks, tags, recurrence, and reminders.\n\n" +
			"RECURRING TASKS: Tasks.org uses two distinct recurrence patterns. " +
			"(1) RRULE on VTODO — a single task with an RRULE property (e.g. FREQ=DAILY). " +
			"Completing one occurrence does not stop the recurrence; the server generates the next instance. " +
			"(2) Instance-per-occurrence — individual VTODO items created for each recurrence, with no RRULE. " +
			"These appear as separate tasks (e.g. weekly chores like laundry, cleaning). " +
			"When searching for recurring work, check BOTH patterns: " +
			"use list_recurring_tasks for RRULE-based tasks, " +
			"and look for repeated summaries or categories to identify instance-per-occurrence tasks. " +
			"A task without RRULE may still recur regularly.",
		Tools: registry,
	})
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
