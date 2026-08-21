/**
 * Relay 0.6.0 device class contract (local, no network).
 *
 * POST /bot/devices with class=mac_mini → GET list includes class.
 * Public payloads never include mailbox_key.
 *
 * Health: 0.5.4 or 0.6.0 until A04. If RELAY_VERSION on disk is 0.6.0,
 * /health must report 0.6.0.
 *
 * Run: node test/class.test.mjs   (from agents/relay)
 */
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { call, makeEnv, findMailboxKey } from "./harness.mjs";

const root = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(root, "../src/index.js"), "utf8");
const onDisk = (src.match(/const RELAY_VERSION = "([^"]+)"/) || [])[1] || "";

let pass = 0;
let fail = 0;

function check(name, cond, detail = "") {
  if (cond) {
    pass++;
    console.log(`  PASS  ${name}${detail ? "  " + detail : ""}`);
  } else {
    fail++;
    console.log(`  FAIL  ${name}${detail ? "  " + detail : ""}`);
  }
}

console.log("=== gbr-relay class tests (local worker) ===");
console.log(`on-disk RELAY_VERSION=${onDisk || "?"}`);

const env = makeEnv();
const mb = "gbr-classtest";
const dummyKey = "qa-dummy-key-not-a-secret-" + "0".repeat(32);

// --- health ---
const health = await call(env, "GET", "/health");
check("health.status_200", health.status === 200, `status=${health.status}`);
check("health.ok", health.json && health.json.ok === true, "");
check("health.proto", health.json && health.json.proto === "gbr/1", `proto=${health.json && health.json.proto}`);

if (onDisk === "0.6.0") {
  check("health.version", health.json && health.json.version === "0.6.0", `version=${health.json && health.json.version}`);
} else {
  const v = health.json && health.json.version;
  check("health.version", v === "0.5.4" || v === "0.6.0", `version=${v} (0.5.4 or 0.6.0 until A04)`);
}

if (onDisk === "0.6.0" || (health.json && health.json.version === "0.6.0")) {
  const classes = (health.json && health.json.classes) || [];
  check("health.classes_mac_mini", classes.includes("mac_mini"), `classes=${JSON.stringify(classes)}`);
}

check("health.no_mailbox_key", !findMailboxKey(health.json), findMailboxKey(health.json) || "clean");

// --- pair (issues X-GBR-Key; /bot/devices is key-guarded) ---
const pair = await call(env, "POST", `/v1/mb/${mb}/pair`, {
  body: { pairing_code: "CLASSTEST", device_id: "dev-hub", device_name: "ClassHub" },
});
const key = pair.json && pair.json.mailbox_key;
check("pair.ok", pair.status === 200 && pair.json && pair.json.ok === true, `status=${pair.status}`);
check("pair.key_issued", typeof key === "string" && key.length >= 32, `len=${key ? key.length : 0}`);

// --- POST device class=mac_mini ---
const post = await call(env, "POST", `/v1/mb/${mb}/bot/devices`, {
  key,
  body: {
    id: "qa-mac-mini",
    name: "QA Mac Mini",
    mailbox_id: "gbr-qaremote01",
    mailbox_key: dummyKey,
    os: "darwin",
    class: "mac_mini",
    hostname: "qa-mini.local",
    impl: "gbr",
  },
});
check("devices.post.status_200", post.status === 200, `status=${post.status} body=${post.text}`);
check("devices.post.ok", post.json && post.json.ok === true, "");
const posted = post.json && post.json.device;
check("devices.post.id", posted && posted.id === "qa-mac-mini", `id=${posted && posted.id}`);
check("devices.post.class_mac_mini", posted && posted.class === "mac_mini", `class=${posted && posted.class}`);
check("devices.post.no_mailbox_key", !findMailboxKey(post.json), findMailboxKey(post.json) || "clean");
check("devices.post.has_key", posted && posted.has_key === true, `has_key=${posted && posted.has_key}`);
check("devices.post.key_not_echoed", posted && !Object.prototype.hasOwnProperty.call(posted, "mailbox_key"), "");

// --- GET list includes class; public payload has no mailbox_key ---
const list = await call(env, "GET", `/v1/mb/${mb}/bot/devices`, { key });
check("devices.get.status_200", list.status === 200, `status=${list.status}`);
check("devices.get.ok", list.json && list.json.ok === true, "");
const devices = (list.json && list.json.devices) || [];
const hit = devices.find((d) => d && d.id === "qa-mac-mini");
check("devices.list.includes_row", !!hit, `n=${devices.length}`);
check("devices.list.class_mac_mini", hit && hit.class === "mac_mini", `class=${hit && hit.class}`);
check("devices.list.no_mailbox_key", !findMailboxKey(list.json), findMailboxKey(list.json) || "clean");
check("devices.list.has_key_only", hit && hit.has_key === true && !("mailbox_key" in hit), "");

const dumped = JSON.stringify(list.json);
check(
  "devices.list.json_has_no_secret",
  !dumped.includes(dummyKey) && !dumped.includes('"mailbox_key"'),
  "secret/key field absent from GET JSON"
);

console.log("");
const colorFail = fail > 0;
console.log(`RESULT: ${pass} passed, ${fail} failed`);
if (colorFail) process.exit(1);
process.exit(0);
