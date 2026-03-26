package sweatfile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type Hooks struct {
	Create               *string `toml:"create"`
	Stop                 *string `toml:"stop"`
	PreMerge             *string `toml:"pre-merge"`
	DisallowMainWorktree *bool   `toml:"disallow-main-worktree"`
	ToolUseLog           *bool   `toml:"tool-use-log"`
}

//go:generate tommy generate
type Sweatfile struct {
	SystemPrompt       *string           `toml:"system-prompt"`
	SystemPromptAppend *string           `toml:"system-prompt-append"`
	GitSkipIndex       []string          `toml:"git-excludes"`
	ClaudeAllow        []string          `toml:"claude-allow"`
	EnvrcDirectives    []string          `toml:"envrc-directives"`
	Env                map[string]string `toml:"env"`
	Hooks              *Hooks            `toml:"hooks"`
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

// baseline excludes and allow rules that are always applied regardless of user
// sweatfile config.
func GetDefault() Sweatfile {
	sweatfile := Sweatfile{
		GitSkipIndex: []string{},
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		claudeDir := filepath.Join(home, ".claude")
		sweatfile.ClaudeAllow = []string{fmt.Sprintf("Read(%s/*)", claudeDir)}
	}

	return sweatfile
}

func (sweatfile Sweatfile) ExecClaude(
	args ...string,
) error {
	if sweatfile.SystemPromptAppend != nil {
		args = append(
			[]string{
				"--append-system-prompt",
				resolvePathOrString(*sweatfile.SystemPromptAppend),
			},
			args...,
		)
	}

	if sweatfile.SystemPrompt != nil {
		args = append(
			[]string{
				"--system-prompt",
				resolvePathOrString(*sweatfile.SystemPrompt),
			},
			args...,
		)
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
