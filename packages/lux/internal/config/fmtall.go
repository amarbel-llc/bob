package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type FmtAllConfig struct {
	Walk         string   `toml:"walk"`
	ExcludeGlobs []string `toml:"exclude_globs"`
}

func fmtAllDefaults() *FmtAllConfig {
	return &FmtAllConfig{
		Walk: "git",
	}
}

func LoadFmtAll() (*FmtAllConfig, error) {
	return loadFmtAllFile(filepath.Join(configDir(), "fmt-all.toml"))
}

func LoadLocalFmtAll(projectRoot string) (*FmtAllConfig, error) {
	return loadFmtAllFile(filepath.Join(projectRoot, ".lux", "fmt-all.toml"))
}

func LoadMergedFmtAll(projectRoot string) (*FmtAllConfig, error) {
	global, err := LoadFmtAll()
	if err != nil {
		return nil, fmt.Errorf("loading global fmt-all config: %w", err)
	}

	if projectRoot == "" {
		return global, nil
	}

	local, err := LoadLocalFmtAll(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("loading local fmt-all config: %w", err)
	}

	return mergeFmtAll(global, local), nil
}

func loadFmtAllFile(path string) (*FmtAllConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmtAllDefaults(), nil
		}
		return nil, fmt.Errorf("reading fmt-all config %s: %w", path, err)
	}

	cfg := fmtAllDefaults()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing fmt-all config %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid fmt-all config %s: %w", path, err)
	}

	return cfg, nil
}

func (c *FmtAllConfig) validate() error {
	switch c.Walk {
	case "git", "all":
		return nil
	default:
		return fmt.Errorf("walk must be %q or %q, got %q", "git", "all", c.Walk)
	}
}

func mergeFmtAll(global, local *FmtAllConfig) *FmtAllConfig {
	merged := *global
	if local.Walk != fmtAllDefaults().Walk {
		merged.Walk = local.Walk
	}
	if local.ExcludeGlobs != nil {
		merged.ExcludeGlobs = local.ExcludeGlobs
	}
	return &merged
}
