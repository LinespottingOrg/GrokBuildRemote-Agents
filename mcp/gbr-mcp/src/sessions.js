/**
 * Is this roster entry something inject can actually reach?
 *
 * EMPIRICALLY DETERMINED — 2026-08-18, agent v0.5.3 on macOS 26.5.2:
 *
 *   session_id "session"      -> inject HANGS (agent's own pseudo-session)
 *   session_id "totally-fake" -> inject HANGS (no such window)
 *   session_id "unknown-0"    -> inject WORKS. The command executed and its
 *                                output came back through /v1/output, ending
 *                                at a live `bash-3.2$` prompt.
 *
 * So `unknown-N` is NOT a dead entry. It is a real discovered window that the
 * classifier failed to *name* — the title and shell read "unknown", but the
 * session is live and injectable.
 *
 * An earlier version of this file excluded `unknown-*` on the assumption that
 * an unnamed session was unusable. That assumption came from a code review
 * reasoning over the project's own bug report rather than from testing, and it
 * produced a FALSE NEGATIVE: diagnose reported "no real sessions" on a machine
 * where inject demonstrably worked. Do not re-add that exclusion without
 * re-running the probe above.
 */
export function isInjectable(s) {
  if (!s || !s.session_id) return false;
  // The agent's own pseudo-session. Injecting into it hangs until timeout.
  if (s.session_id === 'session') return false;
  return true;
}

/** Sessions the classifier could not name. Injectable, but the title is useless
 *  and the underlying naming bug is worth surfacing to the caller. */
export function isUnnamed(s) {
  return Boolean(
    s && (/^unknown(-|$)/.test(s.session_id || '') ||
          s.title === 'unknown' ||
          (typeof s.cwd === 'string' && s.cwd.startsWith('/gbr-ui-'))),
  );
}
