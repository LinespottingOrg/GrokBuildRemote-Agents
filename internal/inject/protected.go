package inject

import (
	"fmt"
	"strings"
)

// IsProtectedTitle reports sessions the Bot API must never steal or type into.
// Live operator windows (Felanmälan, QA PC Android) stay human-owned.
func IsProtectedTitle(title string) bool {
	lt := strings.ToLower(strings.TrimSpace(title))
	if lt == "" {
		return false
	}
	needles := []string{
		"felanmälan",
		"felanmalan",
		"felanm\u00e4lan",
		"qa pc android",
		"qa pc andoid",
		"qa pc andr",
	}
	for _, n := range needles {
		if strings.Contains(lt, n) {
			return true
		}
	}
	return false
}

// RefuseProtected returns ErrProtected when title is an operator-owned window.
func RefuseProtected(title string) error {
	if !IsProtectedTitle(title) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrProtected, strings.TrimSpace(title))
}
