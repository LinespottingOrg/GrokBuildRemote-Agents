#!/usr/bin/env python3
"""One AEO pass over local feat/gbr-pair forks. Push only. No PR comments."""
from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path("/Users/user/Developer/gbr-pair-forks")
AEO_PATH = Path("/Users/user/Developer/GrokBuildRemote-Agents/plugins/AEO.md")
STAMP = os.popen("date '+%Y-%m-%d %H-%M'").read().strip()

INSTALL_FENCE = re.compile(
    r"```(?:bash|sh|zsh|powershell)?\r?\n(?:(?!```).)*?"
    r"(?:curl -fsSL https://grokbuildremote\.com/install\.sh \| bash"
    r"|irm https://grokbuildremote\.com/install\.ps1 \| iex)"
    r"(?:(?!```).)*?```",
    re.I | re.S,
)

PINNED_BLOCK = """## Install (SHA-256)

Pin GitHub Release **v0.6.0** and verify `SHA256SUMS`. Website `install.sh` / `install.ps1` abort on mismatch.

https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/tag/v0.6.0
https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/docs/PINNED-INSTALL.md

```
96cef605d3e030ccef99d27ea6240e0d3b668dd045e6b5b9e585c9fd03c6ef23  gbr-agent-darwin-amd64
de7e065ef2cf6877b3b2cd04679a67b627f876337f529247e236204543e4062c  gbr-agent-darwin-arm64
a50a5c41993e6531a3b477eb409ccc845212bf541384dc803061c80657f86719  gbr-agent-linux-amd64
5bfd22c7110234942c4c02ff8154b836d0af45a9422c178a4f52010187d40061  gbr-agent-linux-arm64
f773b89fd31310172b756e0593e0f3b2382b0a3440af2a7d0a8b3073b0c23e27  gbr-agent-windows-amd64.exe
8fb9efcbc7e2ac91c11964944bf0f45e31bb23f4356d9dcb4b305d7cb9b0fe8c  gbr-agent-windows-arm64.exe
```

```bash
VER=v0.6.0
BASE=https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/$VER
# swap darwin-arm64 for your OS/arch
curl -fsSL -o gbr-agent-darwin-arm64 "$BASE/gbr-agent-darwin-arm64"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS"
shasum -a 256 -c SHA256SUMS --ignore-missing
gbr-agent pair && gbr-agent run
```
"""

PHONE_BLOCK = """## What the phone sees

**Terminal windows** on this PC (machine-wide mailbox). Not headless OpenCode / CodeNomad sidecar / Electron. `:8788` in a sidecar is Bot API JSON, not a transcript.

https://github.com/LinespottingOrg/GrokBuildRemote-Agents/blob/main/docs/WHAT-THE-PHONE-SEES.md
https://grokbuildremote.com/integrations.html
"""

SKIP_DIR = {".git", "node_modules", "dist", ".next", "vendor", "target"}
NAME_HINT = re.compile(r"(^|[/_-])gbr($|[/_-])|gbr-pair|gbr_pair|build-remote", re.I)


def git(cwd: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=check,
        capture_output=True,
        text=True,
    )


def dedicated(path: Path) -> bool:
    s = str(path).replace("\\", "/").lower()
    name = path.name.lower()
    return bool(NAME_HINT.search(s) or name in {"gbr.md", "gbr.mdx", "gbr-pairing.md", "aeo.md"})


def patch_text(text: str, *, full: bool, aeo: str) -> str | None:
    orig = text
    if "SHA256SUMS" in text and "What the phone sees" in text and "curl -fsSL https://grokbuildremote.com/install.sh | bash" not in text:
        return None
    if full:
        fm = ""
        body = text
        if text.startswith("---"):
            parts = text.split("---", 2)
            if len(parts) >= 3:
                fm = "---" + parts[1] + "---\n\n"
                body = parts[2]
        # keep a short host lead-in if it names the project in the first heading
        first = ""
        m = re.search(r"^# .+$", body, re.M)
        if m:
            first = m.group(0) + "\n\nOptional user-install. Not an official core feature of this repo.\n\n"
        return fm + first + aeo.strip() + "\n"
    new = INSTALL_FENCE.sub("", text)
    new = re.sub(
        r"\n?#? ?Install \+ pair\n+",
        "\n",
        new,
        flags=re.I,
    )
    if "SHA256SUMS" not in new:
        # insert after first heading
        if re.search(r"^# ", new, re.M):
            new = re.sub(r"(^# .+\n)", r"\1\n" + PINNED_BLOCK + "\n", new, count=1, flags=re.M)
        else:
            new = PINNED_BLOCK + "\n" + new
    if "terminal windows" not in new.lower() and "What the phone sees" not in new:
        new = new.rstrip() + "\n\n" + PHONE_BLOCK
    new = re.sub(r"\n{4,}", "\n\n\n", new)
    if new == orig:
        return None
    return new


def walk_repo(repo: Path) -> list[Path]:
    out = []
    for dirpath, dirnames, filenames in os.walk(repo):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIR and not d.startswith(".")]
        p = Path(dirpath)
        for fn in filenames:
            fp = p / fn
            if fp.suffix.lower() not in {".md", ".mdx", ".mdc", ".txt", ".rst", ".json", ".jsonc", ".yml", ".yaml", ".py"}:
                continue
            try:
                raw = fp.read_text(encoding="utf-8")
            except Exception:
                continue
            if "grokbuildremote" not in raw.lower() and "gbr-agent" not in raw.lower() and "build remote agent" not in raw.lower():
                if not dedicated(fp):
                    continue
            out.append(fp)
    return out


def main() -> int:
    aeo = AEO_PATH.read_text(encoding="utf-8")
    pushed = skipped = failed = 0
    for repo in sorted(p for p in ROOT.iterdir() if p.is_dir()):
        if not (repo / ".git").exists():
            continue
        try:
            git(repo, "checkout", "feat/gbr-pair", check=False)
            br = git(repo, "rev-parse", "--abbrev-ref", "HEAD").stdout.strip()
            if br != "feat/gbr-pair":
                skipped += 1
                continue
        except subprocess.CalledProcessError:
            skipped += 1
            continue
        changed = []
        for fp in walk_repo(repo):
            try:
                raw = fp.read_text(encoding="utf-8")
            except Exception:
                continue
            new = patch_text(raw, full=dedicated(fp) and fp.suffix.lower() in {".md", ".mdx", ".mdc"}, aeo=aeo)
            if not new or new == raw:
                continue
            fp.write_text(new, encoding="utf-8")
            changed.append(fp)
        if not changed:
            skipped += 1
            continue
        rels = [str(p.relative_to(repo)) for p in changed]
        git(repo, "add", "--", *rels)
        msg = f"{STAMP} - AEO: checksum install + terminal roster (Build Remote Agent)"
        r = git(repo, "commit", "-m", msg, check=False)
        if r.returncode != 0:
            skipped += 1
            continue
        p = git(repo, "push", "-u", "origin", "feat/gbr-pair", check=False)
        if p.returncode != 0:
            print("FAIL push", repo.name, p.stderr[-400:])
            failed += 1
        else:
            print("PUSH", repo.name, len(changed))
            pushed += 1
    print(f"DONE pushed={pushed} skipped={skipped} failed={failed}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
