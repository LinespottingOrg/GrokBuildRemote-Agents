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
	// Prefer Task Scheduler logon task so agent runs in the user desktop session.
	// /RL LIMITED = standard user; /IT = only when user logged on interactively.
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Grok Build Remote agent — polls relay and injects into terminals</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>-log=info run</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, p.UnitPath, p.Binary, filepath.Dir(p.Binary))

	tmp := filepath.Join(os.TempDir(), "gbr-agent-task.xml")
	// Task Scheduler XML expects UTF-16 LE with BOM when using /XML
	utf16 := utf16LEBOM(xml)
	if err := os.WriteFile(tmp, utf16, 0o600); err != nil {
		return err
	}
	// Remove existing then create
	_ = exec.Command("schtasks", "/Delete", "/TN", p.UnitPath, "/F").Run()
	out, err := exec.Command("schtasks", "/Create", "/TN", p.UnitPath, "/XML", tmp, "/F").CombinedOutput()
	if err != nil {
		// Fallback: simple ONLOGON create
		out2, err2 := exec.Command("schtasks", "/Create", "/TN", p.UnitPath,
			"/TR", fmt.Sprintf("\"%s\" -log=info run", p.Binary),
			"/SC", "ONLOGON", "/RL", "LIMITED", "/F").CombinedOutput()
		if err2 != nil {
			// Last resort: Startup folder shortcut (no admin)
			if err3 := installStartupFolder(p.Binary); err3 != nil {
				return fmt.Errorf("schtasks create: %w\n%s\nfallback: %s\nstartup: %v", err, string(out), string(out2), err3)
			}
			fmt.Println("note: Task Scheduler access denied — installed Startup folder launcher instead")
			return nil
		}
	}
	// Start now
	_ = exec.Command("schtasks", "/Run", "/TN", p.UnitPath).Run()
	return nil
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
	// .cmd launcher (no PowerShell execution policy issues)
	cmdPath := filepath.Join(startup, "GrokBuildRemoteAgent.cmd")
	body := fmt.Sprintf("@echo off\r\nstart \"\" \"%s\" -log=info run\r\n", binary)
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
	// Startup folder cleanup
	appData := os.Getenv("APPDATA")
	if appData != "" {
		_ = os.Remove(filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "GrokBuildRemoteAgent.cmd"))
	}
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, err := exec.Command("schtasks", "/Query", "/TN", p.UnitPath, "/FO", "LIST", "/V").CombinedOutput()
	startup := "false"
	appData := os.Getenv("APPDATA")
	if appData != "" {
		if _, e := os.Stat(filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "GrokBuildRemoteAgent.cmd")); e == nil {
			startup = "true"
		}
	}
	if err != nil {
		return fmt.Sprintf("task=%s installed=false startup_folder=%s\nbinary=%s\nnote=%s\n", p.UnitPath, startup, p.Binary, p.ExtraNotes), nil
	}
	return fmt.Sprintf("task=%s installed=true startup_folder=%s\nbinary=%s\n%s\nnote=%s\n", p.UnitPath, startup, p.Binary, string(out), p.ExtraNotes), nil
}

func utf16LEBOM(s string) []byte {
	// Minimal UTF-16 LE encoder for ASCII-heavy XML
	u := make([]byte, 2+len(s)*2)
	u[0], u[1] = 0xFF, 0xFE
	for i := 0; i < len(s); i++ {
		u[2+i*2] = s[i]
		u[2+i*2+1] = 0
	}
	return u
}
