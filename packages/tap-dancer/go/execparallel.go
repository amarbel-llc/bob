package tap

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// ExecResult holds the outcome of a single parallel command execution.
type ExecResult struct {
	Arg      string
	Command  string
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Err      error
}

// Executor runs a template command against a list of arguments in parallel
// and streams results in argument order.
type Executor interface {
	Run(ctx context.Context, template string, args []string) <-chan ExecResult
}

// GoroutineExecutor runs commands concurrently using goroutines.
// MaxJobs limits concurrency; 0 means unlimited.
type GoroutineExecutor struct {
	MaxJobs int
	running atomic.Int32
}

// Running returns the number of commands currently executing.
func (e *GoroutineExecutor) Running() int {
	return int(e.running.Load())
}

// expandTemplate replaces {} with arg. Arguments are interpolated as-is
// into the shell command, mirroring GNU parallel's ::: semantics.
func expandTemplate(template, arg string) string {
	return strings.ReplaceAll(template, "{}", arg)
}

func (e *GoroutineExecutor) Run(ctx context.Context, template string, args []string) <-chan ExecResult {
	ch := make(chan ExecResult, len(args))

	if len(args) == 0 {
		close(ch)
		return ch
	}

	results := make([]ExecResult, len(args))
	done := make([]chan struct{}, len(args))
	for i := range done {
		done[i] = make(chan struct{})
	}

	var sem chan struct{}
	if e.MaxJobs > 0 {
		sem = make(chan struct{}, e.MaxJobs)
	}

	for i, arg := range args {
		go func(idx int, a string) {
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			e.running.Add(1)
			expanded := expandTemplate(template, a)
			results[idx] = runCommand(ctx, a, expanded)
			e.running.Add(-1)
			close(done[idx])
		}(i, arg)
	}

	go func() {
		defer close(ch)
		for i := range args {
			<-done[i]
			ch <- results[i]
		}
	}()

	return ch
}

func runCommand(ctx context.Context, arg, expanded string) ExecResult {
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "sh", "-c", expanded)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
	}

	return ExecResult{
		Arg:      arg,
		Command:  expanded,
		ExitCode: exitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Err:      err,
	}
}

// ConvertExecParallel writes TAP-14 output from parallel execution results.
// Returns 0 if all commands succeeded, 1 if any failed.
func ConvertExecParallel(results <-chan ExecResult, w io.Writer, verbose bool, color bool) int {
	tw := NewColorWriter(w, color)
	exitCode := 0

	for r := range results {
		if r.ExitCode == 0 {
			if verbose {
				tw.OkDiag(r.Command, execResultDiagnostics(r))
			} else {
				tw.Ok(r.Command)
			}
		} else {
			exitCode = 1
			tw.NotOk(r.Command, execResultDiagnosticsMap(r))
		}
	}

	tw.Plan()
	return exitCode
}

// ConvertExecParallelWithStatus runs commands via the executor and writes TAP-14
// output with a tty-build-last-line status line.
//
// When maxJobs == 1 (sequential), the status line shows the last line of stdout
// from the currently running command. When maxJobs != 1 (parallel), the status
// line shows how many commands are currently running.
func ConvertExecParallelWithStatus(ctx context.Context, executor *GoroutineExecutor, template string, args []string, w io.Writer, verbose bool, color bool) int {
	if executor.MaxJobs == 1 {
		return execSequentialWithLastLine(ctx, template, args, w, verbose, color)
	}
	return execParallelWithRunningCount(ctx, executor, template, args, w, verbose, color)
}

func execSequentialWithLastLine(ctx context.Context, template string, args []string, w io.Writer, verbose bool, color bool) int {
	tw := NewColorWriter(w, color)
	tw.EnableTTYBuildLastLine()
	exitCode := 0

	for _, arg := range args {
		expanded := expandTemplate(template, arg)
		if !runWithStatusLine(ctx, tw, arg, expanded, verbose) {
			exitCode = 1
		}
	}

	tw.Plan()
	return exitCode
}

// ConvertExec runs commands sequentially with a tty-build-last-line status line
// showing the last stdout line from the currently running command.
// Each arg is run as: utility + " " + arg. If args is empty, utility is run once.
// Returns 0 if all commands succeeded, 1 if any failed.
func ConvertExec(ctx context.Context, utility string, args []string, w io.Writer, verbose bool, color bool) int {
	tw := NewColorWriter(w, color)
	if color {
		tw.EnableTTYBuildLastLine()
	}
	exitCode := 0

	if len(args) == 0 {
		if !runWithStatusLine(ctx, tw, utility, utility, verbose) {
			exitCode = 1
		}
	} else {
		for _, arg := range args {
			command := utility + " " + arg
			if !runWithStatusLine(ctx, tw, arg, command, verbose) {
				exitCode = 1
			}
		}
	}

	tw.Plan()
	return exitCode
}

// statusSpinner cycles through frames on each call to Frame(), rate-limited
// to maxFPS. When called faster than the limit, it returns the same frame.
//
// TODO: explore a timer-based spinner (goroutine advancing frames independently)
// for smoother animation when commands produce output in bursts.
type statusSpinner struct {
	frames  []string
	index   int
	lastAdv time.Time
	minDur  time.Duration
}

var monkeyFrames = []string{"🙈", "🙉", "🙊"}

func newStatusSpinner() *statusSpinner {
	return &statusSpinner{
		frames: monkeyFrames,
		minDur: time.Second / 3, // 3fps cap
	}
}

func (s *statusSpinner) Frame() string {
	now := time.Now()
	if now.Sub(s.lastAdv) >= s.minDur {
		s.index = (s.index + 1) % len(s.frames)
		s.lastAdv = now
	}
	return s.frames[s.index]
}

// runWithStatusLine runs a single command, streaming its stdout lines to the
// TAP writer's status line. Emits a test point when the command completes.
// Returns true if the command succeeded.
func runWithStatusLine(ctx context.Context, tw *Writer, arg, command string, verbose bool) bool {
	spinner := newStatusSpinner()
	r := runCommandStreamingLines(ctx, arg, command, func(line string) {
		tw.UpdateLastLine(spinner.Frame() + " " + line)
	})
	tw.FinishLastLine()

	if r.ExitCode == 0 {
		if verbose {
			tw.OkDiag(r.Command, execResultDiagnostics(r))
		} else {
			tw.Ok(r.Command)
		}
		return true
	}
	tw.NotOk(r.Command, execResultDiagnosticsMap(r))
	return false
}

func execParallelWithRunningCount(ctx context.Context, executor *GoroutineExecutor, template string, args []string, w io.Writer, verbose bool, color bool) int {
	tw := NewColorWriter(w, color)
	tw.EnableTTYBuildLastLine()
	total := len(args)
	exitCode := 0
	done := 0
	spinner := newStatusSpinner()

	tw.UpdateLastLine(spinner.Frame() + " " + parallelStatusLine(executor.Running(), done, total, color))

	results := executor.Run(ctx, template, args)
	for r := range results {
		tw.FinishLastLine()
		done++

		if r.ExitCode == 0 {
			if verbose {
				tw.OkDiag(r.Command, execResultDiagnostics(r))
			} else {
				tw.Ok(r.Command)
			}
		} else {
			exitCode = 1
			tw.NotOk(r.Command, execResultDiagnosticsMap(r))
		}

		tw.UpdateLastLine(spinner.Frame() + " " + parallelStatusLine(executor.Running(), done, total, color))
	}

	tw.FinishLastLine()
	tw.Plan()
	return exitCode
}

func parallelStatusLine(running, done, total int, color bool) string {
	if color {
		return fmt.Sprintf("%s%d running%s [%d/%d done]",
			ansiYellow, running, ansiReset,
			done, total)
	}
	return fmt.Sprintf("%d running [%d/%d done]", running, done, total)
}

func runCommandStreamingLines(ctx context.Context, arg, expanded string, onLine func(string)) ExecResult {
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "sh", "-c", expanded)
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ExecResult{Arg: arg, Command: expanded, ExitCode: 1, Err: err}
	}

	if err := cmd.Start(); err != nil {
		return ExecResult{Arg: arg, Command: expanded, ExitCode: 1, Err: err}
	}

	var stdoutBuf bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		stdoutBuf.WriteString(line)
		stdoutBuf.WriteByte('\n')
		onLine(line)
	}

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
	}

	return ExecResult{
		Arg:      arg,
		Command:  expanded,
		ExitCode: exitCode,
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderr.Bytes(),
		Err:      err,
	}
}

func execResultDiagnostics(r ExecResult) *Diagnostics {
	d := &Diagnostics{
		Extras: make(map[string]any),
	}

	d.Extras["exit-code"] = r.ExitCode

	stdout := strings.TrimRight(string(r.Stdout), "\n")
	if stdout != "" {
		d.Extras["stdout"] = stdout
	}

	stderr := strings.TrimRight(string(r.Stderr), "\n")
	if stderr != "" {
		d.Extras["stderr"] = stderr
	}

	if r.Err != nil && stdout == "" && stderr == "" {
		d.Extras["error"] = r.Err.Error()
	}

	return d
}

func execResultDiagnosticsMap(r ExecResult) map[string]string {
	d := execResultDiagnostics(r)
	m := make(map[string]string, len(d.Extras))
	for k, v := range d.Extras {
		m[k] = fmt.Sprintf("%v", v)
	}
	return m
}
