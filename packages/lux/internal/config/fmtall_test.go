package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFmtAll_DefaultsWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := LoadFmtAll()
	if err != nil {
		t.Fatalf("LoadFmtAll: %v", err)
	}
	if cfg.Walk != "git" {
		t.Errorf("Walk = %q, want %q", cfg.Walk, "git")
	}
	if len(cfg.ExcludeGlobs) != 0 {
		t.Errorf("ExcludeGlobs = %v, want empty", cfg.ExcludeGlobs)
	}
}

func TestLoadFmtAll_LoadsFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	luxDir := filepath.Join(dir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "all"
exclude_globs = ["**/flake.lock", "**/go.sum"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFmtAll()
	if err != nil {
		t.Fatalf("LoadFmtAll: %v", err)
	}
	if cfg.Walk != "all" {
		t.Errorf("Walk = %q, want %q", cfg.Walk, "all")
	}
	if len(cfg.ExcludeGlobs) != 2 {
		t.Errorf("ExcludeGlobs = %v, want 2 entries", cfg.ExcludeGlobs)
	}
}

func TestLoadFmtAll_InvalidWalk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	luxDir := filepath.Join(dir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "invalid"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFmtAll()
	if err == nil {
		t.Fatal("expected error for invalid walk value")
	}
}

func TestLoadMergedFmtAll_LocalOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalDir)
	luxDir := filepath.Join(globalDir, "lux")
	if err := os.MkdirAll(luxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(luxDir, "fmt-all.toml"), []byte(`
walk = "git"
exclude_globs = ["**/flake.lock"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	localDir := filepath.Join(projectDir, ".lux")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "fmt-all.toml"), []byte(`
walk = "all"
exclude_globs = ["**/Cargo.lock"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMergedFmtAll(projectDir)
	if err != nil {
		t.Fatalf("LoadMergedFmtAll: %v", err)
	}
	if cfg.Walk != "all" {
		t.Errorf("Walk = %q, want %q (local override)", cfg.Walk, "all")
	}
	if len(cfg.ExcludeGlobs) != 1 || cfg.ExcludeGlobs[0] != "**/Cargo.lock" {
		t.Errorf("ExcludeGlobs = %v, want [**/Cargo.lock] (local override)", cfg.ExcludeGlobs)
	}
}

func TestLoadMergedFmtAll_EmptyProjectRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := LoadMergedFmtAll("")
	if err != nil {
		t.Fatalf("LoadMergedFmtAll: %v", err)
	}
	if cfg.Walk != "git" {
		t.Errorf("Walk = %q, want %q", cfg.Walk, "git")
	}
}
