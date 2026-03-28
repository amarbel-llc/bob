package executor

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/amarbel-llc/spinclass2/internal/session"
	tap "github.com/amarbel-llc/bob/packages/tap-dancer/go"
)

type SessionExecutor struct {
	Entrypoint []string
}

func (s SessionExecutor) Attach(dir string, key string, command []string, dryRun bool, tp *tap.TestPoint) error {
	entrypoint := s.Entrypoint
	if len(command) > 0 {
		entrypoint = command
	}
	if len(entrypoint) == 0 {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		entrypoint = []string{shell}
	}

	if dryRun {
		tp.Skip = "dry run"
		tp.Diagnostics = &tap.Diagnostics{
			Extras: map[string]any{
				"command": strings.Join(entrypoint, " "),
			},
		}
		return nil
	}

	tmpDir := filepath.Join(dir, ".tmp")

	cmd := exec.Command(entrypoint[0], entrypoint[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"SPINCLASS_SESSION="+key,
		"TMPDIR="+tmpDir,
		"CLAUDE_CODE_TMPDIR="+tmpDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		<-sighup
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGHUP)
			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()
			<-timer.C
			if cmd.Process != nil {
				cmd.Process.Signal(syscall.SIGTERM)
			}
		}
	}()

	err := cmd.Wait()
	signal.Stop(sighup)
	return err
}

func (s SessionExecutor) Detach() error {
	return nil
}

// RequestClose sends SIGHUP to the PID in the session state file.
func RequestClose(repoPath, branch string) error {
	st, err := session.Read(repoPath, branch)
	if err != nil {
		return nil
	}
	if !session.IsAlive(st.PID) {
		return nil
	}
	return syscall.Kill(st.PID, syscall.SIGHUP)
}
