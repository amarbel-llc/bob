package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func diagnoseWorktreeError(dir string, gitErr error) error {
	dotGitPath := filepath.Join(dir, ".git")

	info, err := os.Lstat(dotGitPath)
	if err != nil || info.IsDir() {
		return nil
	}

	data, err := os.ReadFile(dotGitPath)
	if err != nil {
		return nil
	}

	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return nil
	}

	gitdirTarget := strings.TrimPrefix(line, "gitdir: ")

	if _, err := os.Stat(gitdirTarget); err == nil {
		return nil
	}

	return fmt.Errorf(
		"worktree gitdir path does not exist: %s\n\n"+
			"This usually means the repository was moved or an ancestor symlink changed.\n"+
			"Fix by updating the path in:\n"+
			"  - %s (gitdir: line)\n"+
			"  - <main-repo>/.git/worktrees/<name>/gitdir\n\n"+
			"Or run: git worktree repair",
		gitdirTarget,
		dotGitPath,
	)
}
