import nodeWhich from "which";

/**
 * Find the path to an executable, similar to the `which` command.
 * Uses Bun.which when running in Bun, falls back to the `which` npm
 * package for Node.js (pure JS PATH search — no dependency on /usr/bin/which).
 *
 * @param bin - The name of the executable to find
 * @returns The full path to the executable, or null if not found
 */
export function whichSync(bin: string): string | null {
  if (typeof globalThis.Bun !== "undefined") {
    return globalThis.Bun.which(bin);
  }

  return nodeWhich.sync(bin, { nothrow: true });
}
