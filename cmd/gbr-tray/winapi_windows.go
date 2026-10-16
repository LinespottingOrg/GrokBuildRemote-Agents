//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK            = 0x00000000
	mbIconInfo      = 0x00000040
	mbIconWarning   = 0x00000030
	mbSetForeground = 0x00010000
	mbTopmost       = 0x00040000
)

// messageBox shows a modal information dialog. flags default to info icon.
func messageBox(title, text string, flags uintptr) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)),
		flags|mbOK|mbSetForeground|mbTopmost)
}
