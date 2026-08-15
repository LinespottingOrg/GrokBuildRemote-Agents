# Build Remote Agent — desktop (`gbr-agent`)

**Product brand:** Build Remote Agent  
**Binary:** `gbr-agent`  
**License:** **MIT** (open source)  
**Org:** LinespottingOrg / Linespotting AB  

Desktop agents for **Windows**, **macOS**, and **Linux**. They discover local terminal / **Grok Build** sessions, inject input, capture output, and exchange **protocol `gbr/1`** envelopes over HTTPS — your phone and PC never open ports to each other.

This agent pairs with the paid **Build Remote Agent** mobile apps (iOS / Android, $13 one-time). The mobile clients are **not** open source; this desktop agent **is**.

### Why it exists

Grok Build (SpaceX / xAI agentic coding CLI) gained a new interface and API surface around **16 July 2026**. That made remote control of *local* Grok Build sessions practical. This open-source agent + paid mobile remote implements that workflow. Independent product — not affiliated with SpaceX or xAI except as a client of public APIs / open tooling.

---


## Independence & trademarks

**Build Remote Agent is an independent, third-party product from Linespotting AB.**
It is **not** affiliated with, endorsed by, or sponsored by **xAI** or **SpaceX**, and it is
**not** covered by the **Grok** trademark or brand. "Grok" and "Grok Build" are used solely to
describe compatibility with the user's own locally installed Grok Build CLI.

- **Desktop agent (this repo): 100% open source** under the MIT license.
- **Mobile apps ("Build Remote Agent" on iOS / Android): separate commercial products**, closed source (private repository), sold by Linespotting AB.

## Open source

| What | Policy |
|------|--------|
| Source (`cmd/`, `internal/`, this repo) | **Public · MIT** |
| Official binaries | Free for end users |
| Mobile apps | Separate private repo; paid $13 |

**Repo:** https://github.com/LinespottingOrg/GrokBuildRemote-Agents  
**Website:** https://grokbuildremote.com/

---

## Free download

- **Website:** https://grokbuildremote.com/#download  
- **GitHub Releases:** tagged `v*` assets  
- **Clone & build:** see below  

| File | Platform |
|------|----------|
| `gbr-agent-windows-amd64.exe` | Windows x64 |
| `gbr-agent-darwin-amd64` | macOS Intel |
| `gbr-agent-darwin-arm64` | macOS Apple Silicon |
| `gbr-agent-linux-amd64` | Linux x64 |
| `gbr-agent-linux-arm64` | Linux ARM64 |

---

## Quick install

### One-liner (macOS / Linux)

```bash
curl -fsSL https://grokbuildremote.com/install.sh | bash
```

### One-liner (Windows PowerShell)

```powershell
irm https://grokbuildremote.com/install.ps1 | iex
```

### From source

```bash
git clone https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents
# build per Makefile / go build in cmd/
```

Default install locations:

| OS | Directory |
|----|-----------|
| macOS / Linux | `~/.local/bin/gbr-agent` |
| Windows (user) | `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` |

### Typical flow

```bash
gbr-agent pair
# opens browser QR — scan with the phone camera
# legacy: gbr-agent pair -code YOURCODE

# Firewall / VPN / ports (no inbound ports — outbound HTTPS 443 only)
gbr-agent netcheck
gbr-agent netcheck -doc
# See NETWORK.md
gbr-agent run

# Optional: start at login in background (after pair)
gbr-agent service install
gbr-agent service status
```

### Self-hosted relay

Optional — most users keep the production Worker. See **[SELF-HOSTED-RELAY.md](SELF-HOSTED-RELAY.md)**.

```bash
export GBR_RELAY_URL=https://your-worker.workers.dev
gbr-agent pair && gbr-agent run
# same URL in phone Settings → Relay
```

### What’s new in 0.5.0

- Richer heartbeat (`agent_version`, `relay`, `os`) for phone session health  
- `service install` messaging + embed `GBR_RELAY_URL` into LaunchAgent / systemd when set  
- Service working directory = user home (avoids synthetic `dist` sessions)  
- Feedback peeks (0.4.5+) unchanged; inject/pair protocol still `gbr/1`  

Optional: `sessions`, `status`, `version`, `rename -name MyPC`.

### Session names (this branch)

The phone shows a **title**, not the raw `conhost` / folder slug. See **[SESSION-NAMES.md](SESSION-NAMES.md)**.

```text
/rename Phone Grok                  # in Grok Build (slash command)
gbr-agent sessions                  # list grok-build-… ids
gbr-agent rename -session grok-build-40a22 -name "Phone Grok"
```

No six-session feedback cap — every discovered window is advertised; Grok is first.

### API key

Agent may read relay / model API credentials from env (`GBR_API_KEY`, `XAI_API_KEY`, etc.) or local config under `~/.gbr/` — see support docs on the website.

---

## Contributing

Issues and PRs welcome on GitHub. Please keep protocol `gbr/1` compatibility stable unless versioned intentionally.

## Licensing note

This desktop agent is MIT-licensed and free. The companion **mobile apps**
("Build Remote Agent") are separate commercial products and are not covered
by this repository's MIT license.
