package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
	"github.com/amarbel-llc/lux/internal/formatter"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/lux/internal/subprocess"
)

func runFmtAll(ctx context.Context, paths []string) error {
	fmtAllCfg, err := loadFmtAllConfig()
	if err != nil {
		return err
	}

	filetypes, err := filetype.LoadMerged()
	if err != nil {
		return fmt.Errorf("loading filetype configs: %w", err)
	}

	fmtCfg, err := config.LoadMergedFormatters()
	if err != nil {
		return fmt.Errorf("loading formatter config: %w", err)
	}

	if err := fmtCfg.Validate(); err != nil {
		return fmt.Errorf("invalid formatter config: %w", err)
	}

	fmtMap := make(map[string]*config.Formatter)
	for i := range fmtCfg.Formatters {
		f := &fmtCfg.Formatters[i]
		if !f.Disabled {
			fmtMap[f.Name] = f
		}
	}

	router, err := formatter.NewRouter(filetypes, fmtMap)
	if err != nil {
		return fmt.Errorf("creating formatter router: %w", err)
	}

	executor := subprocess.NewNixExecutor()

	var files []string
	if len(paths) == 0 {
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		files, err = walkFiles(root, fmtAllCfg.Walk)
		if err != nil {
			return err
		}
		files = applyExcludeGlobs(files, fmtAllCfg.ExcludeGlobs, root)
	} else {
		for _, p := range paths {
			abs, err := filepath.Abs(p)
			if err != nil {
				fmt.Fprintf(logfile.Writer(), "resolving path %s: %v\n", p, err)
				continue
			}
			info, err := os.Stat(abs)
			if err != nil {
				fmt.Fprintf(logfile.Writer(), "stat %s: %v\n", abs, err)
				continue
			}
			if info.IsDir() {
				dirFiles, err := walkFiles(abs, fmtAllCfg.Walk)
				if err != nil {
					fmt.Fprintf(logfile.Writer(), "walking %s: %v\n", abs, err)
					continue
				}
				dirFiles = applyExcludeGlobs(dirFiles, fmtAllCfg.ExcludeGlobs, abs)
				files = append(files, dirFiles...)
			} else {
				files = append(files, abs)
			}
		}
	}

	for _, f := range files {
		if err := formatFile(ctx, f, router, executor); err != nil {
			fmt.Fprintf(logfile.Writer(), "fmt %s: %v\n", f, err)
		}
	}

	return nil
}

func loadFmtAllConfig() (*config.FmtAllConfig, error) {
	root, err := os.Getwd()
	if err != nil {
		return config.LoadFmtAll()
	}

	projectRoot, err := config.FindProjectRoot(root)
	if err != nil {
		return config.LoadFmtAll()
	}

	return config.LoadMergedFmtAll(projectRoot)
}

func walkFiles(root, strategy string) ([]string, error) {
	switch strategy {
	case "git":
		files, err := gitWalk(root)
		if err != nil {
			return allWalk(root)
		}
		return files, nil
	case "all":
		return allWalk(root)
	default:
		return nil, fmt.Errorf("unknown walk strategy: %s", strategy)
	}
}

func gitWalk(root string) ([]string, error) {
	tracked, err := gitLsFiles(root, nil)
	if err != nil {
		return nil, err
	}

	untracked, err := gitLsFiles(root, []string{"--others", "--exclude-standard"})
	if err != nil {
		return nil, err
	}

	var files []string
	for _, f := range append(tracked, untracked...) {
		files = append(files, filepath.Join(root, f))
	}
	return files, nil
}

func gitLsFiles(dir string, extraArgs []string) ([]string, error) {
	args := []string{"ls-files"}
	args = append(args, extraArgs...)

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", dir, err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func allWalk(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func applyExcludeGlobs(files []string, patterns []string, root string) []string {
	if len(patterns) == 0 {
		return files
	}

	var compiled []glob.Glob
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			fmt.Fprintf(logfile.Writer(), "bad exclude glob %q: %v\n", p, err)
			continue
		}
		compiled = append(compiled, g)
	}

	var result []string
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			result = append(result, f)
			continue
		}
		excluded := false
		for _, g := range compiled {
			if g.Match(rel) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, f)
		}
	}
	return result
}

func formatFile(ctx context.Context, filePath string, router *formatter.Router, executor subprocess.Executor) error {
	match := router.Match(filePath)
	if match == nil {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}

	var result *formatter.Result
	switch match.Mode {
	case "chain":
		result, err = formatter.FormatChain(ctx, match.Formatters, filePath, content, executor)
	case "fallback":
		result, err = formatter.FormatFallback(ctx, match.Formatters, filePath, content, executor)
	default:
		return fmt.Errorf("unknown formatter mode: %s", match.Mode)
	}
	if err != nil {
		return err
	}

	if !result.Changed {
		return nil
	}

	return os.WriteFile(filePath, []byte(result.Formatted), 0o644)
}
