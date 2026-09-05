# External / third-party merge — malicious-code review

Standing gate before anything from **outside** LinespottingOrg lands on `main` of GrokBuildRemote-* (Agents, Ops, MobileApps).

**Why:** GBR can inject into local terminals and holds mailbox keys. Hostile or supply-chain diffs are high impact. See also [SECURITY.md](SECURITY.md) and the 2026-09-05 defensive ultrareview (grok-build-inbox #124).

**Classify by content and provenance, not author login alone.** An org-member copy-paste of a vendor/ClawHub patch is still external.

**Full checklist (treat as external):**
- outside-org PRs, fork syncs, vendor/ClawHub/plugin drops, copy-pasted third-party patches, unknown CI artifacts
- Dependabot **major** bumps
- any GitHub Actions pin or SHA change (digest must match the intended tagged commit — Agents #53)
- any `gomod` bump that is not a pure patch on `go.sum` (no `go` directive, `replace`, or `go.mod` rewrite)

**Lighter path (still David/BOSS yes):** only when the diff is a **literal patch-level** pin/lock bump with no `go` directive, `replace`, workflow, `postinstall`/`prepare`, or new network code. Confirm CI green. Ecosystems: Agents = `gomod` + `github-actions`; gradle/npm only on MobileApps/Ops where those files actually change.

**Out of scope here:** writing exploit PoCs; public disclosure without David.

Related process issue: [#56](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/56).

---

## Checklist (every external merge candidate)

Copy onto the PR (or comment) and fill.

### 1. Provenance
- [ ] Author login / org known?
- [ ] First-time contributor to this repo?
- [ ] Source: GitHub PR / fork / vendor tarball / chat paste / other?
- [ ] Linked issue or ticket?

### 2. Diff scope
- [ ] Diff size reasonable for the claim?
- [ ] No unexpected new network clients, DNS, raw sockets, or crypto primitives unrelated to the PR title
- [ ] No new env / home / keychain / `.gbr` / device file reads beyond existing patterns
- [ ] No new install hooks, `postinstall`, CI piped shell downloads, or download-and-exec
- [ ] No obfuscated blobs, unexpected binaries, or large base64 dumps
- [ ] No new `pull_request_target` / `workflow_run` / write permissions on fork PRs

### 3. Secrets
- [ ] No mailbox keys, `X-GBR-Key`, tokens, `.env`, `device.json`, or private PEM in the diff
- [ ] No secrets in CI logs introduced by the change

### 4. Pair / Bot / inject surfaces (extra human review if any touch)
Flag and require a designated reviewer if the diff touches any of:
- [ ] Relay `/pair` or mailbox key issuance
- [ ] Relay **push / poll / ack** (key required; push becomes keystrokes) or `GBR_AUTH_MODE` (especially `warn`)
- [ ] Bot API `:8788` auth / inject / open / lock
- [ ] Bot API **bind** (`127.0.0.1` vs `0.0.0.0`, `-bot-port` / `GBR_BOT_PORT`)
- [ ] Inject path, session roster, title allow/deny lists
- [ ] Fleet hub remote keys / `fleet.json`
- [ ] `GBR_BOT_REQUIRE_KEY`, `GBR_INJECT_HALT`, `GBR_INJECT_MAX`, `GBR_NO_AUTO_OPEN`
- [ ] MCP stdio (`gbr-mcp` / `gbr_inject` / `gbr_open` / `gbr_lock`)
- [ ] Mobile pairing / Settings — Bot API key display

### 5. Lockfile / dependencies
- [ ] Lockfile drift explained and reviewed
- [ ] No unexpected `postinstall` / `prepare` scripts in new packages
- [ ] Pins match intent (prefer exact versions for high-trust paths; Actions SHA = tagged commit, see Agents #53)

### 6. Verdict
Pick one and comment on the PR with a short rationale:

| Verdict | When |
|---------|-----|
| **APPROVE** | Checklist clean; CI green; scope matches claim |
| **REQUEST CHANGES** | Fixable issues; do not merge until addressed |
| **BLOCK** | Hostile, opaque, secret leak, or high-risk surface without clear need |

Do **not** merge Dependabot (or any external PR) without an explicit **David or BOSS yes** after this review — even if CI is green.

---

## How to run the review

1. List open PRs on `GrokBuildRemote-Agents`, `GrokBuildRemote-Ops`, `GrokBuildRemote-MobileApps`.
2. Classify by **content and provenance** (not author login alone): org-authored product code vs Dependabot vs outside human vs vendor/plugin/copy-paste.
3. Full checklist for everything in **Scope** above (outside humans, fork syncs, vendor/plugin drops, copy-paste, unknown CI artifacts, Dependabot major, any Actions pin/SHA change, any `gomod` bump that is not a pure `go.sum` patch).
4. Lighter path **only** for a literal patch-level pin/lock bump (Agents: `gomod`/`github-actions`; gradle/npm only if MobileApps/Ops). Confirm 1-line (or lock-only) + CI; still David/BOSS yes. If MobileApps has no CI, do not treat it as green.
5. Comment verdict on the PR; link this doc.
6. If BLOCK or sensitive surface: ping David / BOSS; do not merge.

## Display name note

User-visible service/program name is **Grok Build Remote** (not bare gbr) on Mac/Win/Linux — see Agents #55 when reviewing service-wrapper diffs (#54). Branding only; not a §4 security surface.

## Report template (paste on PR)

```
External merge review (SECURITY-EXTERNAL-MERGES.md)
Provenance: ...
Diff scope: ...
Secrets: clean / findings
Pair-Bot-inject surfaces: none / listed ...
Lockfile: ...
Verdict: APPROVE | REQUEST CHANGES | BLOCK
Rationale: ...
```
