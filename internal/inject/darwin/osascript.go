//go:build darwin

package darwin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

func runOSAscript(ctx context.Context, source string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "osascript", "-e", source)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("osascript: %s", msg)
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

func escapeAS(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
		case '\t':
			b.WriteString(`\t`)
		default:
			if r == 0 {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func keystrokeSafe(s string) bool {
	if len(s) == 0 || len(s) > 200 {
		return false
	}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 || r == 127 || r > unicode.MaxASCII {
			return false
		}
		if r == '"' || r == '\\' {
			return false
		}
	}
	return true
}

func activateApp(ctx context.Context, app string) error {
	_, err := runOSAscript(ctx, fmt.Sprintf(`tell application %q to activate`, app))
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
