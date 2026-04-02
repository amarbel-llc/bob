package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
)

func runInit(useDefault, force bool) error {
	configDir := config.ConfigDir()
	filetypeDir := filetype.GlobalDir()

	if err := os.MkdirAll(filetypeDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", filetypeDir, err)
	}
	fmt.Printf("created %s/\n", filetypeDir)

	var files map[string]string
	if useDefault {
		files = defaultConfigFiles(configDir, filetypeDir)
	} else {
		files = emptyConfigFiles(configDir)
	}

	for path, content := range files {
		if err := writeConfigFile(path, content, force); err != nil {
			return err
		}
	}

	return nil
}

func writeConfigFile(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("skipped %s (already exists, use --force to overwrite)\n", path)
			return nil
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	fmt.Printf("wrote %s\n", path)
	return nil
}

func emptyConfigFiles(configDir string) map[string]string {
	return map[string]string{
		filepath.Join(configDir, "lsps.toml"):       "",
		filepath.Join(configDir, "formatters.toml"): "",
	}
}

func defaultConfigFiles(configDir, filetypeDir string) map[string]string {
	return map[string]string{
		filepath.Join(configDir, "lsps.toml"):         defaultLSPsConfig,
		filepath.Join(configDir, "formatters.toml"):   defaultFormattersConfig,
		filepath.Join(filetypeDir, "go.toml"):         defaultFiletypeGo,
		filepath.Join(filetypeDir, "python.toml"):     defaultFiletypePython,
		filepath.Join(filetypeDir, "javascript.toml"): defaultFiletypeJavascript,
		filepath.Join(filetypeDir, "typescript.toml"): defaultFiletypeTypescript,
		filepath.Join(filetypeDir, "rust.toml"):       defaultFiletypeRust,
		filepath.Join(filetypeDir, "lua.toml"):        defaultFiletypeLua,
		filepath.Join(filetypeDir, "nix.toml"):        defaultFiletypeNix,
		filepath.Join(filetypeDir, "shell.toml"):      defaultFiletypeShell,
		filepath.Join(filetypeDir, "toml.toml"):       defaultFiletypeToml,
		filepath.Join(filetypeDir, "yaml.toml"):       defaultFiletypeYaml,
		filepath.Join(filetypeDir, "css.toml"):        defaultFiletypeCss,
		filepath.Join(filetypeDir, "html.toml"):       defaultFiletypeHtml,
		filepath.Join(filetypeDir, "json.toml"):       defaultFiletypeJson,
		filepath.Join(filetypeDir, "zig.toml"):        defaultFiletypeZig,
		filepath.Join(filetypeDir, "c.toml"):          defaultFiletypeC,
		filepath.Join(filetypeDir, "java.toml"):       defaultFiletypeJava,
		filepath.Join(filetypeDir, "swift.toml"):      defaultFiletypeSwift,
		filepath.Join(filetypeDir, "bats.toml"):       defaultFiletypeBats,
	}
}

const defaultLSPsConfig = `[[lsp]]
name = "gopls"
flake = "nixpkgs#gopls"

[[lsp]]
name = "pyright"
flake = "nixpkgs#pyright"

[[lsp]]
name = "typescript-language-server"
flake = "nixpkgs#nodePackages.typescript-language-server"

[[lsp]]
name = "rust-analyzer"
flake = "nixpkgs#rust-analyzer"

[[lsp]]
name = "lua-language-server"
flake = "nixpkgs#lua-language-server"

[[lsp]]
name = "nil"
flake = "nixpkgs#nil"

[[lsp]]
name = "bash-language-server"
flake = "nixpkgs#nodePackages.bash-language-server"
`

const defaultFormattersConfig = `[[formatter]]
name = "goimports"
flake = "nixpkgs#gotools"
binary = "goimports"
args = ["-srcdir", "{file}"]
mode = "stdin"

[[formatter]]
name = "gofumpt"
flake = "nixpkgs#gofumpt"
mode = "stdin"

[[formatter]]
name = "isort"
flake = "nixpkgs#isort"

[[formatter]]
name = "black"
flake = "nixpkgs#black"

[[formatter]]
name = "prettierd"
flake = "nixpkgs#prettierd"
args = ["--stdin-filepath", "{file}"]
mode = "stdin"

[[formatter]]
name = "prettier"
flake = "nixpkgs#nodePackages.prettier"
args = ["--stdin-filepath", "{file}"]
mode = "stdin"

[[formatter]]
name = "nixfmt"
flake = "nixpkgs#nixfmt-rfc-style"
mode = "stdin"

[[formatter]]
name = "rustfmt"
flake = "nixpkgs#rustfmt"
mode = "stdin"

[[formatter]]
name = "tommy"
flake = "github:amarbel-llc/tommy"
args = ["fmt", "-"]
mode = "stdin"

[[formatter]]
name = "stylua"
flake = "nixpkgs#stylua"
args = ["-"]
mode = "stdin"

[[formatter]]
name = "zig-fmt"
flake = "nixpkgs#zig"
binary = "zig"
args = ["fmt", "--stdin"]
mode = "stdin"

[[formatter]]
name = "clang-format"
flake = "nixpkgs#clang-tools"
binary = "clang-format"
mode = "stdin"

[[formatter]]
name = "google-java-format"
flake = "nixpkgs#google-java-format"
args = ["-"]
mode = "stdin"

[[formatter]]
name = "swift-format"
flake = "nixpkgs#swift-format"
mode = "stdin"

[[formatter]]
name = "jq"
flake = "nixpkgs#jq"
args = ["."]
mode = "stdin"

[[formatter]]
name = "shfmt"
flake = "nixpkgs#shfmt"
args = ["-s", "-i=2"]
mode = "stdin"
`

const defaultFiletypeGo = `extensions = ["go"]
language_ids = ["go"]
lsp = "gopls"
formatters = ["goimports", "gofumpt"]
formatter_mode = "chain"
`

const defaultFiletypePython = `extensions = ["py"]
language_ids = ["python"]
lsp = "pyright"
formatters = ["isort", "black"]
formatter_mode = "chain"
`

const defaultFiletypeJavascript = `extensions = ["js", "jsx"]
language_ids = ["javascript", "javascriptreact"]
lsp = "typescript-language-server"
formatters = ["prettierd", "prettier"]
formatter_mode = "fallback"
`

const defaultFiletypeTypescript = `extensions = ["ts", "tsx"]
language_ids = ["typescript", "typescriptreact"]
lsp = "typescript-language-server"
formatters = ["prettierd", "prettier"]
formatter_mode = "fallback"
`

const defaultFiletypeRust = `extensions = ["rs"]
language_ids = ["rust"]
lsp = "rust-analyzer"
formatters = ["rustfmt"]
lsp_format = "fallback"
`

const defaultFiletypeLua = `extensions = ["lua"]
language_ids = ["lua"]
lsp = "lua-language-server"
formatters = ["stylua"]
`

const defaultFiletypeNix = `extensions = ["nix"]
language_ids = ["nix"]
lsp = "nil"
formatters = ["nixfmt"]
`

const defaultFiletypeShell = `extensions = ["sh", "bash"]
language_ids = ["shellscript"]
lsp = "bash-language-server"
formatters = ["shfmt"]
`

const defaultFiletypeToml = `extensions = ["toml"]
formatters = ["tommy"]
`

const defaultFiletypeYaml = `extensions = ["yaml", "yml"]
formatters = ["prettier"]
`

const defaultFiletypeCss = `extensions = ["css", "scss"]
language_ids = ["css", "scss"]
formatters = ["prettier"]
`

const defaultFiletypeHtml = `extensions = ["html", "htm"]
language_ids = ["html"]
formatters = ["prettier"]
`

const defaultFiletypeJson = `extensions = ["json"]
formatters = ["jq"]
`

const defaultFiletypeZig = `extensions = ["zig"]
language_ids = ["zig"]
formatters = ["zig-fmt"]
`

const defaultFiletypeC = `extensions = ["c", "h", "cc", "cpp", "hpp", "cxx", "hxx"]
language_ids = ["c", "cpp"]
formatters = ["clang-format"]
`

const defaultFiletypeJava = `extensions = ["java"]
language_ids = ["java"]
formatters = ["google-java-format"]
`

const defaultFiletypeSwift = `extensions = ["swift"]
language_ids = ["swift"]
formatters = ["swift-format"]
`

const defaultFiletypeBats = `extensions = ["bats"]
formatters = ["shfmt"]
`
