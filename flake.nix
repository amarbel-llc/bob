{
  description = "MCP servers, CLI tools, and development workflow skills";

  inputs = {
    purse-first.url = "github:amarbel-llc/purse-first";

    # Follow purse-first's pins for stable nixpkgs.
    nixpkgs.follows = "purse-first/nixpkgs";
    utils.follows = "purse-first/utils";

    # Master nixpkgs pinned directly for go_1_26 availability.
    nixpkgs-master.url = "github:NixOS/nixpkgs/e2dde111aea2c0699531dc616112a96cd55ab8b5";

    # Fork carrying pkgs/build-support/bun2nix (buildZxScriptFromFile).
    amarbel-nixpkgs.url = "github:amarbel-llc/nixpkgs/9bad1e489bd4c713da002618bd825372d35430af";

    # Build tooling
    gomod2nix = {
      url = "github:amarbel-llc/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    crane.follows = "purse-first/crane";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    # cacheEntryCreator (Zig binary) consumed by bun2nix's buildZxScriptFromFile.
    bun2nix = {
      url = "github:nix-community/bun2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      purse-first,
      nixpkgs,
      nixpkgs-master,
      amarbel-nixpkgs,
      utils,
      gomod2nix,
      crane,
      rust-overlay,
      bun2nix,
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
      goVendorHash = "sha256-E+U9wODTOqR+rAwab5Oktje7uUsrlwinqYWYXjTPE5c=";

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
          pkgs.nodePackages.bash-language-server
          pkgs.shellcheck
          pkgs.shfmt
          pkgs.parallel

          # CLI
          pkgs-unfree.claude-code
        ];

      buildPackages =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          pkgs-master = import nixpkgs-master { inherit system; };
          pkgs-overlay = import nixpkgs {
            inherit system;
            overlays = [ (import rust-overlay) ];
          };
          craneLib = (crane.mkLib pkgs).overrideToolchain (pkgs-overlay.rust-bin.stable.latest.default);
          rustWorkspaceSrc = craneLib.cleanCargoSource ./.;
          rustCommonArgs = {
            src = rustWorkspaceSrc;
            pname = "rust-workspace-deps";
            version = "0.1.0";
            strictDeps = true;
          };
          rustCargoArtifacts = craneLib.buildDepsOnly rustCommonArgs;
          purse-first-cli = purse-first.packages.${system}.purse-first;

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
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          luxPkg = import ./lib/packages/lux.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          tapDancerPkgs = import ./lib/packages/tap-dancer.nix {
            inherit
              pkgs
              craneLib
              purse-first-cli
              goWorkspaceSrc
              goVendorHash
              go
              rustWorkspaceSrc
              rustCargoArtifacts
              ;
            src = ./packages/tap-dancer;
          };

          batmanPkgs = import ./lib/packages/batman.nix {
            inherit pkgs;
            sandcastle = sandcastlePkg;
            tap-dancer-cli = tapDancerPkgs.cli;
            src = ./packages/batman;
          };

          polkadotsPkg = import ./lib/packages/polkadots.nix {
            inherit pkgs;
            src = ./packages/polkadots;
          };
        in
        {
          inherit
            caldavPkg
            luxPkg
            tapDancerPkgs
            batmanPkgs
            sandcastlePkg
            andSoCanYouRepoPkg
            potatoPkg
            polkadotsPkg
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
            pkgs.tapDancerPkgs.default
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
              tap-dancer = localPkgs.tapDancerPkgs.default;
              tap-dancer-bash = localPkgs.tapDancerPkgs.bash-lib;
              sandcastle = localPkgs.sandcastlePkg;
              and-so-can-you-repo = localPkgs.andSoCanYouRepoPkg;
              potato = localPkgs.potatoPkg;
              polkadots = localPkgs.polkadotsPkg;
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
        }
      )
    );
}
