/**
 * Is this roster entry something inject can actually reach?
 *
 * `session` is the agent's own pseudo-session. `unknown-N` entries with a
 * synthetic `/gbr-ui-*` cwd are windows the agent enumerated but could not
 * classify — the macOS 26.5 discovery bug. Neither is injectable, and treating
 * `unknown-0` as real made diagnose report a false all-clear in exactly the
 * broken state the project documents.
 */
export function isInjectable(s) {
  if (!s || !s.session_id) return false;
  if (s.session_id === 'session') return false;
  if (/^unknown(-|$)/.test(s.session_id)) return false;
  if (typeof s.cwd === 'string' && s.cwd.startsWith('/gbr-ui-')) return false;
  return true;
}
