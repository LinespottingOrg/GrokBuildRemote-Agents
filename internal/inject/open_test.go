package inject

import "testing"

func TestSanitizeOpenID(t *testing.T) {
	if sanitizeOpenID("session") != "" {
		t.Fatal("pseudo session refused")
	}
	if sanitizeOpenID("Grok-Build-40A22") != "grok-build-40a22" {
		t.Fatal(sanitizeOpenID("Grok-Build-40A22"))
	}
	if sanitizeOpenID("../etc") != "etc" && sanitizeOpenID("../etc") != "" {
		// slashes stripped; leftover must be a slug or empty
		t.Log(sanitizeOpenID("../etc"))
	}
}

func TestNewOpenSessionIDResume(t *testing.T) {
	id := newOpenSessionID("40a22abc-1111-2222-3333-444444444444")
	if id != "grok-40a22abc" {
		t.Fatalf("got %q", id)
	}
}

func TestLookGrokDoesNotPanic(t *testing.T) {
	_, _ = LookGrok()
}
