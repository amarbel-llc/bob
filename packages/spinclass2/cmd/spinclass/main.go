package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/amarbel-llc/spinclass2/internal/clean"
	"github.com/amarbel-llc/spinclass2/internal/completions"
	"github.com/amarbel-llc/spinclass2/internal/executor"
	"github.com/amarbel-llc/spinclass2/internal/git"
	"github.com/amarbel-llc/spinclass2/internal/hooks"
	"github.com/amarbel-llc/spinclass2/internal/session"
	"github.com/amarbel-llc/spinclass2/internal/merge"
	"github.com/amarbel-llc/spinclass2/internal/perms"
	"github.com/amarbel-llc/spinclass2/internal/pull"
	"github.com/amarbel-llc/spinclass2/internal/shop"
	"github.com/amarbel-llc/spinclass2/internal/sweatfile"
	"github.com/amarbel-llc/spinclass2/internal/validate"
	"github.com/amarbel-llc/spinclass2/internal/worktree"
)

var (
	outputFormat       string
	verbose            bool
	attachMergeOnClose bool
	attachNoAttach     bool
	mergeGitSync       bool
)

var rootCmd = &cobra.Command{
	Use:   "spinclass",
	Short: "Shell-agnostic git worktree session manager",
	Long:  `spinclass manages git worktree lifecycles: creating worktrees + sessions, and offering close workflows (rebase, merge, cleanup, push).`,
}

var attachCmd = &cobra.Command{
	Use:   "attach [name parts...]",
	Short: "Create (if needed) and attach to a worktree session",
	Long:  `Create a worktree if it doesn't exist, then attach to a session. Auto-detects whether to start a new session or resume an active one based on the session state directory. Name parts are joined into a sanitized branch name (snob-case). If an existing branch matches, it is checked out. If no name is provided, a random name is generated.`,
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		format := outputFormat
		if format == "" {
			format = "tap"
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		repoPath, err := worktree.DetectRepo(cwd)
		if err != nil {
			return err
		}

		resolvedPath, err := worktree.ResolvePath(repoPath, args)
		if err != nil {
			return err
		}

		hierarchy, err := sweatfile.LoadWorktreeHierarchy(
			os.Getenv("HOME"), repoPath, resolvedPath.AbsPath,
		)
		if err != nil {
			hierarchy, err = sweatfile.LoadHierarchy(os.Getenv("HOME"), repoPath)
			if err != nil {
				return err
			}
		}

		merged := hierarchy.Merged
		entrypoint := merged.SessionStart()

		// Check for active session and use resume entrypoint if available
		existing, _ := session.Read(resolvedPath.RepoPath, resolvedPath.Branch)
		if existing != nil && existing.ResolveState() == session.StateActive {
			if resume := merged.SessionResume(); resume != nil {
				entrypoint = resume
			} else {
				log.Warn("active session exists, starting second instance",
					"session", resolvedPath.SessionKey)
			}
		}

		exec := executor.SessionExecutor{Entrypoint: entrypoint}

		return shop.Attach(
			os.Stdout,
			exec,
			resolvedPath,
			format,
			attachMergeOnClose,
			attachNoAttach,
			verbose,
		)
	},
}

var mergeCmd = &cobra.Command{
	Use:   "merge [target]",
	Short: "Merge a worktree into main",
	Long:  `Merge a worktree branch into the main repo with --ff-only and remove the worktree. When run from inside a worktree, merges that worktree. When run from the main repo, specify a target or choose interactively.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format := outputFormat
		if format == "" {
			format = "tap"
		}

		var target string
		if len(args) == 1 {
			target = args[0]
		}

		return merge.Run(
			executor.ShellExecutor{},
			format,
			target,
			mergeGitSync,
			verbose,
		)
	},
}

var cleanInteractive bool

var pullDirty bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull repos and rebase worktrees",
	Long:  `Pull all clean repos, then rebase all clean worktrees onto their repo's default branch. Use -d to include dirty repos and worktrees.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format := outputFormat
		if format == "" {
			format = "tap"
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return pull.Run(cwd, pullDirty, format)
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove merged worktrees",
	Long:  `Scan all worktrees, identify those whose branches are fully merged into the main branch, and remove them. Use -i to interactively handle dirty worktrees.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		format := outputFormat
		if format == "" {
			format = "tap"
		}

		return clean.Run(cwd, cleanInteractive, format)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tracked sessions",
	Long:  `List all tracked sessions from the state directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		states, err := session.ListAll()
		if err != nil {
			return err
		}
		for _, s := range states {
			resolved := s.ResolveState()
			fmt.Printf("%s\t%s\t%s\n", s.SessionKey, resolved, s.WorktreePath)
		}
		return nil
	},
}

var completionsSessions bool

var completionsCmd = &cobra.Command{
	Use:    "completions",
	Short:  "Generate tab-separated completions",
	Long:   `Output tab-separated completion entries for shell integration. Use --sessions to list from session state directory instead of scanning local worktrees.`,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		if completionsSessions {
			repoPath, _ := worktree.DetectRepo(cwd)
			completions.Sessions(os.Stdout, repoPath)
			return nil
		}

		completions.Local(cwd, os.Stdout)
		return nil
	},
}

var forkFromDir string

var forkCmd = &cobra.Command{
	Use:   "fork [<new-branch>]",
	Short: "Fork current worktree into a new branch",
	Long:  `Create a new worktree branched from the current worktree's HEAD. If new-branch is omitted, a name is auto-generated as <current-branch>-N. Resolves the source worktree from the current directory or --from flag. Does not attach to the new session.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceDir := forkFromDir
		if sourceDir == "" {
			var err error
			sourceDir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		repoPath, err := worktree.DetectRepo(sourceDir)
		if err != nil {
			return err
		}

		currentBranch, err := git.BranchCurrent(sourceDir)
		if err != nil {
			return fmt.Errorf("could not determine current branch in %s: %w", sourceDir, err)
		}

		currentPath := filepath.Join(
			repoPath,
			worktree.WorktreesDir,
			currentBranch,
		)

		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			return fmt.Errorf(
				"worktree path %s does not exist; fork requires a standard .worktrees layout",
				currentPath,
			)
		}

		sessionKey := filepath.Base(repoPath) + "/" + currentBranch
		rp := worktree.ResolvedPath{
			AbsPath:    currentPath,
			RepoPath:   repoPath,
			Branch:     currentBranch,
			SessionKey: sessionKey,
		}

		var newBranch string
		if len(args) == 1 {
			newBranch = args[0]
		}

		format := outputFormat
		if format == "" {
			format = "tap"
		}

		return shop.Fork(os.Stdout, rp, newBranch, format)
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the sweatfile hierarchy",
	Long:  `Walk the sweatfile hierarchy from PWD and validate each file for structural and semantic correctness. Outputs TAP-14 with subtests.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		exitCode := validate.Run(os.Stdout, home, cwd)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

var cmdExecClaude = &cobra.Command{
	Use:                "exec-claude [claude args...]",
	Short:              "Executes claude after applying sweatfile settings",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		hierarchy, err := sweatfile.LoadDefaultHierarchy()
		if err != nil {
			return err
		}

		return hierarchy.Merged.ExecClaude(args...)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&outputFormat,
		"format",
		"",
		"output format: tap or table",
	)
	rootCmd.PersistentFlags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"show detailed output (YAML diagnostics on passing test points)",
	)
	attachCmd.Flags().BoolVar(
		&attachMergeOnClose,
		"merge-on-close",
		false,
		"auto-merge worktree into default branch on session close",
	)
	attachCmd.Flags().BoolVar(
		&attachNoAttach,
		"no-attach",
		false,
		"create worktree but skip attaching (show command that would run)",
	)
	mergeCmd.Flags().BoolVar(
		&mergeGitSync,
		"git-sync",
		false,
		"pull and push after merge",
	)
	cleanCmd.Flags().BoolVarP(
		&cleanInteractive,
		"interactive",
		"i",
		false,
		"interactively discard changes in dirty merged worktrees",
	)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(listCmd)
	completionsCmd.Flags().BoolVar(
		&completionsSessions,
		"sessions",
		false,
		"list completions from session state directory",
	)
	rootCmd.AddCommand(completionsCmd)
	pullCmd.Flags().BoolVarP(
		&pullDirty,
		"dirty",
		"d",
		false,
		"include dirty repos and worktrees",
	)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(perms.NewPermsCmd())
	rootCmd.AddCommand(hooks.NewHooksCmd())
	forkCmd.Flags().StringVar(
		&forkFromDir,
		"from",
		"",
		"source worktree directory to fork from",
	)
	rootCmd.AddCommand(forkCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(cmdExecClaude)
}

func main() {
	rootCmd.Use = filepath.Base(os.Args[0])
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
