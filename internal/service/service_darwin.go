//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const plistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.linespotting.gbr-agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{.Binary}}</string>
    <string>-log=info</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>{{.WorkDir}}</string>
  <key>StandardOutPath</key>
  <string>{{.DataDir}}/agent.out.log</string>
  <key>StandardErrorPath</key>
  <string>{{.DataDir}}/agent.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>{{.Home}}/.local/bin:{{.Home}}/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
    <key>HOME</key>
    <string>{{.Home}}</string>{{if .RelayURL}}
    <key>GBR_RELAY_URL</key>
    <string>{{.RelayURL}}</string>{{end}}
  </dict>
</dict>
</plist>
`

func installPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.UnitPath), 0o755); err != nil {
		return err
	}
	// User home — not the binary folder (install from dist/ used to create a "dist" session).
	workDir, _ := os.UserHomeDir()
	if workDir == "" {
		workDir = p.DataDir
	}
	f, err := os.OpenFile(p.UnitPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("plist").Parse(plistTmpl))
	if err := t.Execute(f, map[string]string{
		"Binary":   p.Binary,
		"WorkDir":  workDir,
		"DataDir":  p.DataDir,
		"Home":     workDir,
		"RelayURL": strings.TrimSpace(os.Getenv("GBR_RELAY_URL")),
	}); err != nil {
		return err
	}
	return launchAgentEnable(p.UnitPath)
}

func launchAgentEnable(plist string) error {
	label := "com.linespotting.gbr-agent"
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	target := domain + "/" + label
	_ = exec.Command("launchctl", "bootout", target).Run()
	_ = exec.Command("launchctl", "unload", plist).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, plist).CombinedOutput(); err != nil {
		out2, err2 := exec.Command("launchctl", "load", plist).CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("launchctl bootstrap: %v\n%s\nlaunchctl load: %w\n%s", err, string(out), err2, string(out2))
		}
	}
	_ = exec.Command("launchctl", "enable", target).Run()
	if out, err := exec.Command("launchctl", "kickstart", "-k", target).CombinedOutput(); err != nil {
		_ = exec.Command("launchctl", "start", label).Run()
		_ = out
	}
	return nil
}

func uninstallPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	label := "com.linespotting.gbr-agent"
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	_ = exec.Command("launchctl", "bootout", target).Run()
	_ = exec.Command("launchctl", "stop", label).Run()
	_ = exec.Command("launchctl", "unload", p.UnitPath).Run()
	_ = os.Remove(p.UnitPath)
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, _ := exec.Command("launchctl", "list", "com.linespotting.gbr-agent").CombinedOutput()
	exists := "false"
	if _, err := os.Stat(p.UnitPath); err == nil {
		exists = "true"
	}
	return fmt.Sprintf("plist=%s installed=%s\nbinary=%s\n%s\nnote=%s\n",
		p.UnitPath, exists, p.Binary, string(out), p.ExtraNotes), nil
}
