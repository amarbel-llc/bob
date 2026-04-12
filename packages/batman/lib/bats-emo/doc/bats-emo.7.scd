bats-emo(7)

# NAME

bats-emo - binary conformance helpers for Bats test files

# DESCRIPTION

*bats-emo* provides helpers for validating that binaries under test are
available before running integration tests. It supports discovery via
environment variables and PATH lookup.

Load the library from a BATS test file or helper:

```
bats_load_library bats-emo
```

# FUNCTIONS

*require_bin* <var_name> [<command_name>]
	Validate that a binary under test is available. Checks in order:

	. If *$var_name* is set and its value is an executable path, succeeds.
	. If *$var_name* is unset and _command_name_ is provided, looks up
	  _command_name_ on *$PATH*.
	. Otherwise, prints an error and exits with status 1.

	This function is typically called in a BATS *setup* or *setup_file*
	function to fail early with a clear message when a required binary is
	missing.

# EXAMPLE

```
setup_file() {
  bats_load_library bats-emo
  require_bin GRIT_BIN grit       # env var or PATH fallback
  require_bin BATS_WRAPPER        # env var only, no fallback
}
```

# SEE ALSO

*bats-island*(7), *bats-assert*(7)
