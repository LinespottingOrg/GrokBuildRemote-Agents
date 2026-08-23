# What the phone actually sees

`gbr-agent` discovers **terminal windows** on the paired PC (Windows Terminal, conhost, iTerm, gnome-terminal, …). After `gbr-agent pair` + `run`, the Build Remote Agent app shows that **live roster** (titles, in theory 255). Inject types into a listed terminal.

It does **not** attach a headless agent server (OpenCode serve, CodeNomad sidecar, Goose HTTP, Electron UI). Pairing the mailbox does not make those sessions appear on the phone.

| You run | On the phone |
|---------|----------------|
| Grok Build / Aider / OpenCode **in a terminal window** | That window’s title; inject goes into that TTY |
| Headless OpenCode / CodeNomad sidecar / :8788 in a browser tab | Nothing useful — Bot API JSON is not a session transcript |
| MCP `gbr-mcp` inside Claude Code / OpenCode | The **desktop agent** can call Bot API. The phone still only lists **terminals**. |
| 1Password CLI (`op`) / LastPass CLI (`lpass`) on the host | **Nothing from the vault.** Host-side CLI after pair. Do not inject secrets into a listed TTY. [PASSWORD-MANAGERS.md](PASSWORD-MANAGERS.md) |

One pair = one **mailbox for the whole machine**. The app can observe and inject **every discovered terminal** on that PC, not “this one omp/qwen tab.” Unpair before handing the PC to someone else.

Pinned binaries (preferred over `curl | bash`): [PINNED-INSTALL.md](PINNED-INSTALL.md).

CodeNomad + Grok Build CLI (worked example): [USE-CASE-CODENOMAD.md](USE-CASE-CODENOMAD.md) · https://grokbuildremote.com/use-cases/codenomad.html
