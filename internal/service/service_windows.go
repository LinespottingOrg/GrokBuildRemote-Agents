//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func installPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return err
	}
	// Prefer S4U+Highest Hidden AtLogon (no InteractiveToken, no desktop popup).
	// Task list name is "Grok Build Remote" (issue #55). Spaces are valid in /TN.
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	xml := windowsTaskXML(p.UnitPath, p.Binary, serviceWorkDir(p.Binary), user)

	tmp := filepath.Join(os.TempDir(), "gbr-agent-task.xml")
	// Task Scheduler XML expects UTF-16 LE with BOM when using /XML
	encoded := utf16LEBOM(xml)
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	// Remove existing then create (current human name only — not the legacy id).
	_ = exec.Command("schtasks", "/Delete", "/TN", p.UnitPath, "/F").Run()
	out, err := exec.Command("schtasks", "/Create", "/TN", p.UnitPath, "/XML", tmp, "/F").CombinedOutput()
	if err != nil {
		// Fallback: simple ONLOGON create
		out2, err2 := exec.Command("schtasks", "/Create", "/TN", p.UnitPath,
			"/TR", fmt.Sprintf("\"%s\" -log=info run -inject-halt -no-auto-open -no-inbox-watch", p.Binary),
			"/SC", "ONLOGON", "/RL", "HIGHEST", "/F").CombinedOutput()
		if err2 != nil {
			// Last resort: Startup folder shortcut (no admin)
			if err3 := installStartupFolder(p.Binary); err3 != nil {
				return fmt.Errorf("schtasks create: %w\n%s\nfallback: %s\nstartup: %v", err, string(out), string(out2), err3)
			}
			fmt.Println("note: Task Scheduler access denied — installed Startup folder launcher instead")
			disableLegacyWindowsTasks()
			return nil
		}
	}
	// Task registration succeeded — drop any Startup-folder launcher left behind
	// by an earlier unelevated attempt. Otherwise BOTH fire at logon: the task
	// wins, the launcher's agent hits the singleton lock and exits with an error
	// every single login.
	removeStartupFolder()

	// Migrate pre-#55 ids: disable, do not delete (David-yes required to remove).
	disableLegacyWindowsTasks()

	// Start now
	_ = exec.Command("schtasks", "/Run", "/TN", p.UnitPath).Run()
	return nil
}

// disableLegacyWindowsTasks stops auto-start of old Task Scheduler ids.
// The tasks stay registered until David-yes to delete.
func disableLegacyWindowsTasks() {
	for _, name := range windowsLegacyTaskNames() {
		if name == "" || name == WindowsTaskName {
			continue
		}
		_ = exec.Command("schtasks", "/End", "/TN", name).Run()
		_ = exec.Command("schtasks", "/Change", "/TN", name, "/DISABLE").Run()
	}
}

// serviceWorkDir is the agent's working directory when launched by the service.
//
// It must NOT be the binary's own folder: the agent tracks its cwd as a session,
// so installing from dist/ produced a session literally named "dist". The user
// home is stable and neutral; real sessions come from discovered terminals, a
// .grok-session file, or `gbr-agent rename -session`.
func serviceWorkDir(binary string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return filepath.Dir(binary)
}

func startupFolderPath(name string) string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return ""
	}
	return filepath.Join(appData, "Microsoft", "Windows",
		"Start Menu", "Programs", "Startup", name)
}

func removeStartupFolder() {
	if p := startupFolderPath(WindowsStartupCmd); p != "" {
		_ = os.Remove(p)
	}
	if p := startupFolderPath(WindowsLegacyStartupCmd); p != "" {
		_ = os.Remove(p)
	}
}

func installStartupFolder(binary string) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return fmt.Errorf("APPDATA not set")
	}
	startup := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	if err := os.MkdirAll(startup, 0o755); err != nil {
		return err
	}
	// Drop the pre-#55 launcher so Startup shows "Grok Build Remote".
	_ = os.Remove(filepath.Join(startup, WindowsLegacyStartupCmd))
	// Hidden launcher — do not use `start` on a console binary (Win11 Terminal flash).
	cmdPath := filepath.Join(startup, WindowsStartupCmd)
	body := fmt.Sprintf("@echo off\r\npowershell.exe -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -Command \"Start-Process -FilePath '%s' -ArgumentList '-log=info','run','-inject-halt','-no-auto-open','-no-inbox-watch' -WindowStyle Hidden\"\r\n", binary)
	return os.WriteFile(cmdPath, []byte(body), 0o644)
}

func uninstallPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	out, err := exec.Command("schtasks", "/Delete", "/TN", p.UnitPath, "/F").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "cannot find") && !strings.Contains(strings.ToLower(string(out)), "not found") {
		// ignore if missing
		_ = out
	}
	// Legacy ids: disable again, do not delete without David-yes.
	disableLegacyWindowsTasks()
	removeStartupFolder()
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, err := exec.Command("schtasks", "/Query", "/TN", p.UnitPath, "/FO", "LIST", "/V").CombinedOutput()
	startup := "false"
	if sp := startupFolderPath(WindowsStartupCmd); sp != "" {
		if _, e := os.Stat(sp); e == nil {
			startup = "true"
		}
	}
	legacy := windowsLegacyStatus()
	if err != nil {
		return fmt.Sprintf("task=%s installed=false startup_folder=%s\nbinary=%s\n%snote=%s\n", p.UnitPath, startup, p.Binary, legacy, p.ExtraNotes), nil
	}
	return fmt.Sprintf("task=%s installed=true startup_folder=%s\nbinary=%s\n%s%s\nnote=%s\n", p.UnitPath, startup, p.Binary, legacy, string(out), p.ExtraNotes), nil
}

func windowsLegacyStatus() string {
	var b strings.Builder
	for _, name := range windowsLegacyTaskNames() {
		out, err := exec.Command("schtasks", "/Query", "/TN", name, "/FO", "LIST").CombinedOutput()
		if err != nil {
			continue
		}
		state := "registered"
		low := strings.ToLower(string(out))
		if strings.Contains(low, "disabled") {
			state = "disabled"
		}
		fmt.Fprintf(&b, "legacy_task=%s %s (not deleted; needs David-yes to remove)\n", name, state)
	}
	return b.String()
}
