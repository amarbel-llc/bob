# Lux Gopls Extensions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Surface gopls's `workspace/executeCommand` capabilities through a
generic MCP tool and four curated gopls-specific resources.

**Architecture:** New `ExecuteCommand` bridge method routes by explicit LSP name
(bypassing file-type routing). A generic `execute-command` MCP tool wraps it.
Four curated resources (`packages`, `package-symbols`, `imports`, `modules`)
call it internally with gopls-specific argument shaping and typed response
structs. A `lux://commands` resource exposes advertised commands per LSP.

**Tech Stack:** Go, go-mcp (`command.Command`, `mcpserver.ResourceRegistry`),
LSP JSON-RPC

**Rollback:** Purely additive. Revert commits to remove.

--------------------------------------------------------------------------------

### Task 1: Bridge `ExecuteCommand` method

**Files:** - Modify: `packages/lux/internal/tools/bridge.go` - Test:
`packages/lux/internal/tools/bridge_test.go`

**Step 1: Add `ExecuteCommand` method signature to bridge_test.go**

Add a compile-time signature check alongside the existing ones:

``` go
_ = func() (json.RawMessage, error) { return b.ExecuteCommand(ctx, "gopls", "gopls.packages", json.RawMessage("{}")) }
```

Add this line after line 141 in `bridge_test.go` (after the `OutgoingCallsRaw`
check).

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestBridgeRawMethodSignatures ./packages/lux/internal/tools/...`
Expected: FAIL --- `b.ExecuteCommand` undefined

**Step 3: Implement `ExecuteCommand` on Bridge**

Add to `bridge.go` after `OutgoingCallsRaw`:

``` go
// ExecuteCommand calls workspace/executeCommand on a specific LSP by name.
// Unlike other bridge methods, this routes by explicit LSP name rather than file URI.
func (b *Bridge) ExecuteCommand(ctx context.Context, lspName, command string, arguments json.RawMessage) (json.RawMessage, error) {
    inst, ok := b.pool.Get(lspName)
    if !ok {
        initParams := b.DefaultInitParamsForCwd()
        var err error
        inst, err = b.pool.GetOrStart(ctx, lspName, initParams)
        if err != nil {
            return nil, fmt.Errorf("starting LSP %s: %w", lspName, err)
        }
    }

    if err := b.waitForLSPReady(ctx, inst); err != nil {
        return nil, fmt.Errorf("waiting for LSP %s readiness: %w", lspName, err)
    }

    var args []json.RawMessage
    if len(arguments) > 0 && string(arguments) != "null" {
        args = []json.RawMessage{arguments}
    }

    params := map[string]any{
        "command":   command,
        "arguments": args,
    }

    return inst.Call(ctx, lsp.MethodWorkspaceExecuteCommand, params)
}
```

**Step 4: Check if `DefaultInitParamsForCwd` exists, if not add it**

Look at existing `DefaultInitParams(uri)` and add a cwd-based variant if needed.
It likely takes a URI to infer workspace root --- for the cwd case, use
`os.Getwd()`.

``` go
func (b *Bridge) DefaultInitParamsForCwd() *lsp.InitializeParams {
    cwd, _ := os.Getwd()
    return b.DefaultInitParams(lsp.DocumentURI("file://" + cwd + "/dummy.go"))
}
```

**Step 5: Run test to verify it passes**

Run:
`nix develop --command go test -run TestBridgeRawMethodSignatures ./packages/lux/internal/tools/...`
Expected: PASS

**Step 6: Commit**

    feat(lux): add ExecuteCommand bridge method

    Routes workspace/executeCommand by explicit LSP name rather than file
    URI. Foundation for generic execute-command tool and gopls resources.

--------------------------------------------------------------------------------

### Task 2: `lux://commands` resource

**Files:** - Modify: `packages/lux/internal/mcp/resources.go` - Modify:
`packages/lux/internal/subprocess/pool.go`

**Step 1: Add `Commands` method to Pool**

Add to `pool.go` after the `Status()` method:

``` go
// Commands returns advertised executeCommand commands grouped by LSP name.
func (p *Pool) Commands() map[string][]string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    result := make(map[string][]string)
    for name, inst := range p.instances {
        if inst.mu.TryRLock() {
            if inst.Capabilities != nil && inst.Capabilities.ExecuteCommandProvider != nil {
                result[name] = inst.Capabilities.ExecuteCommandProvider.Commands
            }
            inst.mu.RUnlock()
        }
    }
    return result
}
```

**Step 2: Register `lux://commands` static resource**

Add to `registerResources()` in `resources.go`, after the `lux://languages`
registration:

``` go
registry.RegisterResource(
    protocol.Resource{
        URI:         "lux://commands",
        Name:        "LSP Commands",
        Description: "Available workspace/executeCommand commands grouped by LSP name",
        MimeType:    "application/json",
    },
    func(ctx context.Context, uri string) (*protocol.ResourceReadResult, error) {
        return readCommands(pool)
    },
)
```

Add the `readCommands` function:

``` go
func readCommands(pool *subprocess.Pool) (*protocol.ResourceReadResult, error) {
    commands := pool.Commands()

    data, err := json.MarshalIndent(commands, "", "  ")
    if err != nil {
        return nil, err
    }

    return &protocol.ResourceReadResult{
        Contents: []protocol.ResourceContent{
            {
                URI:      "lux://commands",
                MimeType: "application/json",
                Text:     string(data),
            },
        },
    }, nil
}
```

**Step 3: Add commands to `lux://status` response**

Extend `statusResponse` struct:

``` go
type statusResponse struct {
    ConfiguredLSPs      []lspStatus        `json:"configured_lsps"`
    SupportedExtensions []string           `json:"supported_extensions"`
    SupportedLanguages  []string           `json:"supported_languages"`
    Commands            map[string][]string `json:"commands,omitempty"`
}
```

In `readStatus`, add before marshalling:

``` go
resp.Commands = pool.Commands()
```

Update `readStatus` signature to accept `pool`:

``` go
func readStatus(pool *subprocess.Pool, cfg *config.Config, ftConfigs []*filetype.Config) (*protocol.ResourceReadResult, error) {
```

(It already accepts pool --- just add the `resp.Commands = pool.Commands()`
line.)

**Step 4: Verify build**

Run: `nix develop --command go build ./packages/lux/...` Expected: builds
cleanly

**Step 5: Commit**

    feat(lux): add lux://commands resource and commands in status

    Exposes advertised workspace/executeCommand commands grouped by LSP
    name. Available as standalone resource and in lux://status response.

--------------------------------------------------------------------------------

### Task 3: `execute-command` MCP tool

**Files:** - Modify: `packages/lux/internal/mcp/server.go` - Modify:
`packages/lux/internal/mcp/server_test.go`

**Step 1: Write test for tool registration**

Update the existing `TestMCPToolsList` in `server_test.go` to expect 3 tools:

``` go
if len(result.Tools) != 3 {
    t.Errorf("expected 3 tools (resource-templates, resource-read, execute-command), got %d", len(result.Tools))
}
```

Add a check for the new tool name:

``` go
if !toolNames["execute-command"] {
    t.Error("expected execute-command tool to be registered")
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestMCPToolsList ./packages/lux/internal/mcp/...`
Expected: FAIL --- expected 3 tools, got 2

**Step 3: Register `execute-command` tool**

Add to `server.go` after the `resource-read` command registration, before
`toolRegistry`:

``` go
notReadOnly := false
mcpApp.AddCommand(&command.Command{
    Name: "execute-command",
    Description: command.Description{
        Short: "Execute a workspace/executeCommand on a specific LSP server. Use lux://commands to discover available commands.",
    },
    Annotations: &protocol.ToolAnnotations{
        ReadOnlyHint:    &notReadOnly,
        DestructiveHint: &notDestructive,
        IdempotentHint:  &idempotent,
    },
    Params: []command.Param{
        {Name: "lsp", Type: command.String, Description: "LSP server name (e.g. 'gopls'). See lux://status for configured LSPs.", Required: true},
        {Name: "command", Type: command.String, Description: "Command ID to execute (e.g. 'gopls.packages'). See lux://commands for available commands.", Required: true},
        {Name: "arguments", Type: command.String, Description: "JSON-encoded command arguments (optional)", Required: false},
    },
    Run: func(ctx context.Context, args json.RawMessage, _ command.Prompter) (*command.Result, error) {
        var a struct {
            LSP       string `json:"lsp"`
            Command   string `json:"command"`
            Arguments string `json:"arguments"`
        }
        if err := json.Unmarshal(args, &a); err != nil {
            return command.TextErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
        }

        // Validate LSP name against config
        found := false
        for _, l := range cfg.LSPs {
            if l.Name == a.LSP {
                found = true
                break
            }
        }
        if !found {
            return command.TextErrorResult(fmt.Sprintf("unknown LSP %q — check lux://status for configured LSPs", a.LSP)), nil
        }

        // Validate command against advertised commands
        commands := pool.Commands()
        if cmds, ok := commands[a.LSP]; ok {
            cmdFound := false
            for _, c := range cmds {
                if c == a.Command {
                    cmdFound = true
                    break
                }
            }
            if !cmdFound {
                return command.TextErrorResult(fmt.Sprintf("LSP %q does not advertise command %q — check lux://commands", a.LSP, a.Command)), nil
            }
        }
        // If LSP hasn't started yet (no commands advertised), allow the call through

        var arguments json.RawMessage
        if a.Arguments != "" {
            arguments = json.RawMessage(a.Arguments)
        }

        result, err := bridge.ExecuteCommand(ctx, a.LSP, a.Command, arguments)
        if err != nil {
            return command.TextErrorResult(err.Error()), nil
        }

        // Pretty-print the JSON result
        var pretty bytes.Buffer
        if err := json.Indent(&pretty, result, "", "  "); err != nil {
            return command.TextResult(string(result)), nil
        }
        return command.TextResult(pretty.String()), nil
    },
})
```

Note: `server.go` needs access to `bridge`, `pool`, and `cfg`. Check the
existing `NewMCPServer` or equivalent function signature --- `resProvider`
already has these, so either pass them directly or access via the provider.

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestMCPToolsList ./packages/lux/internal/mcp/...`
Expected: PASS

**Step 5: Commit**

    feat(lux): add execute-command MCP tool

    Generic workspace/executeCommand passthrough. Validates LSP name
    against config and command against advertised commands. Returns raw
    JSON from the LSP.

--------------------------------------------------------------------------------

### Task 4: Gopls result types

**Files:** - Modify: `packages/lux/internal/tools/bridge.go`

**Step 1: Define gopls response structs**

Add after the existing result types at the bottom of `bridge.go`:

``` go
// Gopls executeCommand result types

type PackagesResult struct {
    Packages []PackageInfo          `json:"packages"`
    Module   map[string]ModuleInfo  `json:"module,omitempty"`
}

type PackageInfo struct {
    Path       string     `json:"path"`
    ModulePath string     `json:"modulePath,omitempty"`
    ForTest    string     `json:"forTest,omitempty"`
    TestFiles  []TestFile `json:"testFiles,omitempty"`
}

type TestFile struct {
    URI   string     `json:"uri"`
    Tests []TestInfo `json:"tests,omitempty"`
}

type TestInfo struct {
    Name string        `json:"name"`
    Loc  *lsp.Location `json:"loc,omitempty"`
}

type ModuleInfo struct {
    Path    string `json:"path"`
    Version string `json:"version,omitempty"`
    GoMod   string `json:"goMod,omitempty"`
}

type PackageSymbolsResult struct {
    PackageName string              `json:"packageName"`
    Files       []string            `json:"files"`
    Symbols     []PackageSymbolItem `json:"symbols"`
}

type PackageSymbolItem struct {
    Name           string              `json:"name"`
    Kind           int                 `json:"kind"`
    Range          lsp.Range           `json:"range,omitempty"`
    SelectionRange lsp.Range           `json:"selectionRange,omitempty"`
    Children       []PackageSymbolItem `json:"children,omitempty"`
    File           int                 `json:"file"`
}

type ImportsResult struct {
    Imports        []ImportInfo        `json:"imports"`
    PackageImports []PackageImportInfo `json:"packageImports"`
}

type ImportInfo struct {
    Path string `json:"path"`
    Name string `json:"name,omitempty"`
}

type PackageImportInfo struct {
    Path string `json:"path"`
}

type ModulesResult struct {
    Modules []ModuleInfo `json:"modules"`
}
```

**Step 2: Verify build**

Run: `nix develop --command go build ./packages/lux/...` Expected: builds
cleanly

**Step 3: Commit**

    feat(lux): add typed result structs for gopls commands

    PackagesResult, PackageSymbolsResult, ImportsResult, ModulesResult
    structs for stable response shapes from gopls adapter resources.

--------------------------------------------------------------------------------

### Task 5: Gopls adapter bridge methods

**Files:** - Modify: `packages/lux/internal/tools/bridge.go` - Test:
`packages/lux/internal/tools/bridge_test.go`

**Step 1: Add compile-time signature checks**

Add to `bridge_test.go` after the `ExecuteCommand` check:

``` go
_ = func() (*PackagesResult, error) { return b.GoplsPackages(ctx, uri, true) }
_ = func() (*PackageSymbolsResult, error) { return b.GoplsPackageSymbols(ctx, uri) }
_ = func() (*ImportsResult, error) { return b.GoplsImports(ctx, uri) }
_ = func() (*ModulesResult, error) { return b.GoplsModules(ctx, uri, 0) }
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestBridgeRawMethodSignatures ./packages/lux/internal/tools/...`
Expected: FAIL --- methods undefined

**Step 3: Implement gopls adapter methods**

Add to `bridge.go`:

``` go
const goplsLSPName = "gopls"

func (b *Bridge) GoplsPackages(ctx context.Context, uri lsp.DocumentURI, recursive bool) (*PackagesResult, error) {
    args, _ := json.Marshal(map[string]any{
        "Files":     []string{string(uri)},
        "Recursive": recursive,
        "Mode":      1,
    })
    result, err := b.ExecuteCommand(ctx, goplsLSPName, "gopls.packages", args)
    if err != nil {
        return nil, err
    }
    var out PackagesResult
    if err := json.Unmarshal(result, &out); err != nil {
        return nil, fmt.Errorf("parsing gopls.packages result: %w", err)
    }
    return &out, nil
}

func (b *Bridge) GoplsPackageSymbols(ctx context.Context, uri lsp.DocumentURI) (*PackageSymbolsResult, error) {
    args, _ := json.Marshal(map[string]any{
        "URI": string(uri),
    })
    result, err := b.ExecuteCommand(ctx, goplsLSPName, "gopls.package_symbols", args)
    if err != nil {
        return nil, err
    }
    var out PackageSymbolsResult
    if err := json.Unmarshal(result, &out); err != nil {
        return nil, fmt.Errorf("parsing gopls.package_symbols result: %w", err)
    }
    return &out, nil
}

func (b *Bridge) GoplsImports(ctx context.Context, uri lsp.DocumentURI) (*ImportsResult, error) {
    args, _ := json.Marshal(map[string]any{
        "URI": string(uri),
    })
    result, err := b.ExecuteCommand(ctx, goplsLSPName, "gopls.list_imports", args)
    if err != nil {
        return nil, err
    }
    var out ImportsResult
    if err := json.Unmarshal(result, &out); err != nil {
        return nil, fmt.Errorf("parsing gopls.list_imports result: %w", err)
    }
    return &out, nil
}

func (b *Bridge) GoplsModules(ctx context.Context, uri lsp.DocumentURI, maxDepth int) (*ModulesResult, error) {
    args := map[string]any{
        "Dir": string(uri),
    }
    if maxDepth > 0 {
        args["MaxDepth"] = maxDepth
    }
    argsJSON, _ := json.Marshal(args)
    result, err := b.ExecuteCommand(ctx, goplsLSPName, "gopls.modules", argsJSON)
    if err != nil {
        return nil, err
    }
    var out ModulesResult
    if err := json.Unmarshal(result, &out); err != nil {
        return nil, fmt.Errorf("parsing gopls.modules result: %w", err)
    }
    return &out, nil
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestBridgeRawMethodSignatures ./packages/lux/internal/tools/...`
Expected: PASS

**Step 5: Commit**

    feat(lux): add gopls adapter bridge methods

    GoplsPackages, GoplsPackageSymbols, GoplsImports, GoplsModules wrap
    ExecuteCommand with typed argument construction and response parsing.

--------------------------------------------------------------------------------

### Task 6: Gopls MCP resources

**Files:** - Modify: `packages/lux/internal/mcp/resources.go`

**Step 1: Register resource templates**

Add to the `lspTemplates` slice in `registerResources()`:

``` go
{
    URITemplate: "lux://lsp/packages?uri={uri}",
    Name:        "Go Packages",
    Description: "Package metadata for a Go file (requires gopls). Returns package path, module info, and test files.",
    MimeType:    "application/json",
},
{
    URITemplate: "lux://lsp/package-symbols?uri={uri}",
    Name:        "Go Package Symbols",
    Description: "All symbols in a Go file's package with hierarchy (requires gopls). Richer than workspace-symbols.",
    MimeType:    "application/json",
},
{
    URITemplate: "lux://lsp/imports?uri={uri}",
    Name:        "Go Imports",
    Description: "Imports in a Go file and across its package (requires gopls).",
    MimeType:    "application/json",
},
{
    URITemplate: "lux://lsp/modules?uri={uri}",
    Name:        "Go Modules",
    Description: "Module information within a directory (requires gopls). Pass a directory file:// URI.",
    MimeType:    "application/json",
},
```

**Step 2: Add switch cases in `readLSPResource`**

Add before the `default:` case:

``` go
case "packages":
    fileURI, err := getFileURI()
    if err != nil {
        return nil, err
    }
    recursive := q.Get("recursive") != "false"
    raw, err := p.bridge.GoplsPackages(ctx, fileURI, recursive)
    if err != nil {
        return nil, err
    }
    data, err := json.MarshalIndent(raw, "", "  ")
    if err != nil {
        return nil, err
    }
    text = string(data)
    mimeType = "application/json"

case "package-symbols":
    fileURI, err := getFileURI()
    if err != nil {
        return nil, err
    }
    raw, err := p.bridge.GoplsPackageSymbols(ctx, fileURI)
    if err != nil {
        return nil, err
    }
    data, err := json.MarshalIndent(raw, "", "  ")
    if err != nil {
        return nil, err
    }
    text = string(data)
    mimeType = "application/json"

case "imports":
    fileURI, err := getFileURI()
    if err != nil {
        return nil, err
    }
    raw, err := p.bridge.GoplsImports(ctx, fileURI)
    if err != nil {
        return nil, err
    }
    data, err := json.MarshalIndent(raw, "", "  ")
    if err != nil {
        return nil, err
    }
    text = string(data)
    mimeType = "application/json"

case "modules":
    fileURI, err := getFileURI()
    if err != nil {
        return nil, err
    }
    maxDepth := 0
    if v := q.Get("max_depth"); v != "" {
        maxDepth, _ = strconv.Atoi(v)
    }
    raw, err := p.bridge.GoplsModules(ctx, fileURI, maxDepth)
    if err != nil {
        return nil, err
    }
    data, err := json.MarshalIndent(raw, "", "  ")
    if err != nil {
        return nil, err
    }
    text = string(data)
    mimeType = "application/json"
```

**Step 3: Verify build**

Run: `nix develop --command go build ./packages/lux/...` Expected: builds
cleanly

**Step 4: Commit**

    feat(lux): add gopls adapter MCP resources

    Four curated resources: packages, package-symbols, imports, modules.
    Each wraps gopls executeCommand with clean URIs and typed responses.

--------------------------------------------------------------------------------

### Task 7: Update CLAUDE.md and lux documentation

**Files:** - Modify: `packages/lux/CLAUDE.md`

**Step 1: Add new resources and tool to CLAUDE.md**

Add to the resource URI patterns list:

``` markdown
- `lux://lsp/packages?uri={file_uri}&recursive={bool}` (Go package metadata, requires gopls)
- `lux://lsp/package-symbols?uri={file_uri}` (all symbols in a Go package, requires gopls)
- `lux://lsp/imports?uri={file_uri}` (file and package imports, requires gopls)
- `lux://lsp/modules?uri={dir_uri}&max_depth={n}` (module info in directory, requires gopls)
- `lux://commands` (available workspace/executeCommand commands per LSP)
```

Add a new section for the tool:

``` markdown
### MCP Tools

In addition to `resource-templates` and `resource-read`, lux exposes:

- `execute-command` — generic `workspace/executeCommand` passthrough. Params: `lsp` (LSP name), `command` (command ID), `arguments` (JSON string, optional). Use `lux://commands` to discover available commands.
```

**Step 2: Commit**

    docs(lux): document gopls resources and execute-command tool

--------------------------------------------------------------------------------

### Task 8: Integration test with real gopls

**Files:** - None (manual verification)

**Step 1: Build lux**

Run: `nix build .#lux`

**Step 2: Test `lux://commands` resource**

Start lux, read `lux://commands` after gopls initializes. Verify `gopls` key
exists with command list including `gopls.packages`.

**Step 3: Test `execute-command` tool**

Call `execute-command` with `lsp=gopls`, `command=gopls.list_imports`,
`arguments={"URI":"file:///path/to/some.go"}`. Verify JSON response with
imports.

**Step 4: Test curated resources**

Read `lux://lsp/packages?uri=file:///path/to/some.go`. Verify structured
response with package path and module info.

Read `lux://lsp/imports?uri=file:///path/to/some.go`. Verify imports list.

**Step 5: Verify `lux://status` includes commands**

Read `lux://status`. Verify `commands` field is present with gopls commands.

**Step 6: Note verification in commit message if any fixes are needed**

    test(lux): verify gopls extensions against real gopls

    Tested: execute-command tool, packages/imports/modules/package-symbols
    resources, lux://commands discovery, status commands field.
