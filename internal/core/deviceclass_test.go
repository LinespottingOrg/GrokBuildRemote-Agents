package core

import (
	"runtime"
	"testing"
)

func TestParseClassAliases(t *testing.T) {
	cases := map[string]string{
		"phone":     ClassPhone,
		"iPhone":    ClassPhone,
		"android":   ClassPhone,
		"linux":     ClassLinux,
		"ubuntu":    ClassLinux,
		"pc":        ClassPC,
		"Windows":   ClassPC,
		"desktop":   ClassPC,
		"laptop":    ClassLaptop,
		"MacBook":   ClassLaptop,
		"mac-mini":  ClassMacMini,
		"MacMini":   ClassMacMini,
		"mini":      ClassMacMini,
		"mac_mini":  ClassMacMini,
	}
	for in, want := range cases {
		got, ok := ParseClass(in)
		if !ok || got != want {
			t.Errorf("ParseClass(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	if _, ok := ParseClass("toaster"); ok {
		t.Fatal("unknown class must not parse")
	}
}

func TestDetectClassOverride(t *testing.T) {
	t.Setenv("GBR_DEVICE_CLASS", "mac-mini")
	if got := DetectClass(); got != ClassMacMini {
		t.Fatalf("override: got %s", got)
	}
	t.Setenv("GBR_DEVICE_CLASS", "laptop")
	if got := DetectClass(); got != ClassLaptop {
		t.Fatalf("override laptop: got %s", got)
	}
}

func TestClassFromHardware(t *testing.T) {
	if got := classFromDarwin("Macmini9,1"); got != ClassMacMini {
		t.Fatalf("mac mini: %s", got)
	}
	if got := classFromDarwin("MacBookPro18,3"); got != ClassLaptop {
		t.Fatalf("macbook: %s", got)
	}
	if got := classFromDarwin("iMac21,1"); got != ClassPC {
		t.Fatalf("imac: %s", got)
	}
	if got := classFromLinux("9"); got != ClassLaptop {
		t.Fatalf("linux laptop chassis: %s", got)
	}
	if got := classFromLinux("3"); got != ClassPC {
		t.Fatalf("linux desktop chassis: %s", got)
	}
	if got := classFromLinux(""); got != ClassLinux {
		t.Fatalf("linux default: %s", got)
	}
	if got := classFromWindows("10"); got != ClassLaptop {
		t.Fatalf("windows laptop: %s", got)
	}
	if got := classFromWindows(""); got != ClassPC {
		t.Fatalf("windows default: %s", got)
	}
}

func TestDetectClassFromHWEnv(t *testing.T) {
	t.Setenv("GBR_DEVICE_CLASS", "")
	if runtime.GOOS == "darwin" {
		t.Setenv("GBR_HW_MODEL", "Macmini9,1")
		if got := DetectClass(); got != ClassMacMini {
			t.Fatalf("darwin hw.model: %s", got)
		}
	}
	if runtime.GOOS == "linux" {
		t.Setenv("GBR_CHASSIS_TYPE", "9")
		if got := DetectClass(); got != ClassLaptop {
			t.Fatalf("linux chassis: %s", got)
		}
	}
}

func TestLocalAndPhoneIdentity(t *testing.T) {
	t.Setenv("GBR_DEVICE_CLASS", "laptop")
	local := LocalIdentity("gbr-x", true)
	if local["id"] != "local" || local["class"] != ClassLaptop || local["impl"] != "gbr" {
		t.Fatalf("local %+v", local)
	}
	phone := PhoneIdentity("gbr-x")
	if phone["class"] != ClassPhone || phone["kind"] != "app" || phone["role"] != "spectator" {
		t.Fatalf("phone %+v", phone)
	}
}
