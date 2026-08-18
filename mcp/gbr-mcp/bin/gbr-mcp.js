#!/usr/bin/env node
/**
 * gbr-mcp entry point.
 *
 *   gbr-mcp                 start the MCP stdio server
 *   gbr-mcp --diagnose      run environment checks and exit (human + JSON)
 *   gbr-mcp --version
 *   gbr-mcp --help
 *
 * Exit codes:
 *   0  ok
 *   1  diagnose found at least one failing check
 *   2  bad usage
 *   3  fatal startup error
 */

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(readFileSync(join(here, '..', 'package.json'), 'utf8'));
const argv = process.argv.slice(2);

function help() {
  process.stderr.write(`
gbr-mcp v${pkg.version} — MCP server for the Grok Build Remote Bot API

USAGE
  gbr-mcp                 Start the MCP server on stdio (what MCP clients run)
  gbr-mcp --diagnose      Check the environment and print a repair plan
  gbr-mcp --version       Print version
  gbr-mcp --help          This text

ENVIRONMENT
  GBR_BOT_URL            Local hub URL              (default http://127.0.0.1:8788)
  GBR_RELAY_URL          Relay base for remote mode (e.g. https://gbr-relay.ekobrott.workers.dev)
  GBR_MAILBOX_ID         gbr-xxxx — required for relay mode
  GBR_MAILBOX_KEY        mailbox key — required for relay mode
  GBR_MCP_TIMEOUT_MS     HTTP timeout               (default 20000)

  GBR_MCP_LOG_LEVEL      trace|debug|info|warn|error|silent  (default info)
  GBR_MCP_LOG_DIR        JSONL log directory        (default ~/.gbr/logs)
  GBR_MCP_LOG_STDERR     1|0 human lines on stderr  (default 1)
  GBR_MCP_LOG_BODIES     1|0 log HTTP bodies        (default 1)
  GBR_MCP_LOG_MAX_BODY   truncation length          (default 4000)

REQUIREMENTS
  gbr-agent >= 0.5.3 running (\`gbr-agent run\`). Older builds have no Bot API.

DOCS
  INSTALL.md · TROUBLESHOOTING.md · https://grokbuildremote.com/
`);
}

async function run() {
  if (argv.includes('--help') || argv.includes('-h')) {
    help();
    process.exit(0);
  }

  if (argv.includes('--version') || argv.includes('-v')) {
    process.stdout.write(`${pkg.version}\n`);  // safe: no transport in this mode
    process.exit(0);
  }

  if (argv.includes('--diagnose')) {
    const { diagnose } = await import('../src/diagnose.js');
    const result = await diagnose();
    const mark = (ok) => (ok ? 'PASS' : 'FAIL');
    process.stderr.write('\ngbr-mcp diagnose\n================\n');
    for (const c of result.checks) {
      process.stderr.write(`[${mark(c.ok)}] ${c.name}\n        ${c.detail}\n`);
      if (c.fix) process.stderr.write(`        FIX: ${c.fix}\n`);
    }
    process.stderr.write(`\n${result.summary}\n\n`);
    // JSON on stdout only in diagnose mode — the MCP transport is not active here.
    process.stdout.write(JSON.stringify(result, null, 2) + '\n');
    process.exit(result.ok ? 0 : 1);
  }

  const unknown = argv.filter((a) => a.startsWith('-'));
  if (unknown.length) {
    process.stderr.write(`Unknown option(s): ${unknown.join(', ')}\n`);
    help();
    process.exit(2);
  }

  // Hard guarantee: stdout belongs to the MCP transport. A single console.log
  // from any transitive dependency corrupts the JSON-RPC stream irrecoverably,
  // and the failure mode is a silent hang. Redirect them all to stderr.
  console.log = console.info = console.debug = console.dir = (...a) => console.error(...a);

  const { main } = await import('../src/server.js');
  await main();
}

run().catch((err) => {
  process.stderr.write(`fatal: ${err.stack || err.message}\n`);
  process.exit(3);
});
