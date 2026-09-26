#!/usr/bin/env python3
"""Mac Mini GBR ready ping.

The only target is this machine's loopback Bot API:

    http://127.0.0.1:8788/health

Ready only when that document says hostname botmaster, class mac_mini,
and quality ok. Any other host, including the Workstation relay, is a
failure. Non-loopback URLs are refused.
"""
from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request
from urllib.parse import urlparse

DEFAULT_URL = "http://127.0.0.1:8788/health"
WANT_HOST = "botmaster"
WANT_CLASS = "mac_mini"
WANT_QUALITY = "ok"
WANT_SERVICE = "gbr-agent-bot"


def assert_loopback(url: str) -> None:
    parsed = urlparse(url)
    host = (parsed.hostname or "").lower()
    path = parsed.path.rstrip("/") or "/"
    if parsed.scheme != "http" or host not in ("127.0.0.1", "localhost", "::1"):
        raise ValueError(
            f"refusing non-loopback URL {url!r}; "
            "Mac Mini ping is botmaster http://127.0.0.1:8788/health only"
        )
    if path != "/health":
        raise ValueError(f"refusing path {parsed.path!r}; need /health")


def judge(doc: dict) -> tuple[bool, str]:
    health = doc.get("health") if isinstance(doc.get("health"), dict) else {}
    host = str(health.get("hostname") or "")
    cls = str(health.get("class") or "")
    quality = str(health.get("quality") or "")
    service = str(doc.get("service") or "")
    reasons: list[str] = []
    if service != WANT_SERVICE:
        reasons.append(f"service={service!r}")
    if host != WANT_HOST:
        reasons.append(f"hostname={host!r}")
    if cls != WANT_CLASS:
        reasons.append(f"class={cls!r}")
    if quality != WANT_QUALITY:
        reasons.append(f"quality={quality!r}")
    devices = doc.get("devices") if isinstance(doc.get("devices"), list) else []
    matched = any(
        isinstance(d, dict)
        and d.get("hostname") == WANT_HOST
        and d.get("class") == WANT_CLASS
        for d in devices
    )
    if not matched:
        reasons.append("no device hostname=botmaster class=mac_mini")
    if reasons:
        return False, "; ".join(reasons)
    return True, f"hostname={host} class={cls} quality={quality}"


def fetch(url: str, timeout: float = 5.0) -> dict:
    assert_loopback(url)
    req = urllib.request.Request(
        url,
        headers={"Accept": "application/json", "User-Agent": "gbr-mac-mini-ready"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            code = resp.status
            raw = resp.read()
    except urllib.error.URLError as exc:
        raise RuntimeError(f"unreachable: {exc}") from exc
    if code != 200:
        raise RuntimeError(f"http {code}")
    try:
        doc = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"not json: {exc}") from exc
    if not isinstance(doc, dict):
        raise RuntimeError("health document is not an object")
    return doc


def main(argv: list[str]) -> int:
    url = argv[1] if len(argv) > 1 else DEFAULT_URL
    try:
        doc = fetch(url)
    except ValueError as exc:
        print(f"FAIL {exc}", file=sys.stderr)
        return 2
    except RuntimeError as exc:
        print(f"FAIL {exc}", file=sys.stderr)
        return 1
    ok, msg = judge(doc)
    print(("READY " if ok else "FAIL ") + msg)
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
