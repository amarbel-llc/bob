nix-flake-conventions(7)

# NAME

nix-flake-conventions - Nix flake input and devShell patterns for amarbel-llc projects

# DESCRIPTION

Every Nix flake in amarbel-llc projects follows a stable-first convention
for nixpkgs inputs and a master-first pattern for devShells. These
conventions ensure reproducible builds while keeping development tooling
current.

# STABLE-FIRST NIXPKGS

Every flake declares two nixpkgs inputs:

*nixpkgs*
	Stable branch. Used for runtimes, core tools, and production
	dependencies. Tracks a NixOS release branch (e.g. *nixos-25.11*).

*nixpkgs-master*
	Master/unstable. Used for LSPs, linters, formatters, and tools
	where the latest version matters. Pinned to a specific SHA.

*utils*
	*flake-utils* from FlakeHub. Provides *eachDefaultSystem* and
	related helpers.

Variables in the flake body follow the convention:

```
pkgs = import nixpkgs { inherit system; };
pkgs-master = import nixpkgs-master { inherit system; };
```

# PIN FORMS

The top-level *eng* flake uses two pin forms:

*Branch ref* (stable):

```
nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
```

Bumps automatically via *nix flake update* whenever the release branch
advances.

*SHA literal* (master):

```
nixpkgs-master.url = "github:NixOS/nixpkgs/<sha>";
```

Only moves via *just bump-nixpkgs*, which fetches the new SHA, edits the
literal, refreshes the lockfile, builds to verify, and stages for review.

# FOLLOWS CONVENTION

Child repos follow their parent's stable pin to avoid duplicate nixpkgs
evaluations:

```
inputs = {
  purse-first.url = "github:amarbel-llc/purse-first";
  nixpkgs.follows = "purse-first/nixpkgs";
  utils.follows = "purse-first/utils";
  nixpkgs-master.url = "github:NixOS/nixpkgs/<sha>";
};
```

Rules:

- *nixpkgs* and *utils* use *follows* to inherit the parent's pin.
- *nixpkgs-master* is pinned directly (SHA literal), not followed.
  Each repo may need a different master revision.
- Never follow *nixpkgs-master* across repos unless they share a build
  constraint.

# DEVSHELL MASTER-FIRST

DevShells pull development tools from *pkgs-master* (latest versions)
while core runtimes may come from either source:

```
devShells.default = pkgs.mkShell {
  packages = [
    pkgs-master.go
    pkgs-master.gopls
    pkgs-master.gofumpt
    pkgs-master.golangci-lint
    pkgs.bats
    pkgs.shellcheck
    pkgs.shfmt
  ];
};
```

The split ensures developers get the latest LSP features and linter
rules without destabilizing production builds.

# WRAPPER-PINNED PACKAGES

When a specific package from *nixpkgs-master* has a regression, pin it
via a dedicated frozen flake input consumed by a home-manager wrapper:

. Add a new input with a SHA literal pointing at the last known-good
  revision: *nixpkgs-<name>-pinned.url = "github:NixOS/nixpkgs/<sha>"*.
. Import it as *pkgs-<name>-pinned* in *home/special-args.nix*.
. Define a *writeShellScriptBin* wrapper in *home/wrappers.nix* that
  *exec*s the binary from the pinned tree.
. Remove the package from *systems/common/default.nix* to avoid two
  copies on PATH.

*just bump-nixpkgs* leaves pinned inputs alone — its *sed* is anchored
to *nixpkgs-master.url* specifically.

# EXAMPLE

Minimal flake following all conventions:

```
{
  inputs = {
    parent.url = "github:amarbel-llc/parent-repo";
    nixpkgs.follows = "parent/nixpkgs";
    utils.follows = "parent/utils";
    nixpkgs-master.url = "github:NixOS/nixpkgs/<sha>";
  };

  outputs = { self, parent, nixpkgs, nixpkgs-master, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        pkgs-master = import nixpkgs-master { inherit system; };
      in {
        packages.default = pkgs.stdenvNoCC.mkDerivation { ... };
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs-master.go
            pkgs-master.gopls
            pkgs.bats
          ];
        };
      });
}
```

# SEE ALSO

*nix-binary-wrapping*(7), *manpages*(7)
