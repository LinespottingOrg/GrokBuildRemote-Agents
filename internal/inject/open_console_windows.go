//go:build windows

package inject

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func (h *Hybrid) openGrokWindow(req OpenRequest) (OpenResult, error) {
	if h == nil || h.UI == nil {
		return OpenResult{}, fmt.Errorf("open grok window: no UI injector")
	}
	bin, err := LookGrok()
	if err != nil {
		return OpenResult{}, err
	}
	sid := sanitizeOpenID(req.SessionID)
	if sid == "" {
		sid = newOpenSessionID(req.Resume)
	}
	cwd := resolveOpenCWD(req.CWD)
	args := []string{}
	resume := strings.TrimSpace(req.Resume)
	if resume != "" {
		args = []string{"--resume", resume}
	}

	before := map[uintptr]struct{}{}
	if wins, err := h.UI.Discover(); err == nil {
		for _, w := range wins {
			before[w.HWND] = struct{}{}
		}
	}

	proc, err := startVisibleConsole(bin, args, cwd)
	if err != nil {
		return OpenResult{}, err
	}

	grokPID := findChildNamed(proc.Pid, "grok.exe", 3*time.Second)
	if grokPID == 0 {
		grokPID = proc.Pid
	}

	chosen, err := waitNewConsole(h.UI, before, grokPID, 8*time.Second)
	if err != nil {
		_ = proc.Kill()
		return OpenResult{}, err
	}
	chosen.PID = uint32(grokPID)
	if IsProtectedTitle(chosen.Title) {
		_ = proc.Kill()
		return OpenResult{}, fmt.Errorf("open grok window: refused protected title %q", chosen.Title)
	}
	if err := h.UI.Bind(sid, chosen); err != nil {
		_ = proc.Kill()
		return OpenResult{}, err
	}
	h.rememberWindow(sid, grokPID, chosen.HWND)
	h.waitReady(sid, 4*time.Second)

	note := "spawned grok in a real console window; inject uses SendInput"
	if resume != "" {
		note = "spawned grok --resume in a real console window; inject uses SendInput"
	}
	return OpenResult{
		SessionID: sid,
		Opened:    true,
		Method:    "window",
		Command:   "grok",
		CWD:       cwd,
		Resume:    resume,
		PID:       grokPID,
		HWND:      chosen.HWND,
		Note:      note,
	}, nil
}

func startVisibleConsole(bin string, args []string, cwd string) (*os.Process, error) {
	// Must NOT use os/exec: it always sets STARTF_USESTDHANDLES (NUL),
	// which detaches grok from the new console so no HWND appears.
	cmdline := quoteWinCmd(bin, args)
	cl, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return nil, err
	}
	app, err := syscall.UTF16PtrFromString(bin)
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if cwd != "" {
		dir, err = syscall.UTF16PtrFromString(cwd)
		if err != nil {
			return nil, err
		}
	}
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = syscall.STARTF_USESHOWWINDOW
	si.ShowWindow = 1 // SW_SHOWNORMAL
	var pi syscall.ProcessInformation
	err = syscall.CreateProcess(
		app,
		cl,
		nil,
		nil,
		false,
		0x00000010, // CREATE_NEW_CONSOLE
		nil,
		dir,
		&si,
		&pi,
	)
	if err != nil {
		return nil, fmt.Errorf("open grok window: CreateProcess: %w", err)
	}
	if pi.Thread != 0 {
		_ = syscall.CloseHandle(pi.Thread)
	}
	proc, err := os.FindProcess(int(pi.ProcessId))
	if pi.Process != 0 {
		_ = syscall.CloseHandle(pi.Process)
	}
	if err != nil {
		return nil, err
	}
	slog.Info("open grok window started", "pid", proc.Pid, "bin", bin)
	return proc, nil
}

func waitNewConsole(ui Injector, before map[uintptr]struct{}, pid int, timeout time.Duration) (TerminalWindow, error) {
	deadline := time.Now().Add(timeout)
	var fallback TerminalWindow
	for time.Now().Before(deadline) {
		wins, err := ui.Discover()
		if err == nil {
			for _, w := range wins {
				if w.HWND == 0 || IsProtectedTitle(w.Title) {
					continue
				}
				if _, old := before[w.HWND]; old {
					continue
				}
				if int(w.PID) == pid || containsFold(w.ExeName, "grok") || containsFold(w.Title, "grok") {
					return w, nil
				}
				if fallback.HWND == 0 {
					fallback = w
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	if fallback.HWND != 0 {
		return fallback, nil
	}
	return TerminalWindow{}, fmt.Errorf("open grok window: no new console for pid %d", pid)
}

func findChildNamed(parentPID int, exe string, timeout time.Duration) int {
	if parentPID <= 0 {
		return 0
	}
	want := strings.ToLower(exe)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pid := childNamed(uint32(parentPID), want); pid != 0 {
			return int(pid)
		}
		// grok.exe launched directly (no conhost wrapper)
		if selfNamed(uint32(parentPID), want) {
			return parentPID
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0
}

func childNamed(parent uint32, wantExe string) uint32 {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := syscall.Process32First(snap, &pe); err != nil {
		return 0
	}
	for {
		if pe.ParentProcessID == parent {
			name := strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))
			if name == wantExe {
				return pe.ProcessID
			}
		}
		if err := syscall.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return 0
}

func selfNamed(pid uint32, wantExe string) bool {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := syscall.Process32First(snap, &pe); err != nil {
		return false
	}
	for {
		if pe.ProcessID == pid {
			name := strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))
			return name == wantExe
		}
		if err := syscall.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return false
}
