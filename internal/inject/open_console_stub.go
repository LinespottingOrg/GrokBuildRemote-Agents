//go:build !windows

package inject

import "fmt"

func (h *Hybrid) openGrokWindow(req OpenRequest) (OpenResult, error) {
	return OpenResult{}, fmt.Errorf("open grok window: not supported on this OS")
}
