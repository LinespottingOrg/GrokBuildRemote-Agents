//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
    <string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
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
	f, err := os.OpenFile(p.UnitPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("plist").Parse(plistTmpl))
	if err := t.Execute(f, map[string]string{
		"Binary":  p.Binary,
		"WorkDir": filepath.Dir(p.Binary),
		"DataDir": p.DataDir,
	}); err != nil {
		return err
	}
	// unload then load
	_ = exec.Command("launchctl", "unload", p.UnitPath).Run()
	out, err := exec.Command("launchctl", "load", p.UnitPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %w\n%s", err, string(out))
	}
	_ = exec.Command("launchctl", "start", "com.linespotting.gbr-agent").Run()
	return nil
}

func uninstallPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "stop", "com.linespotting.gbr-agent").Run()
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
