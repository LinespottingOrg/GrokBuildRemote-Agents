/**
 * Tool definitions. Descriptions are written for an LLM caller: each one says
 * when to use it, what it returns, and the failure mode it is most likely to hit.
 */

export const TOOLS = [
  {
    name: 'gbr_diagnose',
    description:
      'Run a full environment check of the Grok Build Remote stack: Node version, gbr-agent on PATH, agent version >= 0.5.3, agent process alive, Bot API reachable, sessions present, fleet devices, and log writability. Returns per-check {ok, detail, fix}. ALWAYS call this first when any other gbr_* tool fails — it returns concrete repair commands.',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
  },
  {
    name: 'gbr_status',
    description:
      'Health snapshot of the agent: agent_online, agent_version, os, mailbox_id, uptime_s, session_count and the live session roster. Cheap; safe to poll.',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
  },
  {
    name: 'gbr_sessions',
    description:
      'List live terminal / Grok Build sessions with their session_id, title, cwd, shell and os. Call this BEFORE gbr_inject — inject requires a real session_id and the agent refuses empty ones. Note: a session literally named "session" is the agent pseudo-session, not a real window; injecting into it will hang.',
    inputSchema: {
      type: 'object',
      properties: {
        device: {
          type: 'string',
          description: 'Fleet device to query. Omit or use "local" for this machine.',
        },
      },
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_devices',
    description:
      'List machines this hub can dispatch to: id, kind (local|remote), name, os, and whether a mailbox key is held. Remotes must first be registered with `gbr-agent fleet add` or gbr_fleet_add.',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
  },
  {
    name: 'gbr_inject',
    description:
      'Type a prompt into a Grok Build / CLI session and optionally submit it. Fire-and-forget: returns {ok, command_id, queued} immediately. Use gbr_output with the returned command_id to read results, or use gbr_inject_and_wait to do both. WARNING: this call blocks server-side if no live window is attached to session_id — it will time out rather than hang forever.',
    inputSchema: {
      type: 'object',
      properties: {
        session_id: {
          type: 'string',
          description: 'REQUIRED. A real session id from gbr_sessions.',
        },
        text: { type: 'string', description: 'REQUIRED. The prompt to type.' },
        device: {
          type: 'string',
          description:
            'Target machine: "local" (default) or a registered fleet name. Unknown names silently fall back to local — the response carries a _warning if that happened.',
        },
        submit: {
          type: 'boolean',
          description: 'Press enter after typing. Default true. Set false to stage text for a human.',
        },
        notify_phone: {
          type: 'boolean',
          description: 'Show a status line on the paired phone. Default true.',
        },
        command_id: { type: 'string', description: 'Optional client-supplied uuid for correlation.' },
        timeout_ms: { type: 'number', description: 'Override the HTTP timeout for this call.' },
      },
      required: ['session_id', 'text'],
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_inject_and_wait',
    description:
      'Inject a prompt then poll for output until EOF or wait_ms elapses. This is the tool to use for "run X on machine Y and tell me what happened". Returns the injected command plus every output item collected, and flags timed_out if no EOF arrived.',
    inputSchema: {
      type: 'object',
      properties: {
        session_id: { type: 'string', description: 'REQUIRED. From gbr_sessions.' },
        text: { type: 'string', description: 'REQUIRED. The prompt to run.' },
        device: { type: 'string', description: 'Target machine. Default local.' },
        wait_ms: { type: 'number', description: 'Total time to wait for EOF. Default 60000.' },
        poll_ms: { type: 'number', description: 'Poll interval. Default 1500.' },
        notify_phone: { type: 'boolean' },
        timeout_ms: { type: 'number', description: 'HTTP timeout for the inject leg.' },
      },
      required: ['session_id', 'text'],
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_output',
    description:
      'Read buffered output. Filter by command_id (results of one inject) or session_id (everything in a window). Items are {ts, session_id, command_id, stream, chunk, eof, reason, method}. An empty items[] usually means the command has not produced output yet — poll again.',
    inputSchema: {
      type: 'object',
      properties: {
        session_id: { type: 'string' },
        command_id: { type: 'string' },
        after: { type: 'string', description: 'Only items after this timestamp/cursor.' },
        limit: { type: 'number', description: 'Max items. Default 100.' },
      },
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_fleet_add',
    description:
      'Register a remote Mac/Linux/Windows machine so it can be targeted by device name. That machine must already be paired (`gbr-agent pair`); copy its mailbox id and key from its own `gbr-agent status`. The key is a credential — never log or echo it.',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: 'Short device name, e.g. studio-linux.' },
        mailbox_id: { type: 'string', description: 'gbr-XXXX from that machine.' },
        key: { type: 'string', description: 'That machine mailbox key.' },
        os: { type: 'string', enum: ['darwin', 'linux', 'windows'] },
      },
      required: ['name', 'mailbox_id', 'key'],
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_discovery',
    description:
      'Raw Bot API discovery document: version, port, bind address, auth modes, require_key, mailbox_id, relay_bot URL and the endpoint map. Useful for confirming which agent build you are talking to.',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
  },
];
