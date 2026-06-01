
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
cmd_nix_dev_go := "nix develop " + justfile_directory() + "#go --command "

default: build test vendor

# Build all packages (default = caldav + non-MCP bundle)
build:
    nix build

# Build individual packages
build-caldav:
    nix build .#caldav

# Realize the batman bundle into the nix store and print its path.
build-batman:
    @nix build --no-link --print-out-paths .#batman

tap-dancer-go-test := "tap-dancer go-test -skip-empty"
tap-dancer-cargo-test := "tap-dancer cargo-test -skip-empty"

# Test individual Go packages
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

# Run every bats lane (authoritative). Each lane runs inside the
# nix build sandbox with binaries injected via env vars from bats.nix.
# Adding a new lane → add it to bats.nix, then re-run this recipe.
test-bats:
    nix build .#bats-default --no-link --print-build-logs

# Run a single named lane (e.g. `just test-bats-lane validate-plugin-repos`).
test-bats-lane name:
    nix build .#bats-{{name}} --no-link --print-build-logs

# Validate own plugin manifest
validate:
    {{cmd_nix_dev}} go run ./dummies/go/cmd/... validate .claude-plugin/plugin.json || true

# Validate MCP server annotations via purse-first validate-mcp
validate-mcp-caldav: build-caldav
    CALDAV_URL="http://localhost:1" CALDAV_USERNAME="test" CALDAV_PASSWORD="test" purse-first validate-mcp result/bin/caldav

validate-mcp: validate-mcp-caldav

# Build dummy Go MCP servers
build-dummies-go:
    {{cmd_nix_dev}} go build -o build/ ./dummies/go/cmd/...

test: \
    test-caldav \
    test-go \
    test-bats \
    validate-mcp

update: update-nix

update-nix:
    nix flake update

# Clean build artifacts
clean:
    rm -rf build/
    rm -rf result

