package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/trace"
)

// cmdSupportLog writes a support bundle to the user's Downloads folder
// (and prints the path). Safe to attach in email / Discord / support.
func cmdSupportLog(args []string) int {
	fs := flag.NewFlagSet("support-log", flag.ExitOnError)
	open := fs.Bool("open", false, "open Downloads folder after write")
	_ = fs.Parse(args)

	path, body, err := writeSupportBundle()
	if err != nil {
		fmt.Fprintf(os.Stderr, "support-log: %v\n", err)
		return 1
	}
	fmt.Printf("support log written:\n  %s\n", path)
	fmt.Printf("size: %d bytes\n", len(body))
	fmt.Println("→ copy/paste that file (or its contents) to support: info@linespotting.com")
	if *open {
		_ = openPath(filepath.Dir(path))
	}
	return 0
}

func writeSupportBundle() (path string, body []byte, err error) {
	var b strings.Builder
	now := time.Now().UTC()
	fmt.Fprintf(&b, "=== GBR agent support log ===\n")
	fmt.Fprintf(&b, "ts: %s\n", now.Format(time.RFC3339))
	fmt.Fprintf(&b, "agent: %s commit=%s %s/%s\n", version, commit, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "go: %s\n\n", runtime.Version())

	dev, derr := core.LoadOrCreateDevice()
	if derr != nil {
		fmt.Fprintf(&b, "device: ERROR %v\n", derr)
	} else {
		fmt.Fprintf(&b, "device_id: %s\n", dev.DeviceID)
		fmt.Fprintf(&b, "device_name: %s\n", dev.DeviceName)
		fmt.Fprintf(&b, "mailbox: %s\n", dev.MailboxConversationID)
		fmt.Fprintf(&b, "mailbox_key: %s\n", keyStatus(dev.MailboxKey))
		fmt.Fprintf(&b, "device_file: %s\n\n", dev.Path())
	}

	rc := relay.New(os.Getenv("GBR_RELAY_URL"), 12*time.Second)
	fmt.Fprintf(&b, "relay: %s\n", rc.Base())
	// netcheck summary
	// (inline short health)
	if err := rc.Health(timeoutCtx(10 * time.Second)); err != nil {
		fmt.Fprintf(&b, "relay_health: FAIL %v\n\n", err)
	} else {
		fmt.Fprintf(&b, "relay_health: OK\n\n")
	}

	// Discover live windows
	fmt.Fprintf(&b, "=== discover (terminals / Grok Build) ===\n")
	ui := inject.Default()
	wins, werr := ui.Discover()
	if werr != nil {
		fmt.Fprintf(&b, "discover_error: %v\n", werr)
	} else {
		fmt.Fprintf(&b, "count: %d\n", len(wins))
		grok := 0
		for i, w := range wins {
			if i >= 40 {
				fmt.Fprintf(&b, "... truncated (%d more)\n", len(wins)-40)
				break
			}
			kind := string(w.Kind)
			if strings.Contains(kind, "grok") {
				grok++
			}
			fmt.Fprintf(&b, "  [%d] kind=%s pid=%d hwnd=0x%x exe=%q title=%q\n",
				i, kind, w.PID, w.HWND, w.ExeName, truncateStr(w.Title, 80))
		}
		fmt.Fprintf(&b, "grok_build_matches: %d\n", grok)
		if grok == 0 {
			fmt.Fprintf(&b, "HINT: Start Grok Build so its window title contains \"Grok Build\".\n")
			fmt.Fprintf(&b, "      Keep the window open (not minimized to tray-only if possible).\n")
			fmt.Fprintf(&b, "      Then: gbr-agent sessions   and  gbr-agent run\n")
		}
	}
	_ = ui.Close()
	fmt.Fprintf(&b, "\n")

	// Session store
	fmt.Fprintf(&b, "=== session store ===\n")
	if st, err := session.OpenStore(""); err != nil {
		fmt.Fprintf(&b, "store: %v\n", err)
	} else {
		fmt.Fprintf(&b, "path: ok renames=%d\n", len(st.Snapshot()))
		for k, v := range st.Snapshot() {
			fmt.Fprintf(&b, "  rename %q → %s\n", k, v)
		}
	}
	fmt.Fprintf(&b, "\n")

	// Recent local trace
	fmt.Fprintf(&b, "=== recent agent trace (last 80 lines) ===\n")
	tpath := trace.New(trace.Config{Actor: "agent"}).Path()
	fmt.Fprintf(&b, "log_path: %s\n", tpath)
	if data, err := os.ReadFile(tpath); err != nil {
		fmt.Fprintf(&b, "(no trace yet — run gbr-agent run first)\n")
	} else {
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 80 {
			start = len(lines) - 80
		}
		for _, ln := range lines[start:] {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			// compact one-line
			var ev map[string]any
			if json.Unmarshal([]byte(ln), &ev) == nil {
				fmt.Fprintf(&b, "  %v  %v  sess=%v  ok=%v  %v\n",
					ev["ts"], ev["hop"], ev["session_id"], ev["ok"], ev["detail"])
			} else {
				fmt.Fprintf(&b, "  %s\n", truncateStr(ln, 200))
			}
		}
	}
	fmt.Fprintf(&b, "\n=== end support log ===\n")

	body = []byte(b.String())
	dir := downloadsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("gbr-agent-support-%s.txt", now.Format("20060102-150405"))
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", nil, err
	}
	// also copy latest symlink-style name
	_ = os.WriteFile(filepath.Join(dir, "gbr-agent-support-latest.txt"), body, 0o644)
	return path, body, nil
}

func keyStatus(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return "NOT SET"
	}
	return fmt.Sprintf("set (%d chars)", len(k))
}

func downloadsDir() string {
	if d := strings.TrimSpace(os.Getenv("GBR_SUPPORT_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	// Windows: ~/Downloads, macOS/Linux: ~/Downloads
	dl := filepath.Join(home, "Downloads")
	if st, err := os.Stat(dl); err == nil && st.IsDir() {
		return dl
	}
	// fallback
	return filepath.Join(home, ".gbr", "support")
}

func openPath(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", dir).Start()
	case "windows":
		return exec.Command("explorer", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func timeoutCtx(d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	// Health is short; cancel after deadline + grace without leaking forever
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}
