// Package clownplugin emits clown plugin manifest artifacts alongside the
// claude-code plugin manifest produced by go-mcp's GenerateAllWithSkills.
//
// See bob#115 and clown FDR 0002 for context.
package clownplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write emits the clown plugin artifacts into pluginRoot:
//
//	<pluginRoot>/clown.json
//	<pluginRoot>/.clown-plugin/clown.json
//	<pluginRoot>/bin/<binaryName> -> ../../../../bin/<binaryName> (relative symlink)
//
// pluginRoot is the existing plugin directory created by go-mcp's
// GenerateAllWithSkills (e.g. <out>/share/purse-first/caldav). The symlink
// target assumes the binary lives at <out>/bin/<binaryName>, which is the
// only layout produced by mkGoWorkspaceModule. Symlink targets resolve
// relative to the link's parent directory, so escaping
// share/purse-first/caldav/bin to <out> takes four ".." segments.
//
// stdioServers.<name>.command is written as the absolute path
// abs(pluginRoot)/bin/<binaryName>. clown's bridge runs exec.LookPath on
// this value from clown's own CWD, so a relative path like
// "bin/<binaryName>" resolves against the wrong directory and surfaces as
// "stdout closed before handshake". An absolute path is the correct shape
// for a Nix-built artifact: $out is the package's permanent install
// location, so the path baked into the manifest is stable for the
// artifact's lifetime. See clown#36 for the upstream fix that makes
// plugin-relative paths well-defined.
//
// Both manifest paths are written so the package works against today's clown
// (which reads <root>/clown.json) and survives the clown#32 migration to
// <root>/.clown-plugin/clown.json.
//
// Re-running over an existing tree is idempotent: files are overwritten and
// the symlink is recreated.
func Write(pluginRoot, binaryName string) error {
	if pluginRoot == "" {
		return fmt.Errorf("clownplugin: pluginRoot is empty")
	}
	if binaryName == "" {
		return fmt.Errorf("clownplugin: binaryName is empty")
	}

	absRoot, err := filepath.Abs(pluginRoot)
	if err != nil {
		return fmt.Errorf("clownplugin: absolutize pluginRoot %q: %w", pluginRoot, err)
	}

	manifest := clownConfig{
		Version: 1,
		StdioServers: map[string]stdioServer{
			binaryName: {Command: filepath.Join(absRoot, "bin", binaryName)},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("clownplugin: marshal manifest: %w", err)
	}
	data = append(data, '\n')

	rootPath := filepath.Join(pluginRoot, "clown.json")
	if err := os.WriteFile(rootPath, data, 0o644); err != nil {
		return fmt.Errorf("clownplugin: write %s: %w", rootPath, err)
	}

	nestedDir := filepath.Join(pluginRoot, ".clown-plugin")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		return fmt.Errorf("clownplugin: mkdir %s: %w", nestedDir, err)
	}
	nestedPath := filepath.Join(nestedDir, "clown.json")
	if err := os.WriteFile(nestedPath, data, 0o644); err != nil {
		return fmt.Errorf("clownplugin: write %s: %w", nestedPath, err)
	}

	binDir := filepath.Join(pluginRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("clownplugin: mkdir %s: %w", binDir, err)
	}
	linkPath := filepath.Join(binDir, binaryName)
	linkTarget := filepath.Join("..", "..", "..", "..", "bin", binaryName)
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clownplugin: remove existing %s: %w", linkPath, err)
	}
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		return fmt.Errorf("clownplugin: symlink %s -> %s: %w", linkPath, linkTarget, err)
	}

	return nil
}

type clownConfig struct {
	Version      int                    `json:"version"`
	StdioServers map[string]stdioServer `json:"stdioServers"`
}

type stdioServer struct {
	Command string `json:"command"`
}
