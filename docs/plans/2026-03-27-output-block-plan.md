# Output Block Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add Output Block support to tap-dancer (Go writer, Rust writer, Go
reader/validator).

**Architecture:** Callback-based API mirroring `Subtest`. The callback receives
an `OutputBlockWriter` for streaming body lines. Returning nil/None signals ok;
returning non-nil signals not ok with YAML diagnostics. SGR filtering reuses
existing `sanitizeYAMLValue`/`sanitize_yaml_value`.

**Tech Stack:** Go (tap package), Rust (tap-dancer crate)

**Rollback:** N/A --- purely additive, no existing behavior changes.

--------------------------------------------------------------------------------

### Task 1: Go Writer --- OutputBlock and OutputBlockWriter

**Files:** - Modify: `packages/tap-dancer/go/tap.go:325-332` (TestPoint
struct) - Modify: `packages/tap-dancer/go/tap.go:334-361` (WriteAll) - Test:
`packages/tap-dancer/go/tap_test.go`

**Step 1: Write the failing tests**

Add to `tap_test.go`:

``` go
func TestOutputBlock(t *testing.T) {
    var buf bytes.Buffer
    tw := NewWriter(&buf)
    tw.OutputBlock("build the project", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("compiling main.rs")
        ob.Line("compiling lib.rs")
        return nil
    })
    tw.Plan()
    want := "TAP version 14\n" +
        "# Output: 1 - build the project\n" +
        "    compiling main.rs\n" +
        "    compiling lib.rs\n" +
        "ok 1 - build the project\n" +
        "1..1\n"
    if got := buf.String(); got != want {
        t.Errorf("got:\n%s\nwant:\n%s", got, want)
    }
}

func TestOutputBlockNotOk(t *testing.T) {
    var buf bytes.Buffer
    tw := NewWriter(&buf)
    tw.OutputBlock("build the project", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("compiling main.rs")
        return &Diagnostics{Message: "compilation failed", Severity: "fail"}
    })
    tw.Plan()
    got := buf.String()
    if !strings.Contains(got, "not ok 1 - build the project") {
        t.Errorf("expected not ok, got:\n%s", got)
    }
    if !strings.Contains(got, "  ---") {
        t.Errorf("expected YAML diagnostics, got:\n%s", got)
    }
    if !strings.Contains(got, "  message: compilation failed") {
        t.Errorf("expected message in diagnostics, got:\n%s", got)
    }
}

func TestOutputBlockColorStripsNonSGR(t *testing.T) {
    var buf bytes.Buffer
    tw := NewColorWriter(&buf, false)
    tw.OutputBlock("test", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("hello \033[32mgreen\033[0m and \033[2Kclear")
        return nil
    })
    tw.Plan()
    got := buf.String()
    // SGR and non-SGR both stripped in non-color mode
    if strings.Contains(got, "\033[") {
        t.Errorf("expected all ANSI stripped in non-color mode, got:\n%s", got)
    }
}

func TestOutputBlockColorPreservesSGR(t *testing.T) {
    var buf bytes.Buffer
    tw := NewColorWriter(&buf, true)
    tw.OutputBlock("test", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("hello \033[32mgreen\033[0m and \033[2Kclear")
        return nil
    })
    tw.Plan()
    got := buf.String()
    // SGR preserved, non-SGR stripped in color mode
    if !strings.Contains(got, "\033[32m") {
        t.Errorf("expected SGR preserved in color mode, got:\n%s", got)
    }
    if strings.Contains(got, "\033[2K") {
        t.Errorf("expected non-SGR stripped in color mode, got:\n%s", got)
    }
}

func TestOutputBlockEmpty(t *testing.T) {
    var buf bytes.Buffer
    tw := NewWriter(&buf)
    tw.OutputBlock("no output", func(ob *OutputBlockWriter) *Diagnostics {
        return nil
    })
    tw.Plan()
    want := "TAP version 14\n" +
        "# Output: 1 - no output\n" +
        "ok 1 - no output\n" +
        "1..1\n"
    if got := buf.String(); got != want {
        t.Errorf("got:\n%s\nwant:\n%s", got, want)
    }
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestOutputBlock' ./packages/tap-dancer/go/...`
Expected: FAIL --- `OutputBlock` and `OutputBlockWriter` not defined

**Step 3: Write minimal implementation**

Add to `tap.go` after `FinishLastLine` (after line 215):

``` go
// OutputBlockWriter writes indented body lines inside an Output Block.
type OutputBlockWriter struct {
    w     io.Writer
    color bool
}

// Line writes a single 4-space-indented output line, applying SGR filtering.
func (ob *OutputBlockWriter) Line(text string) {
    text = sanitizeYAMLValue(text, ob.color)
    fmt.Fprintf(ob.w, "    %s\n", text)
}

// OutputBlock emits an Output Block per the streamed-output amendment.
// The callback receives an OutputBlockWriter for streaming body lines.
// Returning nil emits "ok"; returning non-nil emits "not ok" with YAML diagnostics.
func (tw *Writer) OutputBlock(description string, fn func(*OutputBlockWriter) *Diagnostics) int {
    tw.n++
    num := tw.formatNumber(tw.n)
    fmt.Fprintf(tw.w, "# Output: %s - %s\n", num, description)
    ob := &OutputBlockWriter{w: tw.w, color: tw.color}
    diag := fn(ob)
    if diag != nil {
        tw.failed = true
        fmt.Fprintf(tw.w, "%s %s - %s\n", tw.colorNotOk(), num, description)
        writeDiagnostics(tw.w, diag, tw.color)
    } else {
        fmt.Fprintf(tw.w, "%s %s - %s\n", tw.colorOk(), num, description)
    }
    return tw.n
}
```

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -run 'TestOutputBlock' ./packages/tap-dancer/go/...`
Expected: PASS

**Step 5: Commit**

    feat(tap-dancer): add OutputBlock writer API (Go)

    Implements Output Block support per the TAP-14 streamed-output amendment.
    Callback-based API mirrors Subtest — nil return emits ok, non-nil emits
    not ok with YAML diagnostics. SGR filtering reuses sanitizeYAMLValue.

    Closes #62 (partial)

--------------------------------------------------------------------------------

### Task 2: Go Writer --- TestPoint iterator integration

**Files:** - Modify: `packages/tap-dancer/go/tap.go:325-332` (TestPoint
struct) - Modify: `packages/tap-dancer/go/tap.go:334-361` (WriteAll) - Test:
`packages/tap-dancer/go/tap_test.go`

**Step 1: Write the failing test**

``` go
func TestWriteAllOutputBlock(t *testing.T) {
    var buf bytes.Buffer
    tw := NewWriter(&buf)
    tw.WriteAll(func(yield func(TestPoint) bool) {
        yield(TestPoint{
            Description: "compile",
            Ok:          true,
            OutputBlock: func(ob *OutputBlockWriter) *Diagnostics {
                ob.Line("building...")
                return nil
            },
        })
    })
    got := buf.String()
    if !strings.Contains(got, "# Output: 1 - compile") {
        t.Errorf("expected output header, got:\n%s", got)
    }
    if !strings.Contains(got, "    building...") {
        t.Errorf("expected output body, got:\n%s", got)
    }
    if !strings.Contains(got, "ok 1 - compile") {
        t.Errorf("expected test point, got:\n%s", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run 'TestWriteAllOutputBlock' ./packages/tap-dancer/go/...`
Expected: FAIL --- `OutputBlock` field not in `TestPoint`

**Step 3: Write minimal implementation**

In `TestPoint` struct (line \~325), add the field:

``` go
type TestPoint struct {
    Description string
    Ok          bool
    Skip        string
    Todo        string
    Diagnostics *Diagnostics
    Subtests    func(*Writer)
    OutputBlock func(*OutputBlockWriter) *Diagnostics
}
```

In `WriteAll` (line \~334), add a case before subtests:

``` go
func (tw *Writer) WriteAll(tests iter.Seq[TestPoint]) {
    for tp := range tests {
        if tp.OutputBlock != nil {
            tw.OutputBlock(tp.Description, tp.OutputBlock)
        } else if tp.Subtests != nil {
```

The `Ok` field on `TestPoint` is ignored when `OutputBlock` is set --- the
callback return value determines ok/not ok.

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -run 'TestWriteAllOutputBlock|TestOutputBlock' ./packages/tap-dancer/go/...`
Expected: PASS

**Step 5: Commit**

    feat(tap-dancer): add OutputBlock to TestPoint iterator API

--------------------------------------------------------------------------------

### Task 3: Go Reader --- parse Output Block events

**Files:** - Modify: `packages/tap-dancer/go/diagnostic.go:53-64` (EventType
constants) - Modify: `packages/tap-dancer/go/classify.go:10-22` (lineKind
constants) - Modify: `packages/tap-dancer/go/classify.go:46-88` (classifyLine) -
Modify: `packages/tap-dancer/go/reader.go:24-33` (frame struct) - Modify:
`packages/tap-dancer/go/reader.go:86-288` (Next method) - Test:
`packages/tap-dancer/go/reader_test.go`

**Step 1: Write the failing tests**

Add to `reader_test.go`:

``` go
func TestReaderOutputBlock(t *testing.T) {
    input := "TAP version 14\n" +
        "# Output: 1 - build\n" +
        "    compiling main.rs\n" +
        "    linking binary\n" +
        "ok 1 - build\n" +
        "1..1\n"
    r := NewReader(strings.NewReader(input))

    ev, err := r.Next() // version
    if err != nil || ev.Type != EventVersion {
        t.Fatalf("expected version, got %v %v", ev, err)
    }

    ev, err = r.Next() // output header
    if err != nil || ev.Type != EventOutputHeader {
        t.Fatalf("expected output header, got %v %v", ev, err)
    }
    if ev.OutputHeader == nil || ev.OutputHeader.Number != 1 || ev.OutputHeader.Description != "build" {
        t.Fatalf("bad output header: %+v", ev.OutputHeader)
    }

    ev, err = r.Next() // output line 1
    if err != nil || ev.Type != EventOutputLine {
        t.Fatalf("expected output line, got %v %v", ev, err)
    }
    if ev.OutputLine != "compiling main.rs" {
        t.Fatalf("bad output line: %q", ev.OutputLine)
    }

    ev, err = r.Next() // output line 2
    if err != nil || ev.Type != EventOutputLine {
        t.Fatalf("expected output line, got %v %v", ev, err)
    }
    if ev.OutputLine != "linking binary" {
        t.Fatalf("bad output line: %q", ev.OutputLine)
    }

    ev, err = r.Next() // test point
    if err != nil || ev.Type != EventTestPoint {
        t.Fatalf("expected test point, got %v %v", ev, err)
    }

    ev, err = r.Next() // plan
    if err != nil || ev.Type != EventPlan {
        t.Fatalf("expected plan, got %v %v", ev, err)
    }

    summary := r.Summary()
    if !summary.Valid {
        t.Errorf("expected valid, diagnostics: %v", r.Diagnostics())
    }
}

func TestReaderOutputBlockMismatchedID(t *testing.T) {
    input := "TAP version 14\n" +
        "# Output: 1 - build\n" +
        "    compiling\n" +
        "ok 2 - build\n" +
        "1..1\n"
    r := NewReader(strings.NewReader(input))
    for {
        if _, err := r.Next(); err != nil {
            break
        }
    }
    diags := r.Diagnostics()
    found := false
    for _, d := range diags {
        if d.Rule == "output-block-id-mismatch" {
            found = true
        }
    }
    if !found {
        t.Errorf("expected output-block-id-mismatch diagnostic, got: %v", diags)
    }
}

func TestReaderOutputBlockDescriptionMismatch(t *testing.T) {
    input := "TAP version 14\n" +
        "# Output: 1 - build\n" +
        "    compiling\n" +
        "ok 1 - compile\n" +
        "1..1\n"
    r := NewReader(strings.NewReader(input))
    for {
        if _, err := r.Next(); err != nil {
            break
        }
    }
    diags := r.Diagnostics()
    found := false
    for _, d := range diags {
        if d.Rule == "output-block-description-mismatch" {
            found = true
        }
    }
    if !found {
        t.Errorf("expected output-block-description-mismatch warning, got: %v", diags)
    }
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestReaderOutputBlock' ./packages/tap-dancer/go/...`
Expected: FAIL --- `EventOutputHeader`, `EventOutputLine`, `OutputHeader`,
`OutputLine` not defined

**Step 3: Write implementation**

Add to `diagnostic.go` event types (after `EventSubtestEnd`, line \~63):

``` go
EventOutputHeader
EventOutputLine
```

Add to `Event` struct (after `StreamedOutput` field, line \~104):

``` go
OutputHeader *OutputHeaderResult `json:"output_header,omitempty"`
OutputLine   string              `json:"output_line,omitempty"`
```

Add new type near `TestPointResult`:

``` go
// OutputHeaderResult holds parsed data from an Output Block header.
type OutputHeaderResult struct {
    Number      int    `json:"number"`
    Description string `json:"description"`
}
```

Add to `classify.go` lineKind constants (after `lineEmpty`):

``` go
lineOutputHeader
lineOutputLine
```

Add output header regex in `classify.go` (near other regexps):

``` go
outputHeaderRegexp = regexp.MustCompile(`^# Output:\s+(\d+)\s*-\s*(.+?)(?:\s+#.*)?$`)
```

In `classifyLine`, add before the `# Subtest` check (line \~75):

``` go
if outputHeaderRegexp.MatchString(line) {
    return lineOutputHeader
}
```

Add output line detection --- a line starting with exactly 4 spaces that is
inside an output block. This requires state, so handle it in the reader instead.
In classify, add after `lineEmpty` check:

Actually, output lines (4-space indented) look identical to subtest content. We
need to track state in the reader. Add `inOutputBlock bool` and
`outputBlockNumber int` and `outputBlockDescription string` to the `frame`
struct.

In `reader.go`, add to `frame`:

``` go
inOutputBlock          bool
outputBlockNumber      int
outputBlockDescription string
```

In `Next()`, after depth handling and before `kind := classifyLine(trimmed)`,
add output line detection:

``` go
// Handle output block body lines (4-space indent at current depth)
if r.currentFrame().inOutputBlock && indent == (r.currentFrame().depth*4)+4 {
    content := raw[(r.currentFrame().depth*4)+4:]
    r.lastWasTestPoint = false
    return Event{
        Type:       EventOutputLine,
        Line:       r.lineNum,
        Depth:      r.currentFrame().depth,
        Raw:        raw,
        OutputLine: content,
    }, nil
}
```

Add `lineOutputHeader` case in `Next()` switch:

``` go
case lineOutputHeader:
    m := outputHeaderRegexp.FindStringSubmatch(trimmed)
    num, _ := strconv.Atoi(m[1])
    desc := strings.TrimSpace(m[2])
    f := r.currentFrame()
    f.inOutputBlock = true
    f.outputBlockNumber = num
    f.outputBlockDescription = desc
    r.lastWasTestPoint = false
    return Event{
        Type:  EventOutputHeader,
        Line:  r.lineNum,
        Depth: depth,
        Raw:   raw,
        OutputHeader: &OutputHeaderResult{
            Number:      num,
            Description: desc,
        },
    }, nil
```

In the `lineTestPoint` case, add validation after parsing the test point (after
`f.lastTestNumber = tp.Number`):

``` go
if f.inOutputBlock {
    f.inOutputBlock = false
    if tp.Number != f.outputBlockNumber {
        r.addDiag(SeverityError, "output-block-id-mismatch",
            "output block header declared test "+strconv.Itoa(f.outputBlockNumber)+
                " but correlated test point is "+strconv.Itoa(tp.Number))
    }
    if tp.Description != f.outputBlockDescription {
        r.addDiag(SeverityWarning, "output-block-description-mismatch",
            "output block header description "+strconv.Quote(f.outputBlockDescription)+
                " differs from test point description "+strconv.Quote(tp.Description))
    }
}
```

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -run 'TestReaderOutputBlock' ./packages/tap-dancer/go/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `nix develop --command go test ./packages/tap-dancer/go/...` Expected: PASS
(no regressions)

**Step 6: Commit**

    feat(tap-dancer): add Output Block parsing to reader/validator (Go)

    Adds EventOutputHeader and EventOutputLine event types. Validates that
    output block header IDs match correlated test point IDs and warns on
    description mismatches.

--------------------------------------------------------------------------------

### Task 4: Rust Writer --- output_block and OutputBlockWriter

**Files:** - Modify: `packages/tap-dancer/rust/src/lib.rs` - Test: inline in
`packages/tap-dancer/rust/src/lib.rs` (mod tests)

**Step 1: Write the failing tests**

Add to the `mod tests` block in `lib.rs`:

``` rust
#[test]
fn test_output_block_ok() {
    let mut buf = Vec::new();
    let mut tw = TapWriterBuilder::new(&mut buf).build().unwrap();
    tw.output_block("build the project", |ob| {
        ob.line("compiling main.rs").unwrap();
        ob.line("compiling lib.rs").unwrap();
        None
    })
    .unwrap();
    tw.plan().unwrap();
    let got = String::from_utf8(buf).unwrap();
    let want = "TAP version 14\n\
                # Output: 1 - build the project\n\
                \x20\x20\x20\x20compiling main.rs\n\
                \x20\x20\x20\x20compiling lib.rs\n\
                ok 1 - build the project\n\
                1..1\n";
    assert_eq!(got, want);
}

#[test]
fn test_output_block_not_ok() {
    let mut buf = Vec::new();
    let mut tw = TapWriterBuilder::new(&mut buf).build().unwrap();
    tw.output_block("build", |ob| {
        ob.line("compiling...").unwrap();
        Some(vec![("message", "compilation failed"), ("severity", "fail")])
    })
    .unwrap();
    tw.plan().unwrap();
    let got = String::from_utf8(buf).unwrap();
    assert!(got.contains("not ok 1 - build"));
    assert!(got.contains("  ---"));
    assert!(got.contains("  message: compilation failed"));
}

#[test]
fn test_output_block_sgr_color_mode() {
    let mut buf = Vec::new();
    let mut tw = TapWriterBuilder::new(&mut buf).color(true).build().unwrap();
    tw.output_block("test", |ob| {
        ob.line("hello \x1b[32mgreen\x1b[0m and \x1b[2Kclear").unwrap();
        None
    })
    .unwrap();
    tw.plan().unwrap();
    let got = String::from_utf8(buf).unwrap();
    assert!(got.contains("\x1b[32m"), "SGR should be preserved");
    assert!(!got.contains("\x1b[2K"), "non-SGR should be stripped");
}

#[test]
fn test_output_block_sgr_no_color() {
    let mut buf = Vec::new();
    let mut tw = TapWriterBuilder::new(&mut buf).color(false).build().unwrap();
    tw.output_block("test", |ob| {
        ob.line("hello \x1b[32mgreen\x1b[0m").unwrap();
        None
    })
    .unwrap();
    tw.plan().unwrap();
    let got = String::from_utf8(buf).unwrap();
    assert!(!got.contains("\x1b["), "all ANSI should be stripped");
}

#[test]
fn test_output_block_empty() {
    let mut buf = Vec::new();
    let mut tw = TapWriterBuilder::new(&mut buf).build().unwrap();
    tw.output_block("no output", |_ob| None).unwrap();
    tw.plan().unwrap();
    let got = String::from_utf8(buf).unwrap();
    let want = "TAP version 14\n\
                # Output: 1 - no output\n\
                ok 1 - no output\n\
                1..1\n";
    assert_eq!(got, want);
}
```

**Step 2: Run tests to verify they fail**

Run: `cd packages/tap-dancer/rust && cargo test test_output_block` Expected:
FAIL --- `output_block` and `OutputBlockWriter` not defined

**Step 3: Write minimal implementation**

Add after the `IndentWriter` impl block (around line 431):

``` rust
pub struct OutputBlockWriter<'a> {
    w: &'a mut dyn Write,
    color: bool,
}

impl OutputBlockWriter<'_> {
    pub fn line(&mut self, text: &str) -> io::Result<()> {
        let text = sanitize_yaml_value(text, self.color);
        writeln!(self.w, "    {}", text)
    }
}
```

Add to `impl TapWriter` (after `subtest` method):

``` rust
pub fn output_block<F>(&mut self, desc: &str, f: F) -> io::Result<usize>
where
    F: FnOnce(&mut OutputBlockWriter) -> Option<Vec<(&str, &str)>>,
{
    self.counter += 1;
    let num = self.config.format_number(self.counter);
    writeln!(self.w, "# Output: {} - {}", num, desc)?;
    let mut ob = OutputBlockWriter {
        w: &mut *self.w,
        color: self.config.color(),
    };
    let diag = f(&mut ob);
    match diag {
        Some(diagnostics) => {
            self.failed = true;
            writeln!(
                self.w,
                "{} {} - {}",
                status_not_ok(self.config.color()),
                num,
                desc
            )?;
            write_diagnostics_block(&mut *self.w, &diagnostics, self.config.color())?;
        }
        None => {
            writeln!(
                self.w,
                "{} {} - {}",
                status_ok(self.config.color()),
                num,
                desc
            )?;
        }
    }
    Ok(self.counter)
}
```

Also add free function (after `write_plan_skip`):

``` rust
pub fn write_output_header(w: &mut impl Write, num: usize, desc: &str) -> io::Result<()> {
    writeln!(w, "# Output: {} - {}", num, desc)
}

pub fn write_output_line(w: &mut impl Write, text: &str) -> io::Result<()> {
    writeln!(w, "    {}", text)
}
```

**Step 4: Run tests to verify they pass**

Run: `cd packages/tap-dancer/rust && cargo test test_output_block` Expected:
PASS

**Step 5: Run full Rust test suite**

Run: `cd packages/tap-dancer/rust && cargo test` Expected: PASS (no regressions)

**Step 6: Commit**

    feat(tap-dancer): add output_block writer API (Rust)

    Callback-based API mirroring subtest. Returning None emits ok; returning
    Some(diagnostics) emits not ok with YAML block. SGR filtering reuses
    sanitize_yaml_value.

--------------------------------------------------------------------------------

### Task 5: Full integration test

**Files:** - Test: `packages/tap-dancer/go/tap_test.go`

**Step 1: Write the integration test**

``` go
func TestOutputBlockRoundTrip(t *testing.T) {
    var buf bytes.Buffer
    tw := NewWriter(&buf)
    tw.OutputBlock("build", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("compiling...")
        ob.Line("linking...")
        return nil
    })
    tw.OutputBlock("test", func(ob *OutputBlockWriter) *Diagnostics {
        ob.Line("running tests...")
        return &Diagnostics{Message: "1 test failed", Severity: "fail"}
    })
    tw.Plan()

    // Parse the output
    r := NewReader(strings.NewReader(buf.String()))
    summary := r.Summary()
    if summary.TotalTests != 2 {
        t.Errorf("expected 2 tests, got %d", summary.TotalTests)
    }
    if summary.Passed != 1 {
        t.Errorf("expected 1 passed, got %d", summary.Passed)
    }
    if summary.Failed != 1 {
        t.Errorf("expected 1 failed, got %d", summary.Failed)
    }
    if !summary.Valid {
        t.Errorf("expected valid TAP, diagnostics: %v", r.Diagnostics())
    }
}
```

**Step 2: Run test**

Run:
`nix develop --command go test -run 'TestOutputBlockRoundTrip' ./packages/tap-dancer/go/...`
Expected: PASS

**Step 3: Run full test suites**

Run:
`nix develop --command go test ./packages/tap-dancer/go/... && cd packages/tap-dancer/rust && cargo test`
Expected: PASS

**Step 4: Commit**

    test(tap-dancer): add output block round-trip integration test

--------------------------------------------------------------------------------

### Task 6: Nix build verification

**Step 1: Build tap-dancer**

Run: `nix build .#tap-dancer` Expected: success

**Step 2: Build full marketplace**

Run: `nix build` Expected: success

**Step 3: Commit all work if not already committed**

No new code --- just verification.
