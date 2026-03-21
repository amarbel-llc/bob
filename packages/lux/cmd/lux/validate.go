package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
)

func runValidate() error {
	var errs []string

	// Validate lsps.toml
	cfg, err := config.Load()
	if err != nil {
		errs = append(errs, fmt.Sprintf("lsps.toml: %v", err))
	}

	// Validate formatters.toml
	fmtCfg, err := config.LoadMergedFormatters()
	if err != nil {
		errs = append(errs, fmt.Sprintf("formatters.toml: %v", err))
	} else {
		if err := fmtCfg.Validate(); err != nil {
			errs = append(errs, fmt.Sprintf("formatters.toml: %v", err))
		}
	}

	// Validate filetype configs with cross-references
	filetypes, err := filetype.LoadMerged()
	if err != nil {
		errs = append(errs, fmt.Sprintf("filetype: %v", err))
	} else if cfg != nil && fmtCfg != nil {
		lspNames := make(map[string]bool)
		for _, l := range cfg.LSPs {
			lspNames[l.Name] = true
		}
		fmtNames := make(map[string]bool)
		for _, f := range fmtCfg.Formatters {
			fmtNames[f.Name] = true
		}
		if err := filetype.Validate(filetypes, lspNames, fmtNames); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	fmt.Println("All configs valid.")
	return nil
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

	// Validate after edit
	fmt.Println("\nValidating config...")
	if err := runValidate(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return err
	}

	return nil
}
