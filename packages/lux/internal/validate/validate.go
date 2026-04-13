package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/config/filetype"
	"github.com/amarbel-llc/lux/internal/formatter"
	"github.com/amarbel-llc/lux/internal/lsp"
	"github.com/amarbel-llc/lux/internal/subprocess"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/jsonrpc"
)

type Status int

const (
	Pass Status = iota
	Fail
	Skip
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "✓"
	case Fail:
		return "✗"
	case Skip:
		return "⊘"
	default:
		return "?"
	}
}

type Check struct {
	Category string
	Name     string
	Status   Status
	Message  string
	Duration time.Duration
}

type Result struct {
	Checks  []Check
	Passed  int
	Failed  int
	Skipped int
}

func (r *Result) add(c Check) {
	r.Checks = append(r.Checks, c)
	switch c.Status {
	case Pass:
		r.Passed++
	case Fail:
		r.Failed++
	case Skip:
		r.Skipped++
	}
}

type Options struct {
	CheckFlakes     bool
	CheckFormatters bool
	CheckLSPs       bool
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	result := &Result{}

	cfg, fmtCfg, filetypes, err := validateConfig(result)
	if err != nil {
		return result, nil
	}

	executor := subprocess.NewNixExecutor()

	if opts.CheckFlakes {
		validateFlakes(ctx, result, cfg, fmtCfg, executor)
	}

	if opts.CheckFormatters {
		validateFormatters(ctx, result, fmtCfg, filetypes, executor)
	}

	if opts.CheckLSPs {
		validateLSPs(ctx, result, cfg, executor)
	}

	return result, nil
}

func validateConfig(result *Result) (*config.Config, *config.FormatterConfig, []*filetype.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		result.add(Check{Category: "config", Name: "lsps.toml", Status: Fail, Message: err.Error()})
		return nil, nil, nil, err
	}
	result.add(Check{Category: "config", Name: "lsps.toml", Status: Pass})

	fmtCfg, err := config.LoadMergedFormatters()
	if err != nil {
		result.add(Check{Category: "config", Name: "formatters.toml", Status: Fail, Message: err.Error()})
		return nil, nil, nil, err
	}
	if err := fmtCfg.Validate(); err != nil {
		result.add(Check{Category: "config", Name: "formatters.toml", Status: Fail, Message: err.Error()})
		return nil, nil, nil, err
	}
	result.add(Check{Category: "config", Name: "formatters.toml", Status: Pass})

	filetypes, err := filetype.LoadMerged()
	if err != nil {
		result.add(Check{Category: "config", Name: "filetype cross-references", Status: Fail, Message: err.Error()})
		return nil, nil, nil, err
	}

	lspNames := make(map[string]bool)
	for _, l := range cfg.LSPs {
		lspNames[l.Name] = true
	}
	fmtNames := make(map[string]bool)
	for _, f := range fmtCfg.Formatters {
		fmtNames[f.Name] = true
	}
	if err := filetype.Validate(filetypes, lspNames, fmtNames); err != nil {
		result.add(Check{Category: "config", Name: "filetype cross-references", Status: Fail, Message: err.Error()})
		return nil, nil, nil, err
	}
	result.add(Check{Category: "config", Name: "filetype cross-references", Status: Pass})

	return cfg, fmtCfg, filetypes, nil
}

func validateFlakes(ctx context.Context, result *Result, cfg *config.Config, fmtCfg *config.FormatterConfig, executor *subprocess.NixExecutor) {
	type flakeRef struct {
		flake  string
		binary string
	}

	seen := make(map[string]bool)
	var refs []flakeRef

	for _, l := range cfg.LSPs {
		key := l.Flake + "::" + l.Binary
		if !seen[key] {
			seen[key] = true
			refs = append(refs, flakeRef{l.Flake, l.Binary})
		}
	}
	for _, f := range fmtCfg.Formatters {
		if f.Flake == "" {
			continue
		}
		key := f.Flake + "::" + f.Binary
		if !seen[key] {
			seen[key] = true
			refs = append(refs, flakeRef{f.Flake, f.Binary})
		}
	}

	for _, ref := range refs {
		name := ref.flake
		if ref.binary != "" {
			name += " (binary: " + ref.binary + ")"
		}

		start := time.Now()
		_, err := executor.Build(ctx, ref.flake, ref.binary)
		dur := time.Since(start)

		if err != nil {
			result.add(Check{Category: "flake", Name: name, Status: Fail, Message: err.Error(), Duration: dur})
		} else {
			result.add(Check{Category: "flake", Name: name, Status: Pass, Duration: dur})
		}
	}
}

func validateFormatters(ctx context.Context, result *Result, fmtCfg *config.FormatterConfig, filetypes []*filetype.Config, executor *subprocess.NixExecutor) {
	// Build a map from formatter name to extensions that use it
	fmtExtensions := make(map[string][]string)
	for _, ft := range filetypes {
		for _, fmtName := range ft.Formatters {
			fmtExtensions[fmtName] = append(fmtExtensions[fmtName], ft.Extensions...)
		}
	}

	for i := range fmtCfg.Formatters {
		f := &fmtCfg.Formatters[i]
		if f.Disabled {
			continue
		}

		exts := fmtExtensions[f.Name]
		var sampleContent []byte
		var sampleExt string
		for _, ext := range exts {
			if content := SampleForExtension(ext); content != nil {
				sampleContent = content
				sampleExt = ext
				break
			}
		}

		if sampleContent == nil {
			extList := strings.Join(exts, ", ")
			if extList == "" {
				extList = "(none)"
			}
			result.add(Check{
				Category: "formatter",
				Name:     f.Name,
				Status:   Skip,
				Message:  fmt.Sprintf("no sample file for extensions: %s", extList),
			})
			continue
		}

		start := time.Now()
		samplePath := fmt.Sprintf("/tmp/lux-validate-sample.%s", sampleExt)
		_, err := formatter.Format(ctx, f, samplePath, sampleContent, executor)
		dur := time.Since(start)

		if err != nil {
			result.add(Check{Category: "formatter", Name: f.Name, Status: Fail, Message: err.Error(), Duration: dur})
		} else {
			result.add(Check{
				Category: "formatter",
				Name:     f.Name,
				Status:   Pass,
				Message:  fmt.Sprintf("sample.%s formatted", sampleExt),
				Duration: dur,
			})
		}
	}
}

func validateLSPs(ctx context.Context, result *Result, cfg *config.Config, executor *subprocess.NixExecutor) {
	for _, l := range cfg.LSPs {
		start := time.Now()
		err := testLSPInitialize(ctx, &l, executor)
		dur := time.Since(start)

		if err != nil {
			result.add(Check{Category: "lsp", Name: l.Name, Status: Fail, Message: err.Error(), Duration: dur})
		} else {
			result.add(Check{Category: "lsp", Name: l.Name, Status: Pass, Duration: dur})
		}
	}
}

func testLSPInitialize(ctx context.Context, l *config.LSP, executor *subprocess.NixExecutor) error {
	binPath, err := executor.Build(ctx, l.Flake, l.Binary)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	lspCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	proc, err := executor.Execute(lspCtx, binPath, l.Args, l.Env, os.TempDir())
	if err != nil {
		return fmt.Errorf("execute failed: %w", err)
	}
	defer proc.Kill()

	conn := jsonrpc.NewConn(proc.Stdout, proc.Stdin, func(ctx context.Context, msg *jsonrpc.Message) (*jsonrpc.Message, error) {
		return nil, nil
	})

	go conn.Run(lspCtx)

	initParams := &lsp.InitializeParams{
		ProcessID: intPtr(os.Getpid()),
		Capabilities: lsp.ClientCapabilities{
			TextDocument: &lsp.TextDocumentClientCapabilities{
				Formatting: &lsp.FormattingClientCaps{
					DynamicRegistration: false,
				},
			},
		},
	}

	rawResult, err := conn.Call(lspCtx, lsp.MethodInitialize, initParams)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	var initResult lsp.InitializeResult
	if err := json.Unmarshal(rawResult, &initResult); err != nil {
		return fmt.Errorf("parse init result: %w", err)
	}

	_ = conn.Notify(lsp.MethodInitialized, struct{}{})

	// Graceful shutdown
	_, _ = conn.Call(lspCtx, lsp.MethodShutdown, nil)
	_ = conn.Notify("exit", nil)

	return nil
}

func intPtr(i int) *int {
	return &i
}
