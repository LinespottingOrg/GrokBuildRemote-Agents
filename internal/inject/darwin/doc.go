//go:build darwin

// Package darwin provides macOS terminal inject/capture via AppleScript (osascript).
//
// This package intentionally does not import parent package inject (avoids import
// cycles). Wire through inject.Default() / platform adapters in package inject.
//
// # Injection path
//
// Prefer a bound Terminal.app / iTerm2 tab for the session. Otherwise resolve by
// tab/window title containing session_id, activate the host app, and type via
// System Events keystroke. Complex / non-ASCII payloads fall back to clipboard
// + Cmd+V. Submit sends key code 36 (Return).
//
// # TCC / Accessibility requirements (mandatory)
//
// macOS TCC (Transparency, Consent, and Control) blocks synthetic input unless
// the agent binary (or its parent terminal during `go run`) is granted:
//
//  1. System Settings → Privacy & Security → Accessibility
//     — allow the gbr-agent binary (or Terminal/iTerm for unsigned debug builds).
//  2. System Settings → Privacy & Security → Automation
//     — allow control of Terminal / iTerm / System Events when prompted.
//  3. First run may also prompt for Automation to Terminal/iTerm.
//
// Without Accessibility, osascript keystroke fails (often -1719 / not authorized).
// Without Automation, `tell application "Terminal"` fails.
//
// # Hardened Runtime / notarization
//
// Distributed binaries should declare NSAppleEventsUsageDescription in Info.plist
// and be signed/notarized. Local `go run` from Terminal inherits Terminal's grants
// only if Terminal itself is listed under Accessibility (common developer setup).
//
// # Capture limits
//
// Terminal.app exposes limited tab history via AppleScript; full scrollback is
// not guaranteed. Capture is best-effort.
//
// # Tab listing
//
// ListTabs enumerates open Terminal.app and iTerm2 tabs best-effort for discovery.
// Failures against one host app do not fail the other.
package darwin
