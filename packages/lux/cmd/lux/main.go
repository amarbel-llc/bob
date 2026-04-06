package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/amarbel-llc/lux/internal/hooks"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/lux/internal/tools"
)

var version = "dev"

func main() {
	cleanup := logfile.Init()
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := buildApp()

	if len(os.Args) >= 2 && os.Args[1] == "generate-plugin" {
		tools.RegisterAll(app, nil)
		if err := app.HandleGeneratePlugin(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(logfile.Writer(), "Error: %v\n", err)
			os.Exit(1)
		}

		outDir := generatePluginOutputDir(os.Args[2:])
		if outDir != "" {
			pluginDir := filepath.Join(outDir, "share", "purse-first", "lux")
			if err := hooks.GenerateStopHook(pluginDir); err != nil {
				fmt.Fprintf(logfile.Writer(), "Error generating Stop hook: %v\n", err)
				os.Exit(1)
			}
		}

		return
	}

	if err := app.RunCLI(ctx, os.Args[1:], nil); err != nil {
		if ctx.Err() != nil {
			// Clean shutdown via signal — not an error
			return
		}
		fmt.Fprintf(logfile.Writer(), "Error: %v\n", err)
		os.Exit(1)
	}
}

// generatePluginOutputDir determines where HandleGeneratePlugin wrote its
// output. Returns "" for stdout-only mode ("-").
func generatePluginOutputDir(args []string) string {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("skills-dir", "", "")

	if err := fs.Parse(args); err != nil {
		return "."
	}

	remaining := fs.Args()

	switch len(remaining) {
	case 0:
		return "."
	case 1:
		if remaining[0] == "-" {
			return ""
		}
		return remaining[0]
	default:
		return "."
	}
}
