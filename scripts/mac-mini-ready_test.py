#!/usr/bin/env python3
"""Checks for scripts/mac-mini-ready.py. No network except 127.0.0.1."""
from __future__ import annotations

import importlib.util
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

_path = Path(__file__).resolve().parent / "mac-mini-ready.py"
_spec = importlib.util.spec_from_file_location("mac_mini_ready", _path)
ping = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(ping)


def test_judge_botmaster_ok():
    doc = {
        "service": "gbr-agent-bot",
        "health": {"hostname": "botmaster", "class": "mac_mini", "quality": "ok"},
        "devices": [{"hostname": "botmaster", "class": "mac_mini", "id": "local"}],
    }
    ok, msg = ping.judge(doc)
    assert ok, msg
    assert "botmaster" in msg and "mac_mini" in msg


def test_judge_workstation_fails():
    doc = {
        "service": "gbr-agent-bot",
        "health": {"hostname": "Workstation", "class": "pc", "quality": "ok"},
        "devices": [{"hostname": "Workstation", "class": "pc", "id": "local"}],
    }
    ok, msg = ping.judge(doc)
    assert not ok
    assert "Workstation" in msg
    assert "pc" in msg


def test_refuses_workstation_relay():
    try:
        ping.assert_loopback("https://gbr-relay.ekobrott.workers.dev/health")
    except ValueError as exc:
        assert "non-loopback" in str(exc)
    else:
        raise AssertionError("relay URL must be refused")


def test_loopback_down_fails():
    try:
        ping.fetch("http://127.0.0.1:1/health", timeout=0.5)
    except RuntimeError as exc:
        assert "unreachable" in str(exc)
    else:
        raise AssertionError("closed port must fail")


def test_live_fixture_roundtrip():
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = json.dumps({
                "service": "gbr-agent-bot",
                "ok": True,
                "health": {"hostname": "botmaster", "class": "mac_mini", "quality": "ok"},
                "devices": [{"hostname": "botmaster", "class": "mac_mini"}],
            }).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, fmt, *args):
            return

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        port = server.server_address[1]
        doc = ping.fetch(f"http://127.0.0.1:{port}/health", timeout=2)
        ok, msg = ping.judge(doc)
        assert ok, msg
    finally:
        server.shutdown()


if __name__ == "__main__":
    test_judge_botmaster_ok()
    test_judge_workstation_fails()
    test_refuses_workstation_relay()
    test_loopback_down_fails()
    test_live_fixture_roundtrip()
    print("PASS mac-mini-ready tests")
