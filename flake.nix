{
  description = "MCP servers, CLI tools, and development workflow skills";

  inputs = {
    purse-first.url = "github:amarbel-llc/purse-first";

    # Follow purse-first's pins for stable nixpkgs.
    nixpkgs.follows = "purse-first/nixpkgs";
    utils.follows = "purse-first/utils";

    # Master nixpkgs pinned directly for go_1_26 availability.
    nixpkgs-master.url = "github:NixOS/nixpkgs/e2dde111aea2c0699531dc616112a96cd55ab8b5";

    # Build tooling
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    crane.follows = "purse-first/crane";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    fh.url = "https://flakehub.com/f/DeterminateSystems/fh/*.tar.gz";
  };

  outputs =
    {
      self,
      purse-first,
      nixpkgs,
      nixpkgs-master,
      utils,
      gomod2nix,
      crane,
      rust-overlay,
      fh,
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
          || nixpkgs.lib.hasSuffix ".tmpl" baseName;
      };

      # Computed after first `go work vendor` — placeholder until then.
      goVendorHash = "sha256-ukzUWobHBiOS+5kV66SC01NegxeVG3Zb9b+tDSzVcmU=";

      buildDevShellPackages =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
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
          fhPkg = fh.packages.${system}.default;
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

          gritPkg = import ./lib/packages/grit.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          get-hubbed-unwrapped = import ./lib/packages/get-hubbed.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          get-hubbed-wrapped =
            pkgs.runCommand "get-hubbed"
              {
                nativeBuildInputs = [ pkgs.makeWrapper ];
              }
              ''
                mkdir -p $out/bin
                makeWrapper ${get-hubbed-unwrapped}/bin/get-hubbed $out/bin/get-hubbed \
                  --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.gh ]}
                if [ -d "${get-hubbed-unwrapped}/share" ]; then
                  cp -r ${get-hubbed-unwrapped}/share $out/share
                fi
              '';

          luxPkg = import ./lib/packages/lux.nix {
            inherit
              pkgs
              goWorkspaceSrc
              goVendorHash
              go
              ;
          };

          chixPkg = import ./lib/packages/chix.nix {
            inherit
              pkgs
              craneLib
              fhPkg
              rustWorkspaceSrc
              rustCargoArtifacts
              ;
            src = ./packages/chix;
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
            inherit pkgs purse-first-cli;
            sandcastle = sandcastlePkg;
            tap-dancer-cli = tapDancerPkgs.cli;
            src = ./packages/batman;
          };
        in
        {
          inherit
            caldavPkg
            gritPkg
            get-hubbed-wrapped
            luxPkg
            chixPkg
            tapDancerPkgs
            batmanPkgs
            sandcastlePkg
            andSoCanYouRepoPkg
            potatoPkg
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
            pkgs.gritPkg
            pkgs.luxPkg
            pkgs.chixPkg
            pkgs.get-hubbed-wrapped
            pkgs.batmanPkgs.robin
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
            pkgs-master.claude-code
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
                localPkgs.sandcastlePkg
                localPkgs.andSoCanYouRepoPkg
                localPkgs.potatoPkg
              ];
            in
            marketplacePkgs
            // {
              default = pkgs.symlinkJoin {
                name = "bob-all";
                paths = [ marketplacePkgs.default ] ++ nonPluginPkgs;
              };
              caldav = localPkgs.caldavPkg;
              grit = localPkgs.gritPkg;
              get-hubbed = localPkgs.get-hubbed-wrapped;
              lux = localPkgs.luxPkg;
              chix = localPkgs.chixPkg;
              robin = localPkgs.batmanPkgs.robin;
              batman = localPkgs.batmanPkgs.default;
              tap-dancer = localPkgs.tapDancerPkgs.default;
              tap-dancer-bash = localPkgs.tapDancerPkgs.bash-lib;
              sandcastle = localPkgs.sandcastlePkg;
              and-so-can-you-repo = localPkgs.andSoCanYouRepoPkg;
              potato = localPkgs.potatoPkg;
              mcp-all = pkgs.symlinkJoin {
                name = "mcp-all";
                paths = [
                  localPkgs.caldavPkg
                  localPkgs.gritPkg
                  localPkgs.get-hubbed-wrapped
                  localPkgs.luxPkg
                  localPkgs.chixPkg
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
