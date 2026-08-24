//go:build windows

package session

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func scanGrokWindowsNative() []GrokProc {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return nil
	}

	var out []GrokProc
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		is, resume := ParseGrokCommand(name)
		if is {
			out = append(out, GrokProc{
				PID:      int(pe.ProcessID),
				PPID:     int(pe.ParentProcessID),
				ResumeID: resume,
				Cmd:      name,
			})
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return out
}
