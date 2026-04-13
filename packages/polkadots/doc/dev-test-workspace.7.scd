dev-test-workspace(7)

# NAME

dev-test-workspace - interactive testing pattern using temporary direnv workspaces

# DESCRIPTION

The dev-test-workspace pattern provides a way to interactively test a
locally-built tool in an isolated environment. The workflow builds the tool,
creates a temporary workspace with a *.envrc* that places the built binary on
PATH, runs *direnv allow*, drops the user into an interactive shell, and
cleans up on exit while preserving logs.

This pattern is used for manual verification of CLI tools, MCP servers, and
other interactive programs where automated tests cannot capture the full
user experience.

# WORKFLOW

The pattern follows five phases:

. *Build* — compile or assemble the tool under test.
. *Bootstrap* — create a temp directory with a *.envrc* that sets up the
  environment (PATH, config, log paths).
. *Activate* — run *direnv allow* to enable the environment.
. *Shell* — spawn an interactive shell inheriting the workspace.
. *Teardown* — on shell exit, capture logs and output, then remove the
  workspace (or preserve it on failure).

# BOOTSTRAP PHASE

Create a temporary workspace with a *.envrc* that injects the built binary:

```
workspace="$(mktemp -d)"
trap 'cleanup "$workspace"' EXIT INT TERM

cat > "$workspace/.envrc" <<EOF
export PATH="${bin_dir}:\$PATH"
export LOG_FILE="${workspace}/session.log"
export XDG_CONFIG_HOME="${workspace}/config"
export XDG_DATA_HOME="${workspace}/data"
EOF
```

The *.envrc* should:

- Prepend the build output directory to PATH.
- Set a log file path inside the workspace for capture.
- Override XDG directories to isolate config and data.
- Set any tool-specific environment variables (API keys, config paths).

# ACTIVATION PHASE

Run *direnv allow* against the workspace to mark the *.envrc* as trusted:

```
direnv allow "$workspace"
```

This must happen before spawning the shell. Without it, direnv will
refuse to load the environment.

# SHELL PHASE

Spawn the user's shell with the workspace as the working directory:

```
shell="${SHELL:-/bin/bash}"
(cd "$workspace" && exec "$shell")
exit_code=$?
```

The shell inherits stdio so the user can interact normally. When the
user exits the shell (via *exit* or Ctrl-D), execution continues to
teardown.

For *zx*-based implementations, use Node's *spawn* with *stdio: inherit*:

```
const shell = process.env.SHELL ?? '/bin/bash'
const child = spawn(shell, [], {
  stdio: 'inherit',
  cwd: workspace,
  env: { ...process.env, PATH: `${binDir}:${process.env.PATH}` },
})
await new Promise((resolve) => child.on('close', resolve))
```

# TEARDOWN PHASE

On exit, capture any logs or output before removing the workspace:

```
cleanup() {
  local ws="$1"

  if [[ -f "$ws/session.log" ]]; then
    echo "--- session log ---"
    cat "$ws/session.log"
    echo "---"
  fi

  # Preserve workspace on failure for debugging
  if [[ ${exit_code:-0} -ne 0 ]]; then
    echo "workspace preserved at: $ws"
    return
  fi

  rm -rf "$ws"
}
```

Register the cleanup function via *trap* for EXIT, INT, and TERM signals
to ensure cleanup even on Ctrl-C.

# JUSTFILE INTEGRATION

Expose the workflow as a justfile recipe that depends on the build step:

```
dev: build-go
    zx bin/dev.mjs

test-interactive: build-go
    bin/test-workspace.bash
```

The build dependency ensures the binary is current before the workspace
is created.

# MCP SERVER TESTING

For MCP servers, the bootstrap phase can also write a
*.claude/settings.local.json* that points at the local binary:

```
mkdir -p "$workspace/.claude"
cat > "$workspace/.claude/settings.local.json" <<EOF
{
  "mcpServers": {
    "mytool": {
      "command": "${bin_dir}/mytool"
    }
  }
}
EOF
```

This allows testing the MCP server with a real Claude Code session in
the workspace.

# LOG CAPTURE

Tools under test should write structured logs to the path specified by
the environment. After the shell exits, logs can be parsed for analysis:

```
if [[ -f "$workspace/session.log" ]]; then
  # Parse JSONL logs
  jq -r '.level + ": " + .msg' "$workspace/session.log"

  # Or copy for later analysis
  cp "$workspace/session.log" "test-results/$(date +%s).log"
fi
```

For JSONL log formats, *jq* is the standard tool for extraction and
filtering.

# ANTI-PATTERNS

- *Skipping direnv allow* — without it, the environment won't load and
  the binary won't be on PATH.
- *Hardcoded /tmp paths* — always use *mktemp -d* for unique workspace
  directories.
- *Missing signal handlers* — without trap on INT/TERM, Ctrl-C leaves
  orphaned workspaces.
- *Ignoring exit codes* — preserve the workspace on non-zero exit to
  allow post-mortem debugging.
- *Building inside the workspace* — build before creating the workspace.
  The workspace is for testing, not building.

# SEE ALSO

*justfile-design-patterns*(7), *manpages*(7)
