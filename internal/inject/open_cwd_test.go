package inject

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultProductCWD_envOverride(t *testing.T) {
	t.Setenv("GBR_OPEN_CWD", "/tmp/gbr-open-cwd-test")
	if got := DefaultProductCWD(); got != "/tmp/gbr-open-cwd-test" {
		t.Fatalf("GBR_OPEN_CWD: got %q", got)
	}
}

func TestDefaultProductCWD_platform(t *testing.T) {
	t.Setenv("GBR_OPEN_CWD", "")
	got := DefaultProductCWD()
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, "Developer")
		if got != want {
			t.Fatalf("darwin: got %q want %q", got, want)
		}
	case "windows":
		if got != `C:\pc-build` {
			t.Fatalf("windows: got %q", got)
		}
	}
}

func TestResolveOpenCWD_explicitWins(t *testing.T) {
	t.Setenv("GBR_OPEN_CWD", "/tmp/should-not-use")
	got := resolveOpenCWD("/tmp/explicit")
	if abs, err := filepath.Abs("/tmp/explicit"); err == nil {
		if got != abs {
			t.Fatalf("got %q want %q", got, abs)
		}
	}
}
