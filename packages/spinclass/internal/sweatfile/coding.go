package sweatfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func Parse(data []byte) (*SweatfileDocument, error) {
	doc, err := DecodeSweatfile(data)
	if err != nil {
		return nil, err
	}
	// Tommy's GetFromContainer returns nil for empty TOML arrays (e.g. []).
	// MergeWith relies on nil vs empty to distinguish "absent" from "clear",
	// so normalize consumed array keys to non-nil empty slices.
	if doc.consumed["git-excludes"] && doc.data.GitSkipIndex == nil {
		doc.data.GitSkipIndex = []string{}
	}
	if doc.consumed["claude-allow"] && doc.data.ClaudeAllow == nil {
		doc.data.ClaudeAllow = []string{}
	}
	if doc.consumed["envrc-directives"] && doc.data.EnvrcDirectives == nil {
		doc.data.EnvrcDirectives = []string{}
	}
	return doc, nil
}

func Load(path string) (*SweatfileDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DecodeSweatfile(nil)
		}
		return nil, err
	}
	return Parse(data)
}

// resolvePathOrString expands environment variables and ~ in value, then
// tries to read it as a file path. If the file exists, its contents are
// returned (trimmed). Otherwise value is returned as a literal string.
func resolvePathOrString(value string) string {
	expanded := os.ExpandEnv(value)
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}

	data, err := os.ReadFile(expanded)
	if err != nil {
		return value
	}
	return strings.TrimSpace(string(data))
}

func (doc *SweatfileDocument) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	output, err := doc.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(path, output, 0o644)
}
