nix-binary-wrapping(7)

# NAME

nix-binary-wrapping - patterns for wrapping binaries with dependencies in Nix

# DESCRIPTION

Nix-built binaries often need runtime dependencies on PATH, environment
variables set, or post-build code generation. This manpage documents the
wrapping patterns used across amarbel-llc projects.

# MAKEWRAPPER PATTERN

The standard approach uses *symlinkJoin* with *makeWrapper* to prepend
runtime dependencies to PATH:

```
pkgs.symlinkJoin {
  name = "mytool";
  paths = [ script ] ++ runtimeInputs;
  buildInputs = [ pkgs.makeWrapper ];
  postBuild = ''
    wrapProgram $out/bin/mytool --prefix PATH : $out/bin
  '';
}
```

*wrapProgram* creates a shell wrapper that sets environment variables
before exec-ing the real binary. Use *--prefix PATH* to add dependencies
and *--set VAR value* for environment variables.

# WRITESHELLSCRIPTBIN WRAPPER

For pinned packages or simple delegation, use *writeShellScriptBin*
to create a wrapper that execs a specific binary:

```
pkgs.writeShellScriptBin "claude" ''
  exec ${pkgs-claude-pinned.claude-code}/bin/claude "$@"
''
```

This pattern is used for wrapper-pinned packages where a regression in
*nixpkgs-master* requires holding back a specific version. See
*nix-flake-conventions*(7) for the full wrapper-pinned workflow.

# WRITESHELLAPPLICATION

For wrappers with complex logic, *writeShellApplication* provides
automatic *shellcheck* validation and *set -euo pipefail*:

```
pkgs.writeShellApplication {
  name = "bats";
  runtimeInputs = [
    pkgs.bats
    pkgs.coreutils
    pkgs.gawk
    sandcastle
  ];
  text = ''
    # Parse flags, set up environment, exec bats
    ...
  '';
}
```

*runtimeInputs* are automatically prepended to PATH. The wrapper is
shellchecked at build time.

# POSTINSTALL CODE GENERATION

MCP packages use *postInstall* to run the binary itself for build-time
artifact generation:

```
mkGoModule {
  pname = "lux";
  subPackages = [ "packages/lux/cmd/lux" ];
  nativeBuildInputs = [ pkgs.scdoc ];
  postInstall = ''
    $out/bin/lux generate-plugin $out

    mkdir -p $out/share/man/man5
    scdoc < ${src}/doc/lux-config.5.scd \
      > $out/share/man/man5/lux-config.5
  '';
}
```

The *generate-plugin* call writes *plugin.json*, *mappings.json*, and
*hooks/* to the output directory. This is the build-time phase of the
three-mode main pattern used by all MCP packages.

# SCRIPT WRAPPING WITH RUNTIME DEPS

Shell script packages combine *writeScriptBin* with *symlinkJoin* to
bundle the script with its runtime dependencies:

```
let
  script = (pkgs.writeScriptBin name
    (builtins.readFile "${src}/bin/mytool.bash"))
    .overrideAttrs (old: {
      buildCommand = "${old.buildCommand}\n patchShebangs $out";
    });
  runtimeInputs = with pkgs; [ gum gh jq ];
in
pkgs.symlinkJoin {
  inherit name;
  paths = [ script ] ++ runtimeInputs;
  buildInputs = [ pkgs.makeWrapper ];
  postBuild = "wrapProgram $out/bin/${name} --prefix PATH : $out/bin";
}
```

*patchShebangs* rewrites the script's shebang to point at the Nix store
bash. The symlinkJoin then wraps the script with its dependencies.

# PLATFORM-CONDITIONAL DEPENDENCIES

Use *lib.optionals* for dependencies that only exist on one platform:

```
nativeBuildInputs = [
  pkgs.scdoc
] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
  sandcastle-seccomp
];
```

This prevents build failures when a dependency is unavailable on the
other platform (e.g. seccomp on macOS).

# ANTI-PATTERNS

- *Wrapping in the derivation body* — keep wrapping in *postInstall* or
  *postBuild*, not in the main build phase.
- *Hardcoded store paths* — use Nix interpolation, never literal
  */nix/store/...* paths.
- *Double-wrapping* — if a dependency is already wrapped, don't wrap it
  again. Use *symlinkJoin* to combine paths instead.
- *Missing patchShebangs* — scripts with *#!/usr/bin/env bash* shebangs
  need *patchShebangs* to work in the Nix sandbox.

# SEE ALSO

*nix-flake-conventions*(7), *manpages*(7)
