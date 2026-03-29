package sweatfile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type Claude struct {
	SystemPrompt       *string  `toml:"system-prompt"`
	SystemPromptAppend *string  `toml:"system-prompt-append"`
	Allow              []string `toml:"allow"`
}

type Git struct {
	Excludes []string `toml:"excludes"`
}

type Direnv struct {
	Envrc  []string          `toml:"envrc"`
	Dotenv map[string]string `toml:"dotenv"`
}

type SessionEntry struct {
	Start  []string `toml:"start"`
	Resume []string `toml:"resume"`
}

type Hooks struct {
	Create               *string `toml:"create"`
	Stop                 *string `toml:"stop"`
	PreMerge             *string `toml:"pre-merge"`
	DisallowMainWorktree *bool   `toml:"disallow-main-worktree"`
	ToolUseLog           *bool   `toml:"tool-use-log"`
}

//go:generate tommy generate
type Sweatfile struct {
	Claude       *Claude       `toml:"claude"`
	Git          *Git          `toml:"git"`
	Direnv       *Direnv       `toml:"direnv"`
	Hooks        *Hooks        `toml:"hooks"`
	SessionEntry *SessionEntry `toml:"session-entry"`
}

func (sf Sweatfile) StopHookCommand() *string {
	if sf.Hooks == nil {
		return nil
	}
	return sf.Hooks.Stop
}

func (sf Sweatfile) CreateHookCommand() *string {
	if sf.Hooks == nil {
		return nil
	}
	return sf.Hooks.Create
}

func (sf Sweatfile) PreMergeHookCommand() *string {
	if sf.Hooks == nil {
		return nil
	}
	return sf.Hooks.PreMerge
}

func (sf Sweatfile) DisallowMainWorktreeEnabled() bool {
	return sf.Hooks != nil &&
		sf.Hooks.DisallowMainWorktree != nil &&
		*sf.Hooks.DisallowMainWorktree
}

func (sf Sweatfile) ToolUseLogEnabled() bool {
	return sf.Hooks != nil &&
		sf.Hooks.ToolUseLog != nil &&
		*sf.Hooks.ToolUseLog
}

func (sf Sweatfile) SessionStart() []string {
	if sf.SessionEntry != nil && len(sf.SessionEntry.Start) > 0 {
		return sf.SessionEntry.Start
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell}
}

func (sf Sweatfile) SessionResume() []string {
	if sf.SessionEntry != nil && len(sf.SessionEntry.Resume) > 0 {
		return sf.SessionEntry.Resume
	}
	return nil
}

// baseline excludes and allow rules that are always applied regardless of user
// sweatfile config.
func GetDefault() Sweatfile {
	sf := Sweatfile{
		Git: &Git{Excludes: []string{".spinclass/"}},
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		claudeDir := filepath.Join(home, ".claude")
		sf.Claude = &Claude{Allow: []string{fmt.Sprintf("Read(%s/*)", claudeDir)}}
	}

	return sf
}

func (sweatfile Sweatfile) ExecClaude(
	args ...string,
) error {
	if sweatfile.Claude != nil {
		if sweatfile.Claude.SystemPromptAppend != nil {
			args = append(
				[]string{
					"--append-system-prompt",
					resolvePathOrString(*sweatfile.Claude.SystemPromptAppend),
				},
				args...,
			)
		}

		if sweatfile.Claude.SystemPrompt != nil {
			args = append(
				[]string{
					"--system-prompt",
					resolvePathOrString(*sweatfile.Claude.SystemPrompt),
				},
				args...,
			)
		}
	}

	pathGitDirCommon, err := getGitDirCommon()
	if err != nil {
		return err
	}

	pathSweatfileBin := filepath.Join(pathGitDirCommon, "spinclass/bin/")

	envVarPath := filepath.SplitList(os.Getenv("PATH"))
	envVarPath = slices.DeleteFunc(envVarPath, func(value string) bool {
		return filepath.Clean(value) == pathSweatfileBin
	})
	os.Setenv("PATH", strings.Join(envVarPath, string(filepath.ListSeparator)))

	cmdClaude := exec.Command("claude", args...)
	cmdClaude.Stdout = os.Stdout
	cmdClaude.Stderr = os.Stderr
	cmdClaude.Stdin = os.Stdin

	if err := cmdClaude.Run(); err != nil {
		return err
	}

	return nil
}
