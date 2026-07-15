// Package doctor checks platform readiness for gbr-agent.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
	"context"
	"time"
)

// Result is one check line.
type Result struct {
	Name    string
	OK      bool
	Detail  string
	FixHint string
}

// Run executes platform + relay checks.
func Run() []Result {
	var out []Result
	out = append(out, checkBinary())
	out = append(out, checkDevice())
	out = append(out, checkRelay())
	out = append(out, checkInject())
	out = append(out, checkPlatformExtras()...)
	return out
}

func checkBinary() Result {
	exe, err := os.Executable()
	if err != nil {
		return Result{"binary", false, err.Error(), ""}
	}
	return Result{"binary", true, fmt.Sprintf("%s (%s/%s)", exe, runtime.GOOS, runtime.GOARCH), ""}
}

func checkDevice() Result {
	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		return Result{"device", false, err.Error(), "ensure %%USERPROFILE%%\\.gbr is writable"}
	}
	mb := dev.MailboxConversationID
	if mb == "" {
		return Result{"device", true, fmt.Sprintf("id=%s name=%q mailbox=<not paired>", dev.DeviceID, dev.DeviceName),
			"gbr-agent pair -code YOURCODE"}
	}
	return Result{"device", true, fmt.Sprintf("id=%s name=%q mailbox=%s", dev.DeviceID, dev.DeviceName, mb), ""}
}

func checkRelay() Result {
	c := relay.New(os.Getenv("GBR_RELAY_URL"), 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		return Result{"relay", false, err.Error(), "check network; GBR_RELAY_URL=" + c.Base()}
	}
	return Result{"relay", true, c.Base(), ""}
}

func checkInject() Result {
	inj := inject.NewHybrid(inject.Default(), inject.NewManager(nil))
	defer inj.Close()
	// managed shell always available
	if err := inj.Inject("doctor-probe", inject.InjectRequest{
		SessionID: "doctor-probe",
		Text:      "echo gbr-doctor-ok",
		Submit:    true,
	}); err != nil {
		return Result{"inject.managed", false, err.Error(), "shell (pwsh/bash) must be on PATH"}
	}
	cap, _ := inj.Capture("doctor-probe")
	ok := strings.Contains(cap.Text, "gbr-doctor-ok")
	detail := "managed shell"
	if ok {
		detail += " + capture ok"
	} else {
		detail += " (capture partial)"
	}
	// UI discover best-effort
	wins, err := inj.Discover()
	if err != nil {
		return Result{"inject", true, detail + fmt.Sprintf("; ui discover err: %v", err), ""}
	}
	return Result{"inject", true, fmt.Sprintf("%s; ui_windows=%d", detail, len(wins)), ""}
}

func checkPlatformExtras() []Result {
	switch runtime.GOOS {
	case "windows":
		return []Result{
			{Name: "platform", OK: true, Detail: "windows — SendInput + managed pwsh", FixHint: "run in interactive user session for UI inject"},
		}
	case "darwin":
		_, err := exec.LookPath("osascript")
		r := Result{Name: "osascript", OK: err == nil, Detail: "required for Terminal/iTerm inject"}
		if err != nil {
			r.Detail = err.Error()
			r.FixHint = "osascript is part of macOS"
		}
		r2 := Result{Name: "platform", OK: true, Detail: "darwin — AppleScript + managed bash", FixHint: "System Settings → Privacy → Accessibility + Automation"}
		return []Result{r, r2}
	case "linux":
		_, err := exec.LookPath("xdotool")
		r := Result{Name: "xdotool", OK: err == nil, Detail: "optional for X11 UI inject"}
		if err != nil {
			r.Detail = "not found (managed shell still works)"
			r.FixHint = "sudo apt install xdotool  # or dnf/pacman"
			r.OK = true // not fatal
		}
		disp := os.Getenv("DISPLAY")
		r2 := Result{Name: "DISPLAY", OK: disp != "" || os.Getenv("WAYLAND_DISPLAY") != "", Detail: "DISPLAY=" + disp + " WAYLAND=" + os.Getenv("WAYLAND_DISPLAY")}
		if !r2.OK {
			r2.Detail = "no DISPLAY/WAYLAND — UI inject unavailable; managed shell OK"
			r2.OK = true
		}
		r3 := Result{Name: "platform", OK: true, Detail: "linux — xdotool(X11) + managed bash", FixHint: ""}
		return []Result{r, r2, r3}
	default:
		return []Result{{Name: "platform", OK: false, Detail: runtime.GOOS, FixHint: "unsupported"}}
	}
}

// Print writes results to a builder string.
func Format(results []Result) string {
	var b strings.Builder
	b.WriteString("gbr-agent doctor\n")
	allOK := true
	for _, r := range results {
		mark := "PASS"
		if !r.OK {
			mark = "FAIL"
			allOK = false
		}
		b.WriteString(fmt.Sprintf("  %s  %-16s %s\n", mark, r.Name, r.Detail))
		if r.FixHint != "" && !r.OK {
			b.WriteString(fmt.Sprintf("         fix: %s\n", r.FixHint))
		} else if r.FixHint != "" {
			b.WriteString(fmt.Sprintf("         note: %s\n", r.FixHint))
		}
	}
	if allOK {
		b.WriteString("overall: OK\n")
	} else {
		b.WriteString("overall: ISSUES\n")
	}
	return b.String()
}
