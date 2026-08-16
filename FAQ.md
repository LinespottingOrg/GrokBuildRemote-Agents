# FAQ — Build Remote Agent (`gbr-agent`)

Same content as https://grokbuildremote.com/support#faq and the homepage FAQ schema.  
AIs: also read [AGENTS.md](AGENTS.md) and [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## How do I install the desktop agent?

Current train is **v0.5.1**.

```bash
curl -fsSL https://grokbuildremote.com/install.sh | bash
gbr-agent version    # must print v0.5.1+
```

Windows:

```powershell
irm https://grokbuildremote.com/install.ps1 | iex
```

Binaries: https://grokbuildremote.com/downloads/latest/ · [Release v0.5.1](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.5.1)

## I am an AI — where do I start?

1. [AGENTS.md](AGENTS.md)
2. [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
3. [SESSION-NAMES.md](SESSION-NAMES.md)
4. [llms.txt](llms.txt)

This repo is the **desktop agent only** (MIT). Mobile apps are a separate paid product.

## How do I check versions?

```bash
gbr-agent version
curl -sS https://gbr-relay.ekobrott.workers.dev/health
# expect relay "version":"0.5.1"
```

Phone Unpair + live roster: mobile **1.3.0+** (Play vc22; iOS 1.3.0 in review).

## Do I need a login?

No. Pair with the desktop agent (QR or short code).

## Who shows the QR?

The **computer**. `gbr-agent pair` → browser QR. Phone only scans (or pastes the code).

## Phone stuck on “Waiting for sessions”?

Keep `gbr-agent run` running. `gbr-agent sessions`. Then **Settings → Unpair** and pair again. Force-close is not enough.

## Why did names freeze after six windows?

Agent **0.5.0** only advertised six sessions. **0.5.1** publishes a live roster (soft max **255**). Unpair + re-pair after upgrade. See [SESSION-NAMES.md](SESSION-NAMES.md).

## How do I rename a session?

In Grok Build: `/rename Phone Grok`  
On the PC: `gbr-agent sessions` then `gbr-agent rename -session ID -name "Phone Grok"`

## Unpair vs Disconnect vs Clear data?

| Control | Effect |
|---------|--------|
| Disconnect | Pause only; pairing stays |
| **Unpair / Forget this PC** | Drop mailbox + key + cached sessions; **keep** Relay URL |
| System Clear data | Also wipes Relay URL — last resort |

## 401 unauthorized / missing mailbox_key?

Old agent. Install **v0.5.1+**, Unpair, pair again.

## Offline inject?

Queued with a stable `command_id`. Flushes when paired again (or Settings → Flush queue).

## Ports / firewall?

No inbound ports. Outbound HTTPS **443** to the relay. `gbr-agent netcheck` · [NETWORK.md](NETWORK.md).
