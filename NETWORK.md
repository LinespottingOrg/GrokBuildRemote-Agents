# Network · firewalls · VPN · ports

**Product:** Build Remote Agent (`gbr-agent` + mobile apps)  
**Protocol:** `gbr/1` over **HTTPS**

---

## Short answer

| Question | Answer |
|----------|--------|
| Open inbound ports on the PC? | **No** |
| Open inbound ports on the phone? | **No** |
| Phone ↔ PC direct connection? | **Never** |
| What must work? | **Outbound HTTPS (TCP 443)** from **both** phone and PC to the **relay** |
| VPN between phone and PC? | **Not needed** (and often irrelevant) |
| LAN / same Wi‑Fi required? | **No** |

Corporate firewalls and “strict” VPNs only break GBR when they **block or MITM outbound HTTPS** to the relay host.

---

## Architecture

```
┌─────────────┐         HTTPS 443          ┌──────────────────┐
│  Phone app  │ ─────────────────────────► │  GBR relay       │
└─────────────┘                            │  (Cloudflare     │
                                           │   Worker + KV)   │
┌─────────────┐         HTTPS 443          │                  │
│  gbr-agent  │ ─────────────────────────► │                  │
│  (PC/Mac)   │                            └──────────────────┘
└─────────────┘
        │
        └── local inject into Grok Build / terminal (no network)
```

- Pair, inject, poll, heartbeat, and output all go through the relay.
- The agent injects keystrokes **locally** on the PC after receiving an inject envelope from the relay.

---

## Hosts and ports

### Required (production default)

| Direction | Protocol | Port | Destination | Purpose |
|-----------|----------|------|-------------|---------|
| Outbound from **PC** | TCP / HTTPS | **443** | `gbr-relay.ekobrott.workers.dev` | pair, push, poll, ack, health |
| Outbound from **phone** | TCP / HTTPS | **443** | same relay host | pair, push, poll |

**Base URL:** `https://gbr-relay.ekobrott.workers.dev`  
Override: env `GBR_RELAY_URL` (agent) / app debug build may use env on Android.

**Paths used:**

| Path | Used by |
|------|---------|
| `GET /health` | agent doctor / netcheck |
| `POST /v1/mb/{id}/pair` | phone + agent |
| `POST /v1/mb/{id}/push` | phone + agent |
| `GET /v1/mb/{id}/poll` | phone + agent |
| `POST /v1/mb/{id}/ack` | agent (optional) |
| `POST /v1/mb/{id}/trace` | observability |

### Optional (nice to have)

| Direction | Port | Destination | Purpose |
|-----------|------|-------------|---------|
| Outbound | 443 | `grokbuildremote.com` | install scripts, **pair.html** QR browser page |
| Outbound | 443 | `github.com` / release CDN | downloading agent binaries |

If the optional site is blocked but the **relay** works, pair still works via terminal code + phone **type code manually** (QR browser page is convenience only).

### Explicitly NOT required

- Inbound TCP/UDP any port on phone or PC  
- UPnP / port forwarding / DMZ  
- mDNS / Bonjour / NetBIOS  
- UDP hole punching  
- WireGuard/OpenVPN **between** phone and PC  
- Allowlisting the phone’s LAN IP on the PC  

---

## Firewall allowlist (IT template)

```
# Build Remote Agent — outbound only
# Protocol: HTTPS
Allow: TCP 443 → gbr-relay.ekobrott.workers.dev
# Optional:
Allow: TCP 443 → grokbuildremote.com
```

If your relay is custom (`GBR_RELAY_URL=https://my-relay.example.com`), allowlist **that** host instead.

**TLS inspection:** If the proxy re-signs certificates, either:

1. Exempt `gbr-relay.ekobrott.workers.dev` from inspection, **or**  
2. Install the corporate root CA so the agent and mobile OS trust the MITM cert.

---

## VPN scenarios

| Scenario | Expected result |
|----------|-----------------|
| Consumer VPN on phone only | Usually OK if VPN allows HTTPS |
| Consumer VPN on PC only | Usually OK |
| Split tunnel blocking Cloudflare | **FAIL** netcheck `dns` or `tcp` |
| “Kill switch” with no internet | **FAIL** all HTTPS |
| Corp VPN + SSL inspection | May **FAIL** `tls` or `https.health` |
| Phone on cellular, PC on office LAN | **OK** — both use public HTTPS |

---

## How to test

### On the PC (agent)

```bash
# Full firewall/VPN path (DNS → TCP 443 → TLS → /health)
gbr-agent netcheck

# Print this policy
gbr-agent netcheck -doc

# Platform inject + network
gbr-agent doctor
```

Exit code **0** = OK, **1** = blocked.

### On the phone

Settings → **Test relay** (network check).  
Expect: DNS/HTTPS reachability to the same relay host.

### Simulate block (lab)

```bash
# Example: force bad relay to see FAIL (do not use in production)
GBR_RELAY_URL=https://127.0.0.1:1 gbr-agent netcheck
```

---

## Failure → meaning

| netcheck line | Typical cause |
|---------------|---------------|
| `dns` FAIL | DNS filter, captive portal, VPN DNS blackhole |
| `tcp` FAIL | Firewall blocks outbound 443 / proxy required |
| `tls` FAIL | SSL inspection without trusted CA |
| `https.health` FAIL | HTTP proxy auth, category block, WAF |
| all PASS but pair fails | Wrong code / mailbox key — not network |

---

## Security note

Opening **inbound** ports for GBR is unnecessary and **increases** attack surface.  
Do not punch holes “to make pairing work” — fix **outbound** HTTPS to the relay instead.
