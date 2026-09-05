//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
	// User home — not the binary folder (avoids a synthetic "dist" session).
	workDir, _ := os.UserHomeDir()
	if workDir == "" {
		workDir = p.DataDir
	}
	body, err := renderLinuxUnit(p.Binary, workDir, os.Getenv("GBR_RELAY_URL"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.UnitPath, []byte(body), 0o644); err != nil {
		return err
	}
	if err := installLinuxDesktop(workDir, p.Binary); err != nil {
		return err
	}
	// enable --user
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	out, err := exec.Command("systemctl", "--user", "enable", "--now", LinuxUnitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl enable: %w\n%s\n(hint: loginctl enable-linger $USER for headless)", err, string(out))
	}
	return nil
}

func linuxDesktopPath(home string) string {
	return filepath.Join(home, ".local", "share", "applications", LinuxDesktopFile)
}

func installLinuxDesktop(home, binary string) error {
	path := linuxDesktopPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(linuxDesktopEntry(binary)), 0o644)
}

func uninstallPlatform() error {
	p, err := Resolve()
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", LinuxUnitName).Run()
	_ = os.Remove(p.UnitPath)
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(linuxDesktopPath(home))
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, _ := exec.Command("systemctl", "--user", "status", LinuxUnitName, "--no-pager").CombinedOutput()
	exists := "false"
	if _, err := os.Stat(p.UnitPath); err == nil {
		exists = "true"
	}
	return fmt.Sprintf("unit=%s installed=%s\nbinary=%s\n%s\nnote=%s\n",
		p.UnitPath, exists, p.Binary, string(out), p.ExtraNotes), nil
}
