/**
 * Offline unit + stub-server tests. No agent required, fully deterministic.
 *
 * Covers the pure functions and the four documented Bot API gotchas, all of
 * which were previously untested — the smoke suite gated every live assertion
 * behind an `agentUp` flag, so with no agent running it reported all-green
 * while exercising almost nothing.
 *
 * Run:  cd ~/Dropbox/MCP/gbr-mcp && node test/unit.mjs
 */

import http from 'node:http';
import { mkdtempSync, readFileSync, existsSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const LOGDIR = mkdtempSync(join(tmpdir(), 'gbr-unit-'));
process.env.GBR_MCP_LOG_DIR = LOGDIR;
process.env.GBR_MCP_LOG_STDERR = '0';
process.env.GBR_MCP_LOG_LEVEL = 'debug';

const { redact, clampBody, registerSecret, log } = await import('../src/logger.js');
const { GbrClient, GbrError } = await import('../src/client.js');

let passed = 0, failed = 0;
const check = (name, cond, detail = '') => {
  if (cond) { passed++; console.log(`  PASS  ${name}`); }
  else { failed++; console.log(`  FAIL  ${name}${detail ? ` — ${detail}` : ''}`); }
};

console.log('\ngbr-mcp unit + stub tests\n=========================');

// ---------------------------------------------------------------- redaction
console.log('\n[redaction]');

const HEX = 'a'.repeat(64);
const NONHEX = 'sk_live_Ab3-xY9_ZqW8pLm2NnQr7TvU';

check('hex key redacted by key name', !JSON.stringify(redact({ key: HEX })).includes(HEX));
check('non-hex key redacted by key name', !JSON.stringify(redact({ key: NONHEX })).includes(NONHEX));
check('compound name mailbox_key redacted', !JSON.stringify(redact({ mailbox_key: NONHEX })).includes(NONHEX));
check('nested key redacted', !JSON.stringify(redact({ a: { b: { api_key: NONHEX } } })).includes(NONHEX));
check('array member redacted', !JSON.stringify(redact([{ key: NONHEX }])).includes(NONHEX));
check('hasKey NOT redacted (false positive guard)', redact({ hasKey: true }).hasKey === true);
check('ordinary values survive', redact({ name: 'studio-linux' }).name === 'studio-linux');

// The regression that shipped: clampBody stringified BEFORE redacting, so
// key-name matching never ran and only 48+ hex was caught.
const clamped = clampBody({ name: 'box', key: NONHEX });
check('clampBody redacts before stringify', !String(clamped).includes(NONHEX), String(clamped));

registerSecret(NONHEX);
check('registerSecret scrubs bare value in free text',
  !redact(`the key is ${NONHEX} ok`).includes(NONHEX));

// Truncation must not split a secret below the 48-char regex floor.
const bigHex = 'a1b2c3d4e5f6'.repeat(6); // 72 chars
const padded = { note: 'x'.repeat(3960) + ' ' + bigHex };
const cut = String(clampBody(padded));
check('truncation does not leak a split hex fragment',
  !/[a-f0-9]{20,}/i.test(cut.replace(/\[redacted:\d+ch\]/g, '')), cut.slice(-90));

// A key must never reach the log file.
log.info('tool call', { tool: 'gbr_fleet_add', args: clampBody({ name: 'b', key: NONHEX }) });
const logFiles = readdirSync(LOGDIR).filter((f) => f.endsWith('.jsonl'));
const logText = logFiles.map((f) => readFileSync(join(LOGDIR, f), 'utf8')).join('');
check('non-hex key absent from JSONL log', !logText.includes(NONHEX));
check('log file was actually written', logText.length > 0);

// ---------------------------------------------------------------------- url
console.log('\n[url construction]');

const local = new GbrClient({ baseUrl: 'http://127.0.0.1:8788' });
check('local discovery hits the root', local.url('') === 'http://127.0.0.1:8788/', local.url(''));
check('local status path', local.url('status') === 'http://127.0.0.1:8788/v1/status');
check('local query string', local.url('output', { limit: 5 }) === 'http://127.0.0.1:8788/v1/output?limit=5');
check('empty query values dropped',
  local.url('output', { limit: 5, after: '' }) === 'http://127.0.0.1:8788/v1/output?limit=5');

const relay = new GbrClient({
  relayUrl: 'https://relay.example.com', mailbox: 'gbr-abcd', key: HEX,
});
check('relay mode detected', relay.describe().mode === 'relay');
check('relay discovery has no trailing slash',
  relay.url('') === 'https://relay.example.com/v1/mb/gbr-abcd/bot', relay.url(''));
check('relay status path',
  relay.url('status') === 'https://relay.example.com/v1/mb/gbr-abcd/bot/status');
check('describe() never exposes the key', !JSON.stringify(relay.describe()).includes(HEX));

let threw = null;
try { new GbrClient({ relayUrl: 'http://evil.example.com', mailbox: 'm', key: HEX }); }
catch (e) { threw = e.message; }
check('plaintext relay URL rejected', /https/.test(threw || ''), threw);

threw = null;
try { new GbrClient({ relayUrl: 'https://r.example.com', mailbox: '../etc', key: HEX }); }
catch (e) { threw = e.message; }
check('path-traversal mailbox id rejected', /MAILBOX_ID/.test(threw || ''), threw);

// -------------------------------------------------------------- NaN timeout
console.log('\n[config hardening]');
const nanClient = new GbrClient({ baseUrl: 'http://127.0.0.1:1', timeoutMs: '20s' });
check('NaN timeout falls back to 20000', nanClient.describe().timeoutMs === 20000,
  String(nanClient.describe().timeoutMs));

// ------------------------------------------------------------- stub server
console.log('\n[stub server — the four gotchas]');

const stub = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://x');
  const send = (code, obj) => {
    res.writeHead(code, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(obj));
  };
  if (url.pathname === '/') return send(200, { ok: true, proto: 'gbr/1', version: 'v0.5.3' });
  if (url.pathname === '/v1/refuse') return send(200, { ok: false, error: 'inject: empty session_id refused' });
  if (url.pathname === '/v1/gone') return send(404, { error: 'not_found' });
  if (url.pathname === '/v1/nope') return send(401, { error: 'unauthorized' });
  if (url.pathname === '/v1/flood') return send(429, { error: 'rate limited' });
  if (url.pathname === '/v1/garbage') { res.writeHead(200, { 'Content-Type': 'text/html' }); return res.end('<html>not json'); }
  if (url.pathname === '/v1/inject') return send(200, { ok: true, command_id: 'c1', device: { id: 'local' } });
  if (url.pathname === '/v1/hang') return; // never responds
  send(200, { ok: true });
});
await new Promise((r) => stub.listen(0, '127.0.0.1', r));
const stubUrl = `http://127.0.0.1:${stub.address().port}`;
const c = new GbrClient({ baseUrl: stubUrl, timeoutMs: 1200 });

const expectCode = async (name, fn, code) => {
  try { await fn(); check(name, false, 'no error thrown'); }
  catch (e) { check(name, e.code === code, `got ${e.code}: ${e.message}`); }
};

check('discovery at root parses', (await c.discovery()).proto === 'gbr/1');
await expectCode('200 with ok:false -> GBR_AGENT_REFUSED', () => c.request('GET', 'refuse'), 'GBR_AGENT_REFUSED');
await expectCode('404 -> GBR_NOT_FOUND', () => c.request('GET', 'gone'), 'GBR_NOT_FOUND');
await expectCode('401 -> GBR_UNAUTHORIZED', () => c.request('GET', 'nope'), 'GBR_UNAUTHORIZED');
await expectCode('429 -> GBR_RATE_LIMITED', () => c.request('GET', 'flood'), 'GBR_RATE_LIMITED');
await expectCode('non-JSON -> GBR_BAD_RESPONSE', () => c.request('GET', 'garbage'), 'GBR_BAD_RESPONSE');
await expectCode('hanging request -> GBR_TIMEOUT', () => c.request('GET', 'hang'), 'GBR_TIMEOUT');

const dead = new GbrClient({ baseUrl: 'http://127.0.0.1:1', timeoutMs: 1000 });
await expectCode('closed port -> GBR_UNREACHABLE', () => dead.status(), 'GBR_UNREACHABLE');

// Gotcha 3: device fallback must be flagged.
const inj = await c.inject({ device: 'studio-linux', session_id: 's1', text: 'x' });
check('device fallback produces _warning', typeof inj._warning === 'string', JSON.stringify(inj));
check('_warning names both devices',
  /studio-linux/.test(inj._warning) && /local/.test(inj._warning));

const injOk = await c.inject({ device: 'local', session_id: 's1', text: 'x' });
check('matching device produces no warning', injOk._warning === undefined);

stub.close();

// ------------------------------------------- inject_and_wait dedup (server)
console.log('\n[inject_and_wait cursor]');

let pollCount = 0;
const stub2 = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://x');
  const send = (o) => { res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(o)); };
  if (url.pathname === '/v1/inject') return send({ ok: true, command_id: 'c1', device: { id: 'local' } });
  if (url.pathname === '/v1/output') {
    pollCount++;
    // Deliberately ignore `after` and always return the whole buffer — the
    // worst case the client must tolerate without double-counting.
    const items = [];
    for (let i = 0; i < pollCount; i++) {
      items.push({ ts: `2026-01-01T00:00:0${i}Z`, stream: 'stdout', chunk: `line ${i}`, eof: i === 3 });
    }
    return send({ ok: true, items });
  }
  send({ ok: true });
});
await new Promise((r) => stub2.listen(0, '127.0.0.1', r));

const { createServer } = await import('../src/server.js');
const { server } = createServer({ baseUrl: `http://127.0.0.1:${stub2.address().port}` });
// Reach the dispatcher through the registered handler.
const handler = server._requestHandlers.get('tools/call');
const res2 = await handler({ method: 'tools/call', params: { name: 'gbr_inject_and_wait',
  arguments: { session_id: 's1', text: 'go', wait_ms: 12000, poll_ms: 250 } } }, {});
const parsed = JSON.parse(res2.content[0].text);
const uniq = new Set(parsed.items.map((i) => `${i.ts}|${i.chunk}`)).size;
check('completed on EOF', parsed.completed === true, JSON.stringify(parsed._note));
check('no duplicated items', parsed.item_count === uniq, `count=${parsed.item_count} unique=${uniq}`);
check('item_count is truthful', parsed.item_count === parsed.items.length);

stub2.close();

console.log(`\n${passed} passed, ${failed} failed\n`);
process.exit(failed ? 1 : 0);
