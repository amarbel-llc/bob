# Lux LSP Editor Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add `lux lsp` subcommand that speaks LSP JSON-RPC over stdio, letting
editors use lux as an LSP server.

**Architecture:** `internal/server/Server` already is a full LSP proxy (routes
requests to backend LSPs, handles lifecycle, external formatters). The `lux lsp`
subcommand just needs to wire it to stdio directly --- bypassing the MCP layer.
A `--lang` flag restricts routing to a single LSP. Phase 1 constrains
capabilities to formatting only.

**Tech Stack:** Go, jsonrpc (go-mcp), BATS + neovim for integration tests.

**Rollback:** Purely additive. Remove the `lsp` subcommand to revert. Editor
native LSP configs continue working throughout.

--------------------------------------------------------------------------------

### Task 1: Add `lux lsp` subcommand (multiplexing mode)

**Files:** - Modify: `packages/lux/cmd/lux/app.go`

**Step 1: Write the failing test**

No unit test needed --- this is CLI wiring. The integration test in Task 3
validates this.

**Step 2: Add the `lsp` subcommand**

In `app.go`, inside `addCLICommands`, add after the `fmt` command:

``` go
app.AddCommand(&command.Command{
    Name: "lsp",
    Description: command.Description{
        Short: "Run as an LSP server over stdio",
        Long:  "Run Lux as an LSP server, proxying requests to backend language servers based on file type. Editors connect via stdio.",
    },
    Params: []command.Param{
        {Name: "lang", Type: command.String, Description: "Restrict to a single language/LSP name (e.g., gopls)"},
    },
    RunCLI: func(ctx context.Context, args json.RawMessage) error {
        var p struct {
            Lang string `json:"lang"`
        }
        if err := json.Unmarshal(args, &p); err != nil {
            return fmt.Errorf("invalid arguments: %w", err)
        }
        return runLSP(ctx, p.Lang)
    },
})
```

**Step 3: Implement `runLSP`**

Add a new file `packages/lux/cmd/lux/lsp.go`:

``` go
package main

import (
    "context"
    "fmt"

    "github.com/amarbel-llc/lux/internal/config"
    "github.com/amarbel-llc/lux/internal/server"
)

func runLSP(ctx context.Context, lang string) error {
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("loading config: %w", err)
    }

    if lang != "" {
        cfg, err = cfg.FilterByLSP(lang)
        if err != nil {
            return fmt.Errorf("filtering config for %s: %w", lang, err)
        }
    }

    srv, err := server.New(cfg)
    if err != nil {
        return fmt.Errorf("creating LSP server: %w", err)
    }

    return srv.Run(ctx)
}
```

**Step 4: Build and verify**

Run: `cd packages/lux && nix develop --command go build ./cmd/lux/` Expected:
builds without errors.

Run: `./lux lsp --help` Expected: shows help for the `lsp` subcommand.

**Step 5: Commit**

Message: `feat(lux): add lsp subcommand for editor integration`

--------------------------------------------------------------------------------

### Task 2: Add `Config.FilterByLSP` method

The `--lang` flag needs to restrict which LSPs are registered. Add a method to
filter the config to a single LSP.

**Files:** - Modify: `packages/lux/internal/config/config.go` - Create:
`packages/lux/internal/config/config_test.go` (if not exists, or add test)

**Step 1: Write the failing test**

``` go
func TestFilterByLSP(t *testing.T) {
    cfg := &Config{
        LSPs: []LSP{
            {Name: "gopls", Flake: "nixpkgs#gopls"},
            {Name: "pyright", Flake: "nixpkgs#pyright"},
        },
    }

    filtered, err := cfg.FilterByLSP("gopls")
    if err != nil {
        t.Fatal(err)
    }
    if len(filtered.LSPs) != 1 {
        t.Fatalf("expected 1 LSP, got %d", len(filtered.LSPs))
    }
    if filtered.LSPs[0].Name != "gopls" {
        t.Fatalf("expected gopls, got %s", filtered.LSPs[0].Name)
    }
}

func TestFilterByLSP_NotFound(t *testing.T) {
    cfg := &Config{
        LSPs: []LSP{
            {Name: "gopls", Flake: "nixpkgs#gopls"},
        },
    }

    _, err := cfg.FilterByLSP("nonexistent")
    if err == nil {
        t.Fatal("expected error for nonexistent LSP")
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`cd packages/lux && nix develop --command go test -run TestFilterByLSP ./internal/config/`
Expected: FAIL --- `FilterByLSP` not defined.

**Step 3: Implement FilterByLSP**

In `config.go`:

``` go
func (c *Config) FilterByLSP(name string) (*Config, error) {
    for _, l := range c.LSPs {
        if l.Name == name {
            return &Config{LSPs: []LSP{l}}, nil
        }
    }
    return nil, fmt.Errorf("LSP %q not found in config", name)
}
```

**Step 4: Run test to verify it passes**

Run:
`cd packages/lux && nix develop --command go test -run TestFilterByLSP ./internal/config/`
Expected: PASS

**Step 5: Commit**

Message: `feat(lux): add Config.FilterByLSP for single-language mode`

--------------------------------------------------------------------------------

### Task 3: Phase 1 capability constraint (formatting only)

The existing server advertises all capabilities from backend LSPs. For Phase 1
of `lux lsp`, we want to constrain to formatting only --- but only in LSP mode,
not MCP mode. This means the Server needs to know which mode it's running in.

**Files:** - Modify: `packages/lux/internal/server/server.go` - Modify:
`packages/lux/internal/server/handler.go`

**Step 1: Add LSP-mode option to Server**

In `server.go`, add an option to `New`:

``` go
type Option func(*Server)

func WithLSPMode() Option {
    return func(s *Server) {
        s.lspMode = true
    }
}
```

Add `lspMode bool` field to the `Server` struct.

Update `New` to accept options:

``` go
func New(cfg *config.Config, opts ...Option) (*Server, error) {
    // ... existing code ...
    for _, opt := range opts {
        opt(s)
    }
    // ...
}
```

**Step 2: Constrain capabilities in LSP mode**

In `handler.go`, modify `aggregateCapabilities` (or the `handleInitialize`
method) to return formatting-only capabilities when `lspMode` is true:

``` go
func (s *Server) aggregateCapabilities() lsp.ServerCapabilities {
    if s.lspMode {
        return lsp.ServerCapabilities{
            TextDocumentSync:                1,
            DocumentFormattingProvider:      true,
            DocumentRangeFormattingProvider: true,
        }
    }
    // ... existing capability aggregation ...
}
```

**Step 3: Reject non-formatting requests in LSP mode**

In `handler.go`, in `handleDefault`, add an early check:

``` go
if h.server.lspMode {
    allowed := map[string]bool{
        lsp.MethodTextDocumentDidOpen:           true,
        lsp.MethodTextDocumentDidClose:          true,
        lsp.MethodTextDocumentDidChange:         true,
        lsp.MethodTextDocumentFormatting:         true,
        lsp.MethodTextDocumentRangeFormatting:    true,
        lsp.MethodWorkspaceDidChangeFolders:      true,
    }
    if !allowed[msg.Method] && !strings.HasPrefix(msg.Method, "$/") {
        if msg.IsRequest() {
            return jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound,
                fmt.Sprintf("method %s not supported in LSP mode (Phase 1: formatting only)", msg.Method), nil)
        }
        return nil, nil
    }
}
```

**Step 4: Update `runLSP` to pass the option**

In `lsp.go`:

``` go
srv, err := server.New(cfg, server.WithLSPMode())
```

**Step 5: Build and verify**

Run: `cd packages/lux && nix develop --command go build ./cmd/lux/` Expected:
builds without errors.

**Step 6: Commit**

Message:
`feat(lux): constrain lsp mode to formatting-only capabilities (Phase 1)`

--------------------------------------------------------------------------------

### Task 4: Add neovim to test devShell

**Files:** - Modify: `packages/lux/flake.nix` (or the relevant devShell
definition)

**Step 1: Find the devShell definition**

Check `packages/lux/flake.nix` or the top-level `flake.nix` for where the lux
devShell or test dependencies are defined. Add `neovim` (from `pkgs` or
`pkgs-master`) to the devShell's `buildInputs` or `packages`.

**Step 2: Verify neovim is available**

Run: `nix develop .#lux --command nvim --version` Expected: shows neovim
version.

**Step 3: Commit**

Message: `build(lux): add neovim to devShell for LSP integration tests`

--------------------------------------------------------------------------------

### Task 5: Integration test --- format Go file via neovim (multiplexing mode)

**Files:** - Create: `packages/lux/zz-tests_bats/lux_lsp.bats` - Create:
`packages/lux/zz-tests_bats/fixtures/lsp/unformatted.go` - Create:
`packages/lux/zz-tests_bats/fixtures/lsp/expected.go` - Create:
`packages/lux/zz-tests_bats/fixtures/lsp/format_go.lua`

**Step 1: Create test fixture --- unformatted Go file**

`fixtures/lsp/unformatted.go`:

``` go
package   main

import "fmt"

func main(  ) {
fmt.Println(   "hello"  )
}
```

**Step 2: Create test fixture --- expected formatted Go file**

`fixtures/lsp/expected.go`:

``` go
package main

import "fmt"

func main() {
    fmt.Println("hello")
}
```

**Step 3: Create neovim LSP test script**

`fixtures/lsp/format_go.lua`:

``` lua
-- Neovim headless LSP format test
-- Usage: nvim --headless -l format_go.lua <lux_binary> <input_file> <output_file>

local lux_bin = vim.fn.argv(0)
local input_file = vim.fn.argv(1)
local output_file = vim.fn.argv(2)

-- Copy input to output (work on the copy)
local input = io.open(input_file, "r")
local content = input:read("*a")
input:close()
local out = io.open(output_file, "w")
out:write(content)
out:close()

-- Configure LSP
vim.lsp.config("lux", {
  cmd = { lux_bin, "lsp" },
})
vim.lsp.enable("lux")

-- Open the output file
vim.cmd("edit " .. output_file)

-- Wait for LSP to attach, then format
vim.defer_fn(function()
  local clients = vim.lsp.get_clients({ name = "lux" })
  if #clients == 0 then
    io.stderr:write("ERROR: no lux LSP client attached\n")
    vim.cmd("cquit 1")
    return
  end

  vim.lsp.buf.format({
    timeout_ms = 30000,
    async = false,
  })

  vim.cmd("write")
  vim.cmd("quit")
end, 5000)  -- 5s for LSP startup + nix build
```

**Step 4: Write the BATS test**

`lux_lsp.bats`:

``` bash
#!/usr/bin/env bats

setup() {
  export test_dir="${BATS_TEST_TMPDIR}"
  export fixtures_dir="${BATS_TEST_DIRNAME}/fixtures/lsp"

  # Build lux if not already built
  if [[ -z "${LUX_BIN:-}" ]]; then
    export LUX_BIN
    LUX_BIN="$(nix build .#lux --no-link --print-out-paths)/bin/lux"
  fi

  # Set up minimal lux config pointing to gopls
  export XDG_CONFIG_HOME="${test_dir}/config"
  mkdir -p "${XDG_CONFIG_HOME}/lux/filetype"

  cat > "${XDG_CONFIG_HOME}/lux/lsps.toml" << 'TOML'
[[lsp]]
name = "gopls"
flake = "nixpkgs#gopls"
TOML

  cat > "${XDG_CONFIG_HOME}/lux/formatters.toml" << 'TOML'
TOML

  cat > "${XDG_CONFIG_HOME}/lux/filetype/go.toml" << 'TOML'
extensions = ["go"]
language_ids = ["go"]
lsp = "gopls"
TOML
}

@test "lux lsp: format Go file via neovim" {
  local output_file="${test_dir}/test.go"

  nvim --headless -l "${fixtures_dir}/format_go.lua" \
    "${LUX_BIN}" "${fixtures_dir}/unformatted.go" "${output_file}"

  diff "${fixtures_dir}/expected.go" "${output_file}"
}

@test "lux lsp --lang gopls: format Go file via neovim" {
  local output_file="${test_dir}/test_lang.go"
  local lua_script="${test_dir}/format_lang.lua"

  # Create a variant script that uses --lang
  sed "s|lux_bin, \"lsp\"|lux_bin, \"lsp\", \"--lang\", \"gopls\"|" \
    "${fixtures_dir}/format_go.lua" > "${lua_script}"

  nvim --headless -l "${lua_script}" \
    "${LUX_BIN}" "${fixtures_dir}/unformatted.go" "${output_file}"

  diff "${fixtures_dir}/expected.go" "${output_file}"
}
```

**Step 5: Run the test**

Run:
`cd packages/lux && nix develop --command bats --tap zz-tests_bats/lux_lsp.bats`
Expected: both tests pass (format via multiplexing and single-lang mode).

If tests fail, debug by: 1. Running lux lsp manually:
`echo '...' | ./result/bin/lux lsp` 2. Checking neovim LSP logs:
`nvim --headless -c "lua vim.lsp.set_log_level('debug')" ...`

**Step 6: Commit**

Message: `test(lux): add neovim integration tests for lux lsp formatting`

--------------------------------------------------------------------------------

### Task 6: Integration test --- format Markdown file via neovim

**Files:** - Create: `packages/lux/zz-tests_bats/fixtures/lsp/unformatted.md` -
Create: `packages/lux/zz-tests_bats/fixtures/lsp/expected.md` - Modify:
`packages/lux/zz-tests_bats/lux_lsp.bats`

**Step 1: Create markdown test fixtures**

`fixtures/lsp/unformatted.md`:

``` markdown
---
title: Test
---
This is a paragraph with   extra   spaces that pandoc will normalize.

*  item one
*  item two
```

`fixtures/lsp/expected.md` --- generate by running pandoc on the unformatted
file with the expected flags: `pandoc -f markdown -t markdown --standalone`

**Step 2: Add formatter config to setup**

In `lux_lsp.bats` setup, add pandoc formatter and markdown filetype:

``` bash
cat >> "${XDG_CONFIG_HOME}/lux/formatters.toml" << 'TOML'
[[formatter]]
name = "pandoc"
flake = "nixpkgs#pandoc"
args = ["-f", "markdown", "-t", "markdown", "--standalone"]
mode = "stdin"
TOML

cat > "${XDG_CONFIG_HOME}/lux/filetype/markdown.toml" << 'TOML'
extensions = ["md"]
language_ids = ["markdown"]
formatters = ["pandoc"]
formatter_mode = "chain"
TOML
```

**Step 3: Create neovim script for markdown formatting**

Adapt the lua script or make a generic version that takes filetype as an
argument.

**Step 4: Add BATS test**

``` bash
@test "lux lsp: format Markdown file via neovim" {
  local output_file="${test_dir}/test.md"

  nvim --headless -l "${fixtures_dir}/format_md.lua" \
    "${LUX_BIN}" "${fixtures_dir}/unformatted.md" "${output_file}"

  diff "${fixtures_dir}/expected.md" "${output_file}"
}
```

**Step 5: Add multiplexing test --- Go then Markdown in same session**

``` bash
@test "lux lsp: format Go then Markdown in same session" {
  # This test validates multiplexing: a single lux lsp process handles
  # both filetypes, routing to gopls for .go and pandoc for .md.
  local go_output="${test_dir}/multi_test.go"
  local md_output="${test_dir}/multi_test.md"

  nvim --headless -l "${fixtures_dir}/format_multi.lua" \
    "${LUX_BIN}" \
    "${fixtures_dir}/unformatted.go" "${go_output}" \
    "${fixtures_dir}/unformatted.md" "${md_output}"

  diff "${fixtures_dir}/expected.go" "${go_output}"
  diff "${fixtures_dir}/expected.md" "${md_output}"
}
```

**Step 6: Run tests**

Run:
`cd packages/lux && nix develop --command bats --tap zz-tests_bats/lux_lsp.bats`
Expected: all tests pass.

**Step 7: Commit**

Message:
`test(lux): add markdown and multiplexing integration tests for lux lsp`

--------------------------------------------------------------------------------

### Task 7: Integration test --- graceful error when backend unavailable

**Files:** - Modify: `packages/lux/zz-tests_bats/lux_lsp.bats`

**Step 1: Add test for missing backend**

``` bash
@test "lux lsp: returns error when backend LSP unavailable" {
  # Configure a filetype with no LSP and no formatter
  cat > "${XDG_CONFIG_HOME}/lux/filetype/txt.toml" << 'TOML'
extensions = ["txt"]
TOML

  local output_file="${test_dir}/test.txt"
  echo "hello" > "${output_file}"

  # Neovim should get an error response from lux when trying to format
  # The test passes if neovim exits without hanging and the file is unchanged
  nvim --headless -l "${fixtures_dir}/format_expect_error.lua" \
    "${LUX_BIN}" "${output_file}"

  [[ "$(cat "${output_file}")" == "hello" ]]
}
```

**Step 2: Create the error-expectation lua script**

`fixtures/lsp/format_expect_error.lua` --- attempts format, expects it to fail
gracefully (no hang, no crash).

**Step 3: Run test**

Run:
`cd packages/lux && nix develop --command bats --tap zz-tests_bats/lux_lsp.bats`
Expected: PASS --- lux returns MethodNotFound or "no LSP configured" error.

**Step 4: Commit**

Message: `test(lux): add error handling integration test for lux lsp`

--------------------------------------------------------------------------------

### Task 8: Update CLAUDE.md and FDR status

**Files:** - Modify: `packages/lux/CLAUDE.md` - Modify:
`docs/features/0001-lsp-editor-integration.md`

**Step 1: Add lsp subcommand to CLAUDE.md**

In the "Key Packages" table, update the `cmd/lux` row to include `lsp`. Add a
new section documenting `lux lsp` usage.

**Step 2: Update FDR status**

Change status from `exploring` to `experimental` (working implementation
exists).

**Step 3: Commit**

Message: `docs(lux): update CLAUDE.md and FDR for lsp subcommand`
