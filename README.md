# Grok Build Remote — Agents (`gbr-agent`)

**Brand:** Grok Build Remote  
**Tagline:** Control Grok Build CLI sessions from your phone. No network, no VPN, Grok is the relay.  
**Binary:** `gbr-agent`  
**Org:** LinespottingOrg / Linespotting AB

Desktop agents for **Windows**, **macOS**, and **Linux**. They discover local terminal sessions, inject input, capture output, and exchange **protocol `gbr/1`** envelopes through the Grok API — your phone and PC never open ports to each other.

---

## Private repository (company IP)

This GitHub repository is **private**. The **source code is proprietary** intellectual property of LinespottingOrg (see [`LICENSE`](./LICENSE)).

| What | Policy |
|------|--------|
| Source (`cmd/`, `internal/`, this repo) | **Private** — all rights reserved |
| Official **binaries** (`gbr-agent`) | **Free** for end users |
| Distribution channels | Product **website** + **Microsoft Store** (planned) |
| Mobile apps | Separate private repo; paid $13.00 per store — NOT free |

You may download and run free binaries under the Free Binary Use terms in `LICENSE`. You may **not** treat this repo as open source (it is **not** MIT).

---

## Free download

- **Website install funnel** (primary for users): product homepage / install page (public web).
- **Release assets**: tagged builds (`v*`) publish platform binaries (CI in [`.github/workflows/release.yml`](./.github/workflows/release.yml)).
- **Microsoft Store**: packaging planned for Windows; sideload + website remain available for full inject capabilities during rollout.

Asset names:

| File | Platform |
|------|----------|
| `gbr-agent-windows-amd64.exe` | Windows x64 |
| `gbr-agent-darwin-amd64` | macOS Intel |
| `gbr-agent-darwin-arm64` | macOS Apple Silicon |
| `gbr-agent-linux-amd64` | Linux x64 |
| `gbr-agent-linux-arm64` | Linux ARM64 |

---

## Quick install

### One-liner (macOS / Linux / Git Bash)

```bash
curl -fsSL https://raw.githubusercontent.com/LinespottingOrg/GrokBuildRemote-Agents/main/install/install.sh | bash
```

Pin a version or CDN base:

```bash
export GBR_VERSION=v0.1.0
export GBR_DOWNLOAD_BASE="https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download"
./install/install.sh
```

Default install locations:

| OS | Directory |
|----|-----------|
| macOS / Linux | `~/.local/bin/gbr-agent` |
| Windows (user) | `%LOCALAPPDATA%\GrokBuildRemote\gbr-agent.exe` |
| Windows (elevated) | `%ProgramFiles%\GrokBuildRemote\gbr-agent.exe` |

### From a release asset

Download the matching asset, `chmod +x` on Unix, place on `PATH`, then:

```bash
gbr-agent run
```

### API key

Agent reads the Grok / xAI API key from:

- Unix: `~/.grok/config.json`
- Windows: `%USERPROFILE%\.grok\config.json`

Do not commit keys. Use a local gitignored `api-passwords.md` if your team tracks secrets offline.

---

## Run as a service

| Platform | Guide | Unit / sample |
|----------|--------|----------------|
| **Windows** | [`install/windows/service.md`](./install/windows/service.md) | [`install/windows/gbr-agent.xml`](./install/windows/gbr-agent.xml) (WinSW) |
| **macOS** | launchd (user agent) | [`install/darwin/launchd.plist.example`](./install/darwin/launchd.plist.example) |
| **Linux** | systemd (prefer **user** unit) | [`install/linux/gbr-agent.service`](./install/linux/gbr-agent.service) |

**Why user session matters:** inject uses OS APIs (`SendInput`, AppleScript / Accessibility, `xdotool`). Session 0 services and headless system units often cannot reach interactive terminals.

---

## Architecture

```
┌─────────────┐     Grok API (HTTPS JSON)      ┌──────────────────┐
│  Mobile app │ ◄──── protocol gbr/1 ────────► │  gbr-agent (PC)  │
│ iOS/Android │   no phone↔PC sockets/VPN     │ Win / Mac / Linux│
└─────────────┘                               └────────┬─────────┘
                                                       │
                                          discover sessions / inject
                                                       ▼
                                              local terminals
                                         (WT, iTerm, gnome-term, …)
```

### Components (this monorepo-style tree)

| Path | Role |
|------|------|
| `cmd/gbr-agent` | Main binary entrypoint |
| `internal/core` | Config, lifecycle |
| `internal/grok` | Grok client / relay mailbox |
| `internal/session` | Session discovery + naming |
| `internal/inject/windows` | SendInput inject + capture |
| `internal/inject/darwin` | AppleScript / macOS inject |
| `internal/inject/linux` | xdotool / Linux inject |
| `install/` | Installer + service units |
| `scripts/` | Cross-compile helpers |

### Link protocol concepts (`gbr/1`)

Shared contract lives in the product **protocol** pack (sibling docs: `protocol/v1.md`, `protocol/schema.json`).

| Concept | Meaning |
|---------|---------|
| `proto: gbr/1` | Envelope version on every message |
| `device_id` | UUID of one PC installation |
| `session_id` | Named terminal slug (e.g. `global-edition`) |
| `command_id` | Idempotent inject id |
| `pairing_code` | One-time mobile↔agent link (8-char Crockford base32) |
| Message types | `pair`, `register`, `list`, `inject`, `output`, `heartbeat` |

**Relay pattern:** mobile and agent never dial each other. Both talk to Grok with a device-scoped mailbox / structured chat envelopes. Fallback: shared pairing token + authenticated Grok API (see protocol v1).

---

## Build from source (contributors with repo access)

Requirements: Go 1.22+, git.

**Unix:**

```bash
./scripts/build-all.sh
# artifacts in ./dist/
```

**PowerShell:**

```powershell
.\scripts\build-all.ps1
```

Targets: `windows/amd64`, `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`.

Single platform:

```bash
CGO_ENABLED=0 go build -o gbr-agent ./cmd/gbr-agent
```

---

## Release CI

Push a tag matching `v*`:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Workflow [`.github/workflows/release.yml`](./.github/workflows/release.yml):

1. Matrix-builds all five platform binaries  
2. Uploads artifacts  
3. Creates a GitHub Release with assets + `SHA256SUMS`

---

## Microsoft Store (future)

Windows Store (MSIX) packaging is planned so non-technical users can install **free** agents with OS update semantics. Until Store certification is complete:

- Website + GitHub Release binaries remain the supported free path  
- WinSW / Task Scheduler docs cover always-on operation  
- Store builds may have capability differences; full inject sideload remains available

---

## Security notes

- No inbound listen port required for the relay design  
- API keys stay on device (`~/.grok/config.json`)  
- Source is private; only use **official** free binaries  
- Report security issues privately to LinespottingOrg — do not open public issues with exploit detail

---

## License

**Proprietary — All Rights Reserved.**  
Free binary use for end users. **Not MIT / not open source.**  

See [`LICENSE`](./LICENSE).

Copyright © 2026 LinespottingOrg / Linespotting AB.
