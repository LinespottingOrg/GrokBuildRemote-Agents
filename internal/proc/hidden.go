// Package proc centralises child-process spawning so that on Windows every
// child (gh, git, grok, schtasks, explorer, …) is created without flashing a
// console/terminal window. On non-Windows platforms the helpers are no-ops.
package proc

import "os/exec"

// Hidden applies the platform's "no visible window" process attributes to cmd
// and returns the same *exec.Cmd for convenient chaining. On Windows this sets
// SysProcAttr{HideWindow:true, CreationFlags:CREATE_NO_WINDOW}; elsewhere it is
// a no-op. Safe to call with a nil cmd.
func Hidden(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return cmd
	}
	applyHidden(cmd)
	return cmd
}

// Command is a drop-in replacement for exec.Command that already has the hidden
// window attributes applied on Windows.
func Command(name string, args ...string) *exec.Cmd {
	return Hidden(exec.Command(name, args...))
}
