#!/usr/bin/env zx
///!dep zx@8.8.5 sha512-SNgDF5L0gfN7FwVOdEFguY3orU5AkfFZm9B5YSHog/UDHv+lvmd82ZAsOenOkQixigwH2+yyH198AwNdKhj+RA==

// batman v0 — fence-based BATS wrapper.
// See docs/plans/2026-04-25-batman-v0-design.md.

import { argv } from "zx";

const args = argv._;
console.log(`batman v0 (stub): received ${args.length} positional args`);
process.exit(0);
