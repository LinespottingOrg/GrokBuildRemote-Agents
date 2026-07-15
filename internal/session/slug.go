package session

import (
	"regexp"
	"strings"
	"unicode"
)

// SessionIDPattern is the protocol v1 session_id grammar:
// lowercase a-z0-9-, length 2–63, must start with alphanumeric.
const SessionIDPattern = `^[a-z0-9][a-z0-9-]{1,62}$`

// MaxSessionIDLen is the maximum allowed session_id length.
const MaxSessionIDLen = 63

var sessionIDRe = regexp.MustCompile(SessionIDPattern)

// ValidSessionID reports whether id matches protocol slug rules.
func ValidSessionID(id string) bool {
	if id == "" || len(id) > MaxSessionIDLen {
		return false
	}
	return sessionIDRe.MatchString(id)
}

// Slugify converts an arbitrary string into a protocol-valid session_id slug.
// Empty or unsalvageable input becomes "session".
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "session"
	}

	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || r == '\\':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				// drop non-ASCII letters/digits rather than mangling
				continue
			}
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session"
	}

	// must start with [a-z0-9]
	if out[0] == '-' {
		out = strings.TrimLeft(out, "-")
		if out == "" {
			return "session"
		}
	}

	if len(out) > MaxSessionIDLen {
		out = out[:MaxSessionIDLen]
		out = strings.TrimRight(out, "-")
	}

	// pattern requires at least 2 chars: [a-z0-9][a-z0-9-]{1,62}
	if len(out) < 2 {
		out = out + "0"
	}

	// collapse any residual double hyphens (defensive)
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if len(out) < 2 {
		return "session"
	}
	if len(out) > MaxSessionIDLen {
		out = strings.TrimRight(out[:MaxSessionIDLen], "-")
		if len(out) < 2 {
			return "session"
		}
	}

	if !ValidSessionID(out) {
		return "session"
	}
	return out
}
