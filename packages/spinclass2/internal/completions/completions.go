package completions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/amarbel-llc/spinclass2/internal/session"
	"github.com/amarbel-llc/spinclass2/internal/worktree"
)

// Sessions outputs completion entries from the session state directory.
// Each line is tab-separated: <session-key>\t<state>\n
// When repoPath is non-empty, only sessions belonging to that repo are listed.
func Sessions(w io.Writer, repoPath string) {
	states, err := session.ListAll()
	if err != nil {
		return
	}
	for _, s := range states {
		resolved := s.ResolveState()
		if resolved == session.StateAbandoned {
			continue
		}
		if repoPath != "" && s.RepoPath != repoPath {
			continue
		}
		// Extract branch name for completion value
		fmt.Fprintf(w, "%s\t%s session (%s)\n", s.Branch, resolved, filepath.Base(s.RepoPath))
	}
}

// Local outputs completion entries by scanning worktree directories.
// Falls back to directory scanning when no session state is available.
func Local(startDir string, w io.Writer) {
	// If startDir is a repo, list its worktrees
	gitDir := filepath.Join(startDir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		repoName := filepath.Base(startDir)
		fmt.Fprintf(w, "%s/\tnew worktree\n", repoName)

		for _, wtPath := range worktree.ListWorktrees(startDir) {
			branch := filepath.Base(wtPath)
			fmt.Fprintf(w, "%s\texisting worktree\n", branch)
		}
		return
	}

	// Otherwise scan children for repos
	entries, err := os.ReadDir(startDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(startDir, entry.Name())
		childGitDir := filepath.Join(child, ".git")
		if info, err := os.Stat(childGitDir); err != nil || !info.IsDir() {
			continue
		}

		repoName := entry.Name()
		fmt.Fprintf(w, "%s/\tnew worktree\n", repoName)

		for _, wtPath := range worktree.ListWorktrees(child) {
			branch := filepath.Base(wtPath)
			fmt.Fprintf(w, "%s\texisting worktree\n", branch)
		}
	}
}
