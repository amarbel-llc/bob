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
			ref := segments[3]
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://contents?path=%s&repo=%s&ref=%s", filePath, repoSlug, ref), false
		}

	case "tree":
		if len(segments) >= 5 {
			ref := segments[3]
			dirPath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://tree?path=%s&repo=%s&ref=%s", dirPath, repoSlug, ref), false
		}
		if len(segments) == 4 {
			ref := segments[3]
			return fmt.Sprintf("get-hubbed://tree?repo=%s&ref=%s", repoSlug, ref), false
		}

	case "blame":
		if len(segments) >= 5 {
			ref := segments[3]
			filePath := strings.Join(segments[4:], "/")
			return fmt.Sprintf("get-hubbed://blame?path=%s&repo=%s&ref=%s", filePath, repoSlug, ref), false
		}

	case "commits":
		if len(segments) >= 4 {
			ref := segments[3]
			return fmt.Sprintf("get-hubbed://commits?repo=%s&ref=%s", repoSlug, ref), false
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
			return fmt.Sprintf("content-compare tool with repo=%s, base and head from %s", repoSlug, segments[3]), true
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

	// Try specific mapping
	resourceURI, isTool := matchGitHubURL(rawURL)
	if resourceURI != "" {
		if isTool {
			return true, writeToolDeny(w, resourceURI)
		}
		return true, writeResourceDeny(w, resourceURI)
	}

	// Catch-all for any GitHub domain
	return true, writeCatchAllDeny(w)
}

func writeResourceDeny(w io.Writer, resourceURI string) error {
	reason := fmt.Sprintf(
		"DENIED: Use %s instead.\n"+
			"Use get-hubbed for ALL GitHub interactions \u2014 do not use WebFetch or Bash with gh/curl for GitHub.\n"+
			"Subagents: use mcp__plugin_get-hubbed_get-hubbed__resource-read with uri %s",
		resourceURI, resourceURI,
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
		"get-hubbed://contents, get-hubbed://tree, get-hubbed://blame, get-hubbed://commits, get-hubbed://runs\n" +
		"Tools (mutations): issue-create, issue-close, issue-comment, pr-create, " +
		"content-search, content-compare, api-get, graphql-query, graphql-mutation\n" +
		"Discovery: resource-templates, resource-read\n" +
		"Subagents: mcp__plugin_get-hubbed_get-hubbed__resource-read or mcp__plugin_get-hubbed_get-hubbed__<tool_name>"
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
