sandcastle(1)

# NAME

sandcastle - sandbox runtime for wrapping security boundaries around processes

# SYNOPSIS

*sandcastle* [_options_] [--] _command_ [_args_...]

# DESCRIPTION

Sandcastle runs arbitrary commands inside a sandboxed environment with
filesystem and network restrictions. On Linux it uses *bubblewrap*(1) for
namespace isolation with seccomp filtering. On macOS it uses the native
*sandbox-exec*(1) facility.

When no config file is found, sandcastle runs in maximally-permissive mode
(no restrictions). Restrictions are defined in a JSON configuration file
(see *CONFIGURATION*).

Sandcastle cannot be nested — running sandcastle inside sandcastle will
fail with a bridge socket error.

# OPTIONS

*-d*, *--debug*
	Enable debug logging. Prints the bwrap command and resolved
	configuration to stderr.

*-s*, *--config* _path_
	Path to a JSON configuration file. Defaults to
	_~/.srt-settings.json_.

*--shell* _shell_
	Shell to execute the command with. When set, the command and
	arguments are wrapped as: _shell_ *-c* '_quoted args_'.

*--tmpdir* _path_
	Override the temporary directory used inside the sandbox.

*--no-tempdir-cleanup*
	Do not remove the temporary directory on exit. Useful for
	inspecting sandbox state after a run.

*--control-fd* _fd_
	Read configuration updates from a file descriptor as JSON lines.

# CONFIGURATION

The configuration file is JSON with two top-level sections: *filesystem*
and *network*.

## Filesystem

*denyRead* = [_path_, ...]
	Paths blocked from reading inside the sandbox.

*allowWrite* = [_path_, ...]
	Paths explicitly allowed for writing. Defaults to none.

*denyWrite* = [_path_, ...]
	Paths blocked from writing.

*allowGitConfig* = _true_ | _false_
	Allow writes to _.git/config_. Defaults to false.

## Network

*allowedDomains* = [_domain_, ...]
	Domain allowlist. When empty, no domains are allowed by default.

*deniedDomains* = [_domain_, ...]
	Domain denylist.

*allowAllUnixSockets* = _true_ | _false_
	Allow all Unix socket connections. Required for binaries that use
	async runtimes (tokio, async-std) which create Unix socketpairs
	during initialization for signal handling. Without this, such
	binaries will panic with "failed to create UnixStream:
	PermissionDenied" before any user code runs.

*allowLocalBinding* = _true_ | _false_
	Allow binding to localhost ports.

*httpProxyPort* = _port_
	External HTTP proxy port. Defaults to 3128.

*socksProxyPort* = _port_
	External SOCKS proxy port. Defaults to 1080.

## Example

```
{
  "filesystem": {
    "denyRead": ["$HOME/.ssh", "$HOME/.aws", "$HOME/.gnupg"],
    "allowWrite": ["/tmp", "/private/tmp"],
    "denyWrite": []
  },
  "network": {
    "allowedDomains": [],
    "deniedDomains": [],
    "allowLocalBinding": false
  }
}
```

# PLATFORM NOTES

On Linux, sandcastle requires *bubblewrap*(1) and uses pre-generated
seccomp BPF filters for syscall restriction. Network traffic is proxied
through socat bridges (HTTP on port 3128, SOCKS on port 1080).

On macOS, sandcastle uses the native sandbox-exec facility with glob-based
path filtering. Sandbox violations are logged to the system log.

# TROUBLESHOOTING

## sandbox-exec: sandbox_apply: Operation not permitted (macOS, exit 71)

The generated sandbox profile could not be applied. Sandcastle surfaces a
diagnostic hint pointing here when a child exits with code 71 on macOS.

Common causes:

- A malformed rule in the sandbox profile (a syntactically invalid TinyScheme
  expression emitted by the profile generator). Run with *-d* / *--debug*
  to see the full profile and check for unbalanced parens or unknown
  predicates.
- A profile rule references a path or Mach service that is denied by
  System Integrity Protection, by the binary's entitlements, or by the
  hardened runtime. Non-Apple-signed binaries can be subject to stricter
  defaults than the profile would otherwise allow.
- Running inside an environment that already applies a sandbox (e.g. a
  nested sandcastle invocation, or an outer sandbox-exec from a CI agent).
  Nested sandbox application is not supported.

If the failure is specific to a multi-process application (Firefox,
Chromium-based browsers), see also the BiDi / Mach IPC discussion tracked
in the project issue tracker — those cases require caller-supplied Mach
global-name allowlists rather than a profile fix.

## bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted (Linux)

On Ubuntu 23.10 and later — including 24.04 LTS by default — the sysctl
*kernel.apparmor_restrict_unprivileged_userns* is set to _1_. Unprivileged
processes can then only create user namespaces if they run under an
AppArmor profile that includes a *userns,* rule, or if they have
*CAP_SYS_ADMIN*. Sandcastle's bwrap invocation satisfies neither on a
Nix-installed bwrap, so the user/network namespaces cannot be fully
configured and bwrap fails during loopback setup.

Workarounds, roughly in order of invasiveness:

- Temporarily set the sysctl to _0_ on the host:
  *sysctl -w kernel.apparmor_restrict_unprivileged_userns=0*. Affects the
  entire host and is not appropriate for shared or production machines.
- Add an AppArmor profile for the Nix-installed bwrap binary that includes
  the *userns,* rule. Distro-specific.
- Run sandcastle with a configuration that does not require network
  namespace isolation, so *--unshare-net* is not passed to bwrap. The
  exact config surface for this is in flight; see the project issue
  tracker.

## Sandcastle cannot be nested

Running *sandcastle* inside another *sandcastle* invocation fails with a
bridge socket error. The outer sandbox's seccomp filter blocks the inner
invocation's setup calls. Use *--no-sandbox* (via *batman*(1) / *bats* or
equivalent) on the inner layer if a nested test harness needs direct
command execution.

# EXAMPLE

Run bats tests inside a sandbox that blocks sensitive directories:

```
sandcastle --shell bash --config test-sandbox.json -- bats --tap tests/
```

Run a command without sandboxing the temporary directory cleanup:

```
sandcastle --no-tempdir-cleanup -- my-command --flag
```

# SEE ALSO

*bats-island*(7)
