# bats integration test lanes for bob.
#
# Wraps `batsLane` from amarbel-llc/bats with bob-specific defaults:
# `bats-libs` on BATS_LIB_PATH, a `BATS_TEST_TIMEOUT` matching the
# devShell `bats` invocations, and bob's binaries (PURSE_FIRST_BIN,
# CLAUDE_BIN) injected via the `binaries` map form.
#
# Each lane is named per test file so failures are immediately legible
# (`bats-validate-plugin-repos` rather than a per-tag derivation). Lanes
# are enumerated explicitly here rather than discovered.
{
  pkgs,
  bats-libs,
  batsLane,
  purseFirstBin,
  claudeBin,
  caldavPkg,
  topLevelBatsSrc,
  batsTestTimeout ? "10",
}:
let
  inherit (pkgs) lib;

  # Lane builder: every bob lane shares the same bats-libs + timeout
  # defaults. Caller supplies the test source, binary map, fixtures,
  # and any extra build inputs.
  mkLane =
    {
      name,
      batsSrc,
      binaries,
      extraEnv ? { },
      nativeBuildInputs ? [ ],
      extraStagedFiles ? [ ],
      testFiles ? [ "*.bats" ],
    }:
    batsLane {
      inherit
        name
        batsSrc
        binaries
        extraStagedFiles
        testFiles
        ;
      batsLibPath = [ bats-libs.batsLibPath ];
      extraEnv = {
        BATS_TEST_TIMEOUT = batsTestTimeout;
      } // extraEnv;
      # `git` is needed by bats-island's setup_test_home (it runs
      # `git init`/`git config`); `jq` is widely used by tests for
      # JSON extraction. Pinned here so every lane gets them without
      # the caller having to remember.
      nativeBuildInputs = nativeBuildInputs ++ [
        pkgs.git
        pkgs.jq
      ];
    };

  # ---- zz-tests_bats/validate_plugin_repos.bats ----
  # Validates that caldav's generated plugin manifest is accepted by
  # both `claude plugin validate` and `purse-first validate`. Needs
  # caldav's share/purse-first/caldav/ layout mounted via
  # PURSE_FIRST_RESULT.
  validatePluginReposLane = mkLane {
    name = "bob-bats-validate-plugin-repos";
    batsSrc = topLevelBatsSrc;
    testFiles = [ "validate_plugin_repos.bats" ];
    binaries = {
      PURSE_FIRST_BIN = {
        base = purseFirstBin;
        name = "purse-first";
      };
      CLAUDE_BIN = {
        base = claudeBin;
        name = "claude";
      };
    };
    extraEnv = {
      PURSE_FIRST_RESULT = "${caldavPkg}";
    };
  };

in
{
  inherit mkLane;

  batsLaneOutputs = {
    bats-validate-plugin-repos = validatePluginReposLane;
    # bats-default aggregates every lane. Each lane's $out is a
    # stamp FILE (not a directory), so symlinkJoin doesn't work;
    # instead, declare every lane as a build dependency of a no-op
    # runCommand. Building bats-default forces nix to realize each
    # lane derivation, which forces each lane's bats suite to run.
    # Add new lanes to `dependsOn` as they migrate.
    bats-default =
      let
        dependsOn = [
          validatePluginReposLane
        ];
      in
      pkgs.runCommand "bob-bats-default" { inherit dependsOn; } ''
        # Touch every dependency so nix preserves the realization
        # ordering in the log even though we don't read their contents.
        for d in $dependsOn; do
          echo "ok: $d"
        done
        touch $out
      '';
  };
}
