package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func makeHookInput(toolName string, toolInput map[string]any) []byte {
	input := map[string]any{
		"tool_name":  toolName,
		"tool_input": toolInput,
	}
	data, _ := json.Marshal(input)
	return data
}

func TestWebFetchHookIssueDetail(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/issues/42",
		"prompt": "get the issue",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle GitHub issue URL")
	}
	if !strings.Contains(out.String(), "get-hubbed://issues?number=42&repo=owner/repo") {
		t.Errorf("expected specific resource URI in output, got %q", out.String())
	}
}

func TestWebFetchHookNonWebFetchTool(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "ls"})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle non-WebFetch tool")
	}
}

func TestWebFetchHookNonGitHubURL(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://example.com/page",
		"prompt": "get the page",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle non-GitHub URL")
	}
}

func TestWebFetchHookMalformedURL(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "not-a-url",
		"prompt": "get something",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to fail-open on malformed URL")
	}
}

func TestWebFetchHookCatchAllGitHub(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/settings",
		"prompt": "get the settings",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to catch-all GitHub URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}

func TestWebFetchHookAllGitHubDomains(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{"https://github.com/owner/repo/settings", "GitHub URLs are served by get-hubbed"},
		{"https://www.github.com/owner/repo/settings", "GitHub URLs are served by get-hubbed"},
		{"https://api.github.com/repos/owner/repo", "get-hubbed://repo"},
		{"https://raw.githubusercontent.com/owner/repo/main/README.md", "get-hubbed://contents?path=README.md&repo=owner/repo"},
		{"https://gist.github.com/owner/abc123", "get-hubbed://gist?id=abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			input := makeHookInput("WebFetch", map[string]any{
				"url":    tt.url,
				"prompt": "fetch",
			})
			var out bytes.Buffer
			handled, err := HandleWebFetchHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %s", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %q in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}

func TestWebFetchHookDenyMessageFormat(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/issues/42",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	HandleWebFetchHook(input, &out)

	output := out.String()
	if !strings.Contains(output, "deny") {
		t.Error("deny message should contain deny decision")
	}
	if !strings.Contains(output, "get-hubbed://issues") {
		t.Error("deny message should contain the resource URI")
	}
	if !strings.Contains(output, "get-hubbed for ALL GitHub interactions") {
		t.Error("deny message should instruct exclusive get-hubbed usage")
	}
}

func TestWebFetchHookAllMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{"https://github.com/owner/repo", "get-hubbed://repo"},
		{"https://github.com/owner/repo/issues", "get-hubbed://issues?repo=owner/repo"},
		{"https://github.com/owner/repo/issues/42", "get-hubbed://issues?number=42&repo=owner/repo"},
		{"https://github.com/owner/repo/pulls", "get-hubbed://pulls?repo=owner/repo"},
		{"https://github.com/owner/repo/pull/7", "get-hubbed://pulls?number=7&repo=owner/repo"},
		{"https://github.com/owner/repo/blob/main/src/foo.go", "get-hubbed://contents?path=src/foo.go&repo=owner/repo"},
		{"https://github.com/owner/repo/tree/main/src", "get-hubbed://tree?path=src&repo=owner/repo"},
		{"https://github.com/owner/repo/blame/main/src/foo.go", "get-hubbed://blame?path=src/foo.go&repo=owner/repo"},
		{"https://github.com/owner/repo/commits/main", "get-hubbed://commits?repo=owner/repo"},
		{"https://github.com/owner/repo/actions", "get-hubbed://runs?repo=owner/repo"},
		{"https://github.com/owner/repo/actions/runs/12345", "get-hubbed://runs?run_id=12345&repo=owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			input := makeHookInput("WebFetch", map[string]any{
				"url":    tt.url,
				"prompt": "fetch",
			})
			var out bytes.Buffer
			handled, err := HandleWebFetchHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}

func TestWebFetchHookCompareURL(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/compare/main...feature",
		"prompt": "compare branches",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle compare URL")
	}
	if !strings.Contains(out.String(), "get-hubbed://compare?repo=owner/repo&base=main&head=feature") {
		t.Errorf("expected compare resource URI in output, got %q", out.String())
	}
}

func TestWebFetchHookURLWithFragment(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/issues/42#issuecomment-123",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle URL with fragment")
	}
	if !strings.Contains(out.String(), "get-hubbed://issues?number=42&repo=owner/repo") {
		t.Errorf("expected issue resource URI, got %q", out.String())
	}
}

func TestWebFetchHookURLWithQueryParams(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://github.com/owner/repo/issues?q=is:open+label:bug",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle URL with query params")
	}
	if !strings.Contains(out.String(), "get-hubbed://issues?repo=owner/repo") {
		t.Errorf("expected issues resource URI, got %q", out.String())
	}
}

func TestWebFetchHookInvalidJSON(t *testing.T) {
	var out bytes.Buffer
	handled, err := HandleWebFetchHook([]byte("not json"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to fail-open on invalid JSON")
	}
}

func TestWebFetchHookEmptyURL(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle empty URL")
	}
}

func TestWebFetchHookRawGitHubURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://raw.githubusercontent.com/owner/repo/main/README.md",
			"get-hubbed://contents?path=README.md&repo=owner/repo",
		},
		{
			"https://raw.githubusercontent.com/owner/repo/v1.0.0/src/lib/foo.go",
			"get-hubbed://contents?path=src/lib/foo.go&repo=owner/repo",
		},
		{
			"https://raw.githubusercontent.com/owner/repo/abc1234/file.txt",
			"get-hubbed://contents?path=file.txt&repo=owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			input := makeHookInput("WebFetch", map[string]any{
				"url":    tt.url,
				"prompt": "fetch",
			})
			var out bytes.Buffer
			handled, err := HandleWebFetchHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}

func TestWebFetchHookRawGitHubURLTooFewSegments(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://raw.githubusercontent.com/owner/repo/main",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle raw.githubusercontent.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message for too-few segments, got %q", out.String())
	}
}

func TestWebFetchHookAPIGitHubURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://api.github.com/repos/owner/repo",
			"get-hubbed://repo",
		},
		{
			"https://api.github.com/repos/owner/repo/issues",
			"get-hubbed://issues?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/issues/42",
			"get-hubbed://issues?number=42&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/pulls",
			"get-hubbed://pulls?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/pulls/7",
			"get-hubbed://pulls?number=7&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/contents/src/foo.go",
			"get-hubbed://contents?path=src/foo.go&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/git/trees/main",
			"get-hubbed://tree?repo=owner/repo&ref=main",
		},
		{
			"https://api.github.com/repos/owner/repo/actions/runs",
			"get-hubbed://runs?repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/actions/runs/12345",
			"get-hubbed://runs?run_id=12345&repo=owner/repo",
		},
		{
			"https://api.github.com/repos/owner/repo/compare/main...feature",
			"get-hubbed://compare?repo=owner/repo&base=main&head=feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			input := makeHookInput("WebFetch", map[string]any{
				"url":    tt.url,
				"prompt": "fetch",
			})
			var out bytes.Buffer
			handled, err := HandleWebFetchHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}

func TestWebFetchHookAPIGitHubURLCatchAll(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://api.github.com/repos/owner/repo/stargazers",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle api.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}

func TestWebFetchHookAPIGitHubURLNonRepos(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://api.github.com/users/owner",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle api.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}

func TestWebFetchHookGistURLMappings(t *testing.T) {
	tests := []struct {
		url         string
		resourceURI string
	}{
		{
			"https://gist.github.com/owner/abc123",
			"get-hubbed://gist?id=abc123",
		},
		{
			"https://gist.github.com/owner/deadbeef1234567890",
			"get-hubbed://gist?id=deadbeef1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			input := makeHookInput("WebFetch", map[string]any{
				"url":    tt.url,
				"prompt": "fetch",
			})
			var out bytes.Buffer
			handled, err := HandleWebFetchHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.url)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}

func TestWebFetchHookGistURLTooFewSegments(t *testing.T) {
	input := makeHookInput("WebFetch", map[string]any{
		"url":    "https://gist.github.com/owner",
		"prompt": "fetch",
	})
	var out bytes.Buffer
	handled, err := HandleWebFetchHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle gist.github.com URL")
	}
	if !strings.Contains(out.String(), "GitHub URLs are served by get-hubbed") {
		t.Errorf("expected catch-all message, got %q", out.String())
	}
}
