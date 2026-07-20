# gbr-relay

The durable mailbox relay for Build Remote Agent. Phone and PC never connect to
each other — both push and poll envelopes against a mailbox on this Worker, so
the pair works through NAT, VPNs and firewalls with no open ports on either side.

Lives in this repo rather than its own because the relay and the agent share one
wire protocol and change together. Every protocol fix so far touched both sides
in the same commit; splitting them would mean a window where `main` in one repo
does not work against `main` in the other.

## Auth

**A mailbox id is not a credential.** It is `gbr-` + the lowercased 8-char
pairing code, and that derivation is public (see `internal/relay` in the agent).
So `/pair` issues a 64-char per-mailbox secret, idempotently — the agent and the
phone both receive the same value by pairing with the same code — and it is sent
as `X-GBR-Key`.

| Endpoint | Key required | Why |
|----------|--------------|-----|
| `POST /v1/mb/:id/push` | yes | An inject becomes keystrokes in a terminal |
| `GET /v1/mb/:id/poll` | yes | Returns terminal output |
| `POST /v1/mb/:id/ack` | yes | Unauthenticated it is a queue DoS |
| `POST /v1/mb/:id/pair` | **no** | It is where the key comes from — throttled instead, 12/hour/mailbox |
| `*/trace` | no | Observability only, no command authority |

A mailbox that has never been paired refuses all traffic. Allowing keyless
mailboxes left a pre-pair injection window: guess a code, queue an inject into a
mailbox that does not exist yet, and the agent executes it on its first poll
after the victim pairs.

`GBR_AUTH_MODE=warn` issues and verifies keys and traces `relay.auth_reject`
without ever blocking — use it only for a staged rollout with an installed base
that still needs to catch up.

## Storage

Both the envelope queue and the trace ring buffer are Durable Objects, one per
mailbox, with **one storage key per entry**.

This is deliberate and was arrived at the hard way. KV read-modify-write loses
concurrent writes — an ack writing back a pre-push snapshot silently erased
freshly pushed injects, which showed up as commands that vanished with no error
anywhere. `blockConcurrencyWhile` fixed the warm path but not a cold-start
instance split. Per-key writes read nothing and so can clobber nothing.
`storage.list()` on a DO is strongly consistent, unlike KV list, so reads stay
live.

## Run it

```bash
cp wrangler.example.toml wrangler.toml   # fill in account_id + KV namespace id
npx wrangler deploy
```

## Test it

`qa/run-relay-regression.ps1` is the protocol conformance suite — 42 assertions
covering role routing, ack semantics, cursor behaviour, concurrency and auth.
It runs against a live deployment.

```powershell
cd qa
.\run-relay-regression.ps1 -Relay https://your-worker.workers.dev
```
