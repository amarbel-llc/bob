# Lux Gopls Extensions Design

## Problem

Lux proxies standard LSP protocol methods but cannot surface non-standard
extensions. Gopls exposes 37 commands via `workspace/executeCommand` that
provide capabilities unavailable through standard LSP --- package metadata,
package-scoped symbols, import lists, and module info.

## Decision

Add a generic `execute-command` MCP tool and four curated gopls-specific MCP
resources. The tool provides an escape hatch for any `workspace/executeCommand`
call; the resources give clean interfaces for the highest-value gopls queries.

## Components

### 1. `execute-command` tool

Generic passthrough for `workspace/executeCommand` on any configured LSP.

  -------------------------------------------------------------------------------
  Parameter             Type        Required            Description
  --------------------- ----------- ------------------- -------------------------
  `lsp`                 string      yes                 LSP name (validated
                                                        against configured LSPs)

  `command`             string      yes                 Command ID (validated
                                                        against LSP's advertised
                                                        commands)

  `arguments`           JSON        no                  Command arguments
  -------------------------------------------------------------------------------

Returns raw JSON from the LSP (`json.RawMessage`). No response transformation.

### 2. `lux://commands` resource

Lists available `workspace/executeCommand` commands grouped by LSP name, sourced
from each LSP's `ExecuteCommandProvider.commands` capability advertised during
`initialize`. Same data also included in `lux://status`.

### 3. Curated gopls resources

  ----------------------------------------------------------------------------------------------------
  Resource                                     Params              Gopls command
  -------------------------------------------- ------------------- -----------------------------------
  `lux://lsp/packages?uri={file_uri}`          `uri` (required),   `gopls.packages`
                                               `recursive`         
                                               (optional, default  
                                               true)               

  `lux://lsp/package-symbols?uri={file_uri}`   `uri` (required)    `gopls.package_symbols`

  `lux://lsp/imports?uri={file_uri}`           `uri` (required)    `gopls.list_imports`

  `lux://lsp/modules?uri={dir_uri}`            `uri` (required),   `gopls.modules`
                                               `max_depth`         
                                               (optional)          
  ----------------------------------------------------------------------------------------------------

Hardcoded to route to gopls. Return typed Go structs as JSON following the
`*Raw` pattern. `format=text` supported for human-readable output. Error if no
gopls configured.

## Bridge layer

New `ExecuteCommand(ctx, lspName, command, arguments)` method on Bridge. Unlike
existing bridge methods that route by file URI, this takes an explicit LSP name
and calls `Pool.GetOrStart(lspName)` directly. Sends `workspace/executeCommand`
with the given command and arguments.

The four gopls resources call `ExecuteCommand` internally, transforming
arguments into gopls's expected shapes and responses into lux's typed structs.

## Validation

- `execute-command` tool validates `lsp` against configured LSP names
- `execute-command` tool validates `command` against the LSP's advertised
  `ExecuteCommandProvider.commands`
- Gopls resources error if no gopls is configured in the workspace

## Rollback

Purely additive --- new tool, new resources. Nothing existing changes. Remove by
reverting the commits.
