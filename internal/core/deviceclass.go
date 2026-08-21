package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Device class vocabulary Grok Bot uses to tell machines apart.
// These are form-factor labels, not OS names: a MacBook is "laptop",
// a Mac mini is "mac_mini", a Windows tower is "pc".
const (
	ClassPhone   = "phone"
	ClassLinux   = "linux"
	ClassPC      = "pc"
	ClassLaptop  = "laptop"
	ClassMacMini = "mac_mini"
)

// CanonicalClasses is the ordered set advertised to Grok Bot and the website.
var CanonicalClasses = []string{ClassPhone, ClassLinux, ClassPC, ClassLaptop, ClassMacMini}

var classAliases = map[string]string{
	"phone": ClassPhone, "ios": ClassPhone, "android": ClassPhone,
	"mobile": ClassPhone, "iphone": ClassPhone, "ipad": ClassPhone,

	"linux": ClassLinux, "gnu-linux": ClassLinux, "debian": ClassLinux,
	"ubuntu": ClassLinux, "server": ClassLinux, "box": ClassLinux,

	"pc": ClassPC, "windows": ClassPC, "windows-pc": ClassPC, "win": ClassPC,
	"desktop": ClassPC, "tower": ClassPC, "imac": ClassPC, "mac-studio": ClassPC,
	"macstudio": ClassPC, "mac-pro": ClassPC, "macpro": ClassPC,

	"laptop": ClassLaptop, "notebook": ClassLaptop, "macbook": ClassLaptop,
	"portable": ClassLaptop,

	"mac-mini": ClassMacMini, "macmini": ClassMacMini, "mac_mini": ClassMacMini,
	"mini": ClassMacMini,
}

// ParseClass maps a user/bot token to a canonical class.
func ParseClass(s string) (string, bool) {
	s = normalizeFleetID(s)
	if s == "" {
		return "", false
	}
	if cls, ok := classAliases[s]; ok {
		return cls, true
	}
	for _, c := range CanonicalClasses {
		if s == c {
			return c, true
		}
	}
	return "", false
}

// DetectClass infers this machine's class from hardware, then GOOS.
// Override with GBR_DEVICE_CLASS (canonical or alias). Tests may set
// GBR_HW_MODEL / GBR_CHASSIS_TYPE instead of touching real firmware.
func DetectClass() string {
	if v := strings.TrimSpace(os.Getenv("GBR_DEVICE_CLASS")); v != "" {
		if cls, ok := ParseClass(v); ok {
			return cls
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return classFromDarwin(hwModel())
	case "linux":
		return classFromLinux(chassisType())
	case "windows":
		return classFromWindows(chassisType())
	default:
		return ClassPC
	}
}

func classFromDarwin(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "macmini"):
		return ClassMacMini
	case strings.HasPrefix(m, "macbook"):
		return ClassLaptop
	case strings.HasPrefix(m, "imac"), strings.HasPrefix(m, "macpro"), strings.HasPrefix(m, "macstudio"):
		return ClassPC
	default:
		return ClassLaptop
	}
}

func classFromLinux(chassis string) string {
	switch strings.TrimSpace(chassis) {
	case "8", "9", "10", "14":
		return ClassLaptop
	case "3", "4", "6", "7", "13", "15":
		return ClassPC
	default:
		return ClassLinux
	}
}

func classFromWindows(chassis string) string {
	switch strings.TrimSpace(chassis) {
	case "8", "9", "10", "14":
		return ClassLaptop
	default:
		return ClassPC
	}
}

func hwModel() string {
	if v := strings.TrimSpace(os.Getenv("GBR_HW_MODEL")); v != "" {
		return v
	}
	if runtime.GOOS != "darwin" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.model").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func chassisType() string {
	if v := strings.TrimSpace(os.Getenv("GBR_CHASSIS_TYPE")); v != "" {
		return v
	}
	if runtime.GOOS != "linux" {
		return ""
	}
	b, err := os.ReadFile("/sys/class/dmi/id/chassis_type")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// HostnameBest is the machine hostname, or empty.
func HostnameBest() string {
	hn, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(hn)
}

// ClassLabel is the short UI string for a class.
func ClassLabel(class string) string {
	cls, ok := ParseClass(class)
	if !ok {
		if class == "" {
			return "device"
		}
		return class
	}
	switch cls {
	case ClassPhone:
		return "phone"
	case ClassLinux:
		return "linux"
	case ClassPC:
		return "PC"
	case ClassLaptop:
		return "laptop"
	case ClassMacMini:
		return "Mac Mini"
	default:
		return cls
	}
}

// LocalIdentity is the hub row Grok Bot sees for this process.
func LocalIdentity(mailboxID string, hasKey bool) map[string]any {
	cls := DetectClass()
	return map[string]any{
		"id":         "local",
		"name":       "this " + ClassLabel(cls),
		"kind":       "local",
		"class":      cls,
		"os":         runtime.GOOS,
		"hostname":   HostnameBest(),
		"mailbox_id": mailboxID,
		"has_key":    hasKey,
		"online":     true,
		"impl":       "gbr",
	}
}

// PhoneIdentity is the spectator app on this mailbox. Injects are refused.
func PhoneIdentity(mailboxID string) map[string]any {
	return map[string]any{
		"id":         "phone",
		"name":       "paired phone / tablet app",
		"kind":       "app",
		"class":      ClassPhone,
		"os":         "mobile",
		"mailbox_id": mailboxID,
		"has_key":    false,
		"online":     true,
		"impl":       "gbr-app",
		"role":       "spectator",
	}
}

// FormatClassHelp is the one-liner for CLI --help / Bot discovery.
func FormatClassHelp() string {
	return fmt.Sprintf("device classes: %s (Grok Bot routes by id, name, or class)", strings.Join(CanonicalClasses, " | "))
}
