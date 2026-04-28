
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
cmd_nix_dev_go := "nix develop " + justfile_directory() + "#go --command "
cmd_batman_bats := justfile_directory() + "/result-batman/bin/bats"

default: build test vendor

# Build all packages (default = marketplace bundle)
build:
    nix build

# Build individual packages
build-lux:
    nix build .#lux

build-caldav:
    nix build .#caldav

build-batman:
    nix build .#batman -o result-batman

cmd-tap-dancer := join(justfile_directory(), "./packages/tap-dancer/go/cmd/tap-dancer")
tap-dancer-go-test := "go run " + cmd-tap-dancer + " go-test -skip-empty"
tap-dancer-cargo-test := "go run " + cmd-tap-dancer + " cargo-test -skip-empty"

# Test individual Go packages
test-lux:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/lux/...

test-caldav:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/caldav/...

test-tap-dancer-go:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/tap-dancer/go/...

# Test Rust packages
[working-directory: 'packages/tap-dancer/rust']
test-tap-dancer-rust:
    {{cmd_nix_dev}} {{tap-dancer-cargo-test}} test

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

test-tap-dancer-bats: build-batman
    nix build .#tap-dancer
    TAP_DANCER_BIN={{justfile_directory()}}/result/bin/tap-dancer PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/tap-dancer/zz-tests_bats/test

test-lux-bats: build-batman
    nix build .#lux
    {{cmd_nix_dev}} {{cmd_batman_bats}} --bin-dir result/bin --jobs {{num_cpus()}} packages/lux/zz-tests_bats/fmt.bats

test-batman-bats: build-batman
    BATS_WRAPPER={{justfile_directory()}}/result-batman/bin/bats BATMAN_BIN={{justfile_directory()}}/result-batman/bin/batman PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/batman/zz-tests_bats/test

# Run batman.bats under PLAIN nixpkgs bats. Filters /home/* dirs out of
# PATH inside the dev shell so a profile-installed sandcastle-wrapped
# `bats` (e.g. /home/$user/eng/result/bin/bats from a purse-first
# install) doesn't shadow the dev-shell's plain pkgs.bats. Without this
# filter, the wrapped bats's default sandbox=true causes sandcastle's
# bwrap to nest inside fence's bwrap and fence fails to set up sockets.
test-batman-fence: build-batman
    BATMAN_BIN={{justfile_directory()}}/result-batman/bin/batman \
      BATS_LIB_PATH={{justfile_directory()}}/result-batman/share/bats \
      {{cmd_nix_dev}} bash -c 'PATH=$(echo "$PATH" | tr ":" "\n" | grep -Ev "^/home/" | tr "\n" ":"); exec bats --tap --jobs $(nproc) packages/batman/zz-tests_bats/batman.bats'

# Invoke the built batman binary with arbitrary args. Useful for manual smoke-testing.
run-batman *args: build-batman
    {{justfile_directory()}}/result-batman/bin/batman {{args}}

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
    test-batman-bats \
    test-caldav \
    test-go \
    test-integration \
    test-lux \
    test-lux-bats \
    test-tap-dancer-bats \
    test-tap-dancer-go \
    test-tap-dancer-rust \
    validate-lux-defaults \
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

