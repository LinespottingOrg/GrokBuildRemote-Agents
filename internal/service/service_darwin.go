//go:build darwin

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
	if err := os.MkdirAll(filepath.Dir(p.UnitPath), 0o755); err != nil {
		return err
	}
	// Dropbox copies carry quarantine/xattrs; unsigned binaries then die with
	// OS_REASON_CODESIGNING under launchd. Ad-hoc sign the installed binary.
	prepareDarwinBinary(p.Binary)
	// Product clone root — not the binary folder (install from dist/ used to
	// create a "dist" session) and not $HOME (inbox #119: ~/Developer).
	workDir := strings.TrimSpace(os.Getenv("GBR_OPEN_CWD"))
	if workDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			workDir = filepath.Join(home, "Developer")
		}
	}
	if workDir == "" {
		workDir = p.DataDir
	}
	if _, err := installDarwinAppBundle(workDir, p.Binary); err != nil {
		return err
	}
	body, err := renderDarwinPlist(p.Binary, workDir, p.DataDir, workDir, os.Getenv("GBR_RELAY_URL"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.UnitPath, []byte(body), 0o644); err != nil {
		return err
	}
	return launchAgentEnable(p.UnitPath)
}

// installDarwinAppBundle writes ~/Applications/Grok Build Remote.app so
// System Settings → Login Items can show CFBundleDisplayName instead of gbr-agent.
// The stub execs the real CLI; Accessibility still applies to p.Binary.
func installDarwinAppBundle(home, binary string) (string, error) {
	app := darwinAppBundlePath(home)
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(darwinInfoPlist), 0o644); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(app, "Contents", "PkgInfo"), []byte("APPL????"), 0o644)
	stub := filepath.Join(macos, ProductName)
	if err := os.WriteFile(stub, []byte(darwinStubScript(binary)), 0o755); err != nil {
		return "", err
	}
	_ = os.Chmod(stub, 0o755)
	return app, nil
}

func prepareDarwinBinary(bin string) {
	_ = exec.Command("xattr", "-cr", bin).Run()
	_ = exec.Command("codesign", "--force", "--sign", "-", bin).Run()
}

func launchAgentEnable(plist string) error {
	label := DarwinLaunchAgentLabel
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
	label := DarwinLaunchAgentLabel
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	_ = exec.Command("launchctl", "bootout", target).Run()
	_ = exec.Command("launchctl", "stop", label).Run()
	_ = exec.Command("launchctl", "unload", p.UnitPath).Run()
	_ = os.Remove(p.UnitPath)
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.RemoveAll(darwinAppBundlePath(home))
	}
	return nil
}

func statusPlatform() (string, error) {
	p, err := Resolve()
	if err != nil {
		return "", err
	}
	out, _ := exec.Command("launchctl", "list", DarwinLaunchAgentLabel).CombinedOutput()
	exists := "false"
	if _, err := os.Stat(p.UnitPath); err == nil {
		exists = "true"
	}
	return fmt.Sprintf("plist=%s installed=%s\nbinary=%s\n%s\nnote=%s\n",
		p.UnitPath, exists, p.Binary, string(out), p.ExtraNotes), nil
}
