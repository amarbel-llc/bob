package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseWorktreeError_BrokenGitdir(t *testing.T) {
	dir := t.TempDir()

	// Write a .git file pointing to a non-existent gitdir (simulates stale worktree)
	staleGitdir := "/nonexistent/path/.git/worktrees/my-worktree"
	os.WriteFile(
		filepath.Join(dir, ".git"),
		[]byte("gitdir: "+staleGitdir+"\n"),
		0o644,
	)

	gitErr := fmt.Errorf("git %v: exit status 128: fatal: not a git repository: %s", []string{"status"}, staleGitdir)

	result := diagnoseWorktreeError(dir, gitErr)
	if result == nil {
		t.Fatal("expected diagnostic error, got nil")
	}

	errMsg := result.Error()

	if !strings.Contains(errMsg, staleGitdir) {
		t.Errorf("error should contain stale gitdir path %q, got: %s", staleGitdir, errMsg)
	}

	if !strings.Contains(errMsg, "git worktree repair") {
		t.Errorf("error should suggest 'git worktree repair', got: %s", errMsg)
	}
}

func TestDiagnoseWorktreeError_NoGitFile(t *testing.T) {
	dir := t.TempDir()

	gitErr := fmt.Errorf("git status: exit status 128: fatal: not a git repository")

	result := diagnoseWorktreeError(dir, gitErr)
	if result != nil {
		t.Errorf("expected nil for non-worktree dir, got: %v", result)
	}
}

func TestDiagnoseWorktreeError_GitDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create .git as a directory (regular repo, not worktree)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	gitErr := fmt.Errorf("git status: exit status 128: fatal: not a git repository")

	result := diagnoseWorktreeError(dir, gitErr)
	if result != nil {
		t.Errorf("expected nil for regular repo, got: %v", result)
	}
}

func TestDiagnoseWorktreeError_ValidGitdir(t *testing.T) {
	// Create a fake but existing gitdir target
	dir := t.TempDir()
	gitdirTarget := filepath.Join(t.TempDir(), "worktrees", "foo")
	os.MkdirAll(gitdirTarget, 0o755)

	os.WriteFile(
		filepath.Join(dir, ".git"),
		[]byte("gitdir: "+gitdirTarget+"\n"),
		0o644,
	)

	gitErr := fmt.Errorf("git status: exit status 128: fatal: some other error")

	result := diagnoseWorktreeError(dir, gitErr)
	if result != nil {
		t.Errorf("expected nil when gitdir target exists, got: %v", result)
	}
}
