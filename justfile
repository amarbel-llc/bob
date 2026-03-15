
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
cmd_nix_dev_go := "nix develop " + justfile_directory() + "#go --command "
cmd_batman_bats := justfile_directory() + "/result-batman/bin/bats"

default: build test

# Build all packages (default = marketplace bundle)
build:
    nix build

# Build individual packages
build-grit:
    nix build .#grit

build-lux:
    nix build .#lux

build-get-hubbed:
    nix build .#get-hubbed

build-chix:
    nix build .#chix

build-robin:
    nix build .#robin

build-spinclass:
    nix build .#spinclass

build-batman:
    nix build .#batman -o result-batman

cmd-tap-dancer := join(justfile_directory(), "./packages/tap-dancer/go/cmd/tap-dancer")
tap-dancer-go-test := "go run " + cmd-tap-dancer + " go-test -skip-empty"
tap-dancer-cargo-test := "go run " + cmd-tap-dancer + " cargo-test -skip-empty"

# Test individual Go packages
test-grit:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/grit/...

test-get-hubbed:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/get-hubbed/...

test-lux:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/lux/...

test-spinclass:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/spinclass/...

test-tap-dancer-go:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/tap-dancer/go/...

# Test Rust packages
[working-directory: 'packages/chix']
test-chix:
  {{cmd_nix_dev}} {{tap-dancer-cargo-test}} test

[working-directory: 'packages/tap-dancer/rust']
test-tap-dancer-rust:
    {{cmd_nix_dev}} {{tap-dancer-cargo-test}} test

# Run all Go tests
test-go:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./...

# Format code
fmt:
    {{cmd_nix_dev}} go fmt ./...

# Lint code
lint:
    {{cmd_nix_dev}} go vet ./...

# Regenerate workspace vendor directory after dependency changes
vendor:
    {{cmd_nix_dev_go}} go work vendor

# Update go dependencies, tidy all modules, and re-vendor
deps:
    {{cmd_nix_dev_go}} go work sync
    {{cmd_nix_dev_go}} go work vendor

# Recompute goVendorHash in flake.nix from the local vendor directory
vendor-hash:
    #!/usr/bin/env bash
    set -euo pipefail
    hash=$(nix hash path vendor/)
    sed -i -E 's|(goVendorHash = )"sha256-[^"]+";|\1"'"$hash"'";|' flake.nix
    echo "updated goVendorHash to $hash"

# Sync Go vendor directory and Nix vendor hash after any Go module change.
# Use after: adding/removing deps, changing module paths, editing go.mod/go.work.
# Uses the standalone Go devShell (#go) to avoid the chicken-and-egg problem
# where the default devShell requires a successful nix build, but nix build
# requires a correct vendor hash.
go-mod-sync:
    #!/usr/bin/env bash
    set -euo pipefail
    nix_go="nix develop {{justfile_directory()}}#go --command"
    echo "==> go work sync"
    $nix_go go work sync
    echo "==> go work vendor"
    $nix_go go work vendor
    echo "==> recomputing goVendorHash"
    hash=$(nix hash path vendor/)
    sed -i -E 's|(goVendorHash = )"sha256-[^"]+";|\1"'"$hash"'";|' flake.nix
    echo "updated goVendorHash to $hash"
    echo "==> nix build (verify)"
    nix build --show-trace
    echo "==> done"

# Run integration tests
test-integration: build-batman
    nix build
    {{cmd_nix_dev}} {{cmd_batman_bats}} --jobs {{num_cpus()}} \
      zz-tests_bats/validate_plugin_repos.bats

# Run lifecycle tests
test-lifecycle: build-batman
    nix build
    {{cmd_nix_dev}} {{cmd_batman_bats}} --jobs {{num_cpus()}} zz-tests_bats/lux_service.bats

# Validate own plugin manifest
validate:
    {{cmd_nix_dev}} go run ./dummies/go/cmd/... validate .claude-plugin/plugin.json || true

test-spinclass-bats: build-batman
    nix build .#spinclass
    SPINCLASS_BIN={{justfile_directory()}}/result/bin/spinclass PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/spinclass/zz-tests_bats/test

test-grit-bats: build-batman
    nix build .#grit
    GRIT_BIN={{justfile_directory()}}/result/bin/grit PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/grit/zz-tests_bats/test

test-tap-dancer-bats: build-batman
    nix build .#tap-dancer
    TAP_DANCER_BIN={{justfile_directory()}}/result/bin/tap-dancer PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/tap-dancer/zz-tests_bats/test

test-batman-bats: build-batman
    BATS_WRAPPER={{justfile_directory()}}/result-batman/bin/bats PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/batman/zz-tests_bats/test

# Bump version for a package. Usage: just bump-version grit 0.2.0
bump-version package version:
  #!/usr/bin/env bash
  set -euo pipefail
  jq --arg pkg "{{package}}" --arg ver "{{version}}" \
    '.plugins[$pkg].version = $ver' marketplace-config.json > marketplace-config.json.tmp
  mv marketplace-config.json.tmp marketplace-config.json
  gum log --level info "{{package}}: version bumped to {{version}}"
  gum log --level warn "Remember to update Cargo.toml and SKILL.md frontmatter if applicable"

# Build dummy Go MCP servers
build-dummies-go:
    {{cmd_nix_dev}} go build -o build/ ./dummies/go/cmd/...

test: \
    test-batman-bats \
    test-chix \
    test-get-hubbed \
    test-go \
    test-grit \
    test-grit-bats \
    test-integration \
    test-lux \
    test-spinclass \
    test-spinclass-bats \
    test-tap-dancer-bats \
    test-tap-dancer-go \
    test-tap-dancer-rust

update: update-nix

update-nix:
    nix flake update

# Clean build artifacts
clean:
    rm -rf build/
    rm -rf result result-batman
