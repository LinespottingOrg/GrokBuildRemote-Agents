# Session names on the phone

The mobile app shows the **title** the agent registers for each window. On a busy Windows desktop those titles used to be empty (`conhost`) or the Windows Terminal mash (`Grok 4.6 - grok`). This build picks a stable name instead.

## What the phone displays

In order:

1. **Agent label** — set on the PC (persisted in `~/.gbr/sessions.json` → `labels`)
2. **Grok Build `/rename`** — pinned title from `~/.grok/sessions/**/summary.json`
3. **Grok `generated_title`** — used when the live window title is the usual Terminal mash
4. **Live window title**
5. **Shell / kind** (`conhost`, `pwsh`, …)

There is **no six-session fan-out cap**. Every discovered window is advertised up to a **soft max of 255** (extras are logged and dropped; the agent never crashes). Grok Build is sorted first, then the agent shell, then other terminals.

`gbr-agent sessions` lists the same windows the phone should see (including Grok).

## Name a session from Grok Build

In the Grok Build TUI (this is the native slash command). **Submit `/rename …` as its own TUI line** — not inside a pasted prompt. Natural language alone does not pin `summary.json`. Inbox watcher uses two submits: (1) `/rename TITLE` (2) the job body.

```
/rename Phone Grok
```

Alias: `/title`. That pins the session title in Grok’s `summary.json`. This agent reads that file and sends it as the phone title.

```
/rename --auto
```

clears the pin and lets Grok auto-title again.

`/rename` is a **Grok** command, not an agent command. Type it in the Grok prompt. The agent only *reads* the result.

## Name a session from the PC agent

List ids first:

```powershell
gbr-agent sessions
```

Look for `grok-build-…` (this chat) vs `conhost-…` / `gbr-agent`. Then:

```powershell
gbr-agent rename -session grok-build-40a22 -name "Phone Grok"
```

Use the id from `sessions`, not a guessed hwnd. The label is stored under `labels` in `%USERPROFILE%\.gbr\sessions.json` and survives agent restarts.

Rename the **PC device** (not a session):

```powershell
gbr-agent rename -name DESKTOP-KALMAR
```

## Why Android showed six identical names

Agent 0.5.0 sent periodic feedback for only the **first six** sessions, in map order. Extra PowerShell windows filled those slots; Grok was dropped. 0.5.1:

- removes the six-session cap (soft max 255)
- sorts Grok first
- uses `/rename` / labels so the remaining names are readable
- pushes a live `list.replace` snapshot + `heartbeat.sessions[]` when windows or titles change

iOS uses the same agent, so it had the same six-session limit.

## After renaming

1. Leave `gbr-agent run` running (or restart it after install).
2. Force-close **Build Remote Agent** on the phone and open it again.
3. Step through **◀ n/m ▶** — Grok should be first and show the name you set.

## Live updates (agent + relay)

The roster is **dynamic**. The agent pushes a full snapshot when a window appears or disappears, when a title changes (`/rename` or `gbr-agent rename -session … -name …`), and every ~15s.

- Heartbeat `payload.sessions[]` = `{session_id, title}`
- List envelopes use `replace: true`
- Relay 0.5.1+ keeps only the **latest** register per session, latest list, and latest heartbeat

The **app** must apply that snapshot (not freeze the first six names). See [APP-TODO.md](APP-TODO.md).

## App TODO

Android currently has **no Unpair button**. Tracked in [APP-TODO.md](APP-TODO.md) and [issue #2](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/2). Workaround: system Settings → Apps → Build Remote Agent → Clear data.
