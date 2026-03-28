package sweatfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMinimal(t *testing.T) {
	input := `
git-excludes = [".claude/"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if len(sf.GitSkipIndex) != 1 || sf.GitSkipIndex[0] != ".claude/" {
		t.Errorf("git-excludes: got %v", sf.GitSkipIndex)
	}
}

func TestParseEmpty(t *testing.T) {
	doc, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.GitSkipIndex != nil {
		t.Errorf("expected nil git-excludes, got %v", sf.GitSkipIndex)
	}
}

func TestLoadFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sweatfile")
	os.WriteFile(path, []byte(`git-excludes = [".direnv/"]`), 0o644)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if len(sf.GitSkipIndex) != 1 || sf.GitSkipIndex[0] != ".direnv/" {
		t.Errorf("git-excludes: got %v", sf.GitSkipIndex)
	}
}

func TestLoadMissing(t *testing.T) {
	doc, err := Load("/nonexistent/sweatfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.GitSkipIndex != nil {
		t.Errorf("expected nil git-excludes, got %v", sf.GitSkipIndex)
	}
}

func TestMergeConcatenatesArrays(t *testing.T) {
	base := Sweatfile{
		GitSkipIndex: []string{".claude/"},
	}
	repo := Sweatfile{
		GitSkipIndex: []string{".direnv/"},
	}
	merged := base.MergeWith(repo)
	if len(merged.GitSkipIndex) != 2 {
		t.Fatalf("expected 2 git-excludes, got %v", merged.GitSkipIndex)
	}
	if merged.GitSkipIndex[0] != ".claude/" ||
		merged.GitSkipIndex[1] != ".direnv/" {
		t.Errorf("git-excludes: got %v", merged.GitSkipIndex)
	}
}

func TestMergeClearSentinel(t *testing.T) {
	base := Sweatfile{
		GitSkipIndex: []string{".claude/"},
	}
	repo := Sweatfile{
		GitSkipIndex: []string{},
	}
	merged := base.MergeWith(repo)
	if len(merged.GitSkipIndex) != 0 {
		t.Errorf("expected cleared git-excludes, got %v", merged.GitSkipIndex)
	}
}

func TestMergeBaseOnly(t *testing.T) {
	base := Sweatfile{GitSkipIndex: []string{".claude/"}}
	merged := base.MergeWith(Sweatfile{})
	if len(merged.GitSkipIndex) != 1 || merged.GitSkipIndex[0] != ".claude/" {
		t.Errorf("expected inherited git-excludes, got %v", merged.GitSkipIndex)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sweatfile")

	input := "git-excludes = [\".claude/\"]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = doc.Save(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	sf := loaded.Data()
	if len(sf.GitSkipIndex) != 1 || sf.GitSkipIndex[0] != ".claude/" {
		t.Errorf("git-excludes roundtrip: got %v", sf.GitSkipIndex)
	}
}

func TestParseClaudeAllow(t *testing.T) {
	input := `
claude-allow = ["Read", "Bash(git *)"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if len(sf.ClaudeAllow) != 2 {
		t.Fatalf("expected 2 claude-allow rules, got %v", sf.ClaudeAllow)
	}
	if sf.ClaudeAllow[0] != "Read" || sf.ClaudeAllow[1] != "Bash(git *)" {
		t.Errorf("claude-allow: got %v", sf.ClaudeAllow)
	}
}

func TestMergeClaudeAllowAppends(t *testing.T) {
	base := Sweatfile{ClaudeAllow: []string{"Read", "Glob"}}
	repo := Sweatfile{ClaudeAllow: []string{"Bash(go test:*)"}}
	merged := base.MergeWith(repo)
	if len(merged.ClaudeAllow) != 3 {
		t.Fatalf("expected 3 claude-allow rules, got %v", merged.ClaudeAllow)
	}
	if merged.ClaudeAllow[2] != "Bash(go test:*)" {
		t.Errorf("expected appended rule, got %v", merged.ClaudeAllow)
	}
}

func TestMergeClaudeAllowClear(t *testing.T) {
	base := Sweatfile{ClaudeAllow: []string{"Read", "Glob"}}
	repo := Sweatfile{ClaudeAllow: []string{}}
	merged := base.MergeWith(repo)
	if len(merged.ClaudeAllow) != 0 {
		t.Errorf("expected cleared claude-allow, got %v", merged.ClaudeAllow)
	}
}

func writeSweatfile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestLoadHierarchyGlobalOnly(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	writeSweatfile(t, globalPath, `
git-excludes = [".DS_Store"]
claude-allow = ["/docs"]
`)

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	// Should have checked: global, eng/sweatfile, eng/repos/sweatfile,
	// myrepo/sweatfile
	if len(result.Sources) != 4 {
		t.Fatalf("expected 4 sources, got %d", len(result.Sources))
	}

	// Only global should be found
	if !result.Sources[0].Found {
		t.Error("expected global source to be found")
	}
	for i := 1; i < len(result.Sources); i++ {
		if result.Sources[i].Found {
			t.Errorf(
				"expected source %d (%s) to not be found",
				i,
				result.Sources[i].Path,
			)
		}
	}

	if len(result.Merged.GitSkipIndex) != 1 ||
		result.Merged.GitSkipIndex[0] != ".DS_Store" {
		t.Errorf(
			"expected GitExcludes=[.DS_Store], got %v",
			result.Merged.GitSkipIndex,
		)
	}
	if len(result.Merged.ClaudeAllow) != 1 ||
		result.Merged.ClaudeAllow[0] != "/docs" {
		t.Errorf(
			"expected ClaudeAllow=[/docs], got %v",
			result.Merged.ClaudeAllow,
		)
	}
}

func TestLoadHierarchyGlobalAndRepo(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	writeSweatfile(t, globalPath, `
git-excludes = [".DS_Store"]
`)

	repoSweatfile := filepath.Join(repoDir, "sweatfile")
	writeSweatfile(t, repoSweatfile, `
git-excludes = [".idea"]
claude-allow = ["/src"]
`)

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	// Merged should have both git-excludes appended
	if len(result.Merged.GitSkipIndex) != 2 {
		t.Fatalf("expected 2 GitExcludes, got %v", result.Merged.GitSkipIndex)
	}
	if result.Merged.GitSkipIndex[0] != ".DS_Store" ||
		result.Merged.GitSkipIndex[1] != ".idea" {
		t.Errorf(
			"expected GitExcludes=[.DS_Store, .idea], got %v",
			result.Merged.GitSkipIndex,
		)
	}

	// ClaudeAllow from repo only
	if len(result.Merged.ClaudeAllow) != 1 ||
		result.Merged.ClaudeAllow[0] != "/src" {
		t.Errorf(
			"expected ClaudeAllow=[/src], got %v",
			result.Merged.ClaudeAllow,
		)
	}
}

func TestLoadHierarchyParentDir(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	writeSweatfile(t, globalPath, `
git-excludes = [".DS_Store"]
`)

	parentPath := filepath.Join(home, "eng", "sweatfile")
	writeSweatfile(t, parentPath, `
git-excludes = [".envrc"]
claude-allow = ["/eng-docs"]
`)

	repoSweatfile := filepath.Join(repoDir, "sweatfile")
	writeSweatfile(t, repoSweatfile, `
claude-allow = ["/src"]
`)

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	// git-excludes: global .DS_Store + parent .envrc = [.DS_Store, .envrc]
	// repo has nil git-excludes so inherits
	if len(result.Merged.GitSkipIndex) != 2 {
		t.Fatalf("expected 2 GitExcludes, got %v", result.Merged.GitSkipIndex)
	}
	if result.Merged.GitSkipIndex[0] != ".DS_Store" ||
		result.Merged.GitSkipIndex[1] != ".envrc" {
		t.Errorf(
			"expected GitExcludes=[.DS_Store, .envrc], got %v",
			result.Merged.GitSkipIndex,
		)
	}

	// claude-allow: parent /eng-docs + repo /src = [/eng-docs, /src]
	if len(result.Merged.ClaudeAllow) != 2 {
		t.Fatalf("expected 2 ClaudeAllow, got %v", result.Merged.ClaudeAllow)
	}
	if result.Merged.ClaudeAllow[0] != "/eng-docs" ||
		result.Merged.ClaudeAllow[1] != "/src" {
		t.Errorf(
			"expected ClaudeAllow=[/eng-docs, /src], got %v",
			result.Merged.ClaudeAllow,
		)
	}

	// Verify sources: global found, eng/sweatfile found, eng/repos/sweatfile
	// not found, myrepo/sweatfile found
	if !result.Sources[0].Found {
		t.Error("expected global source to be found")
	}
	if !result.Sources[1].Found {
		t.Error("expected eng/sweatfile source to be found")
	}
	if result.Sources[2].Found {
		t.Error("expected eng/repos/sweatfile source to not be found")
	}
	if !result.Sources[3].Found {
		t.Error("expected repo sweatfile source to be found")
	}
}

func TestLoadHierarchyNoSweatfiles(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	// All sources should be not found
	for i, src := range result.Sources {
		if src.Found {
			t.Errorf("expected source %d (%s) to not be found", i, src.Path)
		}
	}

	// Merged should be empty
	if result.Merged.GitSkipIndex != nil {
		t.Errorf("expected nil GitExcludes, got %v", result.Merged.GitSkipIndex)
	}
	if result.Merged.ClaudeAllow != nil {
		t.Errorf("expected nil ClaudeAllow, got %v", result.Merged.ClaudeAllow)
	}
}

func TestParseHooksCreate(t *testing.T) {
	input := `
[hooks]
create = "composer install"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Hooks == nil || sf.Hooks.Create == nil ||
		*sf.Hooks.Create != "composer install" {
		t.Errorf("hooks.create: got %v", sf.Hooks)
	}
}

func TestParseHooksStop(t *testing.T) {
	input := `
[hooks]
stop = "just test"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Hooks == nil || sf.Hooks.Stop == nil ||
		*sf.Hooks.Stop != "just test" {
		t.Errorf("hooks.stop: got %v", sf.Hooks)
	}
}

func TestParseHooksBoth(t *testing.T) {
	input := `
[hooks]
create = "npm install"
stop = "just lint"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
	if sf.Hooks.Create == nil || *sf.Hooks.Create != "npm install" {
		t.Errorf("hooks.create: got %v", sf.Hooks.Create)
	}
	if sf.Hooks.Stop == nil || *sf.Hooks.Stop != "just lint" {
		t.Errorf("hooks.stop: got %v", sf.Hooks.Stop)
	}
}

func TestParseHooksAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Hooks != nil {
		t.Errorf("expected nil hooks, got %v", sf.Hooks)
	}
}

func TestMergeHooksCreateInherit(t *testing.T) {
	cmd := "npm install"
	base := Sweatfile{Hooks: &Hooks{Create: &cmd}}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Create == nil ||
		*merged.Hooks.Create != "npm install" {
		t.Errorf("expected inherited hooks.create, got %v", merged.Hooks)
	}
}

func TestMergeHooksCreateOverride(t *testing.T) {
	baseCmd := "npm install"
	repoCmd := "composer install"
	base := Sweatfile{Hooks: &Hooks{Create: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{Create: &repoCmd}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Create == nil ||
		*merged.Hooks.Create != "composer install" {
		t.Errorf("expected overridden hooks.create, got %v", merged.Hooks)
	}
}

func TestMergeHooksCreateClear(t *testing.T) {
	baseCmd := "npm install"
	empty := ""
	base := Sweatfile{Hooks: &Hooks{Create: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{Create: &empty}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Create == nil ||
		*merged.Hooks.Create != "" {
		t.Errorf("expected cleared hooks.create, got %v", merged.Hooks)
	}
}

func TestMergeHooksStopInherit(t *testing.T) {
	cmd := "just test"
	base := Sweatfile{Hooks: &Hooks{Stop: &cmd}}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Stop == nil ||
		*merged.Hooks.Stop != "just test" {
		t.Errorf("expected inherited hooks.stop, got %v", merged.Hooks)
	}
}

func TestMergeHooksStopOverride(t *testing.T) {
	baseCmd := "just test"
	repoCmd := "just lint"
	base := Sweatfile{Hooks: &Hooks{Stop: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{Stop: &repoCmd}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Stop == nil ||
		*merged.Hooks.Stop != "just lint" {
		t.Errorf("expected overridden hooks.stop, got %v", merged.Hooks)
	}
}

func TestMergeHooksStopClear(t *testing.T) {
	baseCmd := "just test"
	empty := ""
	base := Sweatfile{Hooks: &Hooks{Stop: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{Stop: &empty}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.Stop == nil ||
		*merged.Hooks.Stop != "" {
		t.Errorf("expected cleared hooks.stop, got %v", merged.Hooks)
	}
}

func TestMergeHooksIndependentFields(t *testing.T) {
	createCmd := "npm install"
	stopCmd := "just test"
	base := Sweatfile{Hooks: &Hooks{Create: &createCmd}}
	repo := Sweatfile{Hooks: &Hooks{Stop: &stopCmd}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
	if merged.Hooks.Create == nil || *merged.Hooks.Create != "npm install" {
		t.Errorf("expected inherited hooks.create, got %v", merged.Hooks.Create)
	}
	if merged.Hooks.Stop == nil || *merged.Hooks.Stop != "just test" {
		t.Errorf("expected overridden hooks.stop, got %v", merged.Hooks.Stop)
	}
}

func TestParseHooksPreMerge(t *testing.T) {
	input := `
[hooks]
pre-merge = "just test"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Hooks == nil || sf.Hooks.PreMerge == nil ||
		*sf.Hooks.PreMerge != "just test" {
		t.Errorf("hooks.pre-merge: got %v", sf.Hooks)
	}
}

func TestMergeHooksPreMergeInherit(t *testing.T) {
	cmd := "just test"
	base := Sweatfile{Hooks: &Hooks{PreMerge: &cmd}}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.PreMerge == nil ||
		*merged.Hooks.PreMerge != "just test" {
		t.Errorf("expected inherited hooks.pre-merge, got %v", merged.Hooks)
	}
}

func TestMergeHooksPreMergeOverride(t *testing.T) {
	baseCmd := "just test"
	repoCmd := "just lint"
	base := Sweatfile{Hooks: &Hooks{PreMerge: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{PreMerge: &repoCmd}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.PreMerge == nil ||
		*merged.Hooks.PreMerge != "just lint" {
		t.Errorf("expected overridden hooks.pre-merge, got %v", merged.Hooks)
	}
}

func TestMergeHooksPreMergeClear(t *testing.T) {
	baseCmd := "just test"
	empty := ""
	base := Sweatfile{Hooks: &Hooks{PreMerge: &baseCmd}}
	repo := Sweatfile{Hooks: &Hooks{PreMerge: &empty}}
	merged := base.MergeWith(repo)
	if merged.Hooks == nil || merged.Hooks.PreMerge == nil ||
		*merged.Hooks.PreMerge != "" {
		t.Errorf("expected cleared hooks.pre-merge, got %v", merged.Hooks)
	}
}

func TestLoadHierarchyRepoOverridesParent(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	parentPath := filepath.Join(home, "eng", "sweatfile")
	writeSweatfile(t, parentPath, `
git-excludes = [".DS_Store", ".envrc"]
claude-allow = ["/docs"]
`)

	// Repo sweatfile with empty arrays clears parent values
	repoSweatfile := filepath.Join(repoDir, "sweatfile")
	writeSweatfile(t, repoSweatfile, `
git-excludes = []
claude-allow = []
`)

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	// Empty arrays should clear parent values
	if result.Merged.GitSkipIndex == nil ||
		len(result.Merged.GitSkipIndex) != 0 {
		t.Errorf(
			"expected empty GitExcludes (cleared by repo), got %v",
			result.Merged.GitSkipIndex,
		)
	}
	if result.Merged.ClaudeAllow == nil || len(result.Merged.ClaudeAllow) != 0 {
		t.Errorf(
			"expected empty ClaudeAllow (cleared by repo), got %v",
			result.Merged.ClaudeAllow,
		)
	}
}

func TestLoadHierarchyHooksStopInherited(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	os.MkdirAll(repoDir, 0o755)

	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	writeSweatfile(t, globalPath, "[hooks]\nstop = \"just test\"")

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	if result.Merged.StopHookCommand() == nil ||
		*result.Merged.StopHookCommand() != "just test" {
		t.Errorf("expected inherited hooks.stop, got %v", result.Merged.Hooks)
	}
}

func TestParseSystemPrompt(t *testing.T) {
	input := `system-prompt = "do stuff"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.SystemPrompt == nil || *sf.SystemPrompt != "do stuff" {
		t.Errorf("system-prompt: got %v", sf.SystemPrompt)
	}
}

func TestParseSystemPromptEmpty(t *testing.T) {
	input := `system-prompt = ""`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.SystemPrompt == nil {
		t.Fatal("expected non-nil system-prompt for explicit empty string")
	}
	if *sf.SystemPrompt != "" {
		t.Errorf("expected empty system-prompt, got %q", *sf.SystemPrompt)
	}
}

func TestParseSystemPromptAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.SystemPrompt != nil {
		t.Errorf("expected nil system-prompt, got %v", sf.SystemPrompt)
	}
}

func TestParseSystemPromptAppend(t *testing.T) {
	input := `system-prompt-append = "extra instructions"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.SystemPromptAppend == nil ||
		*sf.SystemPromptAppend != "extra instructions" {
		t.Errorf("system-prompt-append: got %v", sf.SystemPromptAppend)
	}
}

func TestParseSystemPromptAppendAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.SystemPromptAppend != nil {
		t.Errorf(
			"expected nil system-prompt-append, got %v",
			sf.SystemPromptAppend,
		)
	}
}

func TestMergeSystemPromptInherit(t *testing.T) {
	prompt := "base prompt"
	base := Sweatfile{SystemPrompt: &prompt}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.SystemPrompt == nil || *merged.SystemPrompt != "base prompt" {
		t.Errorf(
			"expected inherited system-prompt, got %v",
			merged.SystemPrompt,
		)
	}
}

func TestMergeSystemPromptConcatenate(t *testing.T) {
	basePrompt := "base prompt"
	repoPrompt := "repo prompt"
	base := Sweatfile{SystemPrompt: &basePrompt}
	repo := Sweatfile{SystemPrompt: &repoPrompt}
	merged := base.MergeWith(repo)
	if merged.SystemPrompt == nil ||
		*merged.SystemPrompt != "base prompt repo prompt" {
		t.Errorf(
			"expected concatenated system-prompt, got %v",
			merged.SystemPrompt,
		)
	}
}

func TestMergeSystemPromptClear(t *testing.T) {
	basePrompt := "base prompt"
	empty := ""
	base := Sweatfile{SystemPrompt: &basePrompt}
	repo := Sweatfile{SystemPrompt: &empty}
	merged := base.MergeWith(repo)
	if merged.SystemPrompt == nil {
		t.Fatal("expected non-nil system-prompt after clear")
	}
	if *merged.SystemPrompt != "" {
		t.Errorf("expected cleared system-prompt, got %q", *merged.SystemPrompt)
	}
}

func TestMergeSystemPromptAppendInherit(t *testing.T) {
	prompt := "base append"
	base := Sweatfile{SystemPromptAppend: &prompt}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.SystemPromptAppend == nil ||
		*merged.SystemPromptAppend != "base append" {
		t.Errorf(
			"expected inherited system-prompt-append, got %v",
			merged.SystemPromptAppend,
		)
	}
}

func TestMergeSystemPromptAppendConcatenate(t *testing.T) {
	basePrompt := "base append"
	repoPrompt := "repo append"
	base := Sweatfile{SystemPromptAppend: &basePrompt}
	repo := Sweatfile{SystemPromptAppend: &repoPrompt}
	merged := base.MergeWith(repo)
	if merged.SystemPromptAppend == nil ||
		*merged.SystemPromptAppend != "base append repo append" {
		t.Errorf(
			"expected concatenated system-prompt-append, got %v",
			merged.SystemPromptAppend,
		)
	}
}

func TestMergeSystemPromptAppendClear(t *testing.T) {
	basePrompt := "base append"
	empty := ""
	base := Sweatfile{SystemPromptAppend: &basePrompt}
	repo := Sweatfile{SystemPromptAppend: &empty}
	merged := base.MergeWith(repo)
	if merged.SystemPromptAppend == nil {
		t.Fatal("expected non-nil system-prompt-append after clear")
	}
	if *merged.SystemPromptAppend != "" {
		t.Errorf(
			"expected cleared system-prompt-append, got %q",
			*merged.SystemPromptAppend,
		)
	}
}

func TestParseEnvrcDirectives(t *testing.T) {
	input := `envrc-directives = ["source_up", "dotenv_if_exists"]`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if len(sf.EnvrcDirectives) != 2 {
		t.Fatalf("expected 2 envrc-directives, got %v", sf.EnvrcDirectives)
	}
	if sf.EnvrcDirectives[0] != "source_up" ||
		sf.EnvrcDirectives[1] != "dotenv_if_exists" {
		t.Errorf("envrc-directives: got %v", sf.EnvrcDirectives)
	}
}

func TestParseEnvrcDirectivesAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.EnvrcDirectives != nil {
		t.Errorf("expected nil envrc-directives, got %v", sf.EnvrcDirectives)
	}
}

func TestMergeEnvrcDirectivesAppend(t *testing.T) {
	base := Sweatfile{EnvrcDirectives: []string{"source_up"}}
	repo := Sweatfile{EnvrcDirectives: []string{"dotenv_if_exists"}}
	merged := base.MergeWith(repo)
	if len(merged.EnvrcDirectives) != 2 {
		t.Fatalf("expected 2 envrc-directives, got %v", merged.EnvrcDirectives)
	}
}

func TestMergeEnvrcDirectivesClear(t *testing.T) {
	base := Sweatfile{EnvrcDirectives: []string{"source_up"}}
	repo := Sweatfile{EnvrcDirectives: []string{}}
	merged := base.MergeWith(repo)
	if len(merged.EnvrcDirectives) != 0 {
		t.Errorf(
			"expected cleared envrc-directives, got %v",
			merged.EnvrcDirectives,
		)
	}
}

func TestMergeEnvrcDirectivesInherit(t *testing.T) {
	base := Sweatfile{EnvrcDirectives: []string{"source_up"}}
	merged := base.MergeWith(Sweatfile{})
	if len(merged.EnvrcDirectives) != 1 ||
		merged.EnvrcDirectives[0] != "source_up" {
		t.Errorf(
			"expected inherited envrc-directives, got %v",
			merged.EnvrcDirectives,
		)
	}
}

func TestParseEnv(t *testing.T) {
	input := `
[env]
FOO = "bar"
BAZ = "qux"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if len(sf.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %v", sf.Env)
	}
	if sf.Env["FOO"] != "bar" || sf.Env["BAZ"] != "qux" {
		t.Errorf("env: got %v", sf.Env)
	}
}

func TestParseEnvAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.Env != nil {
		t.Errorf("expected nil env, got %v", sf.Env)
	}
}

func TestMergeEnvInherit(t *testing.T) {
	base := Sweatfile{Env: map[string]string{"FOO": "bar"}}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if merged.Env["FOO"] != "bar" {
		t.Errorf("expected inherited env, got %v", merged.Env)
	}
}

func TestMergeEnvOverrideKey(t *testing.T) {
	base := Sweatfile{Env: map[string]string{"FOO": "bar", "BAZ": "qux"}}
	repo := Sweatfile{Env: map[string]string{"FOO": "override"}}
	merged := base.MergeWith(repo)
	if merged.Env["FOO"] != "override" {
		t.Errorf("expected overridden FOO, got %v", merged.Env["FOO"])
	}
	if merged.Env["BAZ"] != "qux" {
		t.Errorf("expected inherited BAZ, got %v", merged.Env["BAZ"])
	}
}

func TestMergeEnvAddKey(t *testing.T) {
	base := Sweatfile{Env: map[string]string{"FOO": "bar"}}
	repo := Sweatfile{Env: map[string]string{"BAZ": "qux"}}
	merged := base.MergeWith(repo)
	if len(merged.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %v", merged.Env)
	}
}

func TestLoadHierarchyHooksStopOverriddenByRepo(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(home, "eng", "repos", "myrepo")
	os.MkdirAll(repoDir, 0o755)

	globalPath := filepath.Join(home, ".config", "spinclass", "sweatfile")
	writeSweatfile(t, globalPath, "[hooks]\nstop = \"just test\"")

	repoSweatfile := filepath.Join(repoDir, "sweatfile")
	writeSweatfile(t, repoSweatfile, "[hooks]\nstop = \"just lint\"")

	result, err := LoadHierarchy(home, repoDir)
	if err != nil {
		t.Fatalf("LoadHierarchy returned error: %v", err)
	}

	if result.Merged.StopHookCommand() == nil ||
		*result.Merged.StopHookCommand() != "just lint" {
		t.Errorf("expected overridden hooks.stop, got %v", result.Merged.Hooks)
	}
}

func TestParseHooksDisallowMainWorktree(t *testing.T) {
	input := `
[hooks]
disallow-main-worktree = true
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if !sf.DisallowMainWorktreeEnabled() {
		t.Error("expected disallow-main-worktree to be enabled")
	}
}

func TestParseHooksDisallowMainWorktreeAbsent(t *testing.T) {
	doc, err := Parse([]byte(`git-excludes = [".claude/"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := doc.Data()
	if sf.DisallowMainWorktreeEnabled() {
		t.Error("expected disallow-main-worktree to be disabled when absent")
	}
}

func TestMergeDisallowMainWorktreeInherit(t *testing.T) {
	enabled := true
	base := Sweatfile{Hooks: &Hooks{DisallowMainWorktree: &enabled}}
	repo := Sweatfile{}
	merged := base.MergeWith(repo)
	if !merged.DisallowMainWorktreeEnabled() {
		t.Error("expected inherited disallow-main-worktree")
	}
}

func TestMergeDisallowMainWorktreeOverride(t *testing.T) {
	enabled := true
	disabled := false
	base := Sweatfile{Hooks: &Hooks{DisallowMainWorktree: &enabled}}
	repo := Sweatfile{Hooks: &Hooks{DisallowMainWorktree: &disabled}}
	merged := base.MergeWith(repo)
	if merged.DisallowMainWorktreeEnabled() {
		t.Error("expected overridden disallow-main-worktree to be disabled")
	}
}

func TestLoadWorktreeHierarchyMainRepoSweatfileIncluded(t *testing.T) {
	home := t.TempDir()
	mainRepo := filepath.Join(home, "eng", "repos", "myrepo")
	worktreeDir := filepath.Join(mainRepo, ".worktrees", "my-branch")
	os.MkdirAll(worktreeDir, 0o755)

	// Main repo sweatfile enables disallow-main-worktree
	writeSweatfile(t, filepath.Join(mainRepo, "sweatfile"),
		"[hooks]\ndisallow-main-worktree = true\n")

	result, err := LoadWorktreeHierarchy(home, mainRepo, worktreeDir)
	if err != nil {
		t.Fatalf("LoadWorktreeHierarchy returned error: %v", err)
	}

	if !result.Merged.DisallowMainWorktreeEnabled() {
		t.Error("expected disallow-main-worktree from main repo sweatfile")
	}
}

func TestLoadWorktreeHierarchyWorktreeOverridesMainRepo(t *testing.T) {
	home := t.TempDir()
	mainRepo := filepath.Join(home, "eng", "repos", "myrepo")
	worktreeDir := filepath.Join(mainRepo, ".worktrees", "my-branch")
	os.MkdirAll(worktreeDir, 0o755)

	// Main repo enables it
	writeSweatfile(t, filepath.Join(mainRepo, "sweatfile"),
		"[hooks]\ndisallow-main-worktree = true\n")

	// Worktree disables it
	writeSweatfile(t, filepath.Join(worktreeDir, "sweatfile"),
		"[hooks]\ndisallow-main-worktree = false\n")

	result, err := LoadWorktreeHierarchy(home, mainRepo, worktreeDir)
	if err != nil {
		t.Fatalf("LoadWorktreeHierarchy returned error: %v", err)
	}

	if result.Merged.DisallowMainWorktreeEnabled() {
		t.Error("expected worktree sweatfile to override main repo")
	}
}

func TestMergeToolUseLogInherit(t *testing.T) {
	enabled := true
	base := Sweatfile{Hooks: &Hooks{ToolUseLog: &enabled}}
	overlay := Sweatfile{}
	merged := base.MergeWith(overlay)
	if !merged.ToolUseLogEnabled() {
		t.Error("expected ToolUseLog to be inherited")
	}
}

func TestMergeToolUseLogOverride(t *testing.T) {
	enabled := true
	disabled := false
	base := Sweatfile{Hooks: &Hooks{ToolUseLog: &enabled}}
	overlay := Sweatfile{Hooks: &Hooks{ToolUseLog: &disabled}}
	merged := base.MergeWith(overlay)
	if merged.ToolUseLogEnabled() {
		t.Error("expected ToolUseLog to be overridden to false")
	}
}

func TestParseToolUseLog(t *testing.T) {
	doc, err := Parse([]byte("[hooks]\ntool-use-log = true\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !doc.Data().ToolUseLogEnabled() {
		t.Error("expected ToolUseLog to be true")
	}
	undecoded := doc.Undecoded()
	for _, key := range undecoded {
		if key == "hooks.tool-use-log" {
			t.Error("tool-use-log should be decoded, not undecoded")
		}
	}
}

func TestResolvePathOrStringLiteral(t *testing.T) {
	result := resolvePathOrString("just a string")
	if result != "just a string" {
		t.Errorf("expected literal string, got %q", result)
	}
}

func TestResolvePathOrStringFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	os.WriteFile(path, []byte("contents from file\n"), 0o644)

	result := resolvePathOrString(path)
	if result != "contents from file" {
		t.Errorf("expected file contents, got %q", result)
	}
}

func TestResolvePathOrStringEnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	os.WriteFile(path, []byte("env resolved\n"), 0o644)

	t.Setenv("TEST_RESOLVE_DIR", dir)
	result := resolvePathOrString("$TEST_RESOLVE_DIR/prompt.txt")
	if result != "env resolved" {
		t.Errorf("expected env-expanded file contents, got %q", result)
	}
}

func TestResolvePathOrStringNonexistentFile(t *testing.T) {
	result := resolvePathOrString("/nonexistent/path/to/file.txt")
	if result != "/nonexistent/path/to/file.txt" {
		t.Errorf("expected literal fallback, got %q", result)
	}
}

func TestRoundTripPreservesComments(t *testing.T) {
	input := `# Global config
system-prompt = "be helpful"
git-excludes = [".claude/", ".direnv/"]
claude-allow = ["Bash(git *)"]
envrc-directives = ["source_up", "use flake"]

[env]
FOO = "bar"

[hooks]
# install deps on create
create = "npm install"
stop = "just test"
disallow-main-worktree = true
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	output, err := doc.Encode()
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	if string(output) != input {
		t.Errorf("round-trip mismatch:\n--- want ---\n%s\n--- got ---\n%s", input, string(output))
	}
}
