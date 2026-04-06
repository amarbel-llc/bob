package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
)

type hookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

var githubDomains = map[string]bool{
	"github.com":                true,
	"www.github.com":            true,
	"api.github.com":            true,
	"raw.githubusercontent.com": true,
	"gist.github.com":           true,
}

// matchGitHubURL attempts to match a github.com URL path to a get-hubbed
// resource URI or tool name. Returns (resourceURI, isToolNotResource) or
// ("", false) if no match.
func matchGitHubURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}

	// Only match github.com paths (not API or raw)
	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", false
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(segments) < 2 || segments[0] == "" {
		return "", false
	}

	owner := segments[0]
	repo := segments[1]
	repoSlug := owner + "/" + repo

	// Exact: /{owner}/{repo}
	if len(segments) == 2 {
		return "get-hubbed://repo", false
	}

	section := segments[2]
	switch section {
	case "issues":
		if len(segments) == 3 {
			return fmt.Sprintf("get-hubbed://issues?repo=%s", repoSlug), false
		}
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://issues?number=%s&repo=%s", segments[3], repoSlug), false
		}

	case "pulls":
		if len(segments) == 3 {
			return fmt.Sprintf("get-hubbed://pulls?repo=%s", repoSlug), false
		}

	case "pull":
		if len(segments) >= 4 {
			return fmt.Sprintf("get-hubbed://pulls?number=%s&repo=%s", segments[3], repoSlug), false
		}

	case "blob":
		if len(segments) >= 5 {
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s", filePath, repoSlug), false
		}

	case "tree":
		if len(segments) >= 5 {
			dirPath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://tree?path=%s&repo=%s", dirPath, repoSlug), false
		}
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://tree?repo=%s", repoSlug), false
		}

	case "blame":
		if len(segments) >= 5 {
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://blame?path=%s&repo=%s", filePath, repoSlug), false
		}

	case "commits":
		if len(segments) >= 4 {
			return fmt.Sprintf("get-hubbed://commits?repo=%s", repoSlug), false
		}

	case "actions":
		if len(segments) == 3 {
			return fmt.Sprintf("get-hubbed://runs?repo=%s", repoSlug), false
		}
		if len(segments) >= 5 && segments[3] == "runs" {
			return fmt.Sprintf("get-hubbed://runs?run_id=%s&repo=%s", segments[4], repoSlug), false
		}

	case "compare":
		if len(segments) >= 4 {
			spec := segments[3]
			if parts := strings.SplitN(spec, "...", 2); len(parts) == 2 {
				return fmt.Sprintf("get-hubbed://compare?repo=%s&base=%s&head=%s", repoSlug, parts[0], parts[1]), false
			}
			return fmt.Sprintf("get-hubbed://compare?repo=%s", repoSlug), false
		}
	}

	return "", false
}

// HandleWebFetchHook checks whether a hook input is a WebFetch targeting a
// GitHub URL. Returns (true, nil) if denied, (false, nil) if no match.
// Follows fail-open: parse errors return (false, nil).
func HandleWebFetchHook(input []byte, w io.Writer) (bool, error) {
	var hi hookInput
	if err := json.Unmarshal(input, &hi); err != nil {
		log.Printf("webfetch hook: ignoring decode error (fail-open): %v", err)
		return false, nil
	}

	if hi.ToolName != "WebFetch" {
		return false, nil
	}

	rawURL, _ := hi.ToolInput["url"].(string)
	if rawURL == "" {
		return false, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("webfetch hook: ignoring URL parse error (fail-open): %v", err)
		return false, nil
	}

	if !githubDomains[parsed.Host] {
		return false, nil
	}

	// Try host-specific mapping
	var resourceURI string
	var isTool bool

	switch parsed.Host {
	case "github.com", "www.github.com":
		resourceURI, isTool = matchGitHubURL(rawURL)
	case "api.github.com":
		resourceURI, isTool = matchAPIGitHubURL(parsed)
	case "raw.githubusercontent.com":
		resourceURI, isTool = matchRawGitHubURL(parsed)
	case "gist.github.com":
		resourceURI, isTool = matchGistGitHubURL(parsed)
	}

	if resourceURI != "" {
		if isTool {
			return true, writeToolDeny(w, resourceURI)
		}
		return true, writeResourceDeny(w, resourceURI)
	}

	// Catch-all for any GitHub domain
	return true, writeCatchAllDeny(w)
}

// matchAPIGitHubURL matches api.github.com REST API paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchAPIGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// API paths start with /repos/{owner}/{repo}/...
	if len(segments) < 3 || segments[0] != "repos" || segments[1] == "" {
		return "", false
	}

	owner := segments[1]
	repo := segments[2]
	repoSlug := owner + "/" + repo

	// Exact: /repos/{owner}/{repo}
	if len(segments) == 3 {
		return "get-hubbed://repo", false
	}

	section := segments[3]
	switch section {
	case "issues":
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://issues?repo=%s", repoSlug), false
		}
		if len(segments) == 5 {
			return fmt.Sprintf("get-hubbed://issues?number=%s&repo=%s", segments[4], repoSlug), false
		}

	case "pulls":
		if len(segments) == 4 {
			return fmt.Sprintf("get-hubbed://pulls?repo=%s", repoSlug), false
		}
		if len(segments) == 5 {
			return fmt.Sprintf("get-hubbed://pulls?number=%s&repo=%s", segments[4], repoSlug), false
		}

	case "contents":
		if len(segments) >= 5 {
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s", filePath, repoSlug), false
		}

	case "git":
		if len(segments) >= 6 && segments[4] == "trees" {
			ref := segments[5]
			return fmt.Sprintf("get-hubbed://tree?repo=%s&ref=%s", repoSlug, ref), false
		}

	case "actions":
		if len(segments) == 5 && segments[4] == "runs" {
			return fmt.Sprintf("get-hubbed://runs?repo=%s", repoSlug), false
		}
		if len(segments) == 6 && segments[4] == "runs" {
			return fmt.Sprintf("get-hubbed://runs?run_id=%s&repo=%s", segments[5], repoSlug), false
		}

	case "compare":
		if len(segments) >= 5 {
			spec := segments[4]
			if parts := strings.SplitN(spec, "...", 2); len(parts) == 2 {
				return fmt.Sprintf("get-hubbed://compare?repo=%s&base=%s&head=%s", repoSlug, parts[0], parts[1]), false
			}
			return fmt.Sprintf("get-hubbed://compare?repo=%s", repoSlug), false
		}
	}

	return "", false
}

// matchRawGitHubURL matches raw.githubusercontent.com paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchRawGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Need at least /{owner}/{repo}/{ref}/{path...}
	if len(segments) < 4 || segments[0] == "" {
		return "", false
	}

	owner := segments[0]
	repo := segments[1]
	filePath := strings.Join(segments[3:], "/")

	return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s/%s", filePath, owner, repo), false
}

// matchGistGitHubURL matches gist.github.com paths to get-hubbed
// resource URIs. Returns ("", false) if no match.
func matchGistGitHubURL(parsed *url.URL) (string, bool) {
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Need at least /{owner}/{gist_id}
	if len(segments) < 2 || segments[0] == "" {
		return "", false
	}

	gistID := segments[1]
	return fmt.Sprintf("get-hubbed://gist?id=%s", gistID), false
}

func writeResourceDeny(w io.Writer, resourceURI string) error {
	reason := fmt.Sprintf(
		"DENIED: Use %s instead.\n"+
			"Use get-hubbed for ALL GitHub interactions \u2014 do not use WebFetch or Bash with gh/curl for GitHub.",
		resourceURI,
	)
	return writeDenyJSON(w, reason)
}

func writeToolDeny(w io.Writer, toolDescription string) error {
	reason := fmt.Sprintf(
		"DENIED: Use the get-hubbed %s instead.\n"+
			"Use get-hubbed for ALL GitHub interactions \u2014 do not use WebFetch or Bash with gh/curl for GitHub.\n"+
			"Subagents: use mcp__plugin_get-hubbed_get-hubbed__content-compare",
		toolDescription,
	)
	return writeDenyJSON(w, reason)
}

func writeCatchAllDeny(w io.Writer) error {
	reason := "DENIED: GitHub URLs are served by get-hubbed. Do not use WebFetch for GitHub.\n" +
		"Use get-hubbed for ALL GitHub interactions \u2014 do not use WebFetch or Bash with gh/curl for GitHub.\n\n" +
		"Resources (read-only): get-hubbed://repo, get-hubbed://issues, get-hubbed://pulls, " +
		"get-hubbed://contents, get-hubbed://tree, get-hubbed://blame, get-hubbed://commits, " +
		"get-hubbed://runs, get-hubbed://compare, get-hubbed://gist\n" +
		"Tools (mutations): issue-create, issue-close, issue-comment, pr-create, " +
		"content-search, content-compare, api-get, graphql-query, graphql-mutation"
	return writeDenyJSON(w, reason)
}

func writeDenyJSON(w io.Writer, reason string) error {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(output)
}
