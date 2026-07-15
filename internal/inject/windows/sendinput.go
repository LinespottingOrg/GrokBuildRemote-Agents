//go:build windows

package windows

import (
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
)

// focusWindow restores (if minimized), raises, and sets foreground on hwnd.
// Uses AttachThreadInput trick when SetForegroundWindow is blocked by OS focus rules.
func focusWindow(hwnd syscall.Handle) error {
	if hwnd == 0 {
		return fmt.Errorf("focus: null hwnd")
	}
	if isIconic(hwnd) {
		showWindow(hwnd, swRestore)
	} else {
		showWindow(hwnd, swShow)
	}
	bringWindowToTop(hwnd)

	if setForegroundWindow(hwnd) {
		// Brief settle so the target console receives the next key events.
		time.Sleep(30 * time.Millisecond)
		return nil
	}

	// Attach our thread input to the current foreground thread, then retry.
	fg := getForegroundWindow()
	if fg != 0 {
		fgThread, _ := getWindowThreadProcessId(fg)
		cur := getCurrentThreadId()
		if fgThread != 0 && fgThread != cur {
			if attachThreadInput(cur, fgThread, true) {
				defer attachThreadInput(cur, fgThread, false)
				bringWindowToTop(hwnd)
				if setForegroundWindow(hwnd) {
					time.Sleep(30 * time.Millisecond)
					return nil
				}
			}
		}
	}

	// Last attempt without attach.
	if setForegroundWindow(hwnd) {
		time.Sleep(30 * time.Millisecond)
		return nil
	}
	return fmt.Errorf("SetForegroundWindow failed for hwnd=%v", hwnd)
}

// sendUnicodeText types s using KEYEVENTF_UNICODE (UTF-16 code units).
// Newline / CR become VK_RETURN keydown+keyup pairs (more reliable in consoles).
func sendUnicodeText(s string) error {
	if s == "" {
		return nil
	}

	var inputs []kiInput
	// Encode to UTF-16, handling surrogate pairs via utf16.Encode.
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// Normalize CRLF / LF / CR to a single Enter.
		if r == '\r' {
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			inputs = append(inputs, vkPair(vkReturn)...)
			continue
		}
		if r == '\n' {
			inputs = append(inputs, vkPair(vkReturn)...)
			continue
		}
		// Tab as Unicode works; leave as-is.
		units := utf16.Encode([]rune{r})
		for _, u := range units {
			inputs = append(inputs,
				kiInput{
					Type: inputKeyboard,
					Ki: keybdInput{
						Vk:    0,
						Scan:  u,
						Flags: keyeventfUnicode,
					},
				},
				kiInput{
					Type: inputKeyboard,
					Ki: keybdInput{
						Vk:    0,
						Scan:  u,
						Flags: keyeventfUnicode | keyeventfKeyup,
					},
				},
			)
		}
	}

	// Chunk large pastes — SendInput has practical limits and UI can stall.
	const chunk = 64 // events; each char = 2 events
	for off := 0; off < len(inputs); off += chunk {
		end := off + chunk
		if end > len(inputs) {
			end = len(inputs)
		}
		n, err := sendInputKeyboard(inputs[off:end])
		if err != nil {
			return err
		}
		if int(n) != end-off {
			return fmt.Errorf("SendInput short write: got %d want %d", n, end-off)
		}
		// Tiny pacing for long strings so the console keeps up.
		if end < len(inputs) {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return nil
}

func vkPair(vk uint16) []kiInput {
	return []kiInput{
		{
			Type: inputKeyboard,
			Ki: keybdInput{
				Vk:    vk,
				Scan:  0,
				Flags: 0,
			},
		},
		{
			Type: inputKeyboard,
			Ki: keybdInput{
				Vk:    vk,
				Scan:  0,
				Flags: keyeventfKeyup,
			},
		},
	}
}
