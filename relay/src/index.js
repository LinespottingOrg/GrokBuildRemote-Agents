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
 */

const MAX_QUEUE = 500;
const MAX_TRACE = 400;
const TRACE_TTL = 60 * 60 * 24 * 7; // 7 days
const RELAY_VERSION = "0.5.0";

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
        });
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
  const presented = request.headers.get("X-GBR-Key") || "";
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
      // Independent write to a unique key — never reads, so it cannot clobber
      // a concurrent write. Serialization alone was not enough: during DO
      // cold-start, concurrent requests briefly saw separate instances and two
      // pushes both reported size=1, losing one envelope. Warm it was 8/8.
      const envelope = await request.json();
      await this.state.storage.put(MailboxQueue.key(), envelope);
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

function json(obj, status = 200) {
  return new Response(JSON.stringify(obj), {
    status,
    headers: { "Content-Type": "application/json", ...CORS },
  });
}
