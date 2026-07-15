//go:build windows

package windows

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procEnumWindows              = modUser32.NewProc("EnumWindows")
	procIsWindowVisible          = modUser32.NewProc("IsWindowVisible")
	procGetWindowTextW           = modUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = modUser32.NewProc("GetWindowTextLengthW")
	procGetClassNameW            = modUser32.NewProc("GetClassNameW")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procShowWindow               = modUser32.NewProc("ShowWindow")
	procBringWindowToTop         = modUser32.NewProc("BringWindowToTop")
	procAttachThreadInput        = modUser32.NewProc("AttachThreadInput")
	procGetForegroundWindow      = modUser32.NewProc("GetForegroundWindow")
	procSendInput                = modUser32.NewProc("SendInput")
	procIsIconic                 = modUser32.NewProc("IsIconic")

	procOpenProcess             = modKernel32.NewProc("OpenProcess")
	procCloseHandle             = modKernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = modKernel32.NewProc("QueryFullProcessImageNameW")
	procGetCurrentThreadId      = modKernel32.NewProc("GetCurrentThreadId")
	procAttachConsole           = modKernel32.NewProc("AttachConsole")
	procFreeConsole             = modKernel32.NewProc("FreeConsole")
	procGetStdHandle            = modKernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBufferInfo = modKernel32.NewProc("GetConsoleScreenBufferInfo")
	procReadConsoleOutputCharacterW = modKernel32.NewProc("ReadConsoleOutputCharacterW")
)

// Win32 constants.
const (
	swRestore = 9
	swShow    = 5

	processQueryLimitedInformation = 0x1000

	inputKeyboard   = 1
	keyeventfKeyup  = 0x0002
	keyeventfUnicode = 0x0004

	vkReturn = 0x0D

	// STD_OUTPUT_HANDLE = (DWORD)(-11)
	stdOutputHandle = ^uintptr(10)

	attachParentProcess = ^uint32(0) // (DWORD)-1
)

// keyboardInput matches the KEYBDINPUT portion of the INPUT union for
// SendInput on both 386 and amd64 when packed carefully.
// We use a fixed layout that matches Win64 INPUT for type=INPUT_KEYBOARD.
//
// typedef struct tagINPUT {
//   DWORD type;
//   union { MOUSEINPUT mi; KEYBDINPUT ki; HARDWAREINPUT hi; };
// } INPUT;
//
// On amd64, the union is 24 bytes (MOUSEINPUT is largest with alignment).
// KEYBDINPUT: wVk, wScan, dwFlags, time, dwExtraInfo → 2+2+4+4+8 = 20, pad to 24.
type keyboardInput struct {
	Type      uint32
	_pad0     uint32 // alignment before union on amd64
	WVk       uint16
	WScan     uint16
	DwFlags   uint32
	Time      uint32
	DwExtraInfo uintptr
	_pad1     uint64 // pad union to MOUSEINPUT size (24 bytes payload after type+pad)
}

// On 64-bit Windows, sizeof(INPUT) is 40 (4 type + 4 pad + 32 union in some
// SDKs) or 28/40 depending on version. We use the golang.org/x/sys style
// layout proven with SendInput for Unicode.

// kiInput is the INPUT structure layout used by golang.org/x/sys/windows
// for keyboard events (amd64).
type kiInput struct {
	Type uint32
	_    uint32
	Ki   keybdInput
}

type keybdInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte // padding to match MOUSEINPUT union size on amd64
}

type coord struct {
	X int16
	Y int16
}

type smallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func enumWindows(cb func(hwnd syscall.Handle) bool) error {
	// WNDENUMPROC: BOOL CALLBACK EnumFunc(HWND hwnd, LPARAM lParam)
	var enumProc uintptr
	enumProc = syscall.NewCallback(func(hwnd syscall.Handle, _ uintptr) uintptr {
		if cb(hwnd) {
			return 1 // continue
		}
		return 0 // stop
	})
	r, _, err := procEnumWindows.Call(enumProc, 0)
	if r == 0 {
		// EnumWindows returns 0 if callback stopped OR on failure.
		// Failure sets last error; stop-by-callback also returns 0 with success.
		if err != syscall.Errno(0) {
			// Common when callback returns FALSE — treat as OK if we meant to stop.
			return nil
		}
	}
	return nil
}

func isWindowVisible(hwnd syscall.Handle) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return r != 0
}

func isIconic(hwnd syscall.Handle) bool {
	r, _, _ := procIsIconic.Call(uintptr(hwnd))
	return r != 0
}

func getWindowText(hwnd syscall.Handle) string {
	n, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func getClassName(hwnd syscall.Handle) string {
	buf := make([]uint16, 256)
	r, _, _ := procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func getWindowThreadProcessId(hwnd syscall.Handle) (threadID, pid uint32) {
	var p uint32
	t, _, _ := procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&p)))
	return uint32(t), p
}

func getCurrentThreadId() uint32 {
	r, _, _ := procGetCurrentThreadId.Call()
	return uint32(r)
}

func getForegroundWindow() syscall.Handle {
	r, _, _ := procGetForegroundWindow.Call()
	return syscall.Handle(r)
}

func showWindow(hwnd syscall.Handle, cmd int) {
	procShowWindow.Call(uintptr(hwnd), uintptr(cmd))
}

func bringWindowToTop(hwnd syscall.Handle) {
	procBringWindowToTop.Call(uintptr(hwnd))
}

func setForegroundWindow(hwnd syscall.Handle) bool {
	r, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	return r != 0
}

func attachThreadInput(idAttach, idAttachTo uint32, attach bool) bool {
	var a uintptr
	if attach {
		a = 1
	}
	r, _, _ := procAttachThreadInput.Call(uintptr(idAttach), uintptr(idAttachTo), a)
	return r != 0
}

func openProcessImage(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)

	var size uint32 = 260
	buf := make([]uint16, size)
	// QueryFullProcessImageNameW(handle, 0, buf, &size)
	r, _, _ := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func sendInputKeyboard(inputs []kiInput) (uint32, error) {
	if len(inputs) == 0 {
		return 0, nil
	}
	n, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if n == 0 {
		if err != nil && err != syscall.Errno(0) {
			return 0, fmt.Errorf("SendInput: %w", err)
		}
		return 0, fmt.Errorf("SendInput: injected 0 events")
	}
	return uint32(n), nil
}

func freeConsole() error {
	r, _, err := procFreeConsole.Call()
	if r == 0 && err != syscall.Errno(0) {
		return err
	}
	return nil
}

func attachConsole(pid uint32) error {
	r, _, err := procAttachConsole.Call(uintptr(pid))
	if r == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return fmt.Errorf("AttachConsole failed")
	}
	return nil
}

func getStdHandle(n uintptr) (syscall.Handle, error) {
	r, _, err := procGetStdHandle.Call(n)
	if r == 0 || r == ^uintptr(0) {
		if err != nil && err != syscall.Errno(0) {
			return 0, err
		}
		return 0, fmt.Errorf("GetStdHandle failed")
	}
	return syscall.Handle(r), nil
}

func getConsoleScreenBufferInfo(h syscall.Handle) (consoleScreenBufferInfo, error) {
	var info consoleScreenBufferInfo
	r, _, err := procGetConsoleScreenBufferInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if err != nil && err != syscall.Errno(0) {
			return info, err
		}
		return info, fmt.Errorf("GetConsoleScreenBufferInfo failed")
	}
	return info, nil
}

func readConsoleOutputCharacter(h syscall.Handle, n uint32, start coord) (string, error) {
	if n == 0 {
		return "", nil
	}
	buf := make([]uint16, n)
	var read uint32
	r, _, err := procReadConsoleOutputCharacterW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(n),
		// COORD is passed by value as a single DWORD on x64 (X low, Y high).
		uintptr(uint32(uint16(start.X))|uint32(uint16(start.Y))<<16),
		uintptr(unsafe.Pointer(&read)),
	)
	if r == 0 {
		if err != nil && err != syscall.Errno(0) {
			return "", err
		}
		return "", fmt.Errorf("ReadConsoleOutputCharacter failed")
	}
	return syscall.UTF16ToString(buf[:read]), nil
}
