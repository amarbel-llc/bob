package clownplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_EmitsBothManifests(t *testing.T) {
	pluginRoot := t.TempDir()

	if err := Write(pluginRoot, "caldav"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rootBytes, err := os.ReadFile(filepath.Join(pluginRoot, "clown.json"))
	if err != nil {
		t.Fatalf("read clown.json: %v", err)
	}
	nestedBytes, err := os.ReadFile(filepath.Join(pluginRoot, ".clown-plugin", "clown.json"))
	if err != nil {
		t.Fatalf("read .clown-plugin/clown.json: %v", err)
	}
	if string(rootBytes) != string(nestedBytes) {
		t.Errorf("root and nested manifests differ:\nroot=%s\nnested=%s", rootBytes, nestedBytes)
	}

	var got struct {
		Version      int `json:"version"`
		StdioServers map[string]struct {
			Command string `json:"command"`
		} `json:"stdioServers"`
	}
	if err := json.Unmarshal(rootBytes, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	srv, ok := got.StdioServers["caldav"]
	if !ok {
		t.Fatalf("stdioServers.caldav missing; got %#v", got.StdioServers)
	}
	wantCmd := filepath.Join(pluginRoot, "bin", "caldav")
	if srv.Command != wantCmd {
		t.Errorf("stdioServers.caldav.command = %q, want %q", srv.Command, wantCmd)
	}
	if !filepath.IsAbs(srv.Command) {
		t.Errorf("stdioServers.caldav.command must be absolute (clown#36 / bridge CWD bug); got %q", srv.Command)
	}
}

func TestWrite_BinarySymlink(t *testing.T) {
	pluginRoot := t.TempDir()

	if err := Write(pluginRoot, "caldav"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	linkPath := filepath.Join(pluginRoot, "bin", "caldav")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join("..", "..", "..", "..", "bin", "caldav")
	if target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink (mode=%s)", linkPath, info.Mode())
	}
}

func TestWrite_Idempotent(t *testing.T) {
	pluginRoot := t.TempDir()

	for i := 0; i < 3; i++ {
		if err := Write(pluginRoot, "caldav"); err != nil {
			t.Fatalf("Write iteration %d: %v", i, err)
		}
	}

	if _, err := os.Stat(filepath.Join(pluginRoot, "clown.json")); err != nil {
		t.Errorf("clown.json missing after repeated Write: %v", err)
	}
	target, err := os.Readlink(filepath.Join(pluginRoot, "bin", "caldav"))
	if err != nil {
		t.Fatalf("Readlink after repeated Write: %v", err)
	}
	if want := filepath.Join("..", "..", "..", "..", "bin", "caldav"); target != want {
		t.Errorf("symlink target after repeated Write = %q, want %q", target, want)
	}
}

func TestWrite_RejectsEmptyArgs(t *testing.T) {
	if err := Write("", "caldav"); err == nil {
		t.Error("Write with empty pluginRoot: expected error, got nil")
	}
	if err := Write(t.TempDir(), ""); err == nil {
		t.Error("Write with empty binaryName: expected error, got nil")
	}
}

func TestWrite_UsesBinaryName(t *testing.T) {
	pluginRoot := t.TempDir()

	if err := Write(pluginRoot, "lux"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pluginRoot, "clown.json"))
	if err != nil {
		t.Fatalf("read clown.json: %v", err)
	}
	var got struct {
		StdioServers map[string]struct {
			Command string `json:"command"`
		} `json:"stdioServers"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got.StdioServers["lux"]; !ok {
		t.Errorf("stdioServers.lux missing for binaryName=lux; got %#v", got.StdioServers)
	}
	target, err := os.Readlink(filepath.Join(pluginRoot, "bin", "lux"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if want := filepath.Join("..", "..", "..", "..", "bin", "lux"); target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}
}
