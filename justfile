
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
cmd_nix_dev_go := "nix develop " + justfile_directory() + "#go --command "
cmd_batman_bats := justfile_directory() + "/result-batman/bin/bats"

default: build test vendor

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

build-caldav:
    nix build .#caldav

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

test-caldav:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/caldav/...

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

# Validate MCP server annotations via purse-first validate-mcp
validate-mcp-grit: build-grit
    purse-first validate-mcp result/bin/grit

validate-mcp-get-hubbed: build-get-hubbed
    purse-first validate-mcp result/bin/get-hubbed

validate-mcp-lux: build-lux
    #!/usr/bin/env bash
    set -euo pipefail
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT
    mkdir -p "$tmpdir/lux"
    touch "$tmpdir/lux/lsps.toml"
    XDG_CONFIG_HOME="$tmpdir" purse-first validate-mcp result/bin/lux mcp-stdio

validate-mcp-chix: build-chix
    purse-first validate-mcp result/bin/chix

validate-mcp-caldav: build-caldav
    CALDAV_URL="http://localhost:1" CALDAV_USERNAME="test" CALDAV_PASSWORD="test" purse-first validate-mcp result/bin/caldav

validate-mcp: validate-mcp-grit validate-mcp-get-hubbed validate-mcp-lux validate-mcp-chix validate-mcp-caldav

test-grit-bats: build-batman
    nix build .#grit
    GRIT_BIN={{justfile_directory()}}/result/bin/grit PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/grit/zz-tests_bats/test

test-tap-dancer-bats: build-batman
    nix build .#tap-dancer
    TAP_DANCER_BIN={{justfile_directory()}}/result/bin/tap-dancer PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/tap-dancer/zz-tests_bats/test

test-lux-bats: build-batman
    nix build .#lux
    {{cmd_nix_dev}} {{cmd_batman_bats}} --bin-dir result/bin --jobs {{num_cpus()}} packages/lux/zz-tests_bats/fmt.bats

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
    test-caldav \
    test-chix \
    test-get-hubbed \
    test-go \
    test-grit \
    test-grit-bats \
    test-integration \
    test-lux \
    test-lux-bats \
    test-tap-dancer-bats \
    test-tap-dancer-go \
    test-tap-dancer-rust \
    validate-mcp

update: update-nix

update-nix:
    nix flake update

# Clean build artifacts
clean:
    rm -rf build/
    rm -rf result result-batman

# ── EXPERIMENTAL ─────────────────────────────────────────────────────

# Tag a semver release for tap-dancer (Go + Rust + Nix + skill).
# Usage: just release-tap-dancer 0.2.0
# Creates two git tags (Go path-prefixed + Rust) and bumps version in
# all manifests. Does NOT push — review with `git log` first, then
# `git push origin --tags`.
release-tap-dancer version:
    #!/usr/bin/env bash
    set -euo pipefail

    v="{{version}}"

    # Validate semver-ish format
    if [[ ! "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      echo "error: version must be semver (e.g. 0.2.0), got: $v" >&2
      exit 1
    fi

    # Ensure clean working tree
    if [[ -n "$(git status --porcelain)" ]]; then
      echo "error: working tree is dirty — commit or stash first" >&2
      exit 1
    fi

    echo "==> bumping tap-dancer to v$v"

    # 1. Cargo.toml
    sed -i -E '0,/^version = "[^"]+"/s//version = "'"$v"'"/' \
      packages/tap-dancer/rust/Cargo.toml

    # 2. lib/packages/tap-dancer.nix (first version = line only)
    sed -i -E '0,/version = "[^"]+";/s//version = "'"$v"'";/' \
      lib/packages/tap-dancer.nix

    # 3. marketplace-config.json
    jq --arg ver "$v" \
      '.plugins["tap-dancer"].version = $ver' \
      marketplace-config.json > marketplace-config.json.tmp
    mv marketplace-config.json.tmp marketplace-config.json

    # 4. SKILL.md frontmatter
    sed -i -E 's/^version: .+/version: '"$v"'/' \
      packages/tap-dancer/skills/tap14/SKILL.md

    # 5. Verify build
    echo "==> nix build (verify)"
    nix build --show-trace

    # 6. Commit + tag
    git add \
      packages/tap-dancer/rust/Cargo.toml \
      lib/packages/tap-dancer.nix \
      marketplace-config.json \
      packages/tap-dancer/skills/tap14/SKILL.md
    git commit -m "release(tap-dancer): v$v"

    git tag "packages/tap-dancer/go/v$v"
    git tag "tap-dancer-v$v"

    echo "==> tagged: packages/tap-dancer/go/v$v (Go) + tap-dancer-v$v (Rust)"
    echo "==> review, then: git push origin master --tags"

