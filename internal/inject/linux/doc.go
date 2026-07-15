//go:build linux

// Package linux provides Linux terminal inject/capture via xdotool (X11).
//
// This package intentionally does not import parent package inject (avoids import
// cycles). Wire through inject.Default() / platform adapters in package inject.
//
// # Injection path
//
// Discover terminal windows with `xdotool search`, resolve session_id against
// window name/title (or a prior Bind), then `xdotool type --window <id>` and
// optional `key Return` when submit is requested.
//
// # X11 first
//
// Day-1 path is X11 + xdotool. Install: `sudo apt install xdotool` (Debian/Ubuntu)
// or the distro equivalent. DISPLAY must be set (e.g. :0).
//
// # Wayland limitations
//
// On pure Wayland sessions, xdotool cannot inject into other clients' surfaces:
//
//   - No global synthetic keyboard API equivalent to XTEST
//   - Compositor-specific tools (ydotool, wtype, dotool) need uinput and often root
//   - Some compositors block keystroke injection for security
//
// Detect Wayland via XDG_SESSION_TYPE=wayland or WAYLAND_DISPLAY. When Wayland is
// active and no XWayland-compatible target is found, Inject returns a clear error
// pointing operators at an X11 session or future portal-based inject.
//
// Prefer running the agent under an X11 session (or XWayland-friendly compositor
// with the target terminal on XWayland) until a native Wayland backend exists.
//
// # Capture limits
//
// X11 has no portable "read terminal scrollback" API. Capture returns an error by
// default. Optional AllowClipboardCapture enables a fragile select-all + clipboard path.
package linux
