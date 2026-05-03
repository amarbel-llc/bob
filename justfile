
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
cmd_nix_dev_go := "nix develop " + justfile_directory() + "#go --command "

default: build test vendor

# Build all packages (default = marketplace bundle)
build:
    nix build

# Build individual packages
build-lux:
    nix build .#lux

build-caldav:
    nix build .#caldav

# Realize the batman bundle into the nix store and print its path.
build-batman:
    @nix build --no-link --print-out-paths .#batman

tap-dancer-go-test := "tap-dancer go-test -skip-empty"
tap-dancer-cargo-test := "tap-dancer cargo-test -skip-empty"

# Test individual Go packages
test-lux:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/lux/...

test-caldav:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/caldav/...

# Run all Go tests
test-go:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./...

# Format code
fmt:
    {{cmd_nix_dev}} go fmt ./...

# Lint code (vet each module listed in go.work; `go vet ./...` from the workspace
# root sees no modules)
lint:
    {{cmd_nix_dev}} bash -c 'set -euo pipefail; for mod in $(go work edit -json | jq -r ".Use[].DiskPath"); do (cd "$mod" && go vet ./...); done'

# Sed-rewrite caldavVersion in flake.nix to the given semver. The version
# string is burnt into the binary at build time via the fork's auto-injected
# -X main.version ldflag (see lib/packages/caldav.nix), so flake.nix is the
# single source of truth. No-op if already at the target version.
# Usage: just bump-caldav-version 0.1.1
bump-caldav-version new_version:
    #!/usr/bin/env bash
    set -euo pipefail
    current=$(grep 'caldavVersion = ' flake.nix | sed 's/.*"\(.*\)".*/\1/')
    if [[ "$current" == "{{new_version}}" ]]; then
      echo "already at {{new_version}}"
      exit 0
    fi
    sed -i.bak 's/caldavVersion = "'"$current"'"/caldavVersion = "{{new_version}}"/' flake.nix && rm flake.nix.bak
    echo "bumped caldavVersion: $current → {{new_version}}"

vendor: vendor-go vendor-hash

# Regenerate workspace vendor directory after dependency changes
# Update go dependencies, tidy all modules, and re-vendor
vendor-go:
    {{cmd_nix_dev_go}} go work sync
    {{cmd_nix_dev_go}} go work vendor

# Recompute goVendorHash in flake.nix from the local vendor directory
vendor-hash:
    #!/usr/bin/env bash
    set -euo pipefail
    hash=$(nix hash path vendor/)
    sed -i -E 's|(goVendorHash = )"sha256-[^"]+";|\1"'"$hash"'";|' flake.nix
    echo "updated goVendorHash to $hash"

# Run integration tests
# claude-code is in the devShell, so claude is on PATH inside nix develop.
test-integration:
    @batman=$(nix build --no-link --print-out-paths .#batman); \
      nix build; \
      {{cmd_nix_dev}} $batman/bin/bats --jobs {{num_cpus()}} \
        zz-tests_bats/validate_plugin_repos.bats

# Run lifecycle tests
test-lifecycle:
    @batman=$(nix build --no-link --print-out-paths .#batman); \
      nix build; \
      {{cmd_nix_dev}} $batman/bin/bats --jobs {{num_cpus()}} zz-tests_bats/lux_service.bats

# Validate own plugin manifest
validate:
    {{cmd_nix_dev}} go run ./dummies/go/cmd/... validate .claude-plugin/plugin.json || true

# Validate MCP server annotations via purse-first validate-mcp
validate-mcp-lux: build-lux
    #!/usr/bin/env bash
    set -euo pipefail
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT
    mkdir -p "$tmpdir/lux"
    touch "$tmpdir/lux/lsps.toml"
    XDG_CONFIG_HOME="$tmpdir" purse-first validate-mcp result/bin/lux mcp-stdio

validate-mcp-caldav: build-caldav
    CALDAV_URL="http://localhost:1" CALDAV_USERNAME="test" CALDAV_PASSWORD="test" purse-first validate-mcp result/bin/caldav

validate-mcp: validate-mcp-lux validate-mcp-caldav

test-lux-bats:
    @batman=$(nix build --no-link --print-out-paths .#batman); \
      nix build .#lux; \
      {{cmd_nix_dev}} $batman/bin/bats --bin-dir result/bin --jobs {{num_cpus()}} packages/lux/zz-tests_bats/fmt.bats

# Bump version for a package. Usage: just bump-version lux 0.2.0
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

# Validate lux default configs are internally consistent
validate-lux-defaults: build-lux
    #!/usr/bin/env bash
    set -euo pipefail
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT
    XDG_CONFIG_HOME="$tmpdir" result/bin/lux init --default --force
    XDG_CONFIG_HOME="$tmpdir" result/bin/lux validate

test: \
    test-caldav \
    test-go \
    test-integration \
    test-lux \
    test-lux-bats \
    validate-lux-defaults \
    validate-mcp

update: update-nix

update-nix:
    nix flake update

# Clean build artifacts
clean:
    rm -rf build/
    rm -rf result

