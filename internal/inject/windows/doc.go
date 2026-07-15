//go:build windows

// Package windows implements terminal discovery, keyboard inject, and
// best-effort console capture for the Grok Build Remote Windows agent.
//
// # Overview
//
// Day-1 strategy (reliability first):
//
//  1. Discover visible Windows Terminal / conhost / PowerShell / cmd windows
//     via EnumWindows + process image name heuristics.
//  2. Bind a protocol session_id to a HWND/PID.
//  3. Focus the target and inject UTF-16 text with SendInput (KEYEVENTF_UNICODE).
//  4. If discovery/focus fails, callers MUST fall back to inject.Manager
//     (managed pwsh/cmd pipes in parent package) — that path is the
//     capture-reliable default.
//
// # Capture limitations (important)
//
// Reading another process's console buffer is inherently fragile on modern
// Windows:
//
//   - AttachConsole(pid) detaches the caller's console and only one attach is
//     allowed at a time.
//   - Windows Terminal hosts panes via ConPTY; classic ReadConsoleOutput does
//     not reliably see WT tab content.
//   - Elevated vs non-elevated mismatch blocks AttachConsole.
//
// Capture() therefore returns best-effort data and sets Partial/Note when the
// method is incomplete. Prefer inject.Manager (pty_session.go) for output
// streaming to mobile.
//
// # Safety
//
//   - Empty session_id is refused.
//   - Empty text without Submit is refused.
//   - Per-session rate limiting (default 150ms min interval, 8/sec burst).
//
// # Build
//
// All files use //go:build windows. This package must not import the parent
// inject package (import cycle with inject.Default). The parent adapts types.
//
//	go test ./internal/inject/windows/
package windows
