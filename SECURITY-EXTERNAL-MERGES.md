# External / third-party merge — malicious-code review

Standing gate before anything from **outside** LinespottingOrg lands on `main` of GrokBuildRemote-* (Agents, Ops, MobileApps).

**Why:** GBR can inject into local terminals and holds mailbox keys. Hostile or supply-chain diffs are high impact. See also [SECURITY.md](SECURITY.md) and the 2026-09-05 defensive ultrareview (grok-build-inbox #124).

**Scope (treat as external):** outside-org PRs, fork syncs, vendor/ClawHub/plugin drops, Dependabot **major** bumps, copy-pasted third-party patches, unknown CI artifacts.
**Usually lighter path:** Dependabot **patch/minor** 1-line lockfile bumps (still run checklist; human APPROVE after CI).
**Out of scope here:** writing exploit PoCs; public disclosure without David.

Related process issue: [#56](https://github.com/LinespottingOrg/GrokBuildRemote-Agents/issues/56).

---

## Checklist (every external merge candidate)

Copy onto the PR (or comment) and fill.

### 1. Provenance
- [ ] Author login / org known?
- [ ] First-time contributor to this repo?
- [ ] Source: GitHub PR / fork / vendor tarball / chat paste / other?
- [ ] linked issue or ticket?

### 2. Diff scope
- [ ] Diff size reasonable for the claim?
- [ ] No unexpected new network clients, DNS, raw sockets, or crypto primitives unrelated to the PR title
- [ ] No new env / home / keychain / `.gbr` / device file reads beyond existing patterns
- [ ] No new install hooks, `postinstall`, CI piped shell downloads, or download-and-exee
- [ ] No obfuscated blobs, unexpected binaries, or large base64 dumps

### 3. Secrets
- [ ] ho mailbox keys, `X-GBR-Key`, tokens, `.env`, `device.json`, or private PEM in the diff
- [ ] No secrets in CI logs introduced by the change

### 4. Pair / Bot / inject surfaces (extra human review if any touch)
Flag and require a designated reviewer if the diff touches any of:
- [ ] Relay `/pair` or mailbox key issuance
- [ ] Bot API `:8788` auth / inject / open / lock
- [ ] Inject path, session roster, title allow/deny lists
- [ ] Fleet hub remote keys / `fleet.json`
- [ ] `GBR_BOT_REQUIRE_KEY`, `GBR_INJECT_HALT`, service wrapper display name
- [ ] Mobile pairing / Settings — Bot API key display

### 5. Lockfile / dependencies
- [ ] Lockfile drift explained and reviewed
- [ ] No unexpected `postinstall` / `prepare` scripts in new packages
- [ ] Pins match intent (prefer exact versions for hight-trust paths; see Agents #53)

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

1. List open Prs on `GrokBuildRemote-Agents`, `GrokBuildRemote-Ops`, `GrokBuildRemote-MobileApps`.
2. Classify author: org member / Dependabot / external human.
3. For external (and Dependabot majors): complete the checklist on the PR.
4. For Dependabot patch/minor gradle/npm 1-liners: confirm 1-line bump + CI; still need David/BOSS yes before merge.
5. Comment verdict on the PR; link this doc.
6. If BLOCK or sensitive surface: ping David / BOSS; do not merge.

## Display name note

User-visible service/program name is **Grok Build Remote** (not bare gbr) on Mac/Win/Linux — see Agents #55 when reviewing service-wrapper diffs (#54).

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
