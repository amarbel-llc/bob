package clone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tap "github.com/amarbel-llc/bob/packages/tap-dancer/go"
	"github.com/charmbracelet/huh"
	"github.com/friedenberg/get-hubbed/internal/gh"
)

type repo struct {
	FullName string
	Name     string
}

func fetchUserRepos(ctx context.Context) ([]repo, error) {
	out, err := gh.Run(ctx,
		"api", "user/repos",
		"--paginate",
		"--jq", ".[].full_name",
	)
	if err != nil {
		return nil, fmt.Errorf("fetching repos: %w", err)
	}

	var repos []repo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			continue
		}
		repos = append(repos, repo{FullName: line, Name: parts[1]})
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].FullName < repos[j].FullName
	})

	return repos, nil
}

func isCloned(ctx context.Context, targetDir, repoName string) bool {
	dir := filepath.Join(targetDir, repoName)
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func Run(ctx context.Context, targetDir string) error {
	info, err := os.Stat(targetDir)
	if err != nil {
		return fmt.Errorf("target directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target path is not a directory: %s", targetDir)
	}

	repos, err := fetchUserRepos(ctx)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Println("No repos found for authenticated user.")
		return nil
	}

	var uncloned []repo
	for _, r := range repos {
		if !isCloned(ctx, targetDir, r.Name) {
			uncloned = append(uncloned, r)
		}
	}

	if len(uncloned) == 0 {
		fmt.Println("All repos already cloned.")
		return nil
	}

	options := make([]huh.Option[string], len(uncloned))
	for i, r := range uncloned {
		options[i] = huh.NewOption(r.FullName, r.FullName)
	}

	var selected []string
	err = huh.NewMultiSelect[string]().
		Title("Select repos to clone").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		return nil
	}

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolving target directory: %w", err)
	}

	nameByFullName := make(map[string]string, len(uncloned))
	for _, r := range uncloned {
		nameByFullName[r.FullName] = r.Name
	}

	args := make([]string, len(selected))
	for i, fullName := range selected {
		name := nameByFullName[fullName]
		args[i] = fullName + " " + filepath.Join(absTarget, name)
	}

	executor := &tap.GoroutineExecutor{MaxJobs: 4}
	exitCode := tap.ConvertExecParallelWithStatus(
		ctx, executor,
		"gh repo clone {}",
		args,
		os.Stdout,
		false,
		true,
	)

	if exitCode != 0 {
		return fmt.Errorf("some repos failed to clone")
	}

	return nil
}
