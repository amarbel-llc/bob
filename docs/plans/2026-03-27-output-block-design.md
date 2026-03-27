# Output Block Support for tap-dancer

**Date:** 2026-03-27 **Issue:** #62

## Problem

The TAP-14 streamed-output amendment introduced Output Blocks, which allow
producers to stream process output before the test point status is known.
tap-dancer (Go and Rust) needs writer, reader, and validator support.

## Wire Format

``` tap
# Output: 1 - build the project
    compiling main.rs
    compiling lib.rs
ok 1 - build the project
```

- Header: `# Output: <id> - <description>`
- Body: zero or more lines indented by exactly 4 spaces (plain text, not YAML)
- Correlated test point: `ok|not ok <id> - <description>` terminates the block
- Optional YAML diagnostic block may follow the test point

## Design

### Writer API

**Callback pattern** --- mirrors existing `Subtest` API. The callback receives
an `OutputBlockWriter` for streaming body lines. Returning nil/None emits `ok`;
returning non-nil emits `not ok` with YAML diagnostics.

**Go:**

``` go
func (w *Writer) OutputBlock(
    description string,
    fn func(*OutputBlockWriter) *Diagnostics,
) int

type OutputBlockWriter struct { /* unexported */ }
func (ob *OutputBlockWriter) Line(text string)
```

**Rust:**

``` rust
pub fn output_block<F>(&mut self, desc: &str, f: F) -> io::Result<usize>
where
    F: FnOnce(&mut OutputBlockWriter) -> Option<Vec<(&str, &str)>>

pub struct OutputBlockWriter<'a> { /* private */ }
impl OutputBlockWriter<'_> {
    pub fn line(&mut self, text: &str) -> io::Result<()>
}
```

**SGR handling:** `Line` applies the same ANSI filtering as YAML diagnostic
values --- preserve SGR sequences in color mode, strip in non-color mode, always
strip non-SGR CSI sequences.

**Future work:** The output line API may benefit from richer SGR support (e.g.,
producer-controlled coloring of output lines, or harness-side re-coloring). Out
of scope for V1 --- V1 reuses the existing YAML diagnostic SGR filtering as-is.

### TestPoint Iterator Integration (Go)

Add `OutputBlock func(*OutputBlockWriter) *Diagnostics` field to the `TestPoint`
struct, parallel to `Subtests func(*Writer)`. When set, `WriteAll` emits an
output block instead of a plain test point.

### Reader/Validator (Go)

New event types:

- `EventOutputHeader` --- parsed from `# Output: N - description` lines
- `EventOutputLine` --- the 4-space indented body content

Validation rules:

- Output header ID must match a subsequent test point ID
- No nested Output Blocks
- Warn if header description doesn't match correlated test point description

### Rollback

This is purely additive --- no existing behavior changes. Output Blocks are only
emitted when the new API is called. Rollback is removing the API; no
dual-architecture period needed.
