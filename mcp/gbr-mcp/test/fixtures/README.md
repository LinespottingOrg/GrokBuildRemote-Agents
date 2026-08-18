# Test fixtures

Offline replacements for a real `gbr-agent`, so the fleet path can be exercised
in CI with no agent, no relay and no network.

| File | What it is |
|---|---|
| `fake-agent.mjs` | Bot API emulator on `127.0.0.1:8788`. Reproduces the real v0.5.3 response shapes **including its quirks**: HTTP 200 on logical errors, silent `device` fallback to `local`, `{"error":"not_found"}` 404s. |
| `fleet-sim.mjs` | Drives `gbr-mcp` over stdio with real JSON-RPC and walks the full fleet workflow: devices → sessions → inject → output, plus both gotchas. |

## Run

Both must run in the same shell — the emulator is a background process.

**bash (macOS / Linux):**
```
cd path/to/GrokBuildRemote-Agents/mcp/gbr-mcp
node test/fixtures/fake-agent.mjs & AGENT=$!
sleep 2
node test/fixtures/fleet-sim.mjs
kill $AGENT
```

Verified on Ubuntu 22.04 / Node 22.22.3 and macOS 26.5 / Node 26.7.0.
