//go:build linux

package linux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const xdotoolBin = "xdotool"

func runXdotool(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, xdotoolBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
			return "", fmt.Errorf("xdotool not found in PATH (install xdotool; X11 required): %w", err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("xdotool %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func ensureXdotool(ctx context.Context) error {
	if _, err := exec.LookPath(xdotoolBin); err != nil {
		return fmt.Errorf("xdotool not found in PATH — install xdotool and use an X11 session: %w", err)
	}
	if _, err := runXdotool(ctx, "version"); err != nil {
		return err
	}
	return nil
}

func sessionIsWayland() bool {
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return true
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == "" {
		return true
	}
	return false
}

func displayEnvOK() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) != ""
}
