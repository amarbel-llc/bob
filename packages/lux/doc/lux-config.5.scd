lux-config(5)

# NAME

lux-config - configuration files for the lux LSP multiplexer

# DESCRIPTION

Lux reads its configuration from TOML files at both global and project
levels. Global configuration lives in _~/.config/lux/_ and project-level
configuration lives in _.lux/_ at the project root.

# FILES

_~/.config/lux/lsps.toml_
	Global LSP server configuration.

_~/.config/lux/formatters.toml_
	Global formatter configuration.

_~/.config/lux/filetype/\*.toml_
	Global filetype configuration. Each file defines how a language is
	handled: which LSP, formatters, and file matching rules to use.

_.lux/lsps.toml_
	Project-level LSP server configuration. Merged with and overrides
	global configuration.

_.lux/formatters.toml_
	Project-level formatter configuration. Merged with and overrides
	global configuration.

_.lux/filetype/\*.toml_
	Project-level filetype configuration. Overrides global filetypes
	with matching names.

_~/.config/lux/fmt-all.toml_
	Global fmt-all configuration. Controls file discovery and exclusion
	for *lux fmt-all*.

_.lux/fmt-all.toml_
	Project-level fmt-all configuration. Merged with global
	configuration.

# LSP CONFIGURATION

The LSP configuration file (_lsps.toml_) defines language servers that lux
will manage and multiplex.

## Top-level fields

*socket* = _path_
	Unix domain socket path for the lux control server. Project-level
	socket overrides global.

## Per-LSP fields

Each language server is defined in a *[[lsp]]* array entry.

*name* = _string_ (required)
	Unique name for this language server.

*flake* = _string_ (required)
	Nix flake reference providing the language server binary.

*binary* = _string_
	Custom binary name or path within the flake output. Useful when the
	flake provides multiple binaries.

*extensions* = [_string_, ...]
	File extensions this server handles (e.g., ["go", "mod"]).

*patterns* = [_string_, ...]
	Glob patterns for matching files (e.g., ["Makefile"]).

*language_ids* = [_string_, ...]
	LSP language identifiers this server supports (e.g., ["go", "gomod"]).

At least one of *extensions*, *patterns*, or *language_ids* is required.

*args* = [_string_, ...]
	Additional arguments passed to the language server binary.

*env* = {_key_ = _value_, ...}
	Environment variables set when launching the language server. Keys must
	match [a-zA-Z\_][a-zA-Z0-9\_]\\.

*init_options* = {_key_ = _value_, ...}
	LSP initialization options sent during the initialize request. Must be
	valid JSON-serializable values.

*capabilities.disable* = [_string_, ...]
	LSP capabilities to disable for this server.

*capabilities.enable* = [_string_, ...]
	LSP capabilities to force-enable for this server.

## Example

```
socket = "/tmp/lux.sock"

[[lsp]]
name = "gopls"
flake = "nixpkgs#gopls"
extensions = ["go", "mod", "sum"]
language_ids = ["go", "gomod", "gosum"]

[[lsp]]
name = "lua-language-server"
flake = "nixpkgs#lua-language-server"
extensions = ["lua"]
language_ids = ["lua"]
args = ["--logpath=/tmp/lua-ls"]
env = { LUA_PATH = "/usr/share/lua/5.4/?.lua" }
```

# FORMATTER CONFIGURATION

The formatter configuration file (_formatters.toml_) defines external
programs used for formatting files.

## Per-formatter fields

Each formatter is defined in a *[[formatter]]* array entry.

*name* = _string_ (required)
	Unique name for this formatter. Referenced by filetype configs.

*flake* = _string_
	Nix flake reference providing the formatter binary. Mutually exclusive
	with *path*.

*path* = _string_
	Direct path to the formatter binary. Mutually exclusive with *flake*.

*binary* = _string_
	Custom binary name within the flake output. Useful when a flake
	provides multiple binaries (e.g., _nixpkgs#gotools_ contains
	_goimports_).

*args* = [_string_, ...]
	Arguments passed to the formatter. Use _{file}_ as a placeholder for
	the file path (e.g., ["-srcdir", "{file}"]).

*env* = {_key_ = _value_, ...}
	Environment variables set when launching the formatter.

*mode* = _"stdin"_ | _"filepath"_
	How the formatter receives input. In *stdin* mode (the default), file
	content is piped to stdin and formatted output is read from stdout. In
	*filepath* mode, the file path is passed as an argument and the
	formatter modifies the file in-place.

*disabled* = _true_ | _false_
	When set to *true* in a project-level config, disables a formatter
	inherited from global config.

Note: formatters do not have their own file matching rules. File-to-formatter
routing is configured in filetype configs (see *FILETYPE CONFIGURATION* below).

## Example

```
[[formatter]]
name = "goimports"
flake = "nixpkgs#gotools"
binary = "goimports"
args = ["-srcdir", "{file}"]

[[formatter]]
name = "gofumpt"
flake = "nixpkgs#gofumpt"

[[formatter]]
name = "shfmt"
flake = "nixpkgs#shfmt"
args = ["-s", "-i=2"]
```

# FILETYPE CONFIGURATION

Filetype configuration files live in _~/.config/lux/filetype/_ (global) and
_.lux/filetype/_ (project-level). Each file is named after the language
(e.g., _go.toml_, _python.toml_) and defines how that language is handled.

## Per-filetype fields

*extensions* = [_string_, ...]
	File extensions this filetype matches (e.g., ["go"]). The leading dot
	is optional.

*patterns* = [_string_, ...]
	Glob patterns for matching files (e.g., ["Makefile"]).

*language_ids* = [_string_, ...]
	LSP language identifiers (e.g., ["go", "gomod"]).

At least one of *extensions*, *patterns*, or *language_ids* is required.

*lsp* = _string_
	Name of the LSP server to use (must match a name in _lsps.toml_).

*formatters* = [_string_, ...]
	Ordered list of formatter names to use (must match names in
	_formatters.toml_). All referenced formatters must be defined or
	lux will error at startup.

*formatter_mode* = _"chain"_ | _"fallback"_
	How multiple formatters are applied. In *chain* mode (the default),
	output of formatter N is piped into formatter N+1. In *fallback*
	mode, formatters are tried in order and the first success wins.

*lsp_format* = _"never"_ | _"fallback"_ | _"prefer"_
	Controls LSP formatting vs external formatters. *never* (default when
	formatters are configured) skips LSP formatting entirely. *fallback*
	uses external formatters first, falls back to LSP on error. *prefer*
	uses LSP formatting if available, falls back to external formatters.
	Defaults to *prefer* when no formatters are configured.

## Example

```
# filetype/go.toml
extensions = ["go"]
language_ids = ["go"]
lsp = "gopls"
formatters = ["goimports", "gofumpt"]
formatter_mode = "chain"

# filetype/python.toml
extensions = ["py"]
language_ids = ["python"]
lsp = "pyright"
formatters = ["isort", "black"]
formatter_mode = "chain"
```

# FMT-ALL CONFIGURATION

The fmt-all configuration file (_fmt-all.toml_) controls how *lux fmt-all*
discovers and filters files.

## Fields

*walk* = _"git"_ | _"all"_
	File discovery strategy. In *git* mode (the default), lux uses
	*git ls-files* to find tracked and untracked (non-ignored) files. In
	*all* mode, lux walks the entire directory tree, skipping _.git/_
	directories and symlinks.

*exclude\_globs* = [_string_, ...]
	Glob patterns for files to skip. Patterns are matched against paths
	relative to the project root using _/_ as the separator.

## Example

```
walk = "git"
exclude_globs = ["vendor/**", "generated/**"]
```

# MERGING BEHAVIOR

Project-level configurations are merged with global configurations.

For _lsps.toml_:
- LSPs with matching names are deeply merged, with project values
  overriding global values.
- Map fields (_env_, _init\_options_) are merged key-by-key.
- LSPs defined only in the project config are added.
- The project _socket_ value overrides the global socket.

For _formatters.toml_:
- Formatters with matching names are overridden entirely by the
  project-level definition.
- Formatters defined only in the project config are added.
- A project-level formatter with *disabled* = _true_ removes the
  corresponding global formatter.

For _filetype/\*.toml_:
- Filetypes with matching names (derived from filename) are overridden
  entirely by the project-level definition.
- Filetypes defined only in the project config are added.

For _fmt-all.toml_:
- A project-level *walk* value overrides the global value. If the
  project config uses the default value (*git*), the global value is
  kept.
- A project-level *exclude\_globs* list replaces the global list
  entirely (no per-element merging).

# PROJECT ROOT DETECTION

Lux locates the project root by walking up from the current working
directory and looking for any of the following markers:

- _.lux/_
- _.git/_
- _go.mod_
- _package.json_
- _Cargo.toml_
- _pyproject.toml_

# SEE ALSO

*lux*(1)
