//go:build windows

package windows

import (
	"fmt"
	"syscall"
)

// captureNote documents Day-1 console capture reality for operators/docs.
const captureNote = "Best-effort AttachConsole+ReadConsoleOutputCharacter. " +
	"Fails for Windows Terminal ConPTY panes, elevated mismatch, or when " +
	"another process holds the attach. Prefer inject.Manager (managed pwsh) " +
	"for reliable session capture/streaming."

// CapturePID attempts to read the visible console buffer of a classic console
// process. This is best-effort and often unavailable for Windows Terminal.
func CapturePID(pid uint32) (Capture, error) {
	if pid == 0 {
		return Capture{
			Partial: true,
			Method:  "none",
			Note:    captureNote,
		}, ErrCaptureUnavail
	}

	// Free any existing console so AttachConsole can succeed.
	_ = freeConsole()
	if err := attachConsole(pid); err != nil {
		return Capture{
			Partial: true,
			Method:  "attachconsole",
			Note:    captureNote + " attach error: " + err.Error(),
		}, fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	}
	defer freeConsole()

	h, err := getStdHandle(stdOutputHandle)
	if err != nil {
		return Capture{
			Partial: true,
			Method:  "attachconsole",
			Note:    captureNote + " std handle: " + err.Error(),
		}, fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	}

	info, err := getConsoleScreenBufferInfo(h)
	if err != nil {
		return Capture{
			Partial: true,
			Method:  "readconsole",
			Note:    captureNote + " buffer info: " + err.Error(),
		}, fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
	}

	width := int(info.Window.Right - info.Window.Left + 1)
	height := int(info.Window.Bottom - info.Window.Top + 1)
	if width <= 0 || height <= 0 {
		return Capture{
			Partial: true,
			Method:  "readconsole",
			Note:    captureNote + " empty window region",
		}, ErrCaptureUnavail
	}

	const maxRows = 200
	if height > maxRows {
		height = maxRows
	}

	var b []byte
	for row := 0; row < height; row++ {
		y := info.Window.Top + int16(row)
		line, err := readConsoleOutputCharacter(h, uint32(width), coord{X: info.Window.Left, Y: y})
		if err != nil {
			return Capture{
				Text:    string(b),
				Partial: true,
				Method:  "readconsole",
				Note:    captureNote + " read row: " + err.Error(),
			}, fmt.Errorf("%w: %v", ErrCaptureUnavail, err)
		}
		line = trimRightSpaces(line)
		b = append(b, line...)
		b = append(b, '\n')
	}

	return Capture{
		Text:    string(b),
		Partial: true, // always partial: no full scrollback, WT unsupported
		Method:  "readconsole",
		Note:    captureNote,
	}, nil
}

// CaptureHWND resolves the PID for hwnd and captures that console.
func CaptureHWND(hwnd syscall.Handle) (Capture, error) {
	if hwnd == 0 {
		return Capture{Partial: true, Method: "none", Note: captureNote}, ErrCaptureUnavail
	}
	_, pid := getWindowThreadProcessId(hwnd)
	return CapturePID(pid)
}

func trimRightSpaces(s string) string {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c != ' ' && c != '\t' {
			break
		}
		i--
	}
	return s[:i]
}
