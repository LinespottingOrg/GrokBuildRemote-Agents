//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const unitTmpl = `[Unit]
Description=Grok Build Remote agent (gbr-agent)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{.Binary}} -log=info run
WorkingDirectory={{.WorkDir}}
Restart=on-failure
RestartSec=3
Environment=PATH=/usr/local/bin:/usr/bin:/bin
# Uncomment if DISPLAY needed for xdotool:
# Environment=DISPLAY=:0

[Install]
WantedBy=default.target
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
	f, err := os.OpenFile(p.UnitPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("unit").Parse(unitTmpl))
	if err := t.Execute(f, map[string]string{
		"Binary":  p.Binary,
		"WorkDir": filepath.Dir(p.Binary),
	}); err != nil {
		return err
	}
	// enable --user
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	out, err := exec.Command("systemctl", "--user", "enable", "--now", "gbr-agent.service").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl enable: %w\n%s\n(hint: loginctl enable-linger $USER for headless)", err, string(out))
	}
	return nil
}

func uninstallPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", "gbr-agent.service").Run()
	_ = os.Remove(p.UnitPath)
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, _ := exec.Command("systemctl", "--user", "status", "gbr-agent.service", "--no-pager").CombinedOutput()
	exists := "false"
	if _, err := os.Stat(p.UnitPath); err == nil {
		exists = "true"
	}
	return fmt.Sprintf("unit=%s installed=%s\nbinary=%s\n%s\nnote=%s\n",
		p.UnitPath, exists, p.Binary, string(out), p.ExtraNotes), nil
}
