/**
 * Protocol-level smoke test. Spawns the real server over stdio and speaks
 * JSON-RPC to it, exactly as an MCP client would.
 *
 * Run:  cd ~/Dropbox/MCP/gbr-mcp && npm test
 * Exit: 0 all passed · 1 a test failed
 *
 * Requires a live `gbr-agent run` for the tool-call assertions. Without it the
 * transport tests still pass and the tool tests report the structured error —
 * which is itself the behaviour we want to verify.
 */

import { spawn } from 'node:child_process';
import { mkdtempSync, readFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const ENTRY = join(here, '..', 'bin', 'gbr-mcp.js');
const LOGDIR = mkdtempSync(join(tmpdir(), 'gbr-smoke-'));
let stderrBuf = '';

let passed = 0;
let failed = 0;

function check(name, cond, detail = '') {
  if (cond) {
    passed++;
    console.log(`  PASS  ${name}`);
  } else {
    failed++;
    console.log(`  FAIL  ${name}${detail ? ` — ${detail}` : ''}`);
  }
}

function startServer() {
  const child = spawn(process.execPath, [ENTRY], {
    stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, GBR_MCP_LOG_LEVEL: 'info', GBR_MCP_LOG_DIR: LOGDIR, GBR_MCP_LOG_STDERR: '1' },
  });
  child.stderr.on('data', (d) => { stderrBuf += d.toString(); });
  let buf = '';
  const pending = new Map();

  child.stdout.on('data', (d) => {
    buf += d.toString();
    let i;
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i).trim();
      buf = buf.slice(i + 1);
      if (!line) continue;
      let msg;
      try { msg = JSON.parse(line); }
      catch {
        // stdout purity is THE invariant for a stdio MCP server. Silently
        // skipping non-JSON here previously hid exactly the failure we care about.
        check('stdout carries only JSON-RPC frames', false, JSON.stringify(line.slice(0, 120)));
        continue;
      }
      if (msg.id && pending.has(msg.id)) {
        pending.get(msg.id)(msg);
        pending.delete(msg.id);
      }
    }
  });

  let nextId = 1;
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = nextId++;
      pending.set(id, resolve);
      child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
      setTimeout(() => {
        if (pending.has(id)) {
          pending.delete(id);
          reject(new Error(`timeout waiting for ${method}`));
        }
      }, 45000);
    });

  const notify = (method, params = {}) =>
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method, params }) + '\n');

  return { child, send, notify };
}

const parse = (res) => {
  try { return JSON.parse(res?.result?.content?.[0]?.text ?? '{}'); }
  catch { return {}; }
};

async function run() {
  console.log('\ngbr-mcp smoke test\n==================');
  const { child, send, notify } = startServer();

  try {
    // --- handshake ---
    const init = await send('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'smoke', version: '0' },
    });
    check('initialize responds', Boolean(init.result), JSON.stringify(init.error));
    check('server identifies as gbr-mcp', init.result?.serverInfo?.name === 'gbr-mcp');
    notify('notifications/initialized');

    // --- tools/list ---
    const list = await send('tools/list');
    const tools = list.result?.tools || [];
    const names = tools.map((t) => t.name);
    check('tools/list returns tools', tools.length >= 9, `got ${tools.length}`);
    for (const t of ['gbr_diagnose', 'gbr_status', 'gbr_sessions', 'gbr_devices',
                     'gbr_inject', 'gbr_inject_and_wait', 'gbr_output',
                     'gbr_fleet_add', 'gbr_discovery']) {
      check(`exposes ${t}`, names.includes(t));
    }
    check('every tool has a description', tools.every((t) => (t.description || '').length > 40));
    check('every tool has an inputSchema', tools.every((t) => t.inputSchema?.type === 'object'));

    // --- diagnose ---
    const diag = parse(await send('tools/call', { name: 'gbr_diagnose', arguments: {} }));
    check('gbr_diagnose returns checks', Array.isArray(diag.checks) && diag.checks.length > 5);
    check('gbr_diagnose reports overall ok flag', typeof diag.ok === 'boolean');

    // --- discovery / status / sessions / devices ---
    const disc = parse(await send('tools/call', { name: 'gbr_discovery', arguments: {} }));
    const agentUp = disc.ok === true;
    if (!agentUp) {
      console.log('  SKIP  live-agent assertions (no agent on 127.0.0.1:8788)');
    } else {
      check('discovery reports proto gbr/1', disc.proto === 'gbr/1');
      check('discovery reports version', /^v\d+\.\d+\.\d+/.test(disc.version || ''));

      const st = parse(await send('tools/call', { name: 'gbr_status', arguments: {} }));
      check('status says agent_online', st.agent_online === true);

      const ss = parse(await send('tools/call', { name: 'gbr_sessions', arguments: {} }));
      check('sessions returns an array', Array.isArray(ss.sessions));

      const dv = parse(await send('tools/call', { name: 'gbr_devices', arguments: {} }));
      check('devices includes local', (dv.devices || []).some((d) => d.id === 'local'));
    }

    // --- error handling: the important part ---
    const noSession = await send('tools/call', {
      name: 'gbr_inject',
      arguments: { text: 'should not run' },
    });
    check('inject without session_id is rejected', noSession.result?.isError === true);
    const errPayload = parse(noSession);
    check('rejection carries a code', typeof errPayload.code === 'string');
    check('rejection carries a hint', Boolean(errPayload.hint));
    check('rejection carries a correlation_id', Boolean(errPayload.correlation_id));
    check('rejection points at the log file', String(errPayload.log_file || '').includes('gbr-mcp-'));

    const bogus = await send('tools/call', { name: 'gbr_not_a_tool', arguments: {} });
    check('unknown tool returns structured error', Boolean(bogus.result?.isError || bogus.error));

    // --- secret redaction ---
    // Use a NON-hex key. The old test used 'a'.repeat(64), the one shape the
    // 48+ hex fallback happens to catch, so it passed while real keys leaked.
    const SECRET = 'sk_live_Ab3-xY9_ZqW8pLm2NnQr7TvU';
    const leak = await send('tools/call', {
      name: 'gbr_fleet_add',
      // Deliberately invalid mailbox id: the agent rejects it before writing
      // anything to ~/.gbr/fleet.json, so `npm test` never mutates a live fleet.
      arguments: { name: '__gbrmcp_test__', mailbox_id: 'gbr-INVALID-TEST', key: SECRET },
    });
    check('mailbox key never echoed in response', !JSON.stringify(leak).includes(SECRET));
    check('mailbox key never written to stderr', !stderrBuf.includes(SECRET));
    const logDay = new Date().toISOString().slice(0, 10);
    const logPath = join(LOGDIR, `gbr-mcp-${logDay}.jsonl`);
    const logged = existsSync(logPath) ? readFileSync(logPath, 'utf8') : '';
    check('mailbox key never written to the JSONL log', !logged.includes(SECRET));
  } catch (err) {
    failed++;
    console.log(`  FAIL  harness error — ${err.message}`);
  } finally {
    child.kill();
  }

  console.log(`\n${passed} passed, ${failed} failed\n`);
  process.exit(failed ? 1 : 0);
}

run();
