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
            'Target machine: "local" (default), a registered id/name, or a unique class (linux|pc|laptop|mac_mini). Unknown names return 404 unknown_device — they do not fall back to local.',
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
  {
    name: 'gbr_open',
    description:
      'Start a Grok Build session (spawns `grok` or `grok --resume`) or attach an existing session_id, then take a lease so Grok bot and Claude Cowork do not type into the same window. Returns {session_id, opened|attached, lock, task?}. Call this instead of hoping a window already exists. Unknown-N sessions ARE attachable. The literal id "session" is the agent pseudo-session — never open/inject that. After open: gbr_inject → gbr_result (wait_ms) → judge → iterate or gbr_lock release.',
    inputSchema: {
      type: 'object',
      properties: {
        session_id: {
          type: 'string',
          description: 'Existing id to attach. Omit to spawn a new gbr-open-* / grok-* session.',
        },
        cwd: { type: 'string', description: 'Working directory for a newly spawned grok.' },
        resume: { type: 'string', description: 'Grok session UUID — runs `grok --resume <uuid>`.' },
        command: { type: 'string', description: 'grok (default) or shell.' },
        title: { type: 'string' },
        holder: {
          type: 'string',
          description: 'Who is driving: grok-bot | claude-coworker | hermes | openclaw | nemoclaw | phone. Default claude-coworker in this MCP.',
        },
        goal: { type: 'string', description: 'Task goal; stored on the lease and as a tasks.json row.' },
        ttl_s: { type: 'number', description: 'Lease TTL seconds. Default 900 (15 min).' },
        steal: { type: 'boolean', description: 'Take the window even if another holder has the lease.' },
        attach: { type: 'boolean', description: 'Force attach-only; do not spawn.' },
        device: { type: 'string' },
        notify_phone: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_result',
    description:
      'Harvest a structured result excerpt from a session. THIS is the feedback loop — do not wait for full Grok TUI scrollback. Pass wait_ms to block until a shell/Grok prompt appears or output goes quiet (idle_ms, default 2500). Returns {state: idle|busy|timeout, excerpt, prompt, lock, retry}. retry is true only on a real prompt. On timeout/splash/quiet: do NOT re-inject or re-open — that loop opens Grok approval cards. Judge from the excerpt or stop.',
    inputSchema: {
      type: 'object',
      properties: {
        session_id: { type: 'string', description: 'REQUIRED.' },
        command_id: { type: 'string' },
        wait_ms: { type: 'number', description: 'Wait for idle. 0 = single peek. Default 0. Max 180000.' },
        idle_ms: { type: 'number', description: 'Quiet period that counts as idle. Default 2500.' },
        excerpt_bytes: { type: 'number', description: 'Tail size. Default 4000.' },
        device: { type: 'string' },
      },
      required: ['session_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_lock',
    description:
      'Session lease so Grok bot and Claude Cowork never share a window. action=acquire|release|status (default acquire). 409 means the other client holds it — wait, steal, or pick another session_id. Always acquire before a long inject loop; release when the task is done.',
    inputSchema: {
      type: 'object',
      properties: {
        action: { type: 'string', enum: ['acquire', 'release', 'status'] },
        session_id: { type: 'string', description: 'Required for acquire/release. Optional for status (lists all).' },
        holder: { type: 'string', description: 'grok-bot | claude-coworker | phone' },
        goal: { type: 'string' },
        ttl_s: { type: 'number' },
        steal: { type: 'boolean' },
        force: { type: 'boolean', description: 'Release even if holder does not match.' },
        device: { type: 'string' },
      },
      additionalProperties: false,
    },
  },
  {
    name: 'gbr_tasks',
    description:
      'Read or write the durable task list (~/.gbr/tasks.json) used by the feedback loop. POST a goal after gbr_open; update status/last_excerpt/last_judge after each gbr_result. status: open|running|idle|done|failed.',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string' },
        session_id: { type: 'string' },
        holder: { type: 'string' },
        goal: { type: 'string' },
        status: { type: 'string' },
        iteration: { type: 'number' },
        last_excerpt: { type: 'string' },
        last_judge: { type: 'string' },
        command_id: { type: 'string' },
        upsert: { type: 'boolean', description: 'If true (or goal is set), write; otherwise list/get.' },
      },
      additionalProperties: false,
    },
  },
];
