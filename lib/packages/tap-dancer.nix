{
  pkgs,
  src,
  craneLib,
  purse-first-cli,
  goWorkspaceSrc,
  goVendorHash,
  go,
  rustWorkspaceSrc,
  rustCargoArtifacts,
  batman,
}:

let
  version = "0.2.0";

  mkGoModule = import ../mkGoWorkspaceModule.nix {
    inherit
      pkgs
      goWorkspaceSrc
      goVendorHash
      go
      ;
  };

  tap-dancer-cli = mkGoModule {
    pname = "tap-dancer";
    inherit version;
    subPackages = [ "packages/tap-dancer/go/cmd/tap-dancer" ];

    postInstall = ''
      $out/bin/tap-dancer generate-plugin $out
    '';

    meta = with pkgs.lib; {
      description = "TAP-14 validator and writer toolkit";
      homepage = "https://github.com/amarbel-llc/tap-dancer";
      license = licenses.mit;
    };
  };

  tap-dancer-rust = craneLib.buildPackage {
    src = rustWorkspaceSrc;
    cargoArtifacts = rustCargoArtifacts;
    pname = "tap-dancer";
    version = "0.1.0";
    cargoExtraArgs = "-p tap-dancer";
    strictDeps = true;
  };

  tap-dancer-skill =
    pkgs.runCommand "tap-dancer-skill" { nativeBuildInputs = [ purse-first-cli ]; }
      ''
        ${purse-first-cli}/bin/purse-first generate-plugin \
          --root ${src} \
          --output $out \
          --skills-dir ${src}/skills
      '';

  tap-dancer-bash = pkgs.stdenvNoCC.mkDerivation {
    pname = "tap-dancer-bash";
    inherit version;
    src = "${src}/bash";
    dontBuild = true;
    installPhase = ''
      mkdir -p $out/share/tap-dancer/lib/src
      cp load.bash $out/share/tap-dancer/lib/
      cp src/*.bash $out/share/tap-dancer/lib/src/
      mkdir -p $out/nix-support
      echo 'export TAP_DANCER_LIB="'"$out"'/share/tap-dancer/lib"' > $out/nix-support/setup-hook
    '';
  };

  # ── Test sources ────────────────────────────────────────────────────
  # Filter so the tests-store-path only changes when actual test inputs
  # change. Avoids invalidating the hermetic-tests derivation on
  # unrelated edits.
  tests-src = pkgs.lib.cleanSourceWith {
    src = "${src}/zz-tests_bats";
    filter =
      path: type:
      let
        bn = builtins.baseNameOf path;
      in
      type == "directory"
      || pkgs.lib.hasSuffix ".bats" bn
      || bn == "common.bash"
      || bn == "fence.jsonc";
  };

  # ── Hermetic suite (CI / `nix flake check`) ─────────────────────────
  # Runs the entire suite under batman/fence inside the Nix build
  # sandbox. PATH = nativeBuildInputs only; no host PATH leakage.
  hermetic-tests =
    pkgs.runCommandLocal "tap-dancer-tests"
      {
        nativeBuildInputs = [
          batman
          tap-dancer-cli
          pkgs.coreutils
        ];
      }
      ''
        # Run from the /nix/store tests-src directly: fence's allowRead
        # covers /nix/store, so the bats files and fence.jsonc are
        # readable. /tmp is allowWrite so BATS_TEST_TMPDIR works.
        cd ${tests-src}
        export TAP_DANCER_BIN=${tap-dancer-cli}/bin/tap-dancer
        export BATS_LIB_PATH=${batman}/share/bats
        export TMPDIR=/tmp
        # Nix stdenv sets HOME=/homeless-shelter (read-only). Bun (which
        # powers batman) tries to mkdir under HOME on startup, so point
        # it at a writable path inside the sandbox.
        export HOME="$TMPDIR/home"
        mkdir -p "$HOME"
        # batman parses argv before `--`; bats flags like --tap go after.
        # --diagnostics-stderr surfaces batman's own errors (missing
        # fence.jsonc, spawn failures) in the build log instead of a
        # discarded XDG file.
        batman --diagnostics-stderr . -- --tap
        touch $out
      '';

  # ── Devloop runner ──────────────────────────────────────────────────
  # writeShellApplication gives us a hermetic PATH (runtimeInputs only)
  # but executes in the user's shell. Args after the runner name flow
  # through to batman/bats — so --filter, --filter-tags, file/dir
  # selection all work natively.
  test-runner = pkgs.writeShellApplication {
    name = "tap-dancer-tests";
    runtimeInputs = [
      batman
      tap-dancer-cli
      pkgs.coreutils
    ];
    text = ''
      export TAP_DANCER_BIN=${tap-dancer-cli}/bin/tap-dancer
      export BATS_LIB_PATH=${batman}/share/bats

      # Default target: in-tree bats dir for live devloop iteration.
      # Pass any path/file/dir as args to override.
      if [[ $# -eq 0 ]]; then
        set -- "$PWD/packages/tap-dancer/zz-tests_bats"
      fi

      exec batman --tap "$@"
    '';
  };
in
{
  default = pkgs.symlinkJoin {
    name = "tap-dancer";
    paths = [
      tap-dancer-cli
      tap-dancer-rust
      tap-dancer-skill
      tap-dancer-bash
    ];
    passthru = {
      tests = {
        default = hermetic-tests;
      };
      test-runner = test-runner;
    };
  };
  cli = tap-dancer-cli;
  rust = tap-dancer-rust;
  skill = tap-dancer-skill;
  bash-lib = tap-dancer-bash;
}
