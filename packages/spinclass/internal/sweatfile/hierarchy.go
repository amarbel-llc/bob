package sweatfile

import (
	"os"
	"path/filepath"
	"strings"
)

type LoadSource struct {
	Path  string
	Found bool
	File  Sweatfile
}

type Hierarchy struct {
	Sources []LoadSource
	Merged  Sweatfile
}

func LoadDefaultHierarchy() (Hierarchy, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Hierarchy{}, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Hierarchy{}, err
	}

	hierarchy, err := LoadHierarchy(home, cwd)
	if err != nil {
		return hierarchy, err
	}

	return hierarchy, nil
}

func LoadHierarchy(home, repoDir string) (Hierarchy, error) {
	var sources []LoadSource
	merged := Sweatfile{}

	loadAndMerge := func(path string) error {
		doc, err := Load(path)
		if err != nil {
			return err
		}
		sf := *doc.Data()
		_, found := fileExists(path)
		sources = append(
			sources,
			LoadSource{Path: path, Found: found, File: sf},
		)
		if found {
			merged = merged.MergeWith(sf)
		}
		return nil
	}

	// 1. Global config
	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	if err := loadAndMerge(globalPath); err != nil {
		return Hierarchy{}, err
	}

	// 2. Parent directories walking DOWN from home to repo dir
	cleanHome := filepath.Clean(home)
	cleanRepo := filepath.Clean(repoDir)

	rel, err := filepath.Rel(cleanHome, cleanRepo)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		parts := strings.Split(rel, string(filepath.Separator))
		// Walk each intermediate directory (excluding repo dir itself)
		for i := 1; i < len(parts); i++ {
			parentDir := filepath.Join(cleanHome, filepath.Join(parts[:i]...))
			parentPath := filepath.Join(parentDir, "sweatfile")
			if err := loadAndMerge(parentPath); err != nil {
				return Hierarchy{}, err
			}
		}
	}

	// 3. Repo sweatfile
	repoPath := filepath.Join(cleanRepo, "sweatfile")
	if err := loadAndMerge(repoPath); err != nil {
		return Hierarchy{}, err
	}

	return Hierarchy{
		Sources: sources,
		Merged:  merged,
	}, nil
}

// LoadWorktreeHierarchy loads the sweatfile cascade for a worktree context.
// It delegates to LoadHierarchy for global → intermediate dirs → main repo,
// then appends the worktree's own sweatfile as the highest-priority layer.
func LoadWorktreeHierarchy(
	home, mainRepoRoot, worktreeDir string,
) (Hierarchy, error) {
	hierarchy, err := LoadHierarchy(home, mainRepoRoot)
	if err != nil {
		return Hierarchy{}, err
	}

	worktreePath := filepath.Join(filepath.Clean(worktreeDir), "sweatfile")
	doc, err := Load(worktreePath)
	if err != nil {
		return Hierarchy{}, err
	}
	sf := *doc.Data()

	_, found := fileExists(worktreePath)
	hierarchy.Sources = append(hierarchy.Sources, LoadSource{
		Path: worktreePath, Found: found, File: sf,
	})
	if found {
		hierarchy.Merged = hierarchy.Merged.MergeWith(sf)
	}

	return hierarchy, nil
}

func (sf Sweatfile) MergeWith(other Sweatfile) Sweatfile {
	merged := sf

	if other.SystemPrompt != nil {
		if *other.SystemPrompt == "" {
			merged.SystemPrompt = other.SystemPrompt
		} else if sf.SystemPrompt != nil && *sf.SystemPrompt != "" {
			joined := *sf.SystemPrompt + " " + *other.SystemPrompt
			merged.SystemPrompt = &joined
		} else {
			merged.SystemPrompt = other.SystemPrompt
		}
	}

	if other.SystemPromptAppend != nil {
		if *other.SystemPromptAppend == "" {
			merged.SystemPromptAppend = other.SystemPromptAppend
		} else if sf.SystemPromptAppend != nil && *sf.SystemPromptAppend != "" {
			joined := *sf.SystemPromptAppend + " " + *other.SystemPromptAppend
			merged.SystemPromptAppend = &joined
		} else {
			merged.SystemPromptAppend = other.SystemPromptAppend
		}
	}

	// Arrays: nil = inherit, empty = clear, non-empty = append
	if other.GitSkipIndex != nil {
		if len(other.GitSkipIndex) == 0 {
			merged.GitSkipIndex = []string{}
		} else {
			merged.GitSkipIndex = append(sf.GitSkipIndex, other.GitSkipIndex...)
		}
	}
	if other.ClaudeAllow != nil {
		if len(other.ClaudeAllow) == 0 {
			merged.ClaudeAllow = []string{}
		} else {
			merged.ClaudeAllow = append(sf.ClaudeAllow, other.ClaudeAllow...)
		}
	}
	if other.EnvrcDirectives != nil {
		if len(other.EnvrcDirectives) == 0 {
			merged.EnvrcDirectives = []string{}
		} else {
			merged.EnvrcDirectives = append(sf.EnvrcDirectives, other.EnvrcDirectives...)
		}
	}

	if other.Env != nil {
		if merged.Env == nil {
			merged.Env = make(map[string]string)
		}
		for k, v := range other.Env {
			merged.Env[k] = v
		}
	}

	if other.Hooks != nil {
		if merged.Hooks == nil {
			merged.Hooks = &Hooks{}
		}
		if other.Hooks.Create != nil {
			merged.Hooks.Create = other.Hooks.Create
		}
		if other.Hooks.Stop != nil {
			merged.Hooks.Stop = other.Hooks.Stop
		}
		if other.Hooks.PreMerge != nil {
			merged.Hooks.PreMerge = other.Hooks.PreMerge
		}
		if other.Hooks.DisallowMainWorktree != nil {
			merged.Hooks.DisallowMainWorktree = other.Hooks.DisallowMainWorktree
		}
	}

	return merged
}
