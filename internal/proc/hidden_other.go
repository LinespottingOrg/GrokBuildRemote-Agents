//go:build !windows

package proc

import "os/exec"

func applyHidden(cmd *exec.Cmd) {}
