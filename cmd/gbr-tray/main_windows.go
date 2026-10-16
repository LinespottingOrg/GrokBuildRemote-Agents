//go:build windows

// gbr-tray is a GUI-subsystem (no console) system-tray app that supervises the
// gbr-agent background process on Windows. Build it with:
//
//	go build -ldflags "-H=windowsgui -X main.version=0.6.5" ./cmd/gbr-tray
//
// Tray menu: Status, Open logs, Pause/Resume polling, Restart agent, Quit.
// It installs itself as the login autostart (HKCU Run, session 1) and removes
// the legacy start-gbr-agent-interactive.cmd Startup shortcut so the agent is
// not launched twice. The agent keeps spawning real grok consoles for SendInput
// (that behaviour is in gbr-agent and is untouched here); only the agent's own
// boss-steer/gh child windows are hidden (handled in gbr-agent).
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows/registry"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/proc"
)

//go:embed icon.ico
var iconICO []byte

var version = "dev"

const (
	botBase       = "http://127.0.0.1:8788"
	runKeyPath    = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName  = "GrokBuildRemoteTray"
	legacyLnkName = "GrokBuildRemoteAgent.lnk"
)

func main() {
	setupLog()
	switch firstArg() {
	case "version":
		// No console in GUI subsystem; still useful when run from a terminal.
		fmt.Println("gbr-tray", version)
		return
	case "install-autostart":
		if err := installAutostart(); err != nil {
			log.Printf("install-autostart: %v", err)
		}
		return
	case "uninstall-autostart":
		_ = uninstallAutostart()
		return
	}
	// Normal launch: make ourselves the autostart (dedupe legacy), then run tray.
	if err := installAutostart(); err != nil {
		log.Printf("autostart install (non-fatal): %v", err)
	}
	systray.Run(onReady, onExit)
}

func firstArg() string {
	if len(os.Args) > 1 {
		return strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	return ""
}

func setupLog() {
	p := filepath.Join(agentDir(), "gbr-tray.log")
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags)
	log.Printf("gbr-tray %s starting pid=%d", version, os.Getpid())
}

// ---- paths ----

func agentDir() string {
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		return filepath.Join(la, "GrokBuildRemote")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "GrokBuildRemote")
}

func agentExe() string {
	// Prefer sitting next to the tray exe, else the known install dir.
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), "gbr-agent.exe")
		if fileExists(cand) {
			return cand
		}
	}
	return filepath.Join(agentDir(), "gbr-agent.exe")
}

func logDir() string {
	if d := strings.TrimSpace(os.Getenv("GBR_LOG_DIR")); d != "" {
		return d
	}
	if fileExists(`C:\pc-build\gbr-agent-out`) {
		return `C:\pc-build\gbr-agent-out`
	}
	return agentDir()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ---- tray ----

var pauseItem *systray.MenuItem

func onReady() {
	systray.SetIcon(iconICO)
	systray.SetTitle("Grok Build Remote")
	systray.SetTooltip("Grok Build Remote agent")

	mStatus := systray.AddMenuItem("Status", "Show agent health")
	mLogs := systray.AddMenuItem("Open logs", "Open the agent log folder")
	pauseItem = systray.AddMenuItem("Pause polling", "Pause boss-steer inbox + relay polling")
	systray.AddSeparator()
	mRestart := systray.AddMenuItem("Restart agent", "Stop and relaunch the agent (hidden)")
	mQuit := systray.AddMenuItem("Quit", "Stop the agent and exit")

	// Ensure the agent is up, then keep it alive.
	ensureAgentRunning()
	go superviseLoop()
	go refreshPauseLabel()

	go func() {
		for {
			select {
			case <-mStatus.ClickedCh:
				showStatus()
			case <-mLogs.ClickedCh:
				openLogs()
			case <-pauseItem.ClickedCh:
				togglePause()
			case <-mRestart.ClickedCh:
				restartAgent()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	log.Printf("tray exiting; stopping agent")
	stopAgent()
}

// superviseLoop relaunches the agent if it disappears (crash / manual kill),
// checking about every 15s. It never kills the running grok -p jobs.
func superviseLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for range t.C {
		if !agentHealthy() {
			log.Printf("agent not healthy; relaunching")
			launchAgent()
		}
	}
}

func refreshPauseLabel() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		setPauseLabel(currentPaused())
		<-t.C
	}
}

// ---- actions ----

func showStatus() {
	h, err := getJSON(botBase + "/health")
	if err != nil {
		messageBox("Grok Build Remote", "Agent not reachable on 127.0.0.1:8788.\n\n"+err.Error(), mbIconWarning)
		return
	}
	ver, _ := h["version"].(string)
	ok, _ := h["ok"].(bool)
	var relay, quality string
	if hh, _ := h["health"].(map[string]any); hh != nil {
		relay, _ = hh["relay_quality"].(string)
		quality, _ = hh["quality"].(string)
	}
	phone := "unknown"
	if devs, _ := h["devices"].([]any); devs != nil {
		for _, d := range devs {
			if m, _ := d.(map[string]any); m != nil {
				if id, _ := m["id"].(string); id == "phone" {
					if on, _ := m["online"].(bool); on {
						phone = "online"
					} else {
						phone = "offline"
					}
				}
			}
		}
	}
	paused := "no"
	if currentPaused() {
		paused = "yes"
	}
	msg := fmt.Sprintf("Agent: %s (ok=%v)\nVersion: %s\nRelay: %s\nQuality: %s\nPhone: %s\nPolling paused: %s",
		botBase, ok, ver, relay, quality, phone, paused)
	messageBox("Grok Build Remote — status", msg, mbIconInfo)
}

func openLogs() {
	dir := logDir()
	_ = proc.Command("explorer.exe", dir).Start()
}

func togglePause() {
	target := !currentPaused()
	ep := "/v1/control/resume"
	if target {
		ep = "/v1/control/pause"
	}
	if _, err := postJSON(botBase+ep, nil); err != nil {
		messageBox("Grok Build Remote", "Could not change polling state:\n"+err.Error(), mbIconWarning)
		return
	}
	setPauseLabel(target)
}

func setPauseLabel(paused bool) {
	if pauseItem == nil {
		return
	}
	if paused {
		pauseItem.SetTitle("Resume polling")
		pauseItem.SetTooltip("Polling is paused — click to resume")
	} else {
		pauseItem.SetTitle("Pause polling")
		pauseItem.SetTooltip("Pause boss-steer inbox + relay polling")
	}
}

func currentPaused() bool {
	m, err := getJSON(botBase + "/v1/control")
	if err != nil {
		return false
	}
	p, _ := m["paused"].(bool)
	return p
}

func restartAgent() {
	stopAgent()
	time.Sleep(1500 * time.Millisecond)
	launchAgent()
	// give it a moment, then refresh label
	go func() {
		time.Sleep(6 * time.Second)
		setPauseLabel(currentPaused())
	}()
}

// ---- agent process control ----

func agentHealthy() bool {
	_, err := getJSON(botBase + "/health")
	return err == nil
}

func ensureAgentRunning() {
	if agentHealthy() {
		return
	}
	launchAgent()
}

func launchAgent() {
	exe := agentExe()
	if !fileExists(exe) {
		log.Printf("agent exe not found: %s", exe)
		messageBox("Grok Build Remote", "gbr-agent.exe not found at:\n"+exe, mbIconWarning)
		return
	}
	cmd := proc.Command(exe, "-log=info", "run")
	cmd.Dir = agentDir()
	// Preserve the behaviour of start-gbr-agent-interactive.cmd.
	env := os.Environ()
	env = append(env,
		"GBR_INJECT_HALT=0",
		"GBR_INBOX_WATCH=1",
		"GBR_NO_AUTO_OPEN=0",
		"GBR_LOG_DIR="+logDir(),
	)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		log.Printf("launch agent: %v", err)
		return
	}
	log.Printf("launched agent pid=%d exe=%s", cmd.Process.Pid, exe)
	_ = cmd.Process.Release()
}

func stopAgent() {
	// Only gbr-agent.exe. The grok -p jobs and their powershell parents are not
	// gbr-agent and are left untouched.
	_ = proc.Command("taskkill.exe", "/IM", "gbr-agent.exe", "/F").Run()
}

// ---- autostart ----

func installAutostart() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	val := `"` + self + `"`
	if cur, _, e := k.GetStringValue(runValueName); e == nil && cur == val {
		// already correct
	} else if err := k.SetStringValue(runValueName, val); err != nil {
		return err
	}
	// Dedupe: remove the legacy Startup-folder shortcut that launched the agent
	// via start-gbr-agent-interactive.cmd, so the agent is not started twice.
	removeLegacyStartup()
	log.Printf("autostart set: %s=%s", runValueName, val)
	return nil
}

func uninstallAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.DeleteValue(runValueName)
}

func removeLegacyStartup() {
	ad := os.Getenv("APPDATA")
	if ad == "" {
		return
	}
	lnk := filepath.Join(ad, `Microsoft\Windows\Start Menu\Programs\Startup`, legacyLnkName)
	if fileExists(lnk) {
		if err := os.Remove(lnk); err != nil {
			log.Printf("remove legacy startup lnk: %v", err)
		} else {
			log.Printf("removed legacy startup shortcut %s", lnk)
		}
	}
}

// ---- http ----

var httpc = &http.Client{Timeout: 5 * time.Second}

func getJSON(url string) (map[string]any, error) {
	resp, err := httpc.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func postJSON(url string, payload any) (map[string]any, error) {
	var buf bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&buf).Encode(payload)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	return m, nil
}
