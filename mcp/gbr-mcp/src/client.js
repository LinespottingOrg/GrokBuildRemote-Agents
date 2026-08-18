/**
 * Bot API client for gbr-agent (protocol gbr/1, agent >= 0.5.3).
 *
 * Hard-won behaviours this client compensates for — verified live against
 * gbr-agent v0.5.3 on 2026-08-17:
 *
 *  1. THE AGENT RETURNS HTTP 200 ON LOGICAL ERRORS.
 *     POST /v1/inject with an empty session_id responds 200 with
 *     {"ok":false,"error":"inject: empty session_id refused",...}.
 *     Checking response.ok (HTTP) is NOT enough — you must check body.ok.
 *
 *  2. INJECT CAN BLOCK INDEFINITELY.
 *     If no real Grok Build window is attached to the session, the agent holds
 *     the request open. Every call therefore gets an AbortController timeout.
 *
 *  3. UNKNOWN DEVICE NAMES SILENTLY FALL BACK TO `local`.
 *     POST with {"device":"nope"} returns device.id === "local". We compare the
 *     echoed device against what was asked for and warn on mismatch, otherwise
 *     an agent thinks it dispatched to a remote box when it did not.
 *
 *  4. 404s return {"error":"not_found"} with no `ok` key at all.
 */

import { withCorrelation, clampBody, registerSecret, log } from './logger.js';

/** Positive integer or fallback. `Number('20s')` is NaN, and setTimeout(NaN)
 *  fires immediately — which would make every request fail instantly with a
 *  timeout the user cannot raise. */
function posInt(v, fallback) {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : fallback;
}

const MAX_RESPONSE_BYTES = posInt(process.env.GBR_MCP_MAX_RESPONSE_BYTES, 8 * 1024 * 1024);

/** Read a response body with a hard size cap, streaming so we never buffer
 *  more than the cap. Protects against a hostile or broken peer on the port. */
async function readCapped(res, max, url) {
  if (!res.body) return '';
  const dec = new TextDecoder();
  let out = '', n = 0;
  for await (const chunk of res.body) {
    n += chunk.length;
    if (n > max) {
      throw new GbrError(`Response exceeded ${max} bytes`, {
        code: 'GBR_RESPONSE_TOO_LARGE',
        status: res.status,
        endpoint: url,
        hint: 'Lower `limit`, or raise GBR_MCP_MAX_RESPONSE_BYTES if this is legitimate.',
      });
    }
    out += dec.decode(chunk, { stream: true });
  }
  return out + dec.decode();
}

export class GbrError extends Error {
  constructor(message, { code, status, endpoint, body, cause, hint } = {}) {
    super(message);
    this.name = 'GbrError';
    this.code = code || 'GBR_ERROR';
    this.status = status;
    this.endpoint = endpoint;
    this.body = body;
    this.hint = hint;
    if (cause) this.cause = cause;
  }
  toPayload() {
    return {
      error: this.message,
      code: this.code,
      status: this.status ?? null,
      endpoint: this.endpoint ?? null,
      hint: this.hint ?? null,
      body: this.body ?? null,
    };
  }
}

const DEFAULT_LOCAL = 'http://127.0.0.1:8788';

export class GbrClient {
  /**
   * @param {object} opts
   * @param {string} [opts.baseUrl]  Local hub, default http://127.0.0.1:8788
   * @param {string} [opts.relayUrl] e.g. https://gbr-relay.ekobrott.workers.dev
   * @param {string} [opts.mailbox]  gbr-xxxx  (required for relay mode)
   * @param {string} [opts.key]      mailbox key (required for relay mode)
   * @param {number} [opts.timeoutMs]
   */
  constructor(opts = {}) {
    this.relayUrl = opts.relayUrl || process.env.GBR_RELAY_URL || null;
    this.mailbox = opts.mailbox || process.env.GBR_MAILBOX_ID || null;
    this.key = opts.key || process.env.GBR_MAILBOX_KEY || null;
    this.timeoutMs = posInt(opts.timeoutMs ?? process.env.GBR_MCP_TIMEOUT_MS, 20000);

    // The key is a credential. Register it before it can reach any log sink.
    if (this.key) registerSecret(this.key);

    this.mode = this.relayUrl && this.mailbox && this.key ? 'relay' : 'local';

    if (this.mode === 'relay') {
      const u = new URL(this.relayUrl);
      const localhost = u.hostname === '127.0.0.1' || u.hostname === 'localhost';
      if (u.protocol !== 'https:' && !localhost) {
        throw new Error(
          `GBR_RELAY_URL must be https:// (got ${u.protocol}) — the mailbox key is sent on every request.`,
        );
      }
      if (!/^[A-Za-z0-9_-]+$/.test(this.mailbox)) {
        throw new Error('GBR_MAILBOX_ID must match [A-Za-z0-9_-]+');
      }
      this.baseUrl =
        `${this.relayUrl.replace(/\/$/, '')}/v1/mb/${encodeURIComponent(this.mailbox)}/bot`;
    } else {
      this.baseUrl = (opts.baseUrl || process.env.GBR_BOT_URL || DEFAULT_LOCAL).replace(/\/$/, '');
      if (this.key && !/^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])(:|\/|$)/.test(this.baseUrl)) {
        log.warn('a mailbox key is set but the bot URL is not loopback', { baseUrl: this.baseUrl });
      }
    }
  }

  /** Full options INCLUDING the key, so diagnose() probes the same target the
   *  tools use. Never log this — use describe() for that. */
  options() {
    return {
      baseUrl: this.mode === 'local' ? this.baseUrl : undefined,
      relayUrl: this.relayUrl,
      mailbox: this.mailbox,
      key: this.key,
      timeoutMs: this.timeoutMs,
    };
  }

  describe() {
    return {
      mode: this.mode,
      baseUrl: this.baseUrl,
      mailbox: this.mailbox,
      hasKey: Boolean(this.key),
      timeoutMs: this.timeoutMs,
    };
  }

  /** Build the URL for a logical endpoint in either mode. */
  url(path, query) {
    // Local mode uses /v1/<path>; relay mode uses <base>/<path> (base already ends /bot).
    // Empty path = discovery, which lives at the API ROOT. The live agent serves it
    // at "/", "/v1" and "/v1/" (all verified 200), but "/" is the documented form.
    const p = String(path ?? '').replace(/^\//, '');
    const full =
      p === ''
        ? (this.mode === 'relay' ? this.baseUrl : `${this.baseUrl}/`)
        : (this.mode === 'relay' ? `${this.baseUrl}/${p}` : `${this.baseUrl}/v1/${p}`);
    if (!query) return full;
    const qs = new URLSearchParams(
      Object.entries(query).filter(([, v]) => v !== undefined && v !== null && v !== ''),
    ).toString();
    return qs ? `${full}?${qs}` : full;
  }

  async request(method, path, { query, body, timeoutMs, cid } = {}) {
    const l = withCorrelation(cid);
    const url = this.url(path, query);
    const label = path || '<discovery>';
    const ms = posInt(timeoutMs, this.timeoutMs);
    const headers = { Accept: 'application/json' };
    if (body) headers['Content-Type'] = 'application/json';
    if (this.key) headers['X-GBR-Key'] = this.key;

    const ac = new AbortController();
    const timer = setTimeout(() => ac.abort(), ms);
    const started = Date.now();

    l.debug('http request', {
      method,
      url,
      mode: this.mode,
      timeoutMs: ms,
      body: clampBody(body),
    });

    let res, text;
    try {
      res = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: ac.signal,
      });
      // Capped read. The abort timer covers a SLOW body, but not a fast huge one,
      // and in relay mode the peer is remote and not trusted-by-locality.
      text = await readCapped(res, MAX_RESPONSE_BYTES, url);
    } catch (err) {
      clearTimeout(timer);
      if (err instanceof GbrError) throw err;
      const elapsed = Date.now() - started;
      if (err.name === 'AbortError') {
        l.error('http timeout', { method, url, elapsedMs: elapsed, timeoutMs: ms });
        throw new GbrError(`Timed out after ${ms}ms calling ${label}`, {
          code: 'GBR_TIMEOUT',
          endpoint: url,
          hint:
            path === 'inject'
              ? 'inject blocks when no live Grok Build window is attached to that session_id. Run `gbr-agent sessions` and confirm a real session exists, or raise GBR_MCP_TIMEOUT_MS.'
              : 'Is `gbr-agent run` still alive? Check `pgrep -fl "gbr-agent run"`.',
        });
      }
      l.error('http transport error', { method, url, elapsedMs: elapsed, err: err.message });
      throw new GbrError(`Cannot reach the gbr-agent Bot API at ${this.baseUrl}`, {
        code: 'GBR_UNREACHABLE',
        endpoint: url,
        cause: err,
        hint:
          this.mode === 'local'
            ? 'Start the agent: `gbr-agent run`. Confirm with `curl -sS http://127.0.0.1:8788/`. Requires agent >= 0.5.3 — older builds have no Bot API at all.'
            : 'Check GBR_RELAY_URL, GBR_MAILBOX_ID and GBR_MAILBOX_KEY, and that the target agent is running.',
      });
    }
    clearTimeout(timer);
    const elapsedMs = Date.now() - started;

    let json = null;
    try {
      json = text ? JSON.parse(text) : null;
    } catch {
      l.error('http non-json response', { method, url, status: res.status, body: clampBody(text) });
      throw new GbrError(`Non-JSON response (HTTP ${res.status}) from ${label}`, {
        code: 'GBR_BAD_RESPONSE',
        status: res.status,
        endpoint: url,
        body: clampBody(text),
        hint: 'Something other than gbr-agent may be bound to that port.',
      });
    }

    l.debug('http response', {
      method,
      url,
      status: res.status,
      elapsedMs,
      body: clampBody(json),
    });

    // Gotcha 4: 404 shape has no `ok` key.
    if (res.status === 404) {
      throw new GbrError(`Endpoint not found: ${label}`, {
        code: 'GBR_NOT_FOUND',
        status: 404,
        endpoint: url,
        body: json,
        hint: 'Agent may be older than 0.5.3. Check `gbr-agent version`.',
      });
    }

    if (res.status === 401 || res.status === 403) {
      throw new GbrError(`Unauthorized (HTTP ${res.status}) calling ${label}`, {
        code: 'GBR_UNAUTHORIZED',
        status: res.status,
        endpoint: url,
        hint: 'Mailbox key missing or stale. Re-pair: `gbr-agent pair`, then copy the key from phone Settings -> Bot API.',
      });
    }

    if (res.status === 429) {
      throw new GbrError('Rate limited by the relay (60 injects/min per mailbox)', {
        code: 'GBR_RATE_LIMITED',
        status: 429,
        endpoint: url,
        hint: 'Back off and retry. Batch prompts instead of sending many small injects.',
      });
    }

    if (!res.ok) {
      throw new GbrError(`HTTP ${res.status} from ${label}`, {
        code: 'GBR_HTTP_ERROR',
        status: res.status,
        endpoint: url,
        body: json,
      });
    }

    // Gotcha 1: logical failure inside a 200.
    if (json && json.ok === false) {
      l.warn('logical error in 200 response', { url, body: clampBody(json) });
      throw new GbrError(json.error || 'Agent reported ok:false', {
        code: 'GBR_AGENT_REFUSED',
        status: res.status,
        endpoint: url,
        body: json,
        hint: hintForAgentError(json.error),
      });
    }

    return json;
  }

  // ---- endpoints -----------------------------------------------------------

  discovery(cid) { return this.request('GET', '', { cid }); }
  status(cid) { return this.request('GET', 'status', { cid }); }
  devices(cid) { return this.request('GET', 'devices', { cid }); }
  sessions(device, cid) { return this.request('GET', 'sessions', { query: { device }, cid }); }

  output({ session_id, command_id, after, limit } = {}, cid) {
    return this.request('GET', 'output', {
      query: { session_id, command_id, after, limit },
      cid,
    });
  }

  async inject({ device, session_id, text, submit = true, notify_phone, command_id, wait_idle, wait_ms, idle_ms }, cid, timeoutMs) {
    const l = withCorrelation(cid);
    const body = { text, submit };
    if (device) body.device = device;
    if (session_id) body.session_id = session_id;
    if (notify_phone !== undefined) body.notify_phone = notify_phone;
    if (command_id) body.command_id = command_id;
    if (wait_idle) body.wait_idle = true;
    if (wait_ms) body.wait_ms = wait_ms;
    if (idle_ms) body.idle_ms = idle_ms;

    const res = await this.request('POST', 'inject', { body, cid, timeoutMs });

    // Gotcha 3: silent fallback to local.
    const echoed = res?.device?.id;
    if (device && echoed && echoed !== device) {
      l.warn('device fallback detected', { requested: device, actual: echoed });
      res._warning =
        `Requested device "${device}" but the agent dispatched to "${echoed}". ` +
        `Unknown device names silently fall back to local. Run gbr_devices and check the name, ` +
        `or register it with: gbr-agent fleet add -name ${device} -mailbox gbr-XXXX -key KEY`;
    }
    return res;
  }

  addDevice({ name, mailbox_id, key, os }, cid) {
    return this.request('POST', 'devices', { body: { name, mailbox_id, key, os }, cid });
  }

  open(body, cid, timeoutMs) {
    return this.request('POST', 'sessions/open', { body, cid, timeoutMs });
  }

  result({ session_id, command_id, wait_ms, idle_ms, excerpt_bytes, device } = {}, cid, timeoutMs) {
    const wait = posInt(wait_ms, 0);
    // Local wait can run up to 180s; give the HTTP abort a little headroom.
    const ms = posInt(timeoutMs, wait > 0 ? wait + 8000 : this.timeoutMs);
    return this.request('GET', 'result', {
      query: { session_id, command_id, wait_ms, idle_ms, excerpt_bytes, device },
      cid,
      timeoutMs: ms,
    });
  }

  lockAcquire(body, cid) {
    return this.request('POST', 'lock', { body, cid });
  }

  lockRelease({ session_id, holder, force } = {}, cid) {
    return this.request('DELETE', 'lock', {
      query: { session_id, holder, force: force ? '1' : undefined },
      cid,
    });
  }

  lockStatus(session_id, cid) {
    return this.request('GET', 'lock', { query: { session_id }, cid });
  }

  tasks(query, cid) {
    return this.request('GET', 'tasks', { query, cid });
  }

  upsertTask(body, cid) {
    return this.request('POST', 'tasks', { body, cid });
  }
}

function hintForAgentError(msg = '') {
  const m = String(msg).toLowerCase();
  if (m.includes('empty session_id')) {
    return 'Pass session_id explicitly. Get valid ids from gbr_sessions. The agent will not guess.';
  }
  if (m.includes('session')) {
    return 'That session_id is not in the live roster. Run gbr_sessions — titles update when windows open/close or after `gbr-agent rename`.';
  }
  if (m.includes('device')) {
    return 'Run gbr_devices. Remotes must be registered with `gbr-agent fleet add` before they can be targeted.';
  }
  return 'Check the agent log: ~/.gbr/logs/agent-<date>.jsonl';
}
