package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// pairIdent is shown on the QR page and encoded into gbr://pair (additive gbr/1 fields).
// The mailbox key itself is never put in the QR — only a 12-hex fingerprint.
type pairIdent struct {
	Code       string
	DeviceName string
	Version    string
	SHA256     string
	OS         string
	Arch       string
	Host       string
	KeyFP      string
	MailboxID  string
}

func runningBinarySHA256() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		p = rp
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func keyFingerprint(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func hostnameShort() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if len(h) > 32 {
		h = h[:32]
	}
	return h
}

func collectPairIdent(code, deviceName, mailboxID, mailboxKey string) pairIdent {
	return pairIdent{
		Code:       code,
		DeviceName: deviceName,
		Version:    version,
		SHA256:     runningBinarySHA256(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Host:       hostnameShort(),
		KeyFP:      keyFingerprint(mailboxKey),
		MailboxID:  mailboxID,
	}
}

func pairDeepLinkIdent(id pairIdent) string {
	q := url.Values{}
	q.Set("v", "1")
	q.Set("code", id.Code)
	if id.DeviceName != "" {
		q.Set("device_name", id.DeviceName)
	}
	if id.Version != "" {
		q.Set("ver", id.Version)
	}
	if id.SHA256 != "" {
		q.Set("sha256", id.SHA256)
	}
	if id.OS != "" {
		q.Set("os", id.OS)
	}
	if id.Arch != "" {
		q.Set("arch", id.Arch)
	}
	if id.Host != "" {
		q.Set("host", id.Host)
	}
	if id.KeyFP != "" {
		q.Set("keyfp", id.KeyFP)
	}
	if id.MailboxID != "" {
		q.Set("mb", id.MailboxID)
	}
	return "gbr://pair?" + q.Encode()
}

func formatPairIdent(id pairIdent) string {
	var b strings.Builder
	b.WriteString("identity (scan this — pair code + key fingerprint, not the mailbox key):\n")
	b.WriteString("  pair code:        " + id.Code + "\n")
	b.WriteString("  mailbox:          " + id.MailboxID + "\n")
	if id.KeyFP != "" {
		b.WriteString("  key fingerprint:  " + id.KeyFP + "  (sha256(mailbox_key)[:12]; key is not in the QR)\n")
	}
	if id.Version != "" {
		b.WriteString("  gbr-agent:        " + id.Version + " (" + id.OS + "/" + id.Arch + ")\n")
	}
	if id.SHA256 != "" {
		b.WriteString("  binary sha256:    " + id.SHA256 + "\n")
		b.WriteString("                    match against GitHub Release SHA256SUMS / grokbuildremote.com/#download\n")
	}
	if id.Host != "" {
		b.WriteString("  host:             " + id.Host + "\n")
	}
	if id.DeviceName != "" {
		b.WriteString("  device name:      " + id.DeviceName + "\n")
	}
	return b.String()
}

func pairPageURL(id pairIdent) string {
	q := url.Values{}
	q.Set("code", id.Code)
	if id.DeviceName != "" {
		q.Set("name", id.DeviceName)
	}
	if id.Version != "" {
		q.Set("ver", id.Version)
	}
	if id.SHA256 != "" {
		q.Set("sha256", id.SHA256)
	}
	if id.OS != "" {
		q.Set("os", id.OS)
	}
	if id.Arch != "" {
		q.Set("arch", id.Arch)
	}
	if id.Host != "" {
		q.Set("host", id.Host)
	}
	if id.KeyFP != "" {
		q.Set("keyfp", id.KeyFP)
	}
	if id.MailboxID != "" {
		q.Set("mb", id.MailboxID)
	}
	return pairPageBase + "?" + q.Encode()
}


