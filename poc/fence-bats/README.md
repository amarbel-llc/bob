# fence-bats POC

**Status: incomplete.** This directory proves that `bats` + `fence` can co-run
in a hermetic Nix devshell with all 6 (well, 2 here) tests green via MCP-driven
agent invocation. It does **not** yet prove that the same combination works
inside the bob repo's actual devshell when invoked via `just test-batman-fence`.

## What this proves

The hermetic flake here (`flake.nix`) declares only `bats`, `fence`, `curl`,
`coreutils`, `bash` in its devShell. With user-profile `PATH` entries filtered
out (so the dedicated devshell's `pkgs.bats` wins instead of a wrapped `bats`
from `~/eng/result/bin/`), the two probe tests in `probe.bats` pass under
`fence` invoked via the dev-shell `bats`.

Reproducer (run from this directory):

```sh
nix flake lock
nix develop --command bash -c '
  PATH=$(echo "$PATH" | tr ":" "\n" | grep -Ev "^/home/" | tr "\n" ":")
  bats --tap probe.bats
'
```

Expected: `1..2` followed by two `ok` lines.

## What this does NOT prove

When the same fence + bats setup is wired into `packages/batman/` and run via
the bob repo's `just test-batman-fence` recipe, the 2 fence-spawn tests still
fail consistently:

- **MCP-context bats**: `Error: failed to initialize sandbox: failed to start
  HTTP proxy: failed to listen: listen tcp 127.0.0.1:0: bind: operation not
  permitted` even after filtering `/home/*` from PATH.
- **Terminal bats with the wrapper + `--no-sandbox`**: passes locally with a
  permissive `fence.jsonc`. Tightening fence config bisected to
  `strictDenyRead: true` + `allowExecute: ["/nix/store"]`. Adding
  `runtimeExecPolicy: "argv"` was bisected and confirmed not to be the
  failure cause.

The bob devshell explicitly includes `localPkgs.batmanPkgs.default`
(`flake.nix:274`), which provides a wrapped `bats` whose default
`sandbox=true` invokes sandcastle's `bwrap` around the test command. That
sandcastle bwrap **nests inside fence's bwrap** and fence fails to set up its
sockets. Even after PATH-filtering `/home/*` to bypass the user-profile
wrapper, the dev-shell's own batman wrapper still wins on PATH.

## Open follow-ups

1. [`amarbel-llc/bob#113`](https://github.com/amarbel-llc/bob/issues/113) --
   fence-in-bats integration in the bob dev shell (sandbox nesting,
   wrapped-bats shadowing, MCP-context-specific bind EPERM).
2. [`amarbel-llc/eng#42`](https://github.com/amarbel-llc/eng/issues/42) --
   remove `~/eng/result/bin` from PATH; audit how PATH is built and
   what's included.
3. Decide whether the v0 batman recipe should reach for a literal
   `${pkgs.bats}/bin/bats` to bypass any wrapper, or whether batman should
   stop shipping a sandcastle-wrapped `bats` to begin with.

## Files

- `flake.nix` -- hermetic devshell with bats + fence + curl + coreutils + bash.
- `flake.lock` -- pinned for reproducibility.
- `fence.jsonc` -- maximally permissive fence config (everything = `/`).
- `probe.bats` -- 2 trivial tests: fence echo, fence + curl to example.com.
