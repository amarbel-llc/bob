package sweatfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func (sf *Sweatfile) Parse(data []byte) error {
	return toml.Unmarshal(data, sf)
}

func (sf *Sweatfile) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return sf.Parse(data)
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

func (sf Sweatfile) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(sf)
}
