import type { Extension, ExtensionContext, ToolDefinition } from '@aiderdesk/extensions';

const BOT = 'http://127.0.0.1:8788';

async function getJson(path: string): Promise<string> {
  const r = await fetch(BOT + path);
  const t = await r.text();
  if (!r.ok) throw new Error(`gbr-agent ${r.status} ${path}: ${t.slice(0, 400)}`);
  return t;
}

export default class GbrPairExtension implements Extension {
  static metadata = {
    name: 'Build Remote Agent',
    version: '0.6.1',
    description:
      'Talk to local gbr-agent Bot API. The phone lists terminal windows on this PC, not this Electron window.',
    author: 'Linespotting AB',
    capabilities: ['tools', 'network'],
  };

  async onLoad(context: ExtensionContext) {
    context.log('GBR extension: GET ' + BOT + '/health after gbr-agent run', 'info');
  }

  getTools(_context: ExtensionContext): ToolDefinition[] {
    return [
      {
        name: 'gbr-status',
        description:
          'GET local gbr-agent /health and /v1/sessions. Roster is terminal windows, not AiderDesk.',
        async execute() {
          const health = await getJson('/health');
          const sessions = await getJson('/v1/sessions');
          return { content: [{ type: 'text', text: health + '\n' + sessions }] };
        },
      },
      {
        name: 'gbr-inject',
        description:
          'POST /v1/inject into a discovered terminal session_id (from gbr-status). Machine-wide mailbox.',
        async execute(input: { session_id?: string; text?: string }) {
          const session_id = String(input?.session_id || '');
          const text = String(input?.text || '');
          if (!session_id || !text) {
            return {
              content: [{ type: 'text', text: 'Need session_id and text' }],
              isError: true,
            };
          }
          const r = await fetch(BOT + '/v1/inject', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ session_id, text, submit: true }),
          });
          const body = await r.text();
          return { content: [{ type: 'text', text: body }] };
        },
      },
    ];
  }
}
