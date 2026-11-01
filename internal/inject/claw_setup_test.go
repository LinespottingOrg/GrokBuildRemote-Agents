package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsClawSetupMatchesMacProtocol(t *testing.T) {
	root := filepath.Join("..", "..", "scripts")
	sh, err := os.ReadFile(filepath.Join(root, "setup-gbr-mcp.sh"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := os.ReadFile(filepath.Join(root, "setup-gbr-mcp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	needles := []string{
		"gbr-mcp.js",
		"hermes",
		"openclaw",
		"127.0.0.1:8788",
		"gbr_bot_require_key",
		"stdio",
		"nemoclaw",
	}
	for _, n := range needles {
		if !strings.Contains(strings.ToLower(string(sh)), n) {
			t.Errorf("setup-gbr-mcp.sh missing %s", n)
		}
		if !strings.Contains(strings.ToLower(string(ps)), n) {
			t.Errorf("setup-gbr-mcp.ps1 missing %s", n)
		}
	}
}
