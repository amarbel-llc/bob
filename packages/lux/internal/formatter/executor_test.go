package formatter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amarbel-llc/lux/internal/config"
)

func TestSubstituteArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		filePath string
		want     []string
	}{
		{
			name:     "no placeholders",
			args:     []string{"--write"},
			filePath: "/tmp/test.go",
			want:     []string{"--write"},
		},
		{
			name:     "single placeholder",
			args:     []string{"--stdin-filepath", "{file}"},
			filePath: "/tmp/test.go",
			want:     []string{"--stdin-filepath", "/tmp/test.go"},
		},
		{
			name:     "multiple placeholders",
			args:     []string{"{file}", "--output", "{file}.bak"},
			filePath: "/tmp/test.go",
			want:     []string{"/tmp/test.go", "--output", "/tmp/test.go.bak"},
		},
		{
			name:     "empty args",
			args:     []string{},
			filePath: "/tmp/test.go",
			want:     []string{},
		},
		{
			name:     "nil args",
			args:     nil,
			filePath: "/tmp/test.go",
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstituteArgs(tt.args, tt.filePath)
			if len(got) != len(tt.want) {
				t.Fatalf("SubstituteArgs() returned %d args, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SubstituteArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstituteFilepathArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		filePath string
		want     []string
	}{
		{
			name:     "with placeholder",
			args:     []string{"--input", "{file}"},
			filePath: "/tmp/test.go",
			want:     []string{"--input", "/tmp/test.go"},
		},
		{
			name:     "without placeholder appends path",
			args:     []string{"--write"},
			filePath: "/tmp/test.go",
			want:     []string{"--write", "/tmp/test.go"},
		},
		{
			name:     "empty args appends path",
			args:     []string{},
			filePath: "/tmp/test.go",
			want:     []string{"/tmp/test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substituteFilepathArgs(tt.args, tt.filePath)
			if len(got) != len(tt.want) {
				t.Fatalf("substituteFilepathArgs() returned %d args, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("substituteFilepathArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFormatStdinPassesContentViaStdin(t *testing.T) {
	dir := t.TempDir()

	// Script that transforms stdin: removes lines containing "REMOVE_ME"
	// and substitutes {file} in args, writing received args to a sidecar file.
	argsFile := filepath.Join(dir, "received-args")
	script := filepath.Join(dir, "filter")
	os.WriteFile(script, []byte("#!/bin/sh\necho \"$*\" > "+argsFile+"\ngrep -v REMOVE_ME"), 0755)

	inputFile := filepath.Join(dir, "input.go")
	os.WriteFile(inputFile, []byte("line1\nREMOVE_ME\nline3\n"), 0644)

	f := &config.Formatter{
		Name: "filter",
		Path: script,
		Mode: "stdin",
		Args: []string{"-srcdir", "{file}"},
	}

	result, err := Format(context.Background(), f, inputFile, []byte("line1\nREMOVE_ME\nline3\n"), nil)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	// Verify the formatter received content via stdin and transformed it.
	if strings.Contains(result.Formatted, "REMOVE_ME") {
		t.Errorf("stdin content was not processed by formatter: %q", result.Formatted)
	}
	if !strings.Contains(result.Formatted, "line1") || !strings.Contains(result.Formatted, "line3") {
		t.Errorf("expected line1 and line3 in output, got: %q", result.Formatted)
	}

	// Verify {file} was substituted in args.
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	if !strings.Contains(string(argsData), inputFile) {
		t.Errorf("args did not contain substituted file path: %q", string(argsData))
	}
}

func TestFormatStdinReceivesFilePathSubstitution(t *testing.T) {
	dir := t.TempDir()

	// Script that echoes its args to stderr and passes stdin through.
	// This lets us verify what arguments the formatter actually received.
	script := filepath.Join(dir, "echo-args")
	os.WriteFile(script, []byte("#!/bin/sh\necho \"ARGS:$*\" >&2\ncat"), 0755)

	filePath := "/home/user/project/main.go"

	f := &config.Formatter{
		Name: "echo-args",
		Path: script,
		Mode: "stdin",
		Args: []string{"-srcdir", "{file}"},
	}

	result, err := Format(context.Background(), f, filePath, []byte("package main\n"), nil)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	// The formatter should have received the substituted file path, not "{file}".
	if strings.Contains(result.Stderr, "{file}") {
		t.Errorf("formatter received literal {file} placeholder instead of substituted path\nstderr: %s", result.Stderr)
	}
	if !strings.Contains(result.Stderr, filePath) {
		t.Errorf("formatter did not receive substituted file path %q\nstderr: %s", filePath, result.Stderr)
	}
}

func TestFormatChain(t *testing.T) {
	dir := t.TempDir()

	script1 := filepath.Join(dir, "fmt1")
	os.WriteFile(script1, []byte("#!/bin/sh\necho PREFIX1\ncat"), 0755)

	script2 := filepath.Join(dir, "fmt2")
	os.WriteFile(script2, []byte("#!/bin/sh\necho PREFIX2\ncat"), 0755)

	f1 := &config.Formatter{Name: "fmt1", Path: script1, Mode: "stdin"}
	f2 := &config.Formatter{Name: "fmt2", Path: script2, Mode: "stdin"}

	result, err := FormatChain(context.Background(), []*config.Formatter{f1, f2}, "/tmp/test.txt", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("FormatChain: %v", err)
	}

	if !strings.Contains(result.Formatted, "PREFIX1") {
		t.Error("expected PREFIX1 in output")
	}
	if !strings.Contains(result.Formatted, "PREFIX2") {
		t.Error("expected PREFIX2 in output")
	}
	if !result.Changed {
		t.Error("expected Changed = true")
	}
}

func TestFormatFallback_FirstSucceeds(t *testing.T) {
	dir := t.TempDir()

	script := filepath.Join(dir, "fmt1")
	os.WriteFile(script, []byte("#!/bin/sh\necho formatted"), 0755)

	f1 := &config.Formatter{Name: "fmt1", Path: script, Mode: "stdin"}
	f2 := &config.Formatter{Name: "fmt2", Path: "/nonexistent/binary", Mode: "stdin"}

	result, err := FormatFallback(context.Background(), []*config.Formatter{f1, f2}, "/tmp/test.txt", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("FormatFallback: %v", err)
	}
	if result.Formatted != "formatted\n" {
		t.Errorf("formatted = %q, want %q", result.Formatted, "formatted\n")
	}
}

func TestFormatFallback_FirstFailsSecondSucceeds(t *testing.T) {
	dir := t.TempDir()

	script := filepath.Join(dir, "fmt2")
	os.WriteFile(script, []byte("#!/bin/sh\necho formatted"), 0755)

	f1 := &config.Formatter{Name: "fmt1", Path: "/nonexistent/binary", Mode: "stdin"}
	f2 := &config.Formatter{Name: "fmt2", Path: script, Mode: "stdin"}

	result, err := FormatFallback(context.Background(), []*config.Formatter{f1, f2}, "/tmp/test.txt", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("FormatFallback: %v", err)
	}
	if result.Formatted != "formatted\n" {
		t.Errorf("formatted = %q, want %q", result.Formatted, "formatted\n")
	}
}

func TestFormatFallback_AllFail(t *testing.T) {
	f1 := &config.Formatter{Name: "fmt1", Path: "/nonexistent/binary1", Mode: "stdin"}
	f2 := &config.Formatter{Name: "fmt2", Path: "/nonexistent/binary2", Mode: "stdin"}

	_, err := FormatFallback(context.Background(), []*config.Formatter{f1, f2}, "/tmp/test.txt", []byte("hello"), nil)
	if err == nil {
		t.Fatal("expected error when all formatters fail")
	}
}
