{
  description = "MCP servers, CLI tools, and development workflow skills";

  inputs = {
    # Fork of upstream nixpkgs. The overlay (`overlays.default`) adds
    # gomod2nix's buildGoApplication / mkGoEnv, bun2nix helpers, and
    # other amarbel-llc additions on top of an upstream pin.
    nixpkgs.url = "github:amarbel-llc/nixpkgs";
    utils.url = "https://flakehub.com/f/numtide/flake-utils/0.1.102";

    # Master nixpkgs pinned directly for go_1_26 availability.
    nixpkgs-master.url = "github:NixOS/nixpkgs/e2dde111aea2c0699531dc616112a96cd55ab8b5";

    purse-first = {
      url = "github:amarbel-llc/purse-first";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    tap = {
      url = "github:amarbel-llc/tap";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    # Build tooling
    gomod2nix = {
      url = "github:amarbel-llc/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      purse-first,
      tap,
      nixpkgs,
      nixpkgs-master,
      utils,
      gomod2nix,
      rust-overlay,
    }:
    let
      mkMarketplace = purse-first.lib.mkMarketplace;

      goWorkspaceSrc = nixpkgs.lib.cleanSourceWith {
        src = ./.;
        filter =
          path: type:
          let
            baseName = builtins.baseNameOf path;
          in
          type == "directory"
          || nixpkgs.lib.hasSuffix ".go" baseName
          || baseName == "go.mod"
          || baseName == "go.sum"
          || baseName == "go.work"
          || baseName == "go.work.sum"
          || nixpkgs.lib.hasSuffix ".scd" baseName
          || nixpkgs.lib.hasSuffix ".tmpl" baseName
          || builtins.match ".*testdata/.*" path != null;
      };

      # Computed after first `go work vendor` — placeholder until then.
      goVendorHash = "sha256-oCjv7v2R513pLQRWnawd49k2CNQEjg3ZLPlKgkw/AGY=";

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
          pkgs = import nixpkgs { inherit system; };
          pkgs-unfree = import nixpkgs {
            inherit system;
            config.allowUnfreePredicate =
              pkg: builtins.elem (nixpkgs.lib.getName pkg) [ "claude-code" ];
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
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ nixpkgs.overlays.default ];
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

          luxPkg = import ./lib/packages/lux.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          batmanPkgs = import ./lib/packages/batman.nix {
            inherit pkgs;
            sandcastle = sandcastlePkg;
            tap-dancer-cli = tap.packages.${system}.tap-dancer-cli;
            src = ./packages/batman;
            fence = pkgs.fence;
            buildZxScriptFromFile = pkgs.buildZxScriptFromFile;
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
            luxPkg
            batmanPkgs
            sandcastlePkg
            andSoCanYouRepoPkg
            potatoPkg
            polkadotsPkg
            probeFenceSandboxPkg
            ;
        };

      marketplaceOutputs = mkMarketplace {
        inherit nixpkgs nixpkgs-master utils;
        name = "bob";
        owner = {
          name = "friedenberg";
          email = "sasha@friedenberg.me";
        };
        description = "MCP servers, CLI tools, and development workflow skills";
        repo = "amarbel-llc/bob";
        purse-first-cli = purse-first;
        plugins =
          system:
          let
            pkgs = buildPackages system;
          in
          [
            pkgs.caldavPkg
            pkgs.luxPkg
          ];
        skills = ./skills;
        packageToml = ./package.toml;
        pluginConfig = builtins.fromJSON (builtins.readFile ./marketplace-config.json);
        devShellPackages =
          system: pkgs: pkgs-master:
          let
            localPkgs = buildPackages system;
          in
          [
            pkgs.neovim
            localPkgs.batmanPkgs.default
            purse-first.packages.${system}.purse-first
            tap.packages.${system}.tap-dancer
          ]
          ++ buildDevShellPackages system;
        devShellHook = ''
          echo "bob - dev environment"
        '';
      };
    in
    nixpkgs.lib.recursiveUpdate marketplaceOutputs (
      utils.lib.eachDefaultSystem (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          localPkgs = buildPackages system;
        in
        {
          packages =
            let
              marketplacePkgs = marketplaceOutputs.packages.${system} or { };
              nonPluginPkgs = [
                localPkgs.batmanPkgs.default
                localPkgs.sandcastlePkg
                localPkgs.andSoCanYouRepoPkg
                localPkgs.potatoPkg
                localPkgs.polkadotsPkg
              ];
            in
            marketplacePkgs
            // {
              default = pkgs.symlinkJoin {
                name = "bob-all";
                paths = [ marketplacePkgs.default ] ++ nonPluginPkgs;
              };
              caldav = localPkgs.caldavPkg;
              lux = localPkgs.luxPkg;
              batman = localPkgs.batmanPkgs.default;
              sandcastle = localPkgs.sandcastlePkg;
              and-so-can-you-repo = localPkgs.andSoCanYouRepoPkg;
              potato = localPkgs.potatoPkg;
              polkadots = localPkgs.polkadotsPkg;
              probe-fence-sandbox = localPkgs.probeFenceSandboxPkg;
              mcp-all = pkgs.symlinkJoin {
                name = "mcp-all";
                paths = [
                  localPkgs.caldavPkg
                  localPkgs.luxPkg
                ];
              };
            };

          devShells = {
            default = marketplaceOutputs.devShells.${system}.default;
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
          };
        }
      )
    );
}
