# TTY Build Last Line — tap-dancer Implementation

## Summary

Implement the TAP-14 `tty-build-last-line` pragma amendment in both the Rust
and Go tap-dancer libraries. This pragma allows producers to maintain a single
trailing comment line that is rewritten in place using ANSI cursor control
sequences for live progress display.

## Spec Reference

`amarbel-llc/tap:tty-build-last-line-amendment.md`

Key rules:
- Activated with `pragma +tty-build-last-line` (document-wide, cannot be deactivated)
- Trailing line MUST be prefixed with `# ` (valid TAP comment)
- MAY use `\r` and `ESC[2K` to rewrite in place
- MAY use ANSI SGR for color
- Producers SHOULD only emit when stdout is a TTY
- Does NOT propagate to subtests (only outermost document)
- Producer SHOULD emit final newline when stream is complete

## Design

### Rust (`lib.rs`)

- Add `tty_build_last_line: bool` to `TapConfig`
- Add `.tty_build_last_line(bool)` on `TapWriterBuilder`
- Emit `pragma +tty-build-last-line` during `build()` when enabled
- `TapWriter::update_last_line(&mut self, text: &str)` — writes `\r\x1b[2K# {text}` (no newline)
- `TapWriter::finish_last_line(&mut self)` — writes `\n`
- Do NOT propagate to subtests (unlike `streamed_output`)

### Go (`tap.go`)

- Add `ttyBuildLastLine bool` to `Writer`
- `EnableTTYBuildLastLine()` — sets field and emits pragma
- `UpdateLastLine(text string)` — writes `\r\x1b[2K# {text}` (no newline)
- `FinishLastLine()` — writes `\n`
- Do NOT propagate to subtests
- Track in `Pragma()` method like `streamed-output`

### Opt-in only

Both libraries require explicit opt-in. The `auto()` builder path does NOT
enable this pragma automatically.
