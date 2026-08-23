package main

import (
	"strings"
	"testing"
)

func TestKeyFingerprintLengthAndStable(t *testing.T) {
	fp := keyFingerprint("mailbox-secret-example")
	if len(fp) != 12 {
		t.Fatalf("keyfp len=%d want 12", len(fp))
	}
	if keyFingerprint("mailbox-secret-example") != fp {
		t.Fatal("fingerprint not stable")
	}
	if keyFingerprint("other") == fp {
		t.Fatal("different keys must not share fingerprint")
	}
	if keyFingerprint("") != "" {
		t.Fatal("empty key has no fingerprint")
	}
}

func TestPairDeepLinkIdentDoesNotLeakKey(t *testing.T) {
	id := pairIdent{
		Code:       "A1B2C3D4",
		DeviceName: "studio",
		Version:    "0.6.0",
		SHA256:     "de7e065ef2cf6877b3b2cd04679a67b627f876337f529247e236204543e4062c",
		OS:         "darwin",
		Arch:       "arm64",
		Host:       "mac-mini",
		KeyFP:      "aabbccddeeff",
		MailboxID:  "gbr-a1b2c3d4",
	}
	link := pairDeepLinkIdent(id)
	if !strings.HasPrefix(link, "gbr://pair?") {
		t.Fatalf("prefix: %s", link)
	}
	for _, need := range []string{"code=A1B2C3D4", "ver=0.6.0", "sha256=de7e065e", "os=darwin", "keyfp=aabbccddeeff", "host=mac-mini"} {
		if !strings.Contains(link, need) {
			t.Fatalf("missing %q in %s", need, link)
		}
	}
	if strings.Contains(link, "mailbox-secret") || strings.Contains(strings.ToLower(link), "mailbox_key") {
		t.Fatalf("leaked key material: %s", link)
	}
}

func TestPairPageURLCarriesIdentity(t *testing.T) {
	id := pairIdent{Code: "ABCD2345", Version: "0.6.0", SHA256: "abc", KeyFP: "deadbeefcafe", MailboxID: "gbr-abcd2345"}
	u := pairPageURL(id)
	if !strings.HasPrefix(u, pairPageBase+"?") {
		t.Fatalf("url: %s", u)
	}
	if !strings.Contains(u, "code=ABCD2345") || !strings.Contains(u, "sha256=abc") || !strings.Contains(u, "keyfp=deadbeefcafe") {
		t.Fatalf("missing identity: %s", u)
	}
}
