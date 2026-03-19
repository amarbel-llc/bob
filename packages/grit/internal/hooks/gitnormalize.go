package hooks

import "strings"

// normalizeGitCommand strips global git options (-C <path>, --no-pager,
// -c key=val, --git-dir, --work-tree, --bare) from between "git" and
// the subcommand. Non-git commands are returned unchanged.
func normalizeGitCommand(cmd string) string {
	tokens := strings.Fields(cmd)
	if len(tokens) == 0 || tokens[0] != "git" {
		return cmd
	}

	var kept []string
	i := 1
	for i < len(tokens) {
		tok := tokens[i]

		if strings.HasPrefix(tok, "-C=") ||
			strings.HasPrefix(tok, "-c=") ||
			strings.HasPrefix(tok, "--git-dir=") ||
			strings.HasPrefix(tok, "--work-tree=") {
			i++
			continue
		}

		if tok == "-C" || tok == "-c" || tok == "--git-dir" || tok == "--work-tree" {
			i += 2
			continue
		}

		if tok == "--no-pager" || tok == "--bare" {
			i++
			continue
		}

		break
	}

	kept = append(kept, "git")
	kept = append(kept, tokens[i:]...)

	return strings.Join(kept, " ")
}
