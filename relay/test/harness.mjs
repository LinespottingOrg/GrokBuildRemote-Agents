/**
 * In-memory Cloudflare Worker stubs so relay tests run without wrangler.
 * QUEUE / TRACE Durable Objects + KV. No network, no secrets.
 */
import worker, { MailboxQueue, TraceBuffer } from "../src/index.js";

class MemoryStorage {
  constructor() {
    this.map = new Map();
  }
  async get(key) {
    return this.map.has(key) ? this.map.get(key) : undefined;
  }
  async put(key, value) {
    this.map.set(key, value);
  }
  async delete(keys) {
    const list = Array.isArray(keys) ? keys : [keys];
    for (const k of list) this.map.delete(k);
  }
  async list({ prefix } = {}) {
    const out = new Map();
    for (const [k, v] of this.map) {
      if (!prefix || String(k).startsWith(prefix)) out.set(k, v);
    }
    return out;
  }
}

class MemoryState {
  constructor() {
    this.storage = new MemoryStorage();
  }
  async blockConcurrencyWhile(fn) {
    return fn();
  }
}

class DOStub {
  constructor(impl) {
    this.impl = impl;
  }
  fetch(input, init) {
    const req = input instanceof Request ? input : new Request(input, init);
    return this.impl.fetch(req);
  }
}

class MemoryKV {
  constructor() {
    this.map = new Map();
  }
  async get(key, type) {
    if (!this.map.has(key)) return null;
    const v = this.map.get(key);
    if (type === "json") {
      if (v == null) return null;
      if (typeof v === "object") return v;
      try {
        return JSON.parse(v);
      } catch {
        return null;
      }
    }
    return v;
  }
  async put(key, value) {
    this.map.set(key, value);
  }
  async delete(key) {
    this.map.delete(key);
  }
}

class Namespace {
  constructor(Factory) {
    this.Factory = Factory;
    this.instances = new Map();
  }
  idFromName(name) {
    return { name: String(name) };
  }
  get(id) {
    const key = id && id.name ? id.name : String(id);
    if (!this.instances.has(key)) {
      this.instances.set(key, new DOStub(new this.Factory(new MemoryState())));
    }
    return this.instances.get(key);
  }
}

export function makeEnv(overrides = {}) {
  return {
    GBR_AUTH_MODE: "enforce",
    MB: new MemoryKV(),
    QUEUE: new Namespace(MailboxQueue),
    TRACE: new Namespace(TraceBuffer),
    ...overrides,
  };
}

export function makeCtx() {
  return { waitUntil() {} };
}

export async function call(env, method, path, { key, body } = {}) {
  const headers = { Accept: "application/json" };
  if (key) headers["X-GBR-Key"] = key;
  const init = { method, headers };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = typeof body === "string" ? body : JSON.stringify(body);
  }
  const req = new Request("https://gbr-relay.test" + path, init);
  const res = await worker.fetch(req, env, makeCtx());
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  return { status: res.status, json, text };
}

/** Fail if mailbox_key appears anywhere in a public JSON payload. */
export function findMailboxKey(value, path = "$") {
  if (value == null) return null;
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) {
      const hit = findMailboxKey(value[i], `${path}[${i}]`);
      if (hit) return hit;
    }
    return null;
  }
  if (typeof value === "object") {
    for (const [k, v] of Object.entries(value)) {
      if (k === "mailbox_key") return `${path}.${k}`;
      const hit = findMailboxKey(v, `${path}.${k}`);
      if (hit) return hit;
    }
  }
  return null;
}
