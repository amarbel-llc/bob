---
date: 2026-04-12
promotion-criteria: |
  exploring → proposed: design validated, implementation plan written.
  proposed → experimental: all 4 tiers implemented and passing unit tests.
  experimental → testing: 14 days of use across lux, bob, and eng repos
    without false positives or missed failures that runtime validation
    should have caught.
  testing → accepted: promoted to default behavior (no flags needed).
status: exploring
---

# Runtime Validation for lux validate

## Problem Statement

`lux validate` currently performs static-only config validation: TOML syntax,
required fields, and cross-references (filetype configs reference LSPs and
formatters that exist). This catches typos and structural errors but misses the
failures users actually hit:

- A flake reference that doesn't resolve (`nixpkgs#nonexistent-tool`)
- A flake that builds but the expected binary isn't in the output
- A formatter configured in the wrong mode (stdin vs filepath)
- A formatter that exits non-zero on valid input
- An LSP that fails to initialize (bad args, missing runtime deps)

These failures only surface at format/edit time, often deep into a coding
session. Users have no way to pre-flight their configuration.

## Interface

`lux validate` gains optional flags that enable runtime checks beyond static
config validation:

```
lux validate                    # static config only (existing behavior)
lux validate --check-flakes     # + resolve all flake references
lux validate --check-formatters # + smoke-test each formatter
lux validate --check-lsps       # + initialize/shutdown each LSP
lux validate --all              # all of the above
```

### Output

One line per check, grouped by tier:

```
config
  ✓ lsps.toml
  ✓ formatters.toml
  ✓ filetype cross-references

flakes
  ✓ nixpkgs#gopls (1.2s)
  ✓ nixpkgs#gofumpt (0.8s)
  ✗ nixpkgs#nonexistent — nix build failed: ...

formatters
  ✓ gofumpt — sample.go formatted (0.3s)
  ✓ shfmt — sample.sh formatted (0.1s)
  ⊘ prettier — no sample file for extensions: html, yaml

lsps
  ✓ gopls — initialized, capabilities: formatting, hover (4.2s)
  ✗ lua-language-server — initialize timed out after 30s

12 passed, 1 failed, 1 skipped
```

Exit code: 0 if all checks pass or skip, 1 if any check fails.

### Validation Tiers

**Tier 1 — Config (always runs):** Static validation. TOML syntax, required
fields, cross-references between filetype/LSP/formatter configs. This is the
existing `runValidate()` behavior, refactored to produce structured results.

**Tier 2 — Flake resolution (`--check-flakes`):** For each unique flake
reference across LSPs and formatters, call `NixExecutor.Build()` to verify the
flake resolves and the expected binary exists. Flake results are cached, so
duplicate references are free.

**Tier 3 — Formatter smoke-test (`--check-formatters`):** For each configured
formatter, find a matching sample file by extension, run the formatter against
it, and verify it exits 0 with non-empty output. Formatters whose extensions
have no sample file are skipped. The existing empty-output/mode-mismatch hint
from `formatter.Format()` is surfaced directly.

**Tier 4 — LSP initialization (`--check-lsps`):** For each configured LSP,
spawn the process, send an LSP `initialize` request with minimal params, wait
for the response (30s timeout), then send `shutdown` + `exit`. Pass if the
initialize response contains valid capabilities.

### Sample Files

Embedded via `//go:embed` in the validate package. Minimal syntactically-valid
files for each supported language. These are not formatter test vectors — the
check is "does the formatter run without error on valid input," not "does the
formatter produce the correct output."

Sample files are matched to formatters via the filetype config: a formatter is
testable if any filetype that references it has an extension with a matching
sample file.

## Limitations

- Flake resolution requires network access and a Nix store. `--check-flakes`
  will fail in air-gapped environments.
- LSP initialization may be slow (gopls can take 5-10s). The `--check-lsps`
  tier uses a 30s per-LSP timeout.
- Sample files cover common languages but not all. Formatters for unsupported
  languages are skipped, not failed.
- Formatter smoke-testing validates "runs without error" but not "produces
  correct output." A formatter that silently corrupts files would still pass.

## Testing

### Unit tests

- `SampleForExtension()` returns content for known extensions, nil for unknown
- Config tier produces expected checks from valid/invalid configs
- Flake tier with mock executor (pass/fail/cache behavior)
- Formatter tier with mock executor and fake formatter scripts
- LSP tier with mock executor (initialize response parsing, timeout handling)

### Integration tests (BATS)

- `lux validate` — static config only, existing behavior preserved
- `lux validate --check-flakes` — with default config, verifies real flake
  resolution
- `lux validate --check-formatters` — with at least one configured formatter
- `lux validate --all` — full end-to-end

## More Information

- GitHub issue: amarbel-llc/bob#18
- Related: amarbel-llc/purse-first#1 (MCP validation via inspector CLI)
- Related: amarbel-llc/moxy#7 (blocked on purse-first#1)
