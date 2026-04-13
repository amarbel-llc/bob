justfile-design-patterns(7)

# NAME

justfile-design-patterns - conventions for justfiles in amarbel-llc projects

# DESCRIPTION

Justfiles in amarbel-llc projects follow a consistent set of design patterns
for naming, hierarchy, and composition. These conventions apply to all
projects using *just-us* (an amarbel-llc fork of *just*).

# GLOBAL SETTINGS

Every justfile begins with:

```
set output-format := "tap"
```

This requires *just-us*. It does not work with upstream *just*.

# NAMING CONVENTION

Every recipe follows a *verb-noun* pattern where the verb is the action
category and the noun is the tool or domain.

Examples: *build-go*, *test-bats*, *codemod-fmt-go*, *update-nix*.

Verbs nest for sub-categories: *codemod-fmt-go* is *codemod* (verb) + *fmt*
(sub-verb) + *go* (noun).

# VERB CATEGORIES

*build*
	Compile, generate, or produce artifacts.
	Examples: _build-go_, _build-gomod2nix_, _build-nix_, _build-moxins_.

*test*
	Run test suites.
	Examples: _test-go_, _test-bats_, _test-cargo_.

*run*
	Execute the built artifact.
	Examples: _run-nix \*ARGS_.

*clean*
	Remove generated artifacts.
	Examples: _clean-build_.

*update*
	Refresh dependencies or inputs.
	Examples: _update-go_, _update-nix_.

*codemod*
	Automated code modifications. Uses sub-verbs:

	*fmt* — format code (_gofumpt_, _nixfmt_, _shfmt_, _rustfmt_).

	*lint* — lint/check without modifying (_clippy_, _golangci-lint_).

	*fix* — auto-fix issues (_go fix_, _eslint --fix_).

*validate*
	Check or verify correctness distinct from test suites.
	Examples: _validate_, _test-validate-mcp_.

# TASK HIERARCHY

Bare-verb recipes are *aggregate targets* that compose specific verb-noun
leaf recipes as dependencies:

```
build: build-gomod2nix build-go
test: test-go test-bats
codemod-fmt: codemod-fmt-go codemod-fmt-nix
```

This gives two usage modes: *just build* runs everything, *just build-go*
runs one step.

Rules:

- Aggregate recipes have *no body* — they only list dependencies.
- Leaf recipes have a body that does the actual work.
- Every leaf recipe should belong to exactly one aggregate.

# DEFAULT RECIPE

```
default: build test
```

*default* chains aggregate targets into a single "build and verify" pipeline.
If *just* (no arguments) passes, the project is in a good state. This is the
CI-equivalent target.

The *default* recipe is always the first recipe in the file.

# DEPENDENCY ORDERING

*Positional dependencies* (prerequisites — run before body):

```
build-go: build-gomod2nix
    nix develop --command go build -o build/myapp ./cmd/myapp
```

*&& dependencies* (post-steps — run after body):

```
update-go: && build-gomod2nix
    nix develop --command go mod tidy
```

Use positional for prerequisites. Use *&&* for "then regenerate/rebuild after
the main action completes."

# RECIPE ATTRIBUTES

*[group: 'name']*
	Organize related recipes for *just --list* output.

```
[group: 'test']
test-go:
    go test ./...
```

*[private]*
	Hide from *just --list*. Use for internal helper recipes. Private helpers
	are also conventionally prefixed with underscore:

```
[private]
_test-bats-ensure-fixtures:
    ...
```

*[macos]* / *[linux]*
	Platform-conditional recipes. Only runs on the matching OS:

```
[macos]
load-agents:
    launchctl bootstrap gui/$UID ~/Library/LaunchAgents/...

[linux]
load-agents:
    @true
```

*[working-directory: 'path']*
	Change the working directory for a recipe:

```
[working-directory: 'rcm']
build-rcm:
    rcup -v
```

*[no-exit-message]*
	Suppress the exit message on failure.

# VARIABLES

Common variable patterns:

```
dir_build := "build"
cmd_nix_dev := "nix develop " + justfile_directory() + " --command "
```

Use *justfile_directory()* for paths relative to the justfile. Use *export*
to inject variables into recipe environments:

```
export MOXIN_PATH := justfile_directory() / "build" / "moxins"
```

# PARAMETERIZED RECIPES

Recipes accept parameters with optional defaults:

```
run-nix *ARGS:
    nix run . -- {{ARGS}}

man-search query top_k="10":
    bin/man-search-embed.bash "{{query}}" "{{top_k}}"
```

Use *\*args* for variadic pass-through. Use *name=default* for optional
parameters.

# SHEBANG RECIPES

Use shebang recipes for multi-line logic that needs conditionals or loops:

```
deploy-flake-lock:
    #!/usr/bin/env bash
    set -euo pipefail
    if git diff --quiet flake.lock; then
      echo "flake.lock unchanged, skipping deploy"
      exit 0
    fi
    git add flake.lock
    git commit -m "chore: update flake.lock"
```

Always use *set -euo pipefail* in bash shebang recipes.

# CROSS-JUSTFILE DELEGATION

Subprojects may have their own justfiles. Delegate to them using *just*
recursion:

```
test-bats-targets *targets: build
    just zz-tests_bats/test-targets {{targets}}
```

# ANTI-PATTERNS

- *Mixed verb categories* — don't combine verbs like _build-go-test_.
  Use separate recipes.
- *Generic names* — don't use _all_, _dev_, _check_, _compile_. Use
  verb-noun.
- *Logic in aggregates* — aggregate recipes should only list
  dependencies, never have a body.
- *Missing default* — every justfile needs a _default_ recipe.
- *Comments on aggregates* — the dependency list is self-documenting.
- *Redundant nouns* — even with one tool, use _test-go_ not _test_.
  You may add more later.
- *deps as a verb* — use _update-\*_ for dependency management.

# EXAMPLE

```
set output-format := "tap"

default: build test

build: build-gomod2nix build-go build-nix

build-go: build-gomod2nix
    nix develop --command go build -o build/myapp ./cmd/myapp

build-gomod2nix:
    nix develop --command gomod2nix

build-nix:
    nix build --show-trace

run-nix *ARGS:
    nix run . -- {{ARGS}}

test: test-go test-bats

test-go:
    nix develop --command go test ./...

test-bats:
    nix develop --command bats --tap tests/

codemod-fmt: codemod-fmt-go codemod-fmt-nix

codemod-fmt-go:
    nix develop --command gofumpt -w .

codemod-fmt-nix:
    nix run ./devenvs/nix#fmt -- .

update-go: && build-gomod2nix
    nix develop --command go mod tidy

clean: clean-build

clean-build:
    rm -rf result build/
```

# SEE ALSO

*manpages*(7)
