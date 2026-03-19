package tools

import (
	"net/url"
	"testing"
)

func TestParseResourceURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantHost string
		wantPath string
		wantRepo string
	}{
		{
			name:     "status",
			uri:      "grit://status",
			wantHost: "status",
			wantPath: "",
		},
		{
			name:     "status with repo_path",
			uri:      "grit://status?repo_path=/tmp/repo",
			wantHost: "status",
			wantPath: "",
			wantRepo: "/tmp/repo",
		},
		{
			name:     "branches",
			uri:      "grit://branches",
			wantHost: "branches",
			wantPath: "",
		},
		{
			name:     "remotes",
			uri:      "grit://remotes",
			wantHost: "remotes",
			wantPath: "",
		},
		{
			name:     "tags",
			uri:      "grit://tags",
			wantHost: "tags",
			wantPath: "",
		},
		{
			name:     "log with params",
			uri:      "grit://log?max_count=5&ref=main&paths=src/main.go,README.md",
			wantHost: "log",
			wantPath: "",
		},
		{
			name:     "commits with ref",
			uri:      "grit://commits/abc123",
			wantHost: "commits",
			wantPath: "/abc123",
		},
		{
			name:     "commits with branch ref",
			uri:      "grit://commits/main",
			wantHost: "commits",
			wantPath: "/main",
		},
		{
			name:     "blame with path",
			uri:      "grit://blame/src/main.go?ref=HEAD&line_range=10,20",
			wantHost: "blame",
			wantPath: "/src/main.go",
		},
		{
			name:     "blame with nested path",
			uri:      "grit://blame/internal/tools/resources.go",
			wantHost: "blame",
			wantPath: "/internal/tools/resources.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.uri)
			if err != nil {
				t.Fatalf("url.Parse(%q) error: %v", tt.uri, err)
			}

			if parsed.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", parsed.Host, tt.wantHost)
			}

			if parsed.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", parsed.Path, tt.wantPath)
			}

			if tt.wantRepo != "" {
				gotRepo := parsed.Query().Get("repo_path")
				if gotRepo != tt.wantRepo {
					t.Errorf("repo_path = %q, want %q", gotRepo, tt.wantRepo)
				}
			}
		})
	}
}
