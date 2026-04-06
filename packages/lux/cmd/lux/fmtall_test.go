package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitWalk_ReturnsTrackedAndUntrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package main"), 0o644)
	run("add", "tracked.go")
	run("commit", "-m", "init")

	os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package main"), 0o644)

	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.go\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.go"), []byte("package main"), 0o644)

	files, err := gitWalk(dir)
	if err != nil {
		t.Fatalf("gitWalk: %v", err)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[filepath.Base(f)] = true
	}

	if !fileSet["tracked.go"] {
		t.Error("missing tracked.go")
	}
	if !fileSet["untracked.go"] {
		t.Error("missing untracked.go")
	}
	if fileSet["ignored.go"] {
		t.Error("ignored.go should not be included")
	}
}

func TestAllWalk_SkipsGitDir(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "objects", "foo"), []byte("x"), 0o644)

	files, err := allWalk(dir)
	if err != nil {
		t.Fatalf("allWalk: %v", err)
	}

	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		if len(rel) >= 4 && rel[:4] == ".git" {
			t.Errorf("should skip .git: %s", rel)
		}
	}

	found := false
	for _, f := range files {
		if filepath.Base(f) == "file.go" {
			found = true
		}
	}
	if !found {
		t.Error("missing file.go")
	}
}

func TestExcludeGlobs_FiltersMatchingPaths(t *testing.T) {
	root := "/repo"
	files := []string{
		filepath.Join(root, "flake.lock"),
		filepath.Join(root, "sub", "flake.lock"),
		filepath.Join(root, "main.go"),
		filepath.Join(root, "go.sum"),
		filepath.Join(root, "vendor", "go.sum"),
	}
	globs := []string{"**/flake.lock", "flake.lock", "**/go.sum", "go.sum"}

	filtered := applyExcludeGlobs(files, globs, root)
	if len(filtered) != 1 || filepath.Base(filtered[0]) != "main.go" {
		t.Errorf("filtered = %v, want only main.go", filtered)
	}
}

func TestExcludeGlobs_EmptyPatterns(t *testing.T) {
	files := []string{"/repo/a.go", "/repo/b.go"}
	filtered := applyExcludeGlobs(files, nil, "/repo")
	if len(filtered) != 2 {
		t.Errorf("got %d files, want 2 (no filtering)", len(filtered))
	}
}
