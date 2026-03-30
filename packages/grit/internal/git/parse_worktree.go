package git

import (
	"strings"
)

func ParseWorktreeList(output string) []WorktreeEntry {
	var entries []WorktreeEntry

	// Porcelain output separates worktrees with blank lines.
	// Each block has key-value lines: worktree, HEAD, branch, bare, locked, prunable.
	blocks := splitWorktreeBlocks(output)

	for i, block := range blocks {
		entry := parseWorktreeBlock(block)
		if entry.Path == "" {
			continue
		}
		entry.IsMain = (i == 0)
		entries = append(entries, entry)
	}

	if entries == nil {
		entries = []WorktreeEntry{}
	}

	return entries
}

func splitWorktreeBlocks(output string) [][]string {
	var blocks [][]string
	var current []string

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}

	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	return blocks
}

func parseWorktreeBlock(lines []string) WorktreeEntry {
	var entry WorktreeEntry

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "worktree "):
			entry.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			entry.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			entry.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			entry.IsBare = true
		case line == "detached":
			// HEAD is detached; branch remains empty
		case line == "locked":
			entry.Locked = true
		case strings.HasPrefix(line, "locked "):
			entry.Locked = true
			entry.LockReason = strings.TrimPrefix(line, "locked ")
		case line == "prunable":
			entry.Prunable = true
		case strings.HasPrefix(line, "prunable "):
			entry.Prunable = true
		}
	}

	return entry
}
