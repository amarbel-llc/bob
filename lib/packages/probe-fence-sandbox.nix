# Probe: does fence's bwrap work inside a Nix build sandbox?
#
# This derivation pulls fence into nativeBuildInputs and tries to run a
# trivial fence-wrapped command at build time. The Nix build sandbox is
# the same environment installCheckPhase runs in — so a successful build
# means installCheck-driven fence/batman tests are viable; a failure
# shows us the exact mode (almost certainly nested-bwrap, à la bob#113).
{ pkgs, fence }:
pkgs.runCommandLocal "probe-fence-sandbox"
  {
    nativeBuildInputs = [ fence ];
    fenceConfig = builtins.toJSON {
      allowPty = false;
      network.allowedDomains = [ ];
      filesystem = {
        strictDenyRead = true;
        allowRead = [ "/nix/store" ];
        allowExecute = [ "/nix/store" ];
        allowWrite = [ "/tmp" ];
      };
      command.useDefaults = true;
    };
    passAsFile = [ "fenceConfig" ];
  }
  ''
    echo "=== probe: fence inside nix build sandbox ==="
    echo "fence: $(command -v fence)"
    echo "fence config:"
    cat "$fenceConfigPath"
    echo
    echo "=== running: fence --settings ... -- echo hello ==="
    rc=0
    fence --settings "$fenceConfigPath" -- /bin/sh -c 'echo hello-from-inside-fence' || rc=$?
    echo "=== exit code: $rc ==="
    mkdir -p $out
    echo "exit=$rc" > $out/result.txt
    if [ "$rc" -ne 0 ]; then
      echo "PROBE-RESULT: fence failed inside Nix sandbox (exit $rc)" >&2
      exit "$rc"
    fi
    echo "PROBE-RESULT: fence succeeded inside Nix sandbox"
  ''
