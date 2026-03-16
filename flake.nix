{
  description = "MCP servers, CLI tools, and development workflow skills";

  inputs = {
    purse-first.url = "github:amarbel-llc/purse-first";

    # Follow purse-first's pins for consistency.
    nixpkgs.follows = "purse-first/nixpkgs";
    nixpkgs-master.follows = "purse-first/nixpkgs-master";
    utils.follows = "purse-first/utils";

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
          || nixpkgs.lib.hasSuffix ".scd" baseName;
      };

      # Computed after first `go work vendor` — placeholder until then.
      goVendorHash = "sha256-PyuIDFgVsx8MjJdCxEMSM6mVHSR/i3H3pZZ/+Fz3c0I=";

      buildDevenvs =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          pkgs-master = import nixpkgs-master { inherit system; };
        in
        {
          go = import ./devenvs/go { inherit pkgs pkgs-master gomod2nix; };
          shell = import ./devenvs/shell { inherit pkgs; };
          bats = import ./devenvs/bats { inherit pkgs; };
          rust = import ./devenvs/rust { inherit pkgs pkgs-master rust-overlay; };
        };

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

          mkGoModule = import ./lib/mkGoWorkspaceModule.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
          };

          sandcastlePkg = import ./lib/packages/sandcastle.nix {
            inherit pkgs;
            src = ./packages/sandcastle;
          };

          andSoCanYouRepoPkg = import ./lib/packages/and-so-can-you-repo.nix {
            inherit pkgs;
            src = ./packages/and-so-can-you-repo;
          };

          potatoPkg = import ./lib/packages/potato.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
          };

          spinclassPkg = import ./lib/packages/spinclass.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
            src = ./packages/spinclass;
          };

          gritPkg = import ./lib/packages/grit.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
          };

          get-hubbed-unwrapped = import ./lib/packages/get-hubbed.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
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
            inherit pkgs goWorkspaceSrc goVendorHash;
          };

          mgpPkg = import ./lib/packages/mgp.nix {
            inherit pkgs goWorkspaceSrc goVendorHash;
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
            gritPkg
            get-hubbed-wrapped
            luxPkg
            mgpPkg
            chixPkg
            tapDancerPkgs
            batmanPkgs
            sandcastlePkg
            andSoCanYouRepoPkg
            potatoPkg
            spinclassPkg
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
            pkgs.gritPkg
            pkgs.luxPkg
            pkgs.mgpPkg
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
            localPkgs.batmanPkgs.default
            purse-first.packages.${system}.purse-first
          ];
        devShellInputsFrom =
          system:
          let
            devenvs = buildDevenvs system;
          in
          [
            devenvs.go.devShells.default
            devenvs.shell.devShells.default
            devenvs.bats.devShells.default
            devenvs.rust.devShells.default
          ];
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
          devenvs = buildDevenvs system;
        in
        {
          packages =
            let
              marketplacePkgs = marketplaceOutputs.packages.${system} or { };
              nonPluginPkgs = [
                localPkgs.sandcastlePkg
                localPkgs.andSoCanYouRepoPkg
                localPkgs.potatoPkg
                localPkgs.spinclassPkg
              ];
            in
            marketplacePkgs
            // {
              default = pkgs.symlinkJoin {
                name = "bob-all";
                paths = [ marketplacePkgs.default ] ++ nonPluginPkgs;
              };
              grit = localPkgs.gritPkg;
              get-hubbed = localPkgs.get-hubbed-wrapped;
              lux = localPkgs.luxPkg;
              mgp = localPkgs.mgpPkg;
              chix = localPkgs.chixPkg;
              robin = localPkgs.batmanPkgs.robin;
              batman = localPkgs.batmanPkgs.default;
              tap-dancer = localPkgs.tapDancerPkgs.default;
              sandcastle = localPkgs.sandcastlePkg;
              and-so-can-you-repo = localPkgs.andSoCanYouRepoPkg;
              potato = localPkgs.potatoPkg;
              spinclass = localPkgs.spinclassPkg;
              mcp-all = pkgs.symlinkJoin {
                name = "mcp-all";
                paths = [
                  localPkgs.gritPkg
                  localPkgs.get-hubbed-wrapped
                  localPkgs.luxPkg
                  localPkgs.mgpPkg
                  localPkgs.chixPkg
                ];
              };
            };

          devShells = {
            default = marketplaceOutputs.devShells.${system}.default;
            go = devenvs.go.devShells.default;
            shell = devenvs.shell.devShells.default;
            bats = devenvs.bats.devShells.default;
            rust = devenvs.rust.devShells.default;
          };
        }
      )
    );
}
