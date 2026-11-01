/**
 * Parse a process listing into gbr-agent rows.
 * Mac/Linux: pgrep -fl. Windows: tasklist CSV. Same check names either way.
 */
export function agentProcessLines(platform, stdout) {
  const lines = String(stdout || '')
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean);
  if (platform === 'win32') {
    return lines.filter((s) => /gbr-agent\.exe/i.test(s));
  }
  return lines.filter((s) => / run(\s|$)/.test(s));
}
