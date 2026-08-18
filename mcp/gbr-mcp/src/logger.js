/**
 * Structured JSONL logger for gbr-mcp.
 *
 * Design rules:
 *  - NEVER write to stdout. stdout is the MCP stdio transport; one stray byte
 *    corrupts the protocol. Human-readable lines go to stderr, machine-readable
 *    JSONL goes to a file.
 *  - Every log line carries a correlation id so a single tool call can be
 *    traced from MCP request -> HTTP request -> HTTP response -> MCP response.
 *  - Secrets are redacted on the way out, always, even at trace level.
 *
 * SECURITY NOTE (regression guard):
 *   Redaction MUST happen on the OBJECT, before any JSON.stringify. An earlier
 *   version stringified first, which destroyed the key names that key-based
 *   redaction relies on — only 48+ char hex values were caught, so a key like
 *   `sk_live_Ab3-xY9...` was written verbatim. Three defences now run in order:
 *     1. registerSecret() exact-value scrubbing (shape-independent)
 *     2. SECRET_KEYS name matching (runs on the object, never a string)
 *     3. the 48+ hex fallback (last resort)
 *   Truncation also chops on a token boundary so a split secret cannot slip
 *   past the length-based regex.
 *
 * Env:
 *   GBR_MCP_LOG_LEVEL   trace|debug|info|warn|error|silent   (default: info)
 *   GBR_MCP_LOG_DIR     directory for JSONL files            (default: ~/.gbr/logs)
 *   GBR_MCP_LOG_STDERR  1|0 human lines on stderr            (default: 1)
 *   GBR_MCP_LOG_BODIES  1|0 log full HTTP bodies             (default: 1)
 *   GBR_MCP_LOG_MAX_BODY  chars before truncation            (default: 4000)
 *   GBR_MCP_LOG_RETAIN_DAYS  delete older JSONL on start     (default: 7)
 */

import { appendFileSync, mkdirSync, existsSync, readdirSync, statSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { homedir } from 'node:os';
import { randomUUID } from 'node:crypto';

const LEVELS = { trace: 10, debug: 20, info: 30, warn: 40, error: 50, silent: 99 };

/** Positive integer or fallback. Guards against `Number('20s') === NaN`. */
function posInt(v, fallback) {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : fallback;
}

const envLevel = (process.env.GBR_MCP_LOG_LEVEL || 'info').toLowerCase();
const LEVEL = LEVELS[envLevel] ?? LEVELS.info;
const LOG_DIR = process.env.GBR_MCP_LOG_DIR || join(homedir(), '.gbr', 'logs');
const TO_STDERR = process.env.GBR_MCP_LOG_STDERR !== '0';
const LOG_BODIES = process.env.GBR_MCP_LOG_BODIES !== '0';
const MAX_BODY = posInt(process.env.GBR_MCP_LOG_MAX_BODY, 4000);
const RETAIN_DAYS = posInt(process.env.GBR_MCP_LOG_RETAIN_DAYS, 7);

let logDirError = null;
try {
  if (!existsSync(LOG_DIR)) mkdirSync(LOG_DIR, { recursive: true });
} catch (err) {
  logDirError = err.message;
}

/** Resolved per write, so a long-running server rolls over at midnight. */
function currentLogFile() {
  if (logDirError) return null;
  return join(LOG_DIR, `gbr-mcp-${new Date().toISOString().slice(0, 10)}.jsonl`);
}

/** Delete JSONL older than RETAIN_DAYS. Best effort, never throws. */
function sweepOldLogs() {
  if (logDirError) return;
  const cutoff = Date.now() - RETAIN_DAYS * 86400000;
  try {
    for (const f of readdirSync(LOG_DIR)) {
      if (!/^gbr-mcp-\d{4}-\d{2}-\d{2}\.jsonl$/.test(f)) continue;
      const p = join(LOG_DIR, f);
      if (statSync(p).mtimeMs < cutoff) unlinkSync(p);
    }
  } catch { /* ignore */ }
}

// ---- secret handling --------------------------------------------------------

/** Keys whose values must never be written anywhere. Matches compound names. */
const SECRET_KEY_RE =
  /(^|[_-])(key|token|secret|password|passwd|pwd|auth|authorization|bearer|credential|apikey)($|[_-])/i;

/** Test a key name for secrecy. camelCase is normalised first so `authToken`
 *  and `clientSecret` are caught, while `hasKey`/`publicKey`/`monkey` are not. */
function isSecretKey(k) {
  const snake = k.replace(/([a-z0-9])([A-Z])/g, '$1_$2');
  if (/^(has|is|public)_?[A-Za-z]/i.test(k) && /key$/i.test(k)) return false;
  return SECRET_KEY_RE.test(k) || SECRET_KEY_RE.test(snake);
}

/** Exact values known to be secret. Shape- and encoding-independent. */
const SECRET_VALUES = new Set();

/**
 * Register a literal secret so it is scrubbed wherever it appears, regardless
 * of key name or character class. Call this the moment a credential enters
 * the process.
 */
export function registerSecret(v) {
  if (typeof v === 'string' && v.length >= 8) SECRET_VALUES.add(v);
}

function scrub(s) {
  let out = s;
  for (const v of SECRET_VALUES) {
    if (out.includes(v)) out = out.split(v).join(`[redacted:${v.length}ch]`);
  }
  return out;
}

/** Redact secrets recursively. Returns a safe deep copy. */
export function redact(value, depth = 0) {
  if (depth > 8) return '[max-depth]';
  if (value == null) return value;
  if (typeof value === 'string') {
    return scrub(value).replace(/\b[a-f0-9]{48,}\b/gi, (m) => `[redacted:${m.length}ch]`);
  }
  if (typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.map((v) => redact(v, depth + 1));
  const out = {};
  for (const [k, v] of Object.entries(value)) {
    if (isSecretKey(k)) {
      out[k] = typeof v === 'string' && v.length ? `[redacted:${v.length}ch]` : '[redacted]';
    } else {
      out[k] = redact(v, depth + 1);
    }
  }
  return out;
}

/**
 * Redact THEN stringify THEN truncate on a token boundary.
 * Order matters — see the security note at the top of this file.
 */
export function clampBody(body) {
  if (!LOG_BODIES) return '[bodies disabled]';
  if (body == null) return null;
  const safe = redact(body);
  const s = typeof safe === 'string' ? safe : JSON.stringify(safe);
  if (s.length <= MAX_BODY) return s;
  // Never cut mid-token: a split hex run would fall under the 48-char floor.
  let cut = s.slice(0, MAX_BODY);
  const runStart = cut.search(/[A-Za-z0-9_+/=-]*$/);
  if (runStart > 0) cut = cut.slice(0, runStart);
  return redact(`${cut}…[truncated ${s.length - cut.length} chars of ${s.length}]`);
}

// ---- writing ----------------------------------------------------------------

function write(level, msg, fields = {}) {
  if (LEVELS[level] < LEVEL) return;
  const safe = redact(fields);
  const rec = { ts: new Date().toISOString(), level, msg, pid: process.pid, ...safe };
  const line = JSON.stringify(rec);

  const target = currentLogFile();
  if (target) {
    try {
      appendFileSync(target, line + '\n');
    } catch {
      /* disk full / permissions — keep serving */
    }
  }

  if (TO_STDERR) {
    const cid = rec.cid ? ` [${rec.cid}]` : '';
    const extra = Object.keys(safe).length ? ' ' + JSON.stringify(safe) : '';
    process.stderr.write(`${rec.ts} ${level.toUpperCase().padEnd(5)}${cid} ${msg}${extra}\n`);
  }
}

export const log = {
  trace: (m, f) => write('trace', m, f),
  debug: (m, f) => write('debug', m, f),
  info: (m, f) => write('info', m, f),
  warn: (m, f) => write('warn', m, f),
  error: (m, f) => write('error', m, f),
};

/** A child logger that stamps every line with the same correlation id. */
export function withCorrelation(cid = randomUUID().slice(0, 8)) {
  const bind = (level) => (m, f = {}) => write(level, m, { cid, ...f });
  return {
    cid,
    trace: bind('trace'),
    debug: bind('debug'),
    info: bind('info'),
    warn: bind('warn'),
    error: bind('error'),
  };
}

export const logMeta = {
  level: envLevel,
  dir: LOG_DIR,
  get file() { return currentLogFile(); },
  dirError: logDirError,
  bodies: LOG_BODIES,
  maxBody: MAX_BODY,
  stderr: TO_STDERR,
  retainDays: RETAIN_DAYS,
};

sweepOldLogs();
log.info('logger initialised', {
  level: envLevel, dir: LOG_DIR, file: currentLogFile(),
  dirError: logDirError, bodies: LOG_BODIES, maxBody: MAX_BODY, retainDays: RETAIN_DAYS,
});
