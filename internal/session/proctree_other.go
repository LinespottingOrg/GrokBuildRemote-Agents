//go:build !windows

package session

func scanGrokWindowsNative() []GrokProc { return nil }
