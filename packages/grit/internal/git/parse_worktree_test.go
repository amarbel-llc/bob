package git

import (
	"testing"
)

func TestParseWorktreeListMainAndLinked(t *testing.T) {
	input := "worktree /home/user/repo\n" +
		"HEAD abc1234def5678\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /home/user/repo/.worktrees/feature\n" +
		"HEAD 1234abcdef5678\n" +
		"branch refs/heads/feature-branch\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	if entries[0].Path != "/home/user/repo" {
		t.Errorf("entry 0 path = %q, want %q", entries[0].Path, "/home/user/repo")
	}

	if entries[0].Head != "abc1234def5678" {
		t.Errorf("entry 0 head = %q, want %q", entries[0].Head, "abc1234def5678")
	}

	if entries[0].Branch != "master" {
		t.Errorf("entry 0 branch = %q, want %q", entries[0].Branch, "master")
	}

	if !entries[0].IsMain {
		t.Error("entry 0 expected is_main = true")
	}

	if entries[1].Path != "/home/user/repo/.worktrees/feature" {
		t.Errorf("entry 1 path = %q, want %q", entries[1].Path, "/home/user/repo/.worktrees/feature")
	}

	if entries[1].Branch != "feature-branch" {
		t.Errorf("entry 1 branch = %q, want %q", entries[1].Branch, "feature-branch")
	}

	if entries[1].IsMain {
		t.Error("entry 1 expected is_main = false")
	}
}

func TestParseWorktreeListDetachedHead(t *testing.T) {
	input := "worktree /home/user/repo\n" +
		"HEAD abc1234\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /home/user/repo/.worktrees/detached\n" +
		"HEAD def5678\n" +
		"detached\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	if entries[1].Branch != "" {
		t.Errorf("entry 1 branch = %q, want empty (detached)", entries[1].Branch)
	}
}

func TestParseWorktreeListLocked(t *testing.T) {
	input := "worktree /home/user/repo\n" +
		"HEAD abc1234\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /home/user/repo/.worktrees/locked-wt\n" +
		"HEAD def5678\n" +
		"branch refs/heads/locked-branch\n" +
		"locked on external drive\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	if !entries[1].Locked {
		t.Error("entry 1 expected locked = true")
	}

	if entries[1].LockReason != "on external drive" {
		t.Errorf("entry 1 lock_reason = %q, want %q", entries[1].LockReason, "on external drive")
	}
}

func TestParseWorktreeListLockedNoReason(t *testing.T) {
	input := "worktree /home/user/repo\n" +
		"HEAD abc1234\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /home/user/repo/.worktrees/locked-wt\n" +
		"HEAD def5678\n" +
		"branch refs/heads/locked-branch\n" +
		"locked\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if !entries[1].Locked {
		t.Error("entry 1 expected locked = true")
	}

	if entries[1].LockReason != "" {
		t.Errorf("entry 1 lock_reason = %q, want empty", entries[1].LockReason)
	}
}

func TestParseWorktreeListPrunable(t *testing.T) {
	input := "worktree /home/user/repo\n" +
		"HEAD abc1234\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /tmp/stale-worktree\n" +
		"HEAD def5678\n" +
		"branch refs/heads/stale\n" +
		"prunable gitdir file points to non-existent location\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	if !entries[1].Prunable {
		t.Error("entry 1 expected prunable = true")
	}
}

func TestParseWorktreeListBare(t *testing.T) {
	input := "worktree /home/user/repo.git\n" +
		"HEAD abc1234\n" +
		"bare\n" +
		"\n"

	entries := ParseWorktreeList(input)

	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}

	if !entries[0].IsBare {
		t.Error("entry 0 expected is_bare = true")
	}

	if entries[0].Branch != "" {
		t.Errorf("entry 0 branch = %q, want empty (bare)", entries[0].Branch)
	}
}

func TestParseWorktreeListEmpty(t *testing.T) {
	entries := ParseWorktreeList("")

	if len(entries) != 0 {
		t.Errorf("entries count = %d, want 0", len(entries))
	}
}
