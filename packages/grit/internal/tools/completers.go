package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

func gitLines(args ...string) map[string]string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading "* " (current branch) or "+ " (worktree branch)
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "+ ")
		// Strip "remotes/" prefix for display
		line = strings.TrimPrefix(line, "remotes/")
		// Skip symbolic refs like "origin/HEAD -> origin/master"
		if strings.Contains(line, " -> ") {
			continue
		}
		result[line] = ""
	}

	return result
}

func branchCompleter(includeRemote bool) func() map[string]string {
	return func() map[string]string {
		args := []string{"branch", "--no-color"}
		if includeRemote {
			args = append(args, "-a")
		}
		return gitLines(args...)
	}
}

func refCompleter() func() map[string]string {
	return func() map[string]string {
		result := branchCompleter(true)()
		if result == nil {
			result = make(map[string]string)
		}
		for k, v := range tagCompleter()() {
			result[k] = v
		}
		return result
	}
}

func tagCompleter() func() map[string]string {
	return func() map[string]string {
		return gitLines("tag", "--no-color")
	}
}

func stashCompleter() func() map[string]string {
	return func() map[string]string {
		cmd := exec.Command("git", "stash", "list", "--format=%gd\t%gs")
		out, err := cmd.Output()
		if err != nil {
			return nil
		}

		result := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			ref, desc, _ := strings.Cut(line, "\t")
			result[ref] = desc
		}
		return result
	}
}

func commitCompleter(n int) func() map[string]string {
	return func() map[string]string {
		cmd := exec.Command("git", "log", "--oneline", fmt.Sprintf("-%d", n))
		out, err := cmd.Output()
		if err != nil {
			return nil
		}

		result := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			hash, msg, _ := strings.Cut(line, " ")
			result[hash] = msg
		}
		return result
	}
}
