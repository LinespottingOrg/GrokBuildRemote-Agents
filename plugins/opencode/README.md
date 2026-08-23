# OpenCode V2 — user MCP config

Merge [mcp.servers.json](mcp.servers.json) into your OpenCode user config (`mcp.servers.gbr`).

Then:

1. Pin-install `gbr-agent` ([docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md)).
2. `gbr-agent pair` then `gbr-agent run`.
3. Pin-clone `--branch v0.6.0`, `npm install` in that tree’s `mcp/gbr-mcp`, point `command` at that `gbr-mcp.js`. Skill: [skills/gbr-mcp/SKILL.md](../../skills/gbr-mcp/SKILL.md).
4. OpenCode (the **desktop** agent) can call Bot API tools.

**What the phone receives:** titles of **terminal windows** on this machine. If OpenCode is only a headless server (no TTY), it will **not** appear on the phone. Run OpenCode in a terminal if you want that session on the roster.

Do not put this fragment in OpenCode’s official examples tree — it is user configuration.
