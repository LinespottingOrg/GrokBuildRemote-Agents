/**
 * Self-diagnosis. Exposed both as the `gbr_diagnose` MCP tool and as
 * `gbr-mcp --diagnose` on the CLI.
 *
 * Every check returns { name, ok, detail, fix } so an AI agent can read the
 * result and repair the environment without a human in the loop.
 */

import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { existsSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { homedir, platform, release } from 'node:os';
import { GbrClient } from './client.js';
import { isInjectable, isUnnamed } from './sessions.js';
import { logMeta, log } from './logger.js';

const exec = promisify(execFile);

async function sh(cmd, args = []) {
  try {
    const { stdout } = await exec(cmd, args, { timeout: 10000 });
    return { ok: true, out: stdout.trim() };
  } catch (err) {
    return { ok: false, out: (err.stdout || '').trim(), err: err.message };
  }
}

const MIN_AGENT = [0, 5, 3];
const CHAIN_AGENT = [0, 5, 4];

function parseVersion(s = '') {
  const m = s.match(/v?(\d+)\.(\d+)\.(\d+)/);
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
}

function gte(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a[i] > b[i]) return true;
    if (a[i] < b[i]) return false;
  }
  return true;
}

export async function diagnose(clientOpts = {}, cid) {
  const checks = [];
  const add = (name, ok, detail, fix = null) => checks.push({ name, ok, detail, fix });

  // 1. Node
  const nodeMajor = Number(process.versions.node.split('.')[0]);
  add(
    'node_version',
    nodeMajor >= 20,
    `node ${process.version}`,
    nodeMajor >= 20 ? null : 'Install Node 20+ (brew install node / nvm install 20).',
  );

  // 2. Platform
  add('platform', true, `${platform()} ${release()}`);

  // 3. gbr-agent on PATH
  const which = await sh(process.platform === 'win32' ? 'where' : 'which', ['gbr-agent']);
  add(
    'gbr_agent_on_path',
    which.ok,
    which.ok ? which.out : 'gbr-agent not found on PATH',
    which.ok
      ? null
      : 'The installer puts it in ~/.local/bin but does not reload your shell. Run: export PATH="$HOME/.local/bin:$PATH" (and open a new terminal — the installer already appended it to ~/.zshrc).',
  );

  // 4. Agent version >= 0.5.3
  let agentVersion = null;
  if (which.ok) {
    const v = await sh('gbr-agent', ['version']);
    agentVersion = parseVersion(v.out);
    add(
      'gbr_agent_version',
      Boolean(agentVersion && gte(agentVersion, MIN_AGENT)),
      v.out || 'unknown',
      agentVersion && gte(agentVersion, MIN_AGENT)
        ? null
        : 'Bot API requires agent >= 0.5.3. Upgrade: curl -fsSL https://grokbuildremote.com/install.sh | bash — then unpair and pair again.',
    );
  }

  // 5. Agent process alive.
  // NOTE: match on the binary only, never on "gbr-agent run" — the process is
  // commonly started as `gbr-agent -log=info run`, so flags sit between the
  // binary and the subcommand and a literal "gbr-agent run" pattern misses it.
  if (process.platform !== 'win32') {
    const pg = await sh('pgrep', ['-fl', 'gbr-agent']);
    const lines = (pg.out || '')
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s && / run(\s|$)/.test(s));
    add(
      'gbr_agent_running',
      lines.length > 0,
      lines.join(' | ') || 'no running gbr-agent found',
      lines.length
        ? null
        : 'Start it: gbr-agent run  (or persist it: gbr-agent service install)',
    );
    if (lines.length > 1) {
      add(
        'single_agent',
        false,
        `${lines.length} agents running — they will fight over port 8788`,
        'Kill the extras: pkill -f "gbr-agent" then start exactly one with `gbr-agent run`.',
      );
    }
  }

  // 6. Bot API reachable + shape sane
  const client = new GbrClient(clientOpts);
  add('client_mode', true, JSON.stringify(client.describe()));
  try {
    const d = await client.discovery(cid);
    // d.version already carries its own leading "v" — do not add another.
    add('bot_api_reachable', true, `${d.version} on ${d.bind}:${d.port}, mailbox ${d.mailbox_id}`);
    add(
      'bot_api_require_key',
      true,
      `require_key=${d.require_key} (set GBR_BOT_REQUIRE_KEY=1 to harden)`,
    );
    const ver = parseVersion(String(d.version || ''));
    const eps = d.endpoints || {};
    const hasChain = Boolean(eps.open && eps.result && eps.lock);
    add(
      'chain_endpoints',
      hasChain,
      hasChain
        ? `open/result/lock present (agent ${d.version})`
        : `agent ${d.version || 'unknown'} has no open/result/lock — Grok bot / Cowork loop is fire-and-forget only`,
      hasChain
        ? null
        : 'Upgrade the agent to 0.5.4+: curl -fsSL https://grokbuildremote.com/install.sh | bash',
    );
    if (ver && !gte(ver, CHAIN_AGENT)) {
      add(
        'chain_agent_version',
        false,
        `agent ${d.version} < 0.5.4`,
        '0.5.4 adds session open, idle-wait result, and a lease so Grok bot and Claude Cowork do not share a window.',
      );
    }
  } catch (err) {
    add('bot_api_reachable', false, err.message, err.hint);
  }

  // 7. Sessions present
  try {
    const s = await client.sessions(undefined, cid);
    const list = s.sessions || [];
    const realSessions = list.filter(isInjectable);
    add(
      'sessions_available',
      list.length > 0,
      `${list.length} session(s): ${list.map((x) => `${x.session_id}(${x.title})`).join(', ') || 'none'}`,
      list.length ? null : 'Call gbr_open (spawns grok) or open a Grok Build window, then re-run.',
    );
    const unnamed = realSessions.filter(isUnnamed);
    add(
      'injectable_session',
      realSessions.length > 0,
      realSessions.length
        ? `${realSessions.length} injectable session(s)` +
          (unnamed.length ? ` — ${unnamed.length} unnamed (classifier could not title them; still injectable)` : '')
        : 'only the agent pseudo-session "session" is present',
      realSessions.length
        ? (unnamed.length
            ? 'Sessions are injectable but unnamed — the agent enumerated the windows without classifying them. Check: grep \'"hop":"agent.discover"\' ~/.gbr/logs/agent-$(date +%F).jsonl | tail -1'
            : null)
        : 'IMPORTANT: injecting into the pseudo-session "session" hangs until timeout. Open a terminal or Grok Build window first.',
    );
  } catch (err) {
    add('sessions_available', false, err.message, err.hint);
  }

  // 8. Devices / fleet
  try {
    const dv = await client.devices(cid);
    const devices = dv.devices || [];
    add(
      'devices',
      devices.length > 0,
      devices.map((d) => `${d.id}(${d.kind}/${d.os}${d.has_key ? '' : ' NO-KEY'})`).join(', '),
      devices.some((d) => !d.has_key)
        ? 'A device is missing its key — re-add it with `gbr-agent fleet add -name X -mailbox gbr-XXXX -key KEY`.'
        : null,
    );
  } catch (err) {
    add('devices', false, err.message, err.hint);
  }

  // 9. Logging writable
  add(
    'logging',
    Boolean(logMeta.file) && !logMeta.dirError,
    logMeta.dirError
      ? `log dir unusable: ${logMeta.dirError}`
      : `level=${logMeta.level} file=${logMeta.file}`,
    logMeta.dirError ? 'Set GBR_MCP_LOG_DIR to a writable path.' : null,
  );

  // 10. Agent's own trace log
  const agentLogDir = join(homedir(), '.gbr', 'logs');
  let recent = [];
  try {
    if (existsSync(agentLogDir)) {
      recent = readdirSync(agentLogDir)
        .filter((f) => f.endsWith('.jsonl'))
        .map((f) => ({ f, m: statSync(join(agentLogDir, f)).mtimeMs }))
        .sort((a, b) => b.m - a.m)
        .slice(0, 3)
        .map((x) => x.f);
    }
  } catch { /* ignore */ }
  add('agent_logs', recent.length > 0, recent.join(', ') || 'no agent jsonl logs found');

  const failed = checks.filter((c) => !c.ok);
  const result = {
    ok: failed.length === 0,
    summary: failed.length
      ? `${failed.length} of ${checks.length} checks failed: ${failed.map((c) => c.name).join(', ')}`
      : `all ${checks.length} checks passed`,
    checks,
    nextActions: failed.map((c) => c.fix).filter(Boolean),
  };
  log.info('diagnose complete', { ok: result.ok, failed: failed.length });
  return result;
}
