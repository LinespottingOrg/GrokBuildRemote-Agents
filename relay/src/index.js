/**
 * Grok Build Remote — durable mailbox relay (Day-1).
 * Phone and PC never connect; both POST/GET envelopes by mailbox id.
 *
 * Routes:
 *   GET  /health
 *   POST /v1/mb/:id/push   body: envelope JSON (gbr/1)
 *   GET  /v1/mb/:id/poll?after=<iso>&for=<device_id>&role=agent|mobile
 *   POST /v1/mb/:id/pair  body: { pairing_code, device_id, device_name }
 *   POST /v1/mb/:id/ack   body: { command_ids: string[] }  — drop processed injects
 *
 *   --- observability (additive, v0.4.0) ---
 *   POST /v1/mb/:id/trace body: event | { events: [...] }  — append trace events
 *   GET  /v1/mb/:id/trace?after=<iso>&limit=N              — read trace ring buffer
 *   DELETE /v1/mb/:id/trace                                — clear ring buffer
 *
 *   --- bot REST (additive, v0.5.2) — Grok bots / HTTP clients ---
 *   GET  /v1/bot
 *   GET  /v1/mb/:id/bot
 *   GET  /v1/mb/:id/bot/sessions
 *   POST /v1/mb/:id/bot/inject   body: { session_id, text, submit }
 *   GET  /v1/mb/:id/bot/output?after=&session_id=&command_id=&limit=
 *   GET  /v1/mb/:id/bot/status
 *   Auth: X-GBR-Key or Authorization: Bearer <mailbox_key>
 */

const MAX_QUEUE = 500;
const MAX_TRACE = 400;
const TRACE_TTL = 60 * 60 * 24 * 7; // 7 days
const RELAY_VERSION = "0.5.4";
const MAX_FLEET = 32;
const BOT_INJECT_WINDOW_MS = 60 * 1000;
const BOT_INJECT_MAX = 60;
const BOT_TEXT_MAX = 32 * 1024;

// Pair throttling — see MailboxQueue "pairattempt".
const PAIR_WINDOW_MS = 60 * 60 * 1000; // 1 hour
const PAIR_MAX_ATTEMPTS = 12;

// Auth enforcement mode.
//
// PHASE 1 (now): "warn" — keys are issued and verified, mismatches are traced,
// but nothing is rejected. iOS is already RELEASED and Google Play is IN REVIEW;
// flipping straight to "enforce" would brick every shipped client instantly.
//
// PHASE 2: ship agent + app builds that pair, store the key and send it.
// PHASE 3: set GBR_AUTH_MODE=enforce once /health shows unauthenticated traffic
//          has fallen to ~zero. Only then is the relay safe to open-source.
function authMode(env) {
  const m = String(env.GBR_AUTH_MODE || "warn").toLowerCase();
  return m === "enforce" ? "enforce" : "warn";
}
const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, Authorization, X-GBR-Key",
};

export default {
  async fetch(request, env, ctx) {
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: CORS });
    }
    try {
      const url = new URL(request.url);
      if (url.pathname === "/health" || url.pathname === "/") {
        return json({
          ok: true,
          service: "gbr-relay",
          proto: "gbr/1",
          version: RELAY_VERSION,
          trace: true,
          auth_mode: authMode(env),
          auth_header: "X-GBR-Key",
          product: "Grok Build Remote",
          bot: true,
          bot_path: "/v1/mb/:id/bot",
          fleet: true,
        });
      }
      if (url.pathname === "/v1/bot" || url.pathname === "/bot") {
        return json(botDiscovery(""));
      }
      const botPath = url.pathname.match(/^\/v1\/mb\/([^/]+)\/bot(?:\/(.*))?$/);
      if (botPath) {
        const mailboxId = sanitizeId(decodeURIComponent(botPath[1]));
        if (!mailboxId) return json({ error: "bad_mailbox" }, 400);
        const rest = (botPath[2] || "").replace(/\/+$/, "");
        return handleBot(env, ctx, mailboxId, rest, request, url);
      }
      const m = url.pathname.match(/^\/v1\/mb\/([^/]+)\/(push|poll|pair|ack|trace)$/);
      if (!m) return json({ error: "not_found" }, 404);
      const mailboxId = sanitizeId(decodeURIComponent(m[1]));
      if (!mailboxId) return json({ error: "bad_mailbox" }, 400);
      const action = m[2];

      if (action === "push" && request.method === "POST") {
        return handlePush(env, ctx, mailboxId, request);
      }
      if (action === "poll" && request.method === "GET") {
        return handlePoll(env, ctx, mailboxId, url, request);
      }
      // NOTE: /pair is deliberately NOT key-guarded — it is where the key comes
      // from. It is protected by per-mailbox attempt throttling instead.
      if (action === "pair" && request.method === "POST") {
        return handlePair(env, ctx, mailboxId, request);
      }
      if (action === "ack" && request.method === "POST") {
        return handleAck(env, ctx, mailboxId, request);
      }
      if (action === "trace") {
        if (request.method === "POST") return handleTracePush(env, mailboxId, request);
        if (request.method === "GET") return handleTraceRead(env, mailboxId, url);
        if (request.method === "DELETE") return handleTraceClear(env, mailboxId);
      }
      return json({ error: "method_not_allowed" }, 405);
    } catch (e) {
      return json({ error: "internal", message: String(e && e.message ? e.message : e) }, 500);
    }
  },
};

function sanitizeId(id) {
  if (!id || id.length > 128) return "";
  if (!/^[A-Za-z0-9._:-]+$/.test(id)) return "";
  return id;
}

/** X-GBR-Key, or Authorization: Bearer <key> for Grok bots / curl. */
function presentedKey(request) {
  const h = (request.headers.get("X-GBR-Key") || "").trim();
  if (h) return h;
  const auth = request.headers.get("Authorization") || "";
  const m = auth.match(/^Bearer\s+(\S+)/i);
  return m ? m[1].trim() : "";
}

/* ----------------------------- trace core ----------------------------- */

// Trace storage lives in a Durable Object, one per mailbox.
//
// Two earlier designs were wrong and both were caught by tests:
//   1. KV read-modify-write on a single key — concurrent writers (agent batch
//      pusher, phone, relay selfTrace) silently clobbered each other. Observed
//      live: 2 of 4 agent hops missing from the relay while the local agent
//      JSONL had all 4.
//   2. Append-only timestamped KV keys + list() — lossless, but KV list is
//      eventually consistent, so fresh hops took many seconds to appear. A
//      trace you can't read live is useless for debugging a live integration.
//
// A Durable Object gives serialized writes AND strongly consistent reads, which
// is exactly what a live correlated trace needs.
const TRACE_LEGACY_KEY = (m) => `t:${m}`;

function traceStub(env, mailboxId) {
  const id = env.TRACE.idFromName(mailboxId);
  return env.TRACE.get(id);
}

function queueStub(env, mailboxId) {
  const id = env.QUEUE.idFromName(mailboxId);
  return env.QUEUE.get(id);
}

/** Get-or-create the mailbox secret. Idempotent — both pairers get the same key. */
async function mailboxKey(env, mailboxId) {
  const res = await queueStub(env, mailboxId).fetch("https://q/key", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  return body.mailbox_key || "";
}

/**
 * Check a presented X-GBR-Key.
 * Returns one of: no_key | missing | valid | invalid.
 * "no_key" means this mailbox predates auth — legacy clients keep working.
 */
async function checkKey(env, mailboxId, presented) {
  const res = await queueStub(env, mailboxId).fetch(
    `https://q/checkkey?k=${encodeURIComponent(presented || "")}`
  );
  const body = await res.json().catch(() => ({}));
  return body.state || "no_key";
}

/**
 * Gate a mutating request. In "warn" mode this never blocks — it only reports,
 * so the rollout can be measured before anything is enforced.
 */
async function guard(env, ctx, mailboxId, request, op) {
  const presented = presentedKey(request);
  const state = await checkKey(env, mailboxId, presented);

  // In enforce mode a mailbox must be PAIRED (keyed) before it accepts traffic.
  //
  // Treating "no_key" as allowed left a pre-pair injection window: an attacker
  // who guessed a code could queue an inject into a mailbox that did not exist
  // yet, and the agent would execute it on its first poll after pairing. Only
  // /pair may touch a keyless mailbox.
  const ok =
    state === "valid" || (state === "no_key" && authMode(env) !== "enforce");

  if (!ok) {
    selfTrace(env, ctx, mailboxId, {
      hop: "relay.auth_reject",
      type: op,
      ok: false,
      detail: `state=${state} mode=${authMode(env)}`,
    });
  }
  if (!ok && authMode(env) === "enforce") {
    return json({ error: "unauthorized", reason: state }, 401);
  }
  return null; // allowed
}

/** Normalize any inbound object into a trace event. */
function normalizeEvent(raw, fallbackActor) {
  const e = raw && typeof raw === "object" ? raw : {};
  const commandId = str(e.command_id || e.commandId);
  return {
    trace_id: str(e.trace_id || e.traceId || commandId || cryptoId()),
    ts: str(e.ts) || new Date().toISOString(),
    hop: str(e.hop) || "unknown",
    actor: str(e.actor) || fallbackActor || "unknown",
    type: str(e.type),
    device_id: str(e.device_id || e.deviceId),
    session_id: str(e.session_id || e.sessionId),
    command_id: commandId,
    ok: e.ok === undefined ? true : !!e.ok,
    ms: Number.isFinite(e.ms) ? e.ms : undefined,
    detail: str(e.detail).slice(0, 500),
  };
}

/** Append a batch. Serialized by the Durable Object — no lost updates. */
async function appendTrace(env, mailboxId, events) {
  if (!events || !events.length) return 0;
  const res = await traceStub(env, mailboxId).fetch("https://trace/append", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ events }),
  });
  const body = await res.json().catch(() => ({}));
  return body.added || 0;
}

/** Read the full timeline (strongly consistent). */
async function readTrace(env, mailboxId) {
  const res = await traceStub(env, mailboxId).fetch("https://trace/read");
  const body = await res.json().catch(() => ({}));
  const out = Array.isArray(body.events) ? body.events : [];
  out.sort((a, b) => String(a.ts).localeCompare(String(b.ts)));
  return out;
}

async function clearTrace(env, mailboxId) {
  await traceStub(env, mailboxId).fetch("https://trace/clear", { method: "DELETE" });
  await env.MB.delete(TRACE_LEGACY_KEY(mailboxId)).catch(() => {});
}

/**
 * TraceBuffer — one Durable Object per mailbox holding the hop ring buffer.
 * The DO runtime serializes concurrent fetches, so read-append-write is safe.
 */
export class TraceBuffer {
  constructor(state) {
    this.state = state;
  }

  async fetch(request) {
    const method = request.method;
    if (method === "POST") {
      const { events } = await request.json();
      const list = Array.isArray(events) ? events : [];
      // One key per batch — an independent write that reads nothing, so it
      // cannot clobber a concurrent writer. blockConcurrencyWhile fixed the
      // warm-path interleave (14/16 → 16/16) but not the cold-start instance
      // split; per-key writes close both.
      if (list.length) {
        const key = `b:${String(Date.now()).padStart(14, "0")}-${Math.random()
          .toString(36)
          .slice(2, 10)}`;
        await this.state.storage.put(key, list);
      }
      const size = await this.trim();
      return new Response(JSON.stringify({ ok: true, added: list.length, size }), {
        headers: { "Content-Type": "application/json" },
      });
    }
    if (method === "DELETE") {
      const all = await this.state.storage.list({ prefix: "b:" });
      if (all.size) await this.state.storage.delete([...all.keys()]);
      await this.state.storage.delete("buf");
      return new Response(JSON.stringify({ ok: true, cleared: true }), {
        headers: { "Content-Type": "application/json" },
      });
    }
    const all = await this.state.storage.list({ prefix: "b:" });
    const events = [];
    const legacy = await this.state.storage.get("buf");
    if (Array.isArray(legacy)) events.push(...legacy);
    for (const batch of all.values()) {
      if (Array.isArray(batch)) events.push(...batch);
      else if (batch) events.push(batch);
    }
    return new Response(JSON.stringify({ ok: true, events }), {
      headers: { "Content-Type": "application/json" },
    });
  }

  /** Bound the ring buffer by dropping the oldest batch keys. */
  async trim() {
    const all = await this.state.storage.list({ prefix: "b:" });
    let total = 0;
    for (const b of all.values()) total += Array.isArray(b) ? b.length : 1;
    if (total <= MAX_TRACE) return total;
    const keys = [...all.keys()];
    const doomed = [];
    let dropped = 0;
    for (const k of keys) {
      if (total - dropped <= MAX_TRACE) break;
      const b = all.get(k);
      dropped += Array.isArray(b) ? b.length : 1;
      doomed.push(k);
    }
    if (doomed.length) await this.state.storage.delete(doomed);
    return total - dropped;
  }
}

/** Fire-and-forget relay self-trace; never blocks or fails the main response. */
function selfTrace(env, ctx, mailboxId, ev) {
  const event = normalizeEvent(ev, "relay");
  const work = appendTrace(env, mailboxId, [event]).catch(() => {});
  if (ctx && typeof ctx.waitUntil === "function") ctx.waitUntil(work);
}

function str(v) {
  return v === undefined || v === null ? "" : String(v);
}

function cryptoId() {
  try {
    return crypto.randomUUID();
  } catch {
    return `t-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  }
}

async function handleTracePush(env, mailboxId, request) {
  const body = await request.json();
  const list = Array.isArray(body) ? body : Array.isArray(body?.events) ? body.events : [body];
  const events = list.filter(Boolean).slice(0, 100).map((e) => normalizeEvent(e));
  const added = await appendTrace(env, mailboxId, events);
  return json({ ok: true, added, size: added });
}

async function handleTraceRead(env, mailboxId, url) {
  const after = url.searchParams.get("after") || "";
  const limit = Math.min(parseInt(url.searchParams.get("limit") || "200", 10) || 200, MAX_TRACE);
  const commandId = url.searchParams.get("command_id") || "";
  const t = await readTrace(env, mailboxId);

  const afterMs = after ? Date.parse(after) : 0;
  let out = t.filter((e) => {
    if (afterMs) {
      const ts = e.ts ? Date.parse(e.ts) : 0;
      if (ts && ts <= afterMs) return false;
    }
    if (commandId && String(e.command_id) !== commandId) return false;
    return true;
  });
  if (out.length > limit) out = out.slice(out.length - limit);
  return json({ ok: true, events: out, total: t.length, now: new Date().toISOString() });
}

async function handleTraceClear(env, mailboxId) {
  await clearTrace(env, mailboxId);
  return json({ ok: true, cleared: true });
}

/* --------------------------- mailbox queue DO -------------------------- */

/**
 * MailboxQueue — one Durable Object per mailbox holding the envelope queue.
 *
 * This used to be a single KV key mutated with read-modify-write by BOTH
 * handlePush and handleAck. Those race: the agent acks command A while the
 * phone pushes command B, the ack handler reads a snapshot taken before B
 * arrived, filters A out of that stale copy and writes it back — silently
 * erasing B. Observed live: an inject was pushed and traced, then vanished
 * from the queue with no agent hop and no error anywhere. The tell was queue
 * size going DOWN across consecutive pushes (90 → 88).
 *
 * Each envelope now lives under its own storage key, so a push is a single
 * independent write that reads nothing and can clobber nothing — safe even
 * across a cold-start instance split, which blockConcurrencyWhile alone did
 * NOT cover. DO storage.list() is strongly consistent, so reads stay live.
 */
export class MailboxQueue {
  constructor(state) {
    this.state = state;
  }

  /** One storage key per envelope; lexicographic order == chronological. */
  static key() {
    return `e:${String(Date.now()).padStart(14, "0")}-${Math.random().toString(36).slice(2, 10)}`;
  }

  async fetch(request) {
    const url = new URL(request.url);
    const action = url.pathname.replace(/^\//, "");

    if (action === "push") {
      const envelope = await request.json();
      // Latest-wins for the live session roster. The agent re-registers and
      // heartbeats often; keeping every copy made the queue grow (200+), so a
      // phone that later polls still saw old titles / a 6-item cache.
      // Inject/output/pair/control stay append-only (command_id / stream).
      await this.state.blockConcurrencyWhile(async () => {
        await this.dropStaleRoster(envelope);
        await this.state.storage.put(MailboxQueue.key(), envelope);
      });
      const size = await this.trim();
      return jsonResponse({ ok: true, size });
    }

    // ---- auth: per-mailbox key, issued at pairing ----
    //
    // The mailbox id is currently a bearer credential for REMOTE CODE EXECUTION:
    // anyone who knows it can push an inject and have it typed into the user's
    // terminal. The id is just `gbr-` + the lowercased 8-char pairing code, and
    // the derivation is public in the open-source agent. This issues a real
    // secret at pairing so the on-screen code stops being the long-lived key.
    if (action === "key") {
      const key = await this.state.blockConcurrencyWhile(async () => {
        let k = await this.state.storage.get("auth_key");
        if (!k) {
          k = crypto.randomUUID().replace(/-/g, "") + crypto.randomUUID().replace(/-/g, "");
          await this.state.storage.put("auth_key", k);
          await this.state.storage.put("auth_created", new Date().toISOString());
        }
        return k;
      });
      return jsonResponse({ ok: true, mailbox_key: key });
    }

    if (action === "checkkey") {
      const presented = url.searchParams.get("k") || "";
      const stored = await this.state.storage.get("auth_key");
      if (!stored) return jsonResponse({ ok: true, state: "no_key" });
      if (!presented) return jsonResponse({ ok: true, state: "missing" });
      return jsonResponse({ ok: true, state: presented === stored ? "valid" : "invalid" });
    }

    // Pair attempt throttling. The pairing code is the only thing standing
    // between an attacker and command injection, so unbounded guessing is not
    // acceptable even at 32^8 combinations.
    if (action === "pairattempt") {
      const res = await this.state.blockConcurrencyWhile(async () => {
        const now = Date.now();
        let win = (await this.state.storage.get("pair_window")) || { start: now, n: 0 };
        if (now - win.start > PAIR_WINDOW_MS) win = { start: now, n: 0 };
        win.n += 1;
        await this.state.storage.put("pair_window", win);
        return { allowed: win.n <= PAIR_MAX_ATTEMPTS, attempts: win.n };
      });
      return jsonResponse({ ok: true, ...res });
    }

    if (action === "snapshot") {
      const all = await this.state.storage.list({ prefix: "e:" });
      const envelopes = [];
      for (const envl of all.values()) {
        if (envl) envelopes.push(envl);
      }
      return jsonResponse({ ok: true, envelopes, size: envelopes.length });
    }

    if (action === "fleetget") {
      const fleet = (await this.state.storage.get("fleet")) || { devices: [] };
      return jsonResponse({ ok: true, devices: Array.isArray(fleet.devices) ? fleet.devices : [] });
    }

    if (action === "fleetput") {
      const body = await request.json().catch(() => ({}));
      const devices = Array.isArray(body.devices) ? body.devices.slice(0, MAX_FLEET) : [];
      await this.state.storage.put("fleet", { devices, updated: new Date().toISOString() });
      return jsonResponse({ ok: true, n: devices.length });
    }

    if (action === "leaseget") {
      const leases = (await this.state.storage.get("leases")) || { items: [] };
      return jsonResponse({ ok: true, leases: Array.isArray(leases.items) ? leases.items : [] });
    }

    if (action === "leaseput") {
      const body = await request.json().catch(() => ({}));
      const items = Array.isArray(body.leases) ? body.leases.slice(0, 128) : [];
      await this.state.storage.put("leases", { items, updated: new Date().toISOString() });
      return jsonResponse({ ok: true, n: items.length });
    }

    if (action === "taskget") {
      const tasks = (await this.state.storage.get("tasks")) || { items: [] };
      return jsonResponse({ ok: true, tasks: Array.isArray(tasks.items) ? tasks.items : [] });
    }

    if (action === "taskput") {
      const body = await request.json().catch(() => ({}));
      const items = Array.isArray(body.tasks) ? body.tasks.slice(0, 200) : [];
      await this.state.storage.put("tasks", { items, updated: new Date().toISOString() });
      return jsonResponse({ ok: true, n: items.length });
    }

    if (action === "botrate") {
      const res = await this.state.blockConcurrencyWhile(async () => {
        const now = Date.now();
        let win = (await this.state.storage.get("bot_window")) || { start: now, n: 0 };
        if (now - win.start > BOT_INJECT_WINDOW_MS) win = { start: now, n: 0 };
        win.n += 1;
        await this.state.storage.put("bot_window", win);
        return { allowed: win.n <= BOT_INJECT_MAX, attempts: win.n };
      });
      return jsonResponse({ ok: true, ...res });
    }

    if (action === "ack") {
      const { command_ids: ids, from_device: fromDevice } = await request.json();
      const set = new Set((Array.isArray(ids) ? ids : []).map(String));
      const all = await this.state.storage.list({ prefix: "e:" });
      const doomed = [];
      for (const [k, envl] of all) {
        if (!envl || !envl.command_id || !set.has(String(envl.command_id))) continue;
        // Never ack away the acker's OWN envelopes.
        //
        // The agent answers a `list` using the SAME command_id as the request,
        // then acks that command_id — which deleted its own reply. The old KV
        // ack raced and sometimes lost, so the reply survived by luck; once the
        // DO queue made acks reliable, `adv.list_sessions` failed every run.
        if (fromDevice && String(envl.device_id) === String(fromDevice)) continue;
        doomed.push(k);
      }
      if (doomed.length) await this.state.storage.delete(doomed);
      return jsonResponse({ ok: true, removed: doomed.length, size: all.size - doomed.length });
    }

    // poll — DO storage.list is strongly consistent, unlike KV list
    const after = url.searchParams.get("after") || "";
    const role = url.searchParams.get("role") || "agent";
    const all = await this.state.storage.list({ prefix: "e:" });

    const afterMs = after ? Date.parse(after) : 0;
    const out = [];
    for (const envl of all.values()) {
      if (!envl) continue;
      const stamp = envl.recv_ts || envl.ts;
      const ts = stamp ? Date.parse(stamp) : 0;
      if (afterMs && ts && ts <= afterMs) continue;
      if (role === "agent") {
        if (envl.type === "output" || envl.type === "heartbeat" || envl.type === "register") continue;
      } else if (envl.type === "inject") {
        continue;
      }
      out.push(envl);
    }
    return jsonResponse({ ok: true, envelopes: out });
  }

  /** Drop superseded roster envelopes so name/count changes stay current. */
  async dropStaleRoster(envelope) {
    if (!envelope || !envelope.type) return;
    const all = await this.state.storage.list({ prefix: "e:" });
    const doomed = [];
    for (const [k, envl] of all) {
      if (!envl || envl.type !== envelope.type) continue;
      if (envelope.type === "register") {
        if (envelope.session_id && String(envl.session_id) === String(envelope.session_id)) {
          doomed.push(k);
        }
      } else if (envelope.type === "list" || envelope.type === "heartbeat") {
        if (!envelope.device_id || String(envl.device_id) === String(envelope.device_id)) {
          doomed.push(k);
        }
      }
    }
    if (doomed.length) await this.state.storage.delete(doomed);
  }

  /** Keep the queue bounded by dropping the oldest keys. */
  async trim() {
    const all = await this.state.storage.list({ prefix: "e:" });
    if (all.size <= MAX_QUEUE) return all.size;
    const excess = [...all.keys()].slice(0, all.size - MAX_QUEUE);
    await this.state.storage.delete(excess);
    return all.size - excess.length;
  }
}

function jsonResponse(obj) {
  return new Response(JSON.stringify(obj), {
    headers: { "Content-Type": "application/json" },
  });
}

/* --------------------------- mailbox handlers -------------------------- */

async function handlePush(env, ctx, mailboxId, request) {
  // Push is the dangerous verb — an inject here becomes keystrokes in a terminal.
  const denied = await guard(env, ctx, mailboxId, request, "push");
  if (denied) return denied;

  const body = await request.json();
  if (!body || body.proto !== "gbr/1" || !body.type) {
    return json({ error: "invalid_envelope" }, 400);
  }
  if (!body.ts) body.ts = new Date().toISOString();
  // Server-side receive stamp. Poll cursors are server time (`now` from /poll),
  // but `ts` is set by the CLIENT when it builds the envelope. A phone that
  // creates an envelope at T and pushes it at T+1.2s would have it silently
  // dropped by any agent that polled in between, because ts <= after.
  // Observed live: an inject vanished with zero errors anywhere in the chain.
  // Always compare cursors against this server stamp instead.
  body.recv_ts = new Date().toISOString();

  const res = await queueStub(env, mailboxId).fetch("https://q/push", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const { size } = await res.json();

  selfTrace(env, ctx, mailboxId, {
    hop: "relay.push",
    type: body.type,
    device_id: body.device_id,
    session_id: body.session_id,
    command_id: body.command_id,
    detail: `queued size=${size}`,
  });
  return json({ ok: true, size });
}

async function handlePoll(env, ctx, mailboxId, url, request) {
  // Polling returns terminal output — confidentiality, not just integrity.
  const denied = await guard(env, ctx, mailboxId, request, "poll");
  if (denied) return denied;

  const after = url.searchParams.get("after") || "";
  const forDevice = url.searchParams.get("for") || "";
  const role = url.searchParams.get("role") || "agent";

  // Role filtering happens inside the DO so we don't ship the whole queue out
  // on every poll. Agent consumes inject/list/pair; mobile consumes
  // output/register/heartbeat/list replies.
  const q = new URLSearchParams({ role });
  if (after) q.set("after", after);
  const res = await queueStub(env, mailboxId).fetch(`https://q/poll?${q}`);
  const body = await res.json();
  const out = Array.isArray(body.envelopes) ? body.envelopes : [];

  // Only trace non-empty deliveries — idle polling must not churn storage.
  if (out.length) {
    selfTrace(env, ctx, mailboxId, {
      hop: role === "agent" ? "relay.deliver_agent" : "relay.deliver_mobile",
      actor: "relay",
      device_id: forDevice,
      type: out.map((e) => e.type).join(","),
      command_id: out.find((e) => e.command_id)?.command_id || "",
      detail: `delivered=${out.length} role=${role}`,
    });
  }
  return json({ ok: true, envelopes: out, now: new Date().toISOString() });
}

/** Drop processed envelopes by command_id (agent ack after inject). */
async function handleAck(env, ctx, mailboxId, request) {
  // Ack deletes queued envelopes — unauthenticated it is a denial-of-service.
  const denied = await guard(env, ctx, mailboxId, request, "ack");
  if (denied) return denied;

  const body = await request.json();
  const ids = Array.isArray(body.command_ids) ? body.command_ids : [];
  if (!ids.length) return json({ ok: true, removed: 0, size: 0 });
  const res = await queueStub(env, mailboxId).fetch("https://q/ack", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ command_ids: ids, from_device: body.from_device || "" }),
  });
  const result = await res.json();
  return json({ ok: true, removed: result.removed || 0, size: result.size || 0 });
}

async function handlePair(env, ctx, mailboxId, request) {
  const body = await request.json();
  const code = String(body.pairing_code || "").toUpperCase().trim();
  const deviceId = body.device_id || "";
  const deviceName = body.device_name || "";
  if (!code || code.length < 6) return json({ error: "bad_code" }, 400);

  // Throttle before doing any work — this is the brute-force surface.
  const att = await queueStub(env, mailboxId).fetch("https://q/pairattempt", { method: "POST" });
  const attempt = await att.json().catch(() => ({ allowed: true }));
  if (!attempt.allowed) {
    selfTrace(env, ctx, mailboxId, {
      hop: "relay.pair_throttled",
      type: "pair",
      device_id: deviceId,
      ok: false,
      detail: `attempts=${attempt.attempts} window=${PAIR_WINDOW_MS}ms`,
    });
    return json({ error: "too_many_pair_attempts", retry_after_s: 3600 }, 429);
  }

  // Issue (or fetch) the mailbox secret. Both the agent and the phone call
  // /pair with the same code, so both receive the same key.
  const key = await mailboxKey(env, mailboxId);

  selfTrace(env, ctx, mailboxId, {
    hop: "relay.pair",
    type: "pair",
    device_id: deviceId,
    detail: `name=${deviceName} keyed=true`,
  });

  const pkey = `pair:${code}`;
  const existing = await env.MB.get(pkey, "json");
  if (existing && existing.device_id && deviceId && existing.device_id !== deviceId) {
    // second party (mobile) joining — attach mobile marker
    existing.mobile_joined = true;
    existing.mailbox_id = existing.mailbox_id || mailboxId;
    await env.MB.put(pkey, JSON.stringify(existing), { expirationTtl: 600 });
    return json({
      ok: true,
      mailbox_id: existing.mailbox_id,
      device_id: existing.device_id,
      device_name: existing.device_name,
      mailbox_key: key,
    });
  }

  const rec = {
    pairing_code: code,
    device_id: deviceId,
    device_name: deviceName,
    mailbox_id: mailboxId,
    created_at: new Date().toISOString(),
  };
  await env.MB.put(pkey, JSON.stringify(rec), { expirationTtl: 600 });
  // also store reverse lookup
  await env.MB.put(`mbmeta:${mailboxId}`, JSON.stringify(rec), { expirationTtl: 86400 * 30 });
  return json({
    ok: true,
    mailbox_id: mailboxId,
    device_id: deviceId,
    device_name: deviceName,
    mailbox_key: key,
  });
}

/* ------------------------------ bot REST ------------------------------ */

function botDiscovery(mailboxId) {
  const base = mailboxId ? `/v1/mb/${mailboxId}/bot` : "/v1/mb/{mailbox_id}/bot";
  return {
    ok: true,
    service: "gbr-relay-bot",
    proto: "gbr/1",
    version: RELAY_VERSION,
    mailbox_id: mailboxId || undefined,
    auth: ["X-GBR-Key", "Authorization: Bearer <mailbox_key>"],
    endpoints: {
      discovery: `GET ${base}`,
      devices: `GET ${base}/devices`,
      add_device: `POST ${base}/devices`,
      sessions: `GET ${base}/sessions?device=`,
      inject: `POST ${base}/inject`,
      open: `POST ${base}/sessions/open`,
      result: `GET ${base}/result?session_id=&wait_ms=&idle_ms=`,
      lock: `GET|POST|DELETE ${base}/lock`,
      tasks: `GET|POST ${base}/tasks`,
      output: `GET ${base}/output?device=&after=&session_id=&command_id=&limit=`,
      status: `GET ${base}/status?device=`,
    },
    chain: [
      "diagnose",
      "open or attach",
      "lock",
      "inject",
      "wait idle via GET /result",
      "harvest excerpt",
      "judge and iterate or close + notify",
    ],
    inject_body: {
      device: "local | studio-linux | mac-mini",
      session_id: "grok-build-…",
      text: "the prompt to type",
      submit: true,
      notify_phone: true,
    },
    note: "One hub mailbox + key. Register other Mac/Linux PCs on /devices. Grok bot talks to this one URL. Short status lines sync to the phone on the hub mailbox.",
    local_agent: "http://127.0.0.1:8788 (loopback; gbr-agent run — same fleet)",
  };
}

function fleetSlug(id) {
  return String(id || "")
    .toLowerCase()
    .trim()
    .replace(/[_\s]+/g, "-");
}

function isLocalDevice(id) {
  const s = fleetSlug(id);
  return !s || s === "local" || s === "this" || s === "hub" || s === "self" || s === "here";
}

function publicFleetDevice(d) {
  if (!d) return null;
  return {
    id: d.id,
    name: d.name || d.id,
    kind: d.kind || "relay",
    mailbox_id: d.mailbox_id || "",
    os: d.os || "",
    has_key: !!(d.mailbox_key && String(d.mailbox_key).length),
    added_at: d.added_at || "",
  };
}

async function fleetList(env, mailboxId) {
  const res = await queueStub(env, mailboxId).fetch("https://q/fleetget");
  const body = await res.json().catch(() => ({}));
  return Array.isArray(body.devices) ? body.devices : [];
}

async function fleetSave(env, mailboxId, devices) {
  await queueStub(env, mailboxId).fetch("https://q/fleetput", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ devices }),
  });
}

async function fleetFind(env, mailboxId, id) {
  if (isLocalDevice(id)) {
    return { id: "local", name: "this PC", kind: "local", mailbox_id: mailboxId };
  }
  const slug = fleetSlug(id);
  const list = await fleetList(env, mailboxId);
  return list.find((d) => d && (d.id === slug || fleetSlug(d.name) === slug)) || null;
}

async function pushHubStatus(env, ctx, hubMailbox, request, text, commandId, sessionId) {
  const envelope = {
    proto: "gbr/1",
    type: "output",
    device_id: "gbr-bot",
    session_id: sessionId || "bot",
    command_id: commandId || cryptoId(),
    ts: new Date().toISOString(),
    payload: {
      stream: "system",
      chunk: String(text || "").slice(0, 500),
      eof: true,
      reason: "bot",
      method: "status",
    },
  };
  const pushReq = new Request("https://q/push", {
    method: "POST",
    headers: request.headers,
    body: JSON.stringify(envelope),
  });
  await handlePush(env, ctx, hubMailbox, pushReq);
}

async function botInjectInto(env, ctx, targetMailbox, presentedKey, body) {
  const text = String(body.text || body.prompt || body.nl_prompt || "").slice(0, BOT_TEXT_MAX);
  if (!text.trim()) return json({ error: "empty_text" }, 400);
  const sessionId = String(body.session_id || body.session || "").trim();
  const submit = body.submit !== false;
  const mode = String(body.mode || "text").slice(0, 16);
  const commandId = String(body.command_id || cryptoId());
  const rate = await queueStub(env, targetMailbox).fetch("https://q/botrate", { method: "POST" });
  const rateBody = await rate.json().catch(() => ({ allowed: true }));
  if (!rateBody.allowed) {
    return json({ error: "rate_limited", retry_after_s: 60 }, 429);
  }
  const envelope = {
    proto: "gbr/1",
    type: "inject",
    device_id: "gbr-bot",
    session_id: sessionId,
    command_id: commandId,
    ts: new Date().toISOString(),
    payload: {
      mode,
      text,
      nl_prompt: mode === "nl" ? text : "",
      submit,
      source: "bot",
    },
  };
  const headers = new Headers();
  headers.set("Content-Type", "application/json");
  if (presentedKey) headers.set("X-GBR-Key", presentedKey);
  const pushReq = new Request("https://q/push", {
    method: "POST",
    headers,
    body: JSON.stringify(envelope),
  });
  const pushed = await handlePush(env, ctx, targetMailbox, pushReq);
  if (pushed.status >= 400) return pushed;
  const result = await pushed.json().catch(() => ({}));
  return json({
    ok: true,
    command_id: commandId,
    session_id: sessionId,
    mailbox_id: targetMailbox,
    queued: true,
    size: result.size,
  });
}

async function mailboxSnapshot(env, mailboxId) {
  const res = await queueStub(env, mailboxId).fetch("https://q/snapshot");
  const body = await res.json().catch(() => ({}));
  return Array.isArray(body.envelopes) ? body.envelopes : [];
}

function normalizeSession(s) {
  if (!s || typeof s !== "object") return { session_id: String(s || "") };
  return {
    session_id: String(s.session_id || s.id || ""),
    title: String(s.title || ""),
    cwd: String(s.cwd || ""),
    shell: String(s.shell || ""),
    os: String(s.os || ""),
    git_remote: String(s.git_remote || ""),
  };
}

function extractSessions(envelopes) {
  const list = Array.isArray(envelopes) ? envelopes : [];
  let bestList = null;
  let bestHb = null;
  const regs = new Map();
  for (const e of list) {
    if (!e || !e.type) continue;
    const ts = Date.parse(e.recv_ts || e.ts || 0) || 0;
    if (e.type === "list" && e.payload && Array.isArray(e.payload.sessions)) {
      if (!bestList || ts >= bestList._ts) bestList = { env: e, _ts: ts };
    }
    if (e.type === "heartbeat" && e.payload && Array.isArray(e.payload.sessions)) {
      if (!bestHb || ts >= bestHb._ts) bestHb = { env: e, _ts: ts };
    }
    if (e.type === "register" && e.session_id) {
      const prev = regs.get(e.session_id);
      if (!prev || ts >= prev._ts) {
        const p = e.payload || {};
        regs.set(e.session_id, {
          session_id: e.session_id,
          title: p.title || e.session_id,
          cwd: p.cwd || "",
          shell: p.shell || "",
          os: p.os || "",
          git_remote: p.git_remote || "",
          _ts: ts,
        });
      }
    }
  }
  if (bestList && bestList._ts >= (bestHb ? bestHb._ts : 0)) {
    return bestList.env.payload.sessions.map(normalizeSession);
  }
  if (bestHb) {
    return bestHb.env.payload.sessions.map(normalizeSession);
  }
  return [...regs.values()].map(({ _ts, ...s }) => s);
}

function extractOutputs(envelopes, opts) {
  const after = opts.after || "";
  const sessionId = opts.sessionId || "";
  const commandId = opts.commandId || "";
  const limit = opts.limit || 50;
  const afterMs = after ? Date.parse(after) : 0;
  const items = [];
  for (const e of envelopes || []) {
    if (!e || e.type !== "output") continue;
    const ts = e.recv_ts || e.ts || "";
    const tsMs = ts ? Date.parse(ts) : 0;
    if (afterMs && tsMs && tsMs <= afterMs) continue;
    if (sessionId && String(e.session_id) !== sessionId) continue;
    if (commandId && String(e.command_id) !== commandId) continue;
    const p = e.payload || {};
    items.push({
      ts,
      session_id: e.session_id || "",
      command_id: e.command_id || "",
      stream: p.stream || "stdout",
      chunk: p.chunk || "",
      eof: !!p.eof,
      reason: p.reason || "",
      method: p.method || "",
    });
  }
  items.sort((a, b) => String(a.ts).localeCompare(String(b.ts)));
  return items.slice(-limit);
}

function extractStatus(envelopes) {
  let hb = null;
  let lastActivity = "";
  for (const e of envelopes || []) {
    if (!e) continue;
    const ts = e.recv_ts || e.ts || "";
    if (ts > lastActivity) lastActivity = ts;
    if (e.type === "heartbeat") {
      const t = Date.parse(ts) || 0;
      if (!hb || t >= hb._ts) hb = { env: e, _ts: t };
    }
  }
  const p = (hb && hb.env && hb.env.payload) || {};
  const sessions = extractSessions(envelopes);
  const last = hb ? hb.env.recv_ts || hb.env.ts || "" : "";
  const lastMs = last ? Date.parse(last) : 0;
  const age = lastMs ? Math.max(0, Date.now() - lastMs) : null;
  const online = age !== null && age < 90 * 1000;
  return {
    agent_online: online,
    last_seen: last || null,
    last_activity: lastActivity || null,
    age_ms: age,
    session_count: Array.isArray(p.sessions) ? p.sessions.length : sessions.length,
    agent_version: p.agent_version || "",
    os: p.os || "",
    status: p.status || (online ? "alive" : "unknown"),
    sessions,
  };
}

async function handleBot(env, ctx, mailboxId, rest, request, url) {
  const denied = await guard(env, ctx, mailboxId, request, "bot." + (rest || "discover"));
  if (denied) return denied;

  const parts = rest ? rest.split("/").filter(Boolean) : [];
  let action = parts[0] || "";
  let deviceId = url.searchParams.get("device") || "";
  const sub = parts[0] === "devices" ? parts[2] || "" : "";
  if (parts[0] === "devices" && parts[1]) deviceId = parts[1];
  if (action === "devices" && deviceId && ["sessions", "inject", "output", "status", "open", "result", "lock", "tasks"].includes(sub)) {
    action = sub;
  }
  if (parts[0] === "sessions" && parts[1] === "open") {
    action = "open";
  }

  if (!action) {
    if (request.method !== "GET") return json({ error: "method_not_allowed" }, 405);
    const fleet = await fleetList(env, mailboxId);
    const disc = botDiscovery(mailboxId);
    disc.devices = [
      { id: "local", name: "this PC", kind: "local", mailbox_id: mailboxId, has_key: true },
      ...fleet.map(publicFleetDevice),
    ];
    return json(disc);
  }

  if (action === "devices") {
    return handleBotDevices(env, ctx, mailboxId, deviceId, sub, request, url);
  }

  if (action === "sessions" || sub === "sessions") {
    if (request.method !== "GET") return json({ error: "method_not_allowed" }, 405);
    const target = await resolveBotTarget(env, mailboxId, deviceId || url.searchParams.get("device"));
    if (target.error) return target.error;
    const snap = await mailboxSnapshot(env, target.mailbox);
    return json({
      ok: true,
      mailbox_id: target.mailbox,
      device: target.public,
      sessions: extractSessions(snap),
      replace: true,
      now: new Date().toISOString(),
    });
  }

  if (action === "inject" || sub === "inject") {
    if (request.method !== "POST") return json({ error: "method_not_allowed" }, 405);
    let body = {};
    try {
      body = await request.json();
    } catch {
      return json({ error: "invalid_json" }, 400);
    }
    const want = deviceId || body.device || body.device_id || "";
    const target = await resolveBotTarget(env, mailboxId, want);
    if (target.error) return target.error;
    const keyForTarget = target.kind === "local" ? presentedKey(request) : target.mailbox_key;
    const injected = await botInjectInto(env, ctx, target.mailbox, keyForTarget, body);
    if (injected.status >= 400) return injected;
    const result = await injected.json().catch(() => ({}));
    const notify = body.notify_phone !== false;
    const label = target.public.id || "local";
    const sid = result.session_id || "";
    if (notify) {
      await pushHubStatus(
        env,
        ctx,
        mailboxId,
        request,
        `bot · ${label} · inject queued · session ${sid || "(default)"}`,
        result.command_id,
        sid || "bot"
      );
    }
    selfTrace(env, ctx, mailboxId, {
      hop: "relay.bot_inject",
      type: "inject",
      session_id: sid,
      command_id: result.command_id,
      detail: `device=${label} mailbox=${target.mailbox}`,
    });
    return json({
      ok: true,
      command_id: result.command_id,
      session_id: sid,
      device: target.public,
      mailbox_id: target.mailbox,
      queued: true,
      size: result.size,
      phone_status: notify,
    });
  }

  if (action === "output" || sub === "output") {
    if (request.method !== "GET") return json({ error: "method_not_allowed" }, 405);
    const target = await resolveBotTarget(env, mailboxId, deviceId || url.searchParams.get("device"));
    if (target.error) return target.error;
    const snap = await mailboxSnapshot(env, target.mailbox);
    const limit = Math.min(parseInt(url.searchParams.get("limit") || "50", 10) || 50, 200);
    return json({
      ok: true,
      mailbox_id: target.mailbox,
      device: target.public,
      items: extractOutputs(snap, {
        after: url.searchParams.get("after") || "",
        sessionId: url.searchParams.get("session_id") || "",
        commandId: url.searchParams.get("command_id") || "",
        limit,
      }),
      now: new Date().toISOString(),
    });
  }

  if (action === "status" || sub === "status") {
    if (request.method !== "GET") return json({ error: "method_not_allowed" }, 405);
    const want = deviceId || url.searchParams.get("device") || "";
    if (!want) {
      const hub = extractStatus(await mailboxSnapshot(env, mailboxId));
      const fleet = await fleetList(env, mailboxId);
      const devices = [{ id: "local", name: "this PC", kind: "local", mailbox_id: mailboxId, ...hub }];
      for (const d of fleet) {
        const st = extractStatus(await mailboxSnapshot(env, d.mailbox_id));
        devices.push({ ...publicFleetDevice(d), ...st });
      }
      return json({
        ok: true,
        mailbox_id: mailboxId,
        hub: true,
        device_count: devices.length,
        devices,
        now: new Date().toISOString(),
      });
    }
    const target = await resolveBotTarget(env, mailboxId, want);
    if (target.error) return target.error;
    const snap = await mailboxSnapshot(env, target.mailbox);
    return json({
      ok: true,
      mailbox_id: target.mailbox,
      device: target.public,
      ...extractStatus(snap),
      now: new Date().toISOString(),
    });
  }

  if (action === "open") {
    if (request.method !== "POST") return json({ error: "method_not_allowed" }, 405);
    let body = {};
    try {
      body = await request.json();
    } catch {
      return json({ error: "invalid_json" }, 400);
    }
    const want = deviceId || body.device || body.device_id || "";
    const target = await resolveBotTarget(env, mailboxId, want);
    if (target.error) return target.error;
    const keyForTarget = target.kind === "local" ? presentedKey(request) : target.mailbox_key;
    const pushed = await botControl(env, ctx, target.mailbox, keyForTarget, "open", body);
    if (pushed.status >= 400) return pushed;
    const result = await pushed.json().catch(() => ({}));
    if (body.notify_phone !== false) {
      await pushHubStatus(
        env, ctx, mailboxId, request,
        `bot · ${target.public.id || "local"} · open queued · ${body.session_id || body.cwd || "new"}`,
        result.command_id, body.session_id || "bot"
      );
    }
    return json({
      ok: true,
      queued: true,
      command_id: result.command_id,
      device: target.public,
      mailbox_id: target.mailbox,
      hint: "Poll GET /sessions then GET /result?session_id= after the agent opens the window.",
    });
  }

  if (action === "result") {
    if (request.method !== "GET") return json({ error: "method_not_allowed" }, 405);
    const target = await resolveBotTarget(env, mailboxId, deviceId || url.searchParams.get("device"));
    if (target.error) return target.error;
    const snap = await mailboxSnapshot(env, target.mailbox);
    const sessionId = url.searchParams.get("session_id") || "";
    const commandId = url.searchParams.get("command_id") || "";
    const items = extractOutputs(snap, { sessionId, commandId, limit: 30 });
    const excerpt = excerptFromItems(items, 4000);
    const lastTs = items.length ? items[items.length - 1].ts : "";
    const lastMs = lastTs ? Date.parse(lastTs) : 0;
    const quietMs = lastMs ? Math.max(0, Date.now() - lastMs) : 0;
    const prompt = looksLikePromptRelay(excerpt);
    const idle = prompt || (excerpt && quietMs >= 2500);
    const leases = await leaseList(env, target.mailbox);
    const lock = leases.find((l) => l && l.session_id === sessionId) || null;
    return json({
      ok: true,
      session_id: sessionId,
      command_id: commandId,
      state: idle ? "idle" : excerpt ? "busy" : "timeout",
      idle: !!idle,
      reason: prompt ? "prompt" : idle ? "quiet" : excerpt ? "output" : "empty",
      excerpt,
      excerpt_bytes: excerpt.length,
      prompt,
      quiet_ms: quietMs,
      method: "relay-snapshot",
      lock,
      device: target.public,
      mailbox_id: target.mailbox,
      now: new Date().toISOString(),
      note: "Relay result is a snapshot harvest — local GET /result?wait_ms= on 127.0.0.1:8788 waits for idle.",
    });
  }

  if (action === "lock") {
    const want = deviceId || url.searchParams.get("device") || "";
    const target = await resolveBotTarget(env, mailboxId, want);
    if (target.error) return target.error;
    if (request.method === "GET") {
      const leases = await leaseList(env, target.mailbox);
      const sid = url.searchParams.get("session_id") || "";
      if (sid) {
        return json({ ok: true, lock: leases.find((l) => l.session_id === sid) || null, device: target.public });
      }
      return json({ ok: true, locks: leases, device: target.public });
    }
    if (request.method === "POST") {
      let body = {};
      try {
        body = await request.json();
      } catch {
        return json({ error: "invalid_json" }, 400);
      }
      const sid = String(body.session_id || "").trim();
      if (!sid || sid === "session") return json({ error: "session_id_required" }, 400);
      const holder = normalizeHolderRelay(body.holder);
      const leases = await leaseList(env, target.mailbox);
      const now = Date.now();
      const live = leases.filter((l) => l && Date.parse(l.expires || 0) > now);
      const cur = live.find((l) => l.session_id === sid);
      if (cur && cur.holder !== holder && !body.steal) {
        return json({ ok: false, error: "locked", lock: cur, hint: `held by ${cur.holder}` }, 409);
      }
      const ttl = Math.min(Math.max(Number(body.ttl_s) || 900, 30), 7200);
      const rec = {
        session_id: sid,
        holder,
        goal: String(body.goal || (cur && cur.goal) || "").slice(0, 500),
        acquired: (cur && cur.holder === holder && cur.acquired) || new Date().toISOString(),
        expires: new Date(now + ttl * 1000).toISOString(),
      };
      const next = live.filter((l) => l.session_id !== sid);
      next.push(rec);
      await leaseSave(env, target.mailbox, next);
      const keyForTarget = target.kind === "local" ? presentedKey(request) : target.mailbox_key;
      await botControl(env, ctx, target.mailbox, keyForTarget, "lock", {
        session_id: sid, holder, ttl_s: ttl, steal: !!body.steal, goal: rec.goal,
      });
      if (body.notify_phone !== false) {
        await pushHubStatus(env, ctx, mailboxId, request, `bot · ${target.public.id || "local"} · lock · ${sid} · ${holder}`, "", sid);
      }
      return json({ ok: true, lock: rec, device: target.public });
    }
    if (request.method === "DELETE") {
      let body = {};
      try {
        body = await request.json();
      } catch { /* query-only */ }
      const sid = url.searchParams.get("session_id") || body.session_id || "";
      const holder = normalizeHolderRelay(url.searchParams.get("holder") || body.holder || "");
      const force = url.searchParams.get("force") === "1" || body.force;
      if (!sid) return json({ error: "session_id_required" }, 400);
      const leases = await leaseList(env, target.mailbox);
      const cur = leases.find((l) => l.session_id === sid);
      if (cur && holder && cur.holder !== holder && !force) {
        return json({ ok: false, error: "lease held by " + cur.holder, lock: cur }, 409);
      }
      await leaseSave(env, target.mailbox, leases.filter((l) => l.session_id !== sid));
      const keyForTarget = target.kind === "local" ? presentedKey(request) : target.mailbox_key;
      await botControl(env, ctx, target.mailbox, keyForTarget, "unlock", { session_id: sid, holder, steal: force });
      return json({ ok: true, released: sid, device: target.public });
    }
    return json({ error: "method_not_allowed" }, 405);
  }

  if (action === "tasks") {
    const want = deviceId || url.searchParams.get("device") || "";
    const target = await resolveBotTarget(env, mailboxId, want);
    if (target.error) return target.error;
    if (request.method === "GET") {
      const tasks = await taskList(env, target.mailbox);
      const sid = url.searchParams.get("session_id") || "";
      const id = url.searchParams.get("id") || "";
      if (id) return json({ ok: true, task: tasks.find((t) => t.id === id) || null });
      return json({ ok: true, tasks: sid ? tasks.filter((t) => t.session_id === sid) : tasks });
    }
    if (request.method === "POST") {
      let body = {};
      try {
        body = await request.json();
      } catch {
        return json({ error: "invalid_json" }, 400);
      }
      const tasks = await taskList(env, target.mailbox);
      const now = new Date().toISOString();
      const rec = {
        id: String(body.id || cryptoId()),
        session_id: String(body.session_id || "").trim(),
        holder: normalizeHolderRelay(body.holder),
        goal: String(body.goal || "").slice(0, 2000),
        status: String(body.status || "open").slice(0, 32),
        iteration: Number(body.iteration) || 0,
        last_excerpt: String(body.last_excerpt || "").slice(0, 4000),
        last_judge: String(body.last_judge || "").slice(0, 500),
        updated: now,
        created: body.created || now,
      };
      const next = tasks.filter((t) => t.id !== rec.id);
      next.push(rec);
      await taskSave(env, target.mailbox, next);
      const keyForTarget = target.kind === "local" ? presentedKey(request) : target.mailbox_key;
      await botControl(env, ctx, target.mailbox, keyForTarget, "task", rec);
      return json({ ok: true, task: rec });
    }
    return json({ error: "method_not_allowed" }, 405);
  }

  return json({ error: "not_found" }, 404);
}

async function botControl(env, ctx, targetMailbox, presentedKey, action, body) {
  const commandId = String((body && body.command_id) || cryptoId());
  const envelope = {
    proto: "gbr/1",
    type: "control",
    device_id: "gbr-bot",
    session_id: String((body && (body.session_id || body.session)) || ""),
    command_id: commandId,
    ts: new Date().toISOString(),
    payload: { action, ...(body || {}) },
  };
  const headers = new Headers();
  headers.set("Content-Type", "application/json");
  if (presentedKey) headers.set("X-GBR-Key", presentedKey);
  const pushReq = new Request("https://q/push", { method: "POST", headers, body: JSON.stringify(envelope) });
  const pushed = await handlePush(env, ctx, targetMailbox, pushReq);
  if (pushed.status >= 400) return pushed;
  return json({ ok: true, command_id: commandId, queued: true });
}

function normalizeHolderRelay(h) {
  const s = String(h || "").toLowerCase().trim().replace(/_/g, "-");
  if (!s || s === "bot" || s === "grok" || s === "gbr-bot") return "grok-bot";
  if (s === "claude" || s === "coworker" || s === "cowork" || s === "mcp") return "claude-coworker";
  if (s === "phone" || s === "mobile") return "phone";
  return s.slice(0, 64);
}

function excerptFromItems(items, max) {
  let s = "";
  for (const it of items || []) {
    if (!it || !it.chunk) continue;
    s += it.chunk;
    if (!String(it.chunk).endsWith("\n")) s += "\n";
  }
  if (s.length > max) s = s.slice(s.length - max);
  return s;
}

function looksLikePromptRelay(text) {
  const t = String(text || "").trim();
  if (!t) return false;
  const tail = t.slice(-400);
  return /(?:bash|zsh|sh|fish)[-0-9.]*\$\s*$/im.test(tail) ||
    /(?:^|\n)\s*[$#%>❯➜]\s*$/m.test(tail) ||
    /what (?:do you want|next)/i.test(tail);
}

async function leaseList(env, mailboxId) {
  const res = await queueStub(env, mailboxId).fetch("https://q/leaseget");
  const body = await res.json().catch(() => ({}));
  const now = Date.now();
  return (Array.isArray(body.leases) ? body.leases : []).filter((l) => l && Date.parse(l.expires || 0) > now);
}

async function leaseSave(env, mailboxId, leases) {
  await queueStub(env, mailboxId).fetch("https://q/leaseput", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ leases }),
  });
}

async function taskList(env, mailboxId) {
  const res = await queueStub(env, mailboxId).fetch("https://q/taskget");
  const body = await res.json().catch(() => ({}));
  return Array.isArray(body.tasks) ? body.tasks : [];
}

async function taskSave(env, mailboxId, tasks) {
  await queueStub(env, mailboxId).fetch("https://q/taskput", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tasks }),
  });
}

async function resolveBotTarget(env, hubMailbox, deviceId) {
  if (isLocalDevice(deviceId)) {
    return {
      kind: "local",
      mailbox: hubMailbox,
      public: { id: "local", name: "this PC", kind: "local", mailbox_id: hubMailbox },
    };
  }
  const found = await fleetFind(env, hubMailbox, deviceId);
  if (!found) return { error: json({ error: "unknown_device", device: deviceId }, 404) };
  if (!found.mailbox_id || !found.mailbox_key) {
    return { error: json({ error: "device_missing_key", device: found.id }, 400) };
  }
  return {
    kind: "relay",
    mailbox: found.mailbox_id,
    mailbox_key: found.mailbox_key,
    public: publicFleetDevice(found),
  };
}

async function handleBotDevices(env, ctx, mailboxId, deviceId, sub, request, url) {
  if (!deviceId) {
    if (request.method === "GET") {
      const fleet = await fleetList(env, mailboxId);
      return json({
        ok: true,
        mailbox_id: mailboxId,
        devices: [
          { id: "local", name: "this PC", kind: "local", mailbox_id: mailboxId, has_key: true },
          ...fleet.map(publicFleetDevice),
        ],
      });
    }
    if (request.method === "POST") {
      let body = {};
      try {
        body = await request.json();
      } catch {
        return json({ error: "invalid_json" }, 400);
      }
      const id = fleetSlug(body.id || body.name);
      if (!id || isLocalDevice(id) || !/^[a-z0-9][a-z0-9-]{0,62}$/.test(id)) {
        return json({ error: "bad_device_id" }, 400);
      }
      const mb = sanitizeId(String(body.mailbox_id || ""));
      const key = String(body.mailbox_key || body.key || "").trim();
      if (!mb || !key) return json({ error: "mailbox_id_and_key_required" }, 400);
      const fleet = await fleetList(env, mailboxId);
      const rec = {
        id,
        name: String(body.name || id),
        kind: "relay",
        mailbox_id: mb,
        mailbox_key: key,
        os: String(body.os || "").slice(0, 32),
        added_at: new Date().toISOString(),
      };
      const next = fleet.filter((d) => d.id !== id);
      if (next.length >= MAX_FLEET) return json({ error: "fleet_full" }, 400);
      next.push(rec);
      await fleetSave(env, mailboxId, next);
      await pushHubStatus(env, ctx, mailboxId, request, `bot · fleet + ${id} (${mb})`, "", "bot");
      return json({ ok: true, device: publicFleetDevice(rec) });
    }
    return json({ error: "method_not_allowed" }, 405);
  }

  if (request.method === "GET") {
    const found = await fleetFind(env, mailboxId, deviceId);
    if (!found) return json({ error: "unknown_device" }, 404);
    return json({ ok: true, device: publicFleetDevice(found) });
  }
  if (request.method === "DELETE") {
    if (isLocalDevice(deviceId)) return json({ error: "cannot_delete_local" }, 400);
    const slug = fleetSlug(deviceId);
    const fleet = await fleetList(env, mailboxId);
    const next = fleet.filter((d) => d.id !== slug);
    if (next.length === fleet.length) return json({ error: "unknown_device" }, 404);
    await fleetSave(env, mailboxId, next);
    await pushHubStatus(env, ctx, mailboxId, request, `bot · fleet − ${slug}`, "", "bot");
    return json({ ok: true, removed: slug });
  }
  return json({ error: "method_not_allowed" }, 405);
}

function json(obj, status = 200) {
  return new Response(JSON.stringify(obj), {
    status,
    headers: { "Content-Type": "application/json", ...CORS },
  });
}
