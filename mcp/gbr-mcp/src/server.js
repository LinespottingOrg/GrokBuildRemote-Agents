/**
 * gbr-mcp — MCP server over the Grok Build Remote agent Bot API.
 *
 * Transport: stdio. Nothing may ever be written to stdout except MCP frames.
 * All logging goes to stderr + ~/.gbr/logs/gbr-mcp-<date>.jsonl.
 */

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { GbrClient, GbrError } from './client.js';
import { diagnose } from './diagnose.js';
import { log, withCorrelation, clampBody, redact, registerSecret, logMeta } from './logger.js';
import { TOOLS } from './tools.js';
import { isInjectable, isUnnamed } from './sessions.js';

const VERSION = JSON.parse(
  readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', 'package.json'), 'utf8'),
).version;

/** Bound a caller-supplied number. Model-generated args are untrusted:
 *  poll_ms:0 produced 329 req/s against the agent in testing. */
function clampInt(v, min, max, fallback) {
  const n = Number(v);
  return Number.isFinite(n) ? Math.min(max, Math.max(min, Math.trunc(n))) : fallback;
}

export function createServer(clientOpts = {}) {
  const client = new GbrClient(clientOpts);
  const server = new Server(
    { name: 'gbr-mcp', version: VERSION },
    { capabilities: { tools: {} } },
  );

  log.info('server constructed', { client: client.describe(), tools: TOOLS.length });

  server.setRequestHandler(ListToolsRequestSchema, async () => {
    log.debug('tools/list');
    return { tools: TOOLS };
  });

  server.setRequestHandler(CallToolRequestSchema, async (req) => {
    const { name, arguments: args = {} } = req.params;
    const l = withCorrelation();
    const started = Date.now();
    l.info('tool call', { tool: name, args: clampBody(args) });

    try {
      const result = await dispatch(client, name, args, l.cid);
      l.info('tool ok', { tool: name, elapsedMs: Date.now() - started });
      // Redact on the way out too — an agent 4xx can echo the record we posted.
      return {
        content: [{ type: 'text', text: JSON.stringify(redact(result), null, 2) }],
      };
    } catch (err) {
      const payload =
        err instanceof GbrError
          ? err.toPayload()
          : { error: err.message, code: 'GBR_UNEXPECTED' };

      payload.correlation_id = l.cid;
      payload.log_file = logMeta.file ?? '(log file unavailable)';
      if (name !== 'gbr_diagnose') {
        payload.troubleshooting = 'Run the gbr_diagnose tool for a full environment check.';
      }

      // Stack stays in the log — it discloses absolute paths incl. the username.
      l.error('tool failed', {
        tool: name,
        elapsedMs: Date.now() - started,
        code: payload.code,
        error: payload.error,
        stack: err.stack?.split('\n').slice(0, 6),
      });

      return {
        isError: true,
        content: [{ type: 'text', text: JSON.stringify(redact(payload), null, 2) }],
      };
    }
  });

  return { server, client };
}

async function dispatch(client, name, args, cid) {
  switch (name) {
    case 'gbr_diagnose':
      // Pass the FULL options so diagnose probes the same target the tools use.
      // Previously this passed {} either way, so an embedder configuring relay
      // mode programmatically got a diagnose that silently checked loopback.
      return diagnose(client.options(), cid);

    case 'gbr_status':
      return client.status(cid);

    case 'gbr_sessions': {
      const res = await client.sessions(args.device, cid);
      const sessions = res.sessions || [];
      // Filter rather than length-check: a roster of [pseudo, stale] would
      // otherwise produce no warning at all.
      const real = sessions.filter(isInjectable);
      const unnamed = real.filter(isUnnamed);
      let note;
      if (real.length === 0) {
        note = 'No injectable sessions — only the agent pseudo-session "session" is present. Injecting into it hangs until timeout. Open a terminal or Grok Build window first.';
      } else if (unnamed.length === real.length) {
        note = `All ${real.length} session(s) are unnamed (title "unknown"). They ARE injectable — verified — but the agent could not classify them, so titles are useless. This is the known grok_build=0 classifier bug; check: grep '"hop":"agent.discover"' ~/.gbr/logs/agent-$(date +%F).jsonl | tail -1`;
      }
      return { ...res, _note: note };
    }

    case 'gbr_devices':
      return client.devices(cid);

    case 'gbr_inject': {
      if (!args.text) throw new GbrError('text is required', { code: 'GBR_BAD_ARGS' });
      if (!args.session_id) {
        throw new GbrError('session_id is required — the agent refuses empty session ids', {
          code: 'GBR_BAD_ARGS',
          hint: 'Call gbr_sessions first and pass a real session_id.',
        });
      }
      const injected = await client.inject(
        {
          device: args.device,
          session_id: args.session_id,
          text: args.text,
          submit: args.submit !== false,
          notify_phone: args.notify_phone,
          command_id: args.command_id,
        },
        cid,
        clampInt(args.timeout_ms, 1000, 600000, undefined),
      );
      if (injected == null) {
        throw new GbrError('inject returned an empty body', {
          code: 'GBR_BAD_RESPONSE',
          hint: 'The agent accepted the request but said nothing. Check `gbr-agent status`.',
        });
      }
      return injected;
    }

    case 'gbr_output':
      return client.output(
        {
          session_id: args.session_id,
          command_id: args.command_id,
          after: args.after,
          limit: clampInt(args.limit, 1, 1000, 100),
        },
        cid,
      );

    case 'gbr_inject_and_wait': {
      if (!args.session_id || !args.text) {
        throw new GbrError('session_id and text are both required', { code: 'GBR_BAD_ARGS' });
      }
      const waitMs = clampInt(args.wait_ms, 1000, 600000, 60000);
      const pollMs = clampInt(args.poll_ms, 250, 30000, 1500);
      const MAX_ITEMS = 2000;

      const injected = await client.inject(
        {
          device: args.device,
          session_id: args.session_id,
          text: args.text,
          submit: true,
          notify_phone: args.notify_phone,
          wait_idle: true,
          wait_ms: waitMs,
        },
        cid,
        clampInt(args.timeout_ms, 1000, 600000, waitMs + 15000),
      );
      if (injected?.result) {
        return {
          ...injected,
          completed: Boolean(injected.result.idle),
          timed_out: !injected.result.idle,
          result: injected.result,
          _note: injected.result.idle && injected.result.retry
            ? 'Idle at a prompt. Judge the excerpt and iterate or close.'
            : `Not ready to re-inject (state=${injected.result.state}, retry=${injected.result.retry === true}). Do not re-issue this command_id. Poll gbr_result or stop.`,
        };
      }
      const commandId = injected?.command_id;
      if (!commandId) {
        throw new GbrError('inject returned no command_id — cannot poll for output', {
          code: 'GBR_BAD_RESPONSE',
          body: injected,
          hint: 'Use gbr_inject then gbr_output manually.',
        });
      }

      const deadline = Date.now() + waitMs;
      const collected = [];
      const seen = new Set();
      let after;            // poll cursor — advances past the newest item seen
      let resumeAfter;      // cursor reported to the caller — kept items only
      let done = false;
      let truncated = false;

      // NOTE: /v1/output has no implicit cursor. Without advancing `after`, every
      // poll re-reads the whole buffer and items are collected repeatedly — a
      // 3x duplication factor was measured over 5 polls. The `seen` set makes
      // this correct even if `after` turns out to be inclusive.
      while (Date.now() < deadline && !done) {
        await new Promise((r) => setTimeout(r, pollMs));
        const out = await client.output({ command_id: commandId, after, limit: 200 }, cid);
        for (const item of out.items || []) {
          const k = `${item.ts}|${item.stream}|${item.chunk}`;
          if (seen.has(k)) continue;
          seen.add(k);
          if (collected.length < MAX_ITEMS) {
            collected.push(item);
            // Only advance the RESUME cursor for items we actually kept —
            // otherwise the _note tells the caller to page from past the
            // dropped items and they are lost for good.
            if (item.ts && (resumeAfter === undefined || item.ts > resumeAfter)) resumeAfter = item.ts;
          } else {
            truncated = true;
          }
          if (item.ts && (after === undefined || item.ts > after)) after = item.ts;
          if (item.eof) done = true;
        }
      }

      return {
        ...injected,
        completed: done,
        timed_out: !done,
        item_count: collected.length,
        truncated,
        items: collected,
        _note: done
          ? (truncated
              ? `Output capped at ${MAX_ITEMS} items — page the rest with gbr_output after=${resumeAfter ?? ''}.`
              : undefined)
          : `No EOF within ${waitMs}ms — output may still be streaming. Poll gbr_output with command_id=${commandId} and after=${resumeAfter ?? ''}.`,
      };
    }

    case 'gbr_fleet_add': {
      for (const k of ['name', 'mailbox_id', 'key']) {
        if (!args[k]) throw new GbrError(`${k} is required`, { code: 'GBR_BAD_ARGS' });
      }
      // Register before anything can log it. Without this a non-hex key such as
      // `sk_live_...` was written verbatim to the JSONL — verified and fixed.
      registerSecret(args.key);
      return client.addDevice(
        { name: args.name, mailbox_id: args.mailbox_id, key: args.key, os: args.os },
        cid,
      );
    }

    case 'gbr_discovery':
      return client.discovery(cid);

    case 'gbr_open': {
      const holder = args.holder || 'claude-coworker';
      const opened = await client.open(
        {
          session_id: args.session_id,
          cwd: args.cwd,
          resume: args.resume,
          command: args.command,
          title: args.title,
          holder,
          goal: args.goal,
          ttl_s: args.ttl_s,
          steal: args.steal,
          attach: args.attach,
          device: args.device,
          notify_phone: args.notify_phone,
        },
        cid,
        clampInt(args.timeout_ms, 1000, 180000, 60000),
      );
      return {
        ...opened,
        _next: opened?.session_id
          ? `Inject with gbr_inject session_id=${opened.session_id}, then gbr_result wait_ms=60000. Release with gbr_lock action=release when done.`
          : undefined,
      };
    }

    case 'gbr_result': {
      if (!args.session_id) {
        throw new GbrError('session_id is required', { code: 'GBR_BAD_ARGS' });
      }
      return client.result(
        {
          session_id: args.session_id,
          command_id: args.command_id,
          wait_ms: args.wait_ms,
          idle_ms: args.idle_ms,
          excerpt_bytes: args.excerpt_bytes,
          device: args.device,
        },
        cid,
      );
    }

    case 'gbr_lock': {
      const action = String(args.action || 'acquire').toLowerCase();
      const holder = args.holder || 'claude-coworker';
      if (action === 'status') {
        return client.lockStatus(args.session_id, cid);
      }
      if (action === 'release') {
        if (!args.session_id) {
          throw new GbrError('session_id is required to release a lock', { code: 'GBR_BAD_ARGS' });
        }
        return client.lockRelease({ session_id: args.session_id, holder, force: args.force }, cid);
      }
      if (!args.session_id) {
        throw new GbrError('session_id is required to acquire a lock', { code: 'GBR_BAD_ARGS' });
      }
      return client.lockAcquire(
        {
          session_id: args.session_id,
          holder,
          goal: args.goal,
          ttl_s: args.ttl_s,
          steal: args.steal,
          device: args.device,
        },
        cid,
      );
    }

    case 'gbr_tasks': {
      const write = args.upsert || args.goal || args.status || args.last_excerpt || args.last_judge;
      if (write) {
        return client.upsertTask(
          {
            id: args.id,
            session_id: args.session_id,
            holder: args.holder || 'claude-coworker',
            goal: args.goal,
            status: args.status,
            iteration: args.iteration,
            last_excerpt: args.last_excerpt,
            last_judge: args.last_judge,
            command_id: args.command_id,
          },
          cid,
        );
      }
      return client.tasks({ session_id: args.session_id, id: args.id }, cid);
    }

    default:
      throw new GbrError(`Unknown tool: ${name}`, {
        code: 'GBR_UNKNOWN_TOOL',
        hint: `Known tools: ${TOOLS.map((t) => t.name).join(', ')}`,
      });
  }
}

export async function main() {
  const { server } = createServer();
  const transport = new StdioServerTransport();

  process.on('uncaughtException', (err) => {
    // Exit rather than limp on with a possibly corrupt transport — the client
    // will restart us, which is strictly better than silently never replying.
    log.error('uncaughtException — exiting so the client can restart us', {
      err: err.message, stack: err.stack?.split('\n').slice(0, 6),
    });
    process.exit(1);
  });
  process.on('unhandledRejection', (err) => {
    log.error('unhandledRejection', { err: String(err) });
  });
  for (const sig of ['SIGINT', 'SIGTERM']) {
    process.on(sig, () => {
      log.info('shutting down', { sig });
      process.exit(0);
    });
  }

  await server.connect(transport);
  log.info('gbr-mcp connected on stdio');
}
