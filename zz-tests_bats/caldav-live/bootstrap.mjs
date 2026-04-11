#!/usr/bin/env zx

// Bootstrap a read-only CalDAV MCP testing environment.
//
// Creates a temporary working directory with:
//   - .envrc (source_up + dotenv for CalDAV credentials)
//   - .mcp.json (caldav MCP server pointing to the local build)
//   - .claude/settings.local.json (deny all write tools)
//
// Drops the user into a shell inside the temp dir. On exit (clean shell
// exit, Ctrl+C, or signal), removes the temp directory.
//
// Requires:
//   - nix build .#caldav (result/bin/caldav must exist)
//   - ~/.secrets.env with CALDAV_USERNAME and CALDAV_PASSWORD

import { spawn } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'

$.verbose = false

const SCRIPT_DIR = path.dirname(new URL(import.meta.url).pathname)
const REPO_ROOT = path.resolve(SCRIPT_DIR, '..', '..')
const CALDAV_BIN = path.join(REPO_ROOT, 'result', 'bin', 'caldav')

// Verify the caldav binary exists
if (!(await fs.pathExists(CALDAV_BIN))) {
  console.error(`error: caldav binary not found at ${CALDAV_BIN}`)
  console.error(`  run: nix build .#caldav`)
  process.exit(1)
}

// Verify secrets are available
const secretsPath = path.join(homedir(), '.secrets.env')
if (!(await fs.pathExists(secretsPath))) {
  console.error(`error: ${secretsPath} not found`)
  process.exit(1)
}

const secrets = readFileSync(secretsPath, 'utf8')
const required = ['CALDAV_URL', 'CALDAV_USERNAME', 'CALDAV_PASSWORD']
const found = {}
for (const line of secrets.split('\n')) {
  const m = line.match(/^export\s+(\w+)=["']?(.+?)["']?\s*$/)
  if (m) found[m[1]] = m[2]
}
const missing = required.filter((k) => !found[k])
if (missing.length > 0) {
  console.error(`error: missing in ~/.secrets.env: ${missing.join(', ')}`)
  process.exit(1)
}
const CALDAV_URL = found.CALDAV_URL

// Create temp working directory
const WORK_DIR = await fs.mkdtemp(path.join(REPO_ROOT, '.caldav-test-'))

// --- Cleanup ---

let cleaningUp = false
async function cleanup() {
  if (cleaningUp) return
  cleaningUp = true
  await fs.remove(WORK_DIR).catch(() => {})
  console.log('Cleaned up.')
}

let signalReceived = false
async function handleSignal(code) {
  if (signalReceived) return
  signalReceived = true
  await cleanup()
  process.exit(code)
}

process.on('SIGINT', () => handleSignal(130))
process.on('SIGTERM', () => handleSignal(143))

// --- Write config files ---

// .envrc — source parent envrc + load CalDAV secrets
await fs.writeFile(
  path.join(WORK_DIR, '.envrc'),
  `source_up
dotenv "$HOME/.secrets.env"
export CALDAV_URL="${CALDAV_URL}"
`,
)

// Run-caldav wrapper script (the .mcp.json command).
// This loads secrets and execs the caldav binary so the MCP client gets
// credentials even if direnv hasn't loaded in the Claude Code process.
const RUN_WRAPPER = path.join(WORK_DIR, 'run-caldav.mjs')
await fs.writeFile(
  RUN_WRAPPER,
  `#!/usr/bin/env zx
import { readFileSync } from 'fs'
import { homedir } from 'os'
import { spawn } from 'child_process'

$.verbose = false

const secrets = readFileSync(homedir() + '/.secrets.env', 'utf8')
for (const line of secrets.split('\\n')) {
  const m = line.match(/^export\\s+(\\w+)=["']?(.+?)["']?\\s*$/)
  if (m) process.env[m[1]] = m[2]
}
process.env.CALDAV_URL = '${CALDAV_URL}'

const bin = '${CALDAV_BIN}'
const child = spawn(bin, [], { stdio: 'inherit', env: process.env })
child.on('exit', (code) => process.exit(code ?? 1))
`,
)
await fs.chmod(RUN_WRAPPER, 0o755)

// .mcp.json — caldav MCP server
await fs.writeFile(
  path.join(WORK_DIR, '.mcp.json'),
  JSON.stringify(
    {
      mcpServers: {
        caldav: {
          command: RUN_WRAPPER,
          type: 'stdio',
        },
      },
    },
    null,
    2,
  ) + '\n',
)

// .claude/settings.local.json — deny all write tools
await fs.ensureDir(path.join(WORK_DIR, '.claude'))
await fs.writeFile(
  path.join(WORK_DIR, '.claude', 'settings.local.json'),
  JSON.stringify(
    {
      permissions: {
        deny: [
          'mcp__caldav__create_task',
          'mcp__caldav__update_task',
          'mcp__caldav__complete_task',
          'mcp__caldav__delete_task',
          'mcp__caldav__move_task',
          'mcp__caldav__create_event',
          'mcp__caldav__update_event',
          'mcp__caldav__delete_event',
          'mcp__caldav__move_event',
          'mcp__caldav__create_calendar',
        ],
      },
      enabledMcpjsonServers: ['caldav'],
    },
    null,
    2,
  ) + '\n',
)

// --- Print summary and drop into shell ---

console.log('')
console.log('CalDAV MCP test environment bootstrapped (read-only):')
console.log(`  work dir:   ${WORK_DIR}`)
console.log(`  caldav bin: ${CALDAV_BIN}`)
console.log(`  server:     ${CALDAV_URL}`)
console.log('')
console.log('Start Claude Code in this directory to test:')
console.log(`  claude`)
console.log('')
console.log('Then try:')
console.log('  call list_calendars')
console.log('  call list_tasks --calendar_id <id>')
console.log('  call search_tasks --query <word>')
console.log('')
console.log('Exit the shell to tear down.')
console.log('')

const shell = process.env.SHELL ?? '/bin/bash'
const shellChild = spawn(shell, [], {
  stdio: 'inherit',
  cwd: WORK_DIR,
})

await new Promise((resolve) => {
  shellChild.on('exit', () => resolve())
  shellChild.on('error', () => resolve())
})

await cleanup()
