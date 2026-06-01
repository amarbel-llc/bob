{
  description = "MCP servers, CLI tools, and development workflow skills";

  inputs = {
    # Fork of upstream nixpkgs. The overlay (`overlays.default`) adds
    # gomod2nix's buildGoApplication / mkGoEnv, bun2nix helpers, and
    # other amarbel-llc additions on top of an upstream pin.
    igloo.url = "github:amarbel-llc/igloo";
    utils.url = "https://flakehub.com/f/numtide/flake-utils/0.1.102";

    # Master nixpkgs pinned directly for go_1_26 availability.
    nixpkgs-master.url = "github:NixOS/nixpkgs/d233902339c02a9c334e7e593de68855ad26c4cb";

    purse-first = {
      url = "github:amarbel-llc/purse-first";
      inputs.igloo.follows = "igloo";
    };

    tap = {
      url = "github:amarbel-llc/tap";
      inputs.igloo.follows = "igloo";
    };

    bats = {
      url = "github:amarbel-llc/bats";
      inputs.igloo.follows = "igloo";
    };

    # Build tooling
    gomod2nix = {
      url = "github:amarbel-llc/gomod2nix";
    };
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "igloo";
    };
  };

  outputs =
    {
      self,
      purse-first,
      tap,
      bats,
      igloo,
      nixpkgs-master,
      utils,
      gomod2nix,
      rust-overlay,
    }:
    let
      goWorkspaceSrc = igloo.lib.cleanSourceWith {
        src = ./.;
        filter =
          path: type:
          let
            baseName = builtins.baseNameOf path;
          in
          type == "directory"
          || igloo.lib.hasSuffix ".go" baseName
          || baseName == "go.mod"
          || baseName == "go.sum"
          || baseName == "go.work"
          || baseName == "go.work.sum"
          || igloo.lib.hasSuffix ".scd" baseName
          || igloo.lib.hasSuffix ".tmpl" baseName
          || builtins.match ".*testdata/.*" path != null;
      };

      # Computed after first `go work vendor` — placeholder until then.
      goVendorHash = "sha256-1Wh2+2IgHtSo42GPXITBR+kAI6kYrJROticL0TzdTn4=";

      # Burnt into the caldav binary via the fork's auto-injected -ldflags
      # (-X main.version / -X main.commit). Single source of truth for
      # caldav's release version; `just bump-version` sed-rewrites this line.
      caldavVersion = "0.1.0";
      # shortRev for clean builds, dirtyShortRev for dirty working trees
      # (so devshell builds show `dirty-abcdef` rather than masquerading as
      # a clean release), "unknown" as a last-resort fallback.
      caldavCommit = self.shortRev or self.dirtyShortRev or "unknown";

      buildDevShellPackages =
        system:
        let
          pkgs = import igloo { inherit system; };
          pkgs-unfree = import igloo {
            inherit system;
            config.allowUnfreePredicate =
              pkg: builtins.elem (igloo.lib.getName pkg) [ "claude-code" ];
          };
          pkgs-master = import nixpkgs-master { inherit system; };
          pkgs-rust = import nixpkgs-master.outPath {
            inherit system;
            overlays = [
              rust-overlay.overlays.default
              (final: prev: {
                rustToolchain = prev.rust-bin.stable.latest.default.override {
                  extensions = [
                    "rust-src"
                    "rustfmt"
                  ];
                };
              })
            ];
          };
        in
        [
          # Go (from pkgs-master for latest)
          pkgs-master.go
          pkgs-master.delve
          pkgs-master.gofumpt
          pkgs-master.golangci-lint
          pkgs-master.golines
          pkgs-master.gopls
          pkgs-master.gotools
          pkgs-master.govulncheck
          (gomod2nix.packages.${system}.default)

          # Rust
          pkgs-rust.rustToolchain
          pkgs-rust.cargo-deny
          pkgs-rust.cargo-edit
          pkgs-rust.cargo-watch
          pkgs-rust.rust-analyzer
          pkgs.openssl
          pkgs.pkg-config

          # Shell
          pkgs.bats
          pkgs.bash-language-server
          pkgs.shellcheck
          pkgs.shfmt
          pkgs.parallel

          # CLI
          pkgs-unfree.claude-code
        ];

      buildPackages =
        system:
        let
          # nixpkgs is the amarbel-llc fork; overlays.default exposes
          # buildGoApplication, mkGoEnv, fence, bun2nix helpers, etc.
          pkgs = import igloo {
            inherit system;
            overlays = [ igloo.overlays.default ];
          };
          pkgs-master = import nixpkgs-master { inherit system; };

          go = pkgs-master.go;

          mkGoModule = import ./lib/mkGoWorkspaceModule.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          sandcastleSeccompPkg =
            if pkgs.stdenv.isLinux then
              import ./lib/packages/sandcastle-seccomp.nix {
                inherit pkgs;
                src = ./packages/sandcastle/seccomp-src;
              }
            else
              null;

          sandcastlePkg = import ./lib/packages/sandcastle.nix {
            inherit pkgs;
            src = ./packages/sandcastle;
            sandcastle-seccomp = sandcastleSeccompPkg;
          };

          andSoCanYouRepoPkg = import ./lib/packages/and-so-can-you-repo.nix {
            inherit pkgs;
            src = ./packages/and-so-can-you-repo;
          };

          potatoPkg = import ./lib/packages/potato.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          caldavPkg = import ./lib/packages/caldav.nix {
            inherit pkgs go;
            src = ./packages/caldav;
            version = caldavVersion;
            commit = caldavCommit;
          };

          batmanPkgs = bats.lib.${system}.mkBats {
            tap-dancer-go = tap.packages.${system}.tap-dancer-go;
          };

          polkadotsPkg = import ./lib/packages/polkadots.nix {
            inherit pkgs;
            src = ./packages/polkadots;
          };

          probeFenceSandboxPkg = import ./lib/packages/probe-fence-sandbox.nix {
            inherit pkgs;
            fence = pkgs.fence;
          };

        in
        {
          inherit
            caldavPkg
            batmanPkgs
            sandcastlePkg
            andSoCanYouRepoPkg
            potatoPkg
            polkadotsPkg
            probeFenceSandboxPkg
            ;
        };
    in
    utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import igloo { inherit system; };
        localPkgs = buildPackages system;

        # nixpkgs view with claude-code's unfree predicate accepted;
        # used to expose the `claude` binary to the validate lane.
        pkgs-unfree = import igloo {
          inherit system;
          config.allowUnfreePredicate =
            pkg: builtins.elem (igloo.lib.getName pkg) [ "claude-code" ];
        };

        # Cleaned bats source filters. Keeping each lane's source
        # store-path stable across unrelated edits — see the
        # eng:wiring-bats-tests skill's flake.nix template. Includes
        # fixtures/ so tests referencing $BATS_TEST_DIRNAME/fixtures/
        # find their data in the nix sandbox.
        topLevelBatsSrc = pkgs.lib.cleanSourceWith {
          src = ./zz-tests_bats;
          filter =
            path: type:
            let
              bn = builtins.baseNameOf path;
            in
            type == "directory"
            || pkgs.lib.hasSuffix ".bats" bn
            || pkgs.lib.hasSuffix ".lua" bn
            || pkgs.lib.hasSuffix ".go" bn
            || bn == "common.bash";
        };

        batsLib = import ./bats.nix {
          inherit pkgs;
          bats-libs = localPkgs.batmanPkgs.bats-libs;
          batsLane = bats.lib.${system}.batsLane;
          purseFirstBin = purse-first.packages.${system}.purse-first;
          claudeBin = pkgs-unfree.claude-code;
          caldavPkg = localPkgs.caldavPkg;
          inherit topLevelBatsSrc;
        };
      in
      {
        packages =
          let
            nonPluginPkgs = [
              localPkgs.batmanPkgs.default
              localPkgs.sandcastlePkg
              localPkgs.andSoCanYouRepoPkg
              localPkgs.potatoPkg
              localPkgs.polkadotsPkg
            ];
          in
          {
            default = pkgs.symlinkJoin {
              name = "bob-all";
              paths = [ localPkgs.caldavPkg ] ++ nonPluginPkgs;
            };
            caldav = localPkgs.caldavPkg;
            batman = localPkgs.batmanPkgs.default;
            bats-libs = localPkgs.batmanPkgs.bats-libs;
            sandcastle = localPkgs.sandcastlePkg;
            and-so-can-you-repo = localPkgs.andSoCanYouRepoPkg;
            potato = localPkgs.potatoPkg;
            polkadots = localPkgs.polkadotsPkg;
            probe-fence-sandbox = localPkgs.probeFenceSandboxPkg;
          }
          // batsLib.batsLaneOutputs;

        devShells = {
          default = pkgs.mkShell {
            packages = [
              pkgs.just
              pkgs.neovim
              localPkgs.batmanPkgs.default
              purse-first.packages.${system}.purse-first
              tap.packages.${system}.tap-dancer
            ]
            ++ buildDevShellPackages system;
            shellHook = ''
              echo "bob - dev environment"
            '';
          };
          go = pkgs.mkShell {
            packages = [
              (import nixpkgs-master { inherit system; }).go
            ];
          };
        };

        checks = {
          # Regression check: fence's bwrap must keep working inside
          # Nix's build sandbox. If this breaks, every batman-via-Nix
          # consumer (passthru.tests, installCheckPhase) breaks too.
          probe-fence-sandbox = localPkgs.probeFenceSandboxPkg;
          bats-default = batsLib.batsLaneOutputs.bats-default;
        };
      }
    );
}
