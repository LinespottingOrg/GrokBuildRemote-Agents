package main

import "testing"

func TestSettingsURL(t *testing.T) {
	if got := settingsURL(0); got != "http://127.0.0.1:8788/settings" {
		t.Fatalf("default port: %s", got)
	}
	if got := settingsURL(8790); got != "http://127.0.0.1:8790/settings" {
		t.Fatalf("custom port: %s", got)
	}
}

func TestTrayDisabled(t *testing.T) {
	t.Setenv("GBR_TRAY", "0")
	if !trayDisabled() {
		t.Fatal("GBR_TRAY=0 should skip the icon")
	}
	t.Setenv("GBR_TRAY", "")
	if trayDisabled() {
		t.Fatal("empty GBR_TRAY should show the icon")
	}
}
