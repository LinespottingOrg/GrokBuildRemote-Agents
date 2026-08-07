# Self-hosted GBR relay

Default production relay: `https://gbr-relay.ekobrott.workers.dev`

Most users never need this. Self-host when you want the durable mailbox on **your** Cloudflare account (compliance, air-gapped WAN edge, or custom domain).

Phone and PC **never** open ports to each other. Both push/poll the same Worker over **HTTPS 443**.

## 1. Deploy the Worker

From the agents repo:

```bash
cd relay
cp wrangler.example.toml wrangler.toml
# fill account_id + create/bind Durable Object / KV as in wrangler.example.toml
npx wrangler deploy
```

Note the URL, e.g. `https://gbr-relay.YOUR_SUBDOMAIN.workers.dev` (no path suffix).

Optional custom domain in Cloudflare Workers → Triggers → Custom Domains.

Smoke:

```bash
curl -sS https://YOUR_RELAY/health
# expect 200 + JSON/ok
```

## 2. Point the desktop agent

```bash
export GBR_RELAY_URL=https://YOUR_RELAY
gbr-agent pair
gbr-agent run
```

Or one-shot:

```bash
gbr-agent run -relay https://YOUR_RELAY
gbr-agent pair -relay https://YOUR_RELAY
gbr-agent netcheck -relay https://YOUR_RELAY
```

### Auto-start service + custom relay

Install the service **with `GBR_RELAY_URL` set in the same shell** so macOS LaunchAgent / Linux systemd embed it:

```bash
export GBR_RELAY_URL=https://YOUR_RELAY
gbr-agent service install
gbr-agent service status
```

Windows Task Scheduler inherits the user environment at logon; set a user env var `GBR_RELAY_URL` in System Properties → Environment, then re-logon / re-run the task.

## 3. Point the phone app

| Platform | Where |
|----------|--------|
| **iOS** | Settings → Relay URL → paste `https://YOUR_RELAY` → Save relay URL |
| **Android** | Settings → RELAY (self-host optional) → Save relay URL |

Both sides must use the **same** base URL. Re-pair after switching hosts (mailbox key lives on that Worker).

## 4. Pair again

1. On PC: `GBR_RELAY_URL=… gbr-agent pair` (QR in browser).
2. On phone: scan QR / Complete pairing (relay already set).
3. `gbr-agent run` (or service already running).
4. Phone: tap session health / Test relay → expect `https.health: PASS`.

## 5. Firewall

Still **outbound HTTPS 443 only** — allowlist your relay host instead of `*.workers.dev` if you use a custom domain. See [NETWORK.md](NETWORK.md).

## 6. Auth

Production relay enforces `X-GBR-Key` after pair. Self-hosted deploys the same Worker code — pair both phone and agent against **your** host so they share one mailbox key.

Do **not** set `GBR_AUTH_MODE=warn` in production except for a staged migration of old clients.

## Checklist

- [ ] `/health` returns 200 on the new host  
- [ ] `gbr-agent netcheck -relay https://YOUR_RELAY` all core checks PASS  
- [ ] Phone Settings relay URL saved  
- [ ] Fresh pair on that host  
- [ ] Inject + heartbeat visible on phone  

## Docs

- Protocol: [protocol/v1.md](protocol/v1.md)  
- Relay internals: [relay/README.md](relay/README.md)  
- Network / firewall: [NETWORK.md](NETWORK.md)  
