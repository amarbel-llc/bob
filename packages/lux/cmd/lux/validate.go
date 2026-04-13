package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/validate"
)

func runValidate(checkFlakes, checkFormatters, checkLSPs bool) error {
	ctx := context.Background()

	result, err := validate.Run(ctx, validate.Options{
		CheckFlakes:     checkFlakes,
		CheckFormatters: checkFormatters,
		CheckLSPs:       checkLSPs,
	})
	if err != nil {
		return err
	}

	printResults(result)

	if result.Failed > 0 {
		return fmt.Errorf("%d check(s) failed", result.Failed)
	}
	return nil
}

func printResults(result *validate.Result) {
	var lastCategory string
	for _, c := range result.Checks {
		if c.Category != lastCategory {
			if lastCategory != "" {
				fmt.Println()
			}
			fmt.Println(c.Category)
			lastCategory = c.Category
		}

		line := fmt.Sprintf("  %s %s", c.Status, c.Name)
		if c.Message != "" {
			// For failures, put the message on a separate indented line
			if c.Status == validate.Fail {
				line += "\n      " + strings.ReplaceAll(c.Message, "\n", "\n      ")
			} else {
				line += " — " + c.Message
			}
		}
		if c.Duration > 0 {
			line += fmt.Sprintf(" (%.1fs)", c.Duration.Seconds())
		}
		fmt.Println(line)
	}

	fmt.Printf("\n%d passed, %d failed, %d skipped\n", result.Passed, result.Failed, result.Skipped)
}

func runConfigEdit(name string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set")
	}

	configDir := config.ConfigDir()

	var path string
	switch {
	case name == "lsps":
		path = filepath.Join(configDir, "lsps.toml")
	case name == "formatters":
		path = filepath.Join(configDir, "formatters.toml")
	case strings.HasPrefix(name, "filetype/"):
		ftName := strings.TrimPrefix(name, "filetype/")
		if ftName == "" {
			return fmt.Errorf("filetype name is required (e.g., filetype/go)")
		}
		path = filepath.Join(configDir, "filetype", ftName+".toml")
	default:
		return fmt.Errorf("unknown config %q (expected: lsps, formatters, or filetype/NAME)", name)
	}

	// Check file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s\nRun 'lux init' to create config files.", path)
	}

	// Open editor
	editorPath, err := exec.LookPath(editor)
	if err != nil {
		return fmt.Errorf("editor %q not found: %w", editor, err)
	}

	cmd := exec.Command(editorPath, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// If editor was killed by signal, propagate it
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return fmt.Errorf("editor killed by signal")
			}
		}
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Validate after edit (config-only)
	fmt.Println("\nValidating config...")
	if err := runValidate(false, false, false); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return err
	}

	return nil
}
