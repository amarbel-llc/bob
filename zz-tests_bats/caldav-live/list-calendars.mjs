#!/usr/bin/env zx

// List all CalDAV calendar collections with their supported component types.
// Adapted from ~/workspaces/dodder-haustoria-caldav/list-calendars.sh
//
// Usage: ./list-calendars.mjs

import { readFileSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'

// Load ~/.secrets.env (dotenv with `export VAR=value` lines)
const secrets = readFileSync(join(homedir(), '.secrets.env'), 'utf8')
for (const line of secrets.split('\n')) {
  const m = line.match(/^export\s+(\w+)=["']?(.+?)["']?\s*$/)
  if (m) process.env[m[1]] = m[2]
}

const { CALDAV_URL, CALDAV_USERNAME, CALDAV_PASSWORD } = process.env

if (!CALDAV_URL || !CALDAV_USERNAME || !CALDAV_PASSWORD) {
  console.error('CALDAV_URL, CALDAV_USERNAME, or CALDAV_PASSWORD not found in ~/.secrets.env')
  process.exit(1)
}

const propfind = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <c:supported-calendar-component-set/>
  </d:prop>
</d:propfind>`

const parseScript = `
import sys, xml.etree.ElementTree as ET
data = sys.stdin.read()
root = ET.fromstring(data)
ns = {'d': 'DAV:', 'c': 'urn:ietf:params:xml:ns:caldav'}
for resp in root.findall('.//d:response', ns):
    href = resp.find('d:href', ns)
    name = resp.find('.//d:displayname', ns)
    cal = resp.find('.//d:resourcetype/c:calendar', ns)
    comps = resp.findall('.//c:comp', ns)
    comp_names = [c.get('name','') for c in comps]
    if cal is not None:
        h = href.text if href is not None else '?'
        n = name.text if name is not None else '(unnamed)'
        print(f'{n:30s}  {chr(44).join(comp_names):20s}  {h}')
`

$.verbose = false

const xml =
  await $`curl -s -X PROPFIND -u ${CALDAV_USERNAME}:${CALDAV_PASSWORD} -H "Depth: 1" -H "Content-Type: application/xml" ${CALDAV_URL} -d ${propfind}`

const result = await $`python3 -c ${parseScript}`.stdin(xml.stdout)
console.log(result.stdout)
