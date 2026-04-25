#!/bin/bash -e

bats_load_library bats-support
bats_load_library bats-assert
bats_load_library bats-island
bats_load_library bats-emo

require_bin BATMAN_BIN batman
