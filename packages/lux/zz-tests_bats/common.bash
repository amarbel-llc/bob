bats_load_library bats-support
bats_load_library bats-assert
bats_load_library bats-assert-additions
bats_load_library bats-island
bats_load_library bats-emo

require_bin LUX_BIN lux

# Put $LUX_BIN's dir at the front of PATH so the bats files can call
# bare `lux` (the nix-lane injection sets LUX_BIN to a store path
# whose directory isn't otherwise on PATH).
if [[ -n ${LUX_BIN:-} ]]; then
  export PATH="$(dirname "$LUX_BIN"):$PATH"
fi
