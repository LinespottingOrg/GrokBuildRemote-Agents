# Mobile app TODO (Build Remote Agent)

The desktop agent is this repo. The **phone/tablet app** is a separate product.

| Status | Item | Where |
|--------|------|--------|
| **Shipped** in app **1.3.0** (Play vc22; iOS 1.3.0 b20 in review) | **Unpair / Forget this PC** | [#2](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/2) |
| **Shipped** in app **1.3.0** | **Apply live session roster** (`list.replace` / `heartbeat.sessions`, no 6-title freeze, soft max 255) | [#3](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/3) |

Older store binaries still need system **Clear data**. Current builds: Settings → Unpair.

## Unpair (what shipped)

**Settings → Unpair / Forget this PC** (confirm):

- Clear mailbox id, mailbox key, and cached session list
- Keep the Relay URL unless the user clears that too
- Return to Scan QR

Also on **Waiting for sessions**: “Unpair and scan again”.
