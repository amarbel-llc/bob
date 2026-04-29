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
    set +e
    fence --settings "$fenceConfigPath" -- /bin/sh -c 'echo hello-from-inside-fence'
    rc=$?
    set -e
    echo "=== exit code: $rc ==="
    if [ $rc -ne 0 ]; then
      echo "PROBE-RESULT: fence failed inside Nix sandbox (exit $rc)"
      # Don't fail the build — we want the log either way. Let user inspect.
    else
      echo "PROBE-RESULT: fence succeeded inside Nix sandbox"
    fi
    mkdir -p $out
    {
      echo "exit=$rc"
    } > $out/result.txt
  ''
