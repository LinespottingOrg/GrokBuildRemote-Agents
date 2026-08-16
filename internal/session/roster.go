package session

// MaxSessions is the soft advertised-roster cap.
// Extra windows are dropped (caller logs); never crash or hard-fail.
const MaxSessions = 255

// ClipRoster keeps at most MaxSessions items. Caller must sort first
// (Grok Build first). Extra items are dropped; never panics.
func ClipRoster[T any](items []T) (kept []T, dropped int) {
	if len(items) <= MaxSessions {
		return items, 0
	}
	return items[:MaxSessions], len(items) - MaxSessions
}
