# Mobile app TODO (Build Remote Agent)

The desktop agent is this repo. The **phone/tablet app** is a separate product. Track app work here and on GitHub issues.

| Status | Item | Where |
|--------|------|--------|
| Open | **Unpair button** on Android (and iOS if missing) | [LinespottingOrg/GrokBuildRemote-Agents#2](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/2) |
| Open | **Apply live session roster** (names + add/remove) from `list.replace` / `heartbeat.sessions` — do not freeze the first 6 titles | [issue #3](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/3) |

## Unpair button

**Settings → Unpair / Forget this PC** (confirm):

- Clear mailbox id, mailbox key, and cached session list
- Keep the Relay URL unless the user clears that too
- Return to Scan QR

Also useful on the **Waiting for sessions** screen: “Unpair and scan again”.

Today there is no Unpair control. Workaround: Android system Settings → Apps → Build Remote Agent → Storage → Clear data.
