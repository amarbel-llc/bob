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
	Allow all Unix socket connections (macOS only).

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
