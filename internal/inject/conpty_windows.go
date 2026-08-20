//go:build windows

package inject

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultPTYCols = 120
	defaultPTYRows = 40
)

var (
	modKernel32ConPTY                    = syscall.NewLazyDLL("kernel32.dll")
	procCreatePseudoConsole = modKernel32ConPTY.NewProc("CreatePseudoConsole")
	procClosePseudoConsole  = modKernel32ConPTY.NewProc("ClosePseudoConsole")
	procTerminateProcess    = modKernel32ConPTY.NewProc("TerminateProcess")
	procWaitForSingleObject = modKernel32ConPTY.NewProc("WaitForSingleObject")
)

type winConPTY struct {
	hpc      syscall.Handle
	inW      syscall.Handle
	outR     syscall.Handle
	ptyIn    syscall.Handle
	ptyOut   syscall.Handle
	process  syscall.Handle
	thread   syscall.Handle
	attrList *windows.ProcThreadAttributeListContainer
}

type conptyRW struct {
	h syscall.Handle
}

func (r *conptyRW) Read(p []byte) (int, error) {
	if r == nil || r.h == 0 {
		return 0, io.EOF
	}
	var n uint32
	err := syscall.ReadFile(r.h, p, &n, nil)
	if n > 0 && err != nil {
		return int(n), nil
	}
	if err != nil {
		return int(n), err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (r *conptyRW) Write(p []byte) (int, error) {
	if r == nil || r.h == 0 {
		return 0, ErrSessionClosed
	}
	var n uint32
	err := syscall.WriteFile(r.h, p, &n, nil)
	return int(n), err
}

func (r *conptyRW) Close() error {
	if r == nil || r.h == 0 {
		return nil
	}
	h := r.h
	r.h = 0
	return syscall.CloseHandle(h)
}

func (s *ManagedSession) startConPTYLocked() error {
	c, stdin, stdout, err := openWinConPTY(defaultPTYCols, defaultPTYRows)
	if err != nil {
		return err
	}
	s.conpty = c
	s.stdin = stdin
	s.stdout = stdout
	s.stderr = nil
	// Read before the child starts — short-lived commands otherwise race EOF.
	go s.collect(stdout, nil)

	pid, err := c.spawn(s.Shell, s.Args, s.Cwd, defaultPTYCols, defaultPTYRows)
	if err != nil {
		c.close()
		s.conpty = nil
		return err
	}
	s.pid = pid
	return nil
}

func openWinConPTY(cols, rows int16) (*winConPTY, io.WriteCloser, io.ReadCloser, error) {
	if procCreatePseudoConsole.Find() != nil {
		return nil, nil, nil, fmt.Errorf("inject/conpty: CreatePseudoConsole not available")
	}

	var ptyIn, cmdIn syscall.Handle
	if err := syscall.CreatePipe(&ptyIn, &cmdIn, nil, 0); err != nil {
		return nil, nil, nil, fmt.Errorf("inject/conpty: input pipe: %w", err)
	}
	var cmdOut, ptyOut syscall.Handle
	if err := syscall.CreatePipe(&cmdOut, &ptyOut, nil, 0); err != nil {
		_ = syscall.CloseHandle(ptyIn)
		_ = syscall.CloseHandle(cmdIn)
		return nil, nil, nil, fmt.Errorf("inject/conpty: output pipe: %w", err)
	}

	var hpc syscall.Handle
	coord := uintptr(uint32(uint16(cols)) | uint32(uint16(rows))<<16)
	ret, _, err := procCreatePseudoConsole.Call(coord, uintptr(ptyIn), uintptr(ptyOut), 0, uintptr(unsafe.Pointer(&hpc)))
	if ret != 0 {
		_ = syscall.CloseHandle(ptyIn)
		_ = syscall.CloseHandle(cmdIn)
		_ = syscall.CloseHandle(cmdOut)
		_ = syscall.CloseHandle(ptyOut)
		if err == nil || err == syscall.Errno(0) {
			err = fmt.Errorf("status 0x%x", ret)
		}
		return nil, nil, nil, fmt.Errorf("inject/conpty: CreatePseudoConsole: %v", err)
	}
	// ConPTY duplicates these. Closing our copies unblocks CreateProcess.
	_ = syscall.CloseHandle(ptyIn)
	_ = syscall.CloseHandle(ptyOut)

	c := &winConPTY{
		hpc:  hpc,
		inW:  cmdIn,
		outR: cmdOut,
	}
	return c, &conptyRW{h: cmdIn}, &conptyRW{h: cmdOut}, nil
}

func (c *winConPTY) spawn(exe string, args []string, cwd string, cols, rows int16) (int, error) {
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, fmt.Errorf("inject/conpty: attr list: %w", err)
	}
	hpc := windows.Handle(c.hpc)
	if err := attrList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&hpc), unsafe.Sizeof(hpc)); err != nil {
		attrList.Delete()
		return 0, fmt.Errorf("inject/conpty: Update attr: %w", err)
	}
	c.attrList = attrList

	cmdLine, err := windows.UTF16PtrFromString(quoteWinCmd(exe, args))
	if err != nil {
		return 0, err
	}
	var dirPtr *uint16
	if cwd != "" {
		dirPtr, err = windows.UTF16PtrFromString(cwd)
		if err != nil {
			return 0, err
		}
	}
	env := append(os.Environ(),
		"TERM=xterm-256color",
		fmt.Sprintf("COLUMNS=%d", cols),
		fmt.Sprintf("LINES=%d", rows),
	)
	envPtr := makeEnvBlock(env)

	si := new(windows.StartupInfoEx)
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.ProcThreadAttributeList = attrList.List()

	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	err = windows.CreateProcess(
		nil,
		cmdLine,
		nil,
		nil,
		false,
		flags,
		envPtr,
		dirPtr,
		&si.StartupInfo,
		&pi,
	)
	runtime.KeepAlive(si)
	runtime.KeepAlive(hpc)
	runtime.KeepAlive(attrList)
	if err != nil {
		return 0, fmt.Errorf("inject/conpty: CreateProcess: %w", err)
	}
	c.process = syscall.Handle(pi.Process)
	c.thread = syscall.Handle(pi.Thread)
	return int(pi.ProcessId), nil
}

func (c *winConPTY) wait() {
	if c == nil || c.process == 0 {
		return
	}
	procWaitForSingleObject.Call(uintptr(c.process), syscall.INFINITE)
}

func (c *winConPTY) close() {
	if c == nil {
		return
	}
	if c.hpc != 0 {
		procClosePseudoConsole.Call(uintptr(c.hpc))
		c.hpc = 0
	}
	if c.process != 0 {
		procTerminateProcess.Call(uintptr(c.process), 1)
		_ = syscall.CloseHandle(c.process)
		c.process = 0
	}
	if c.thread != 0 {
		_ = syscall.CloseHandle(c.thread)
		c.thread = 0
	}
	for _, h := range []syscall.Handle{c.inW, c.outR, c.ptyIn, c.ptyOut} {
		if h != 0 {
			_ = syscall.CloseHandle(h)
		}
	}
	c.inW, c.outR, c.ptyIn, c.ptyOut = 0, 0, 0, 0
	if c.attrList != nil {
		c.attrList.Delete()
		c.attrList = nil
	}
}

func quoteWinCmd(exe string, args []string) string {
	out := syscall.EscapeArg(exe)
	for _, a := range args {
		out += " " + syscall.EscapeArg(a)
	}
	return out
}

func makeEnvBlock(env []string) *uint16 {
	var u []uint16
	for _, e := range env {
		uu, err := syscall.UTF16FromString(e)
		if err != nil {
			continue
		}
		u = append(u, uu...)
	}
	u = append(u, 0)
	return &u[0]
}
