#!/usr/bin/env bash
# GBR agent diagnostics — status + hop summary + failures + optional inject probe.
# Usage:
#   ./scripts/gbr-diag.sh              # one-shot report
#   ./scripts/gbr-diag.sh watch        # tail hops live
#   ./scripts/gbr-diag.sh probe        # push a marked inject and wait for full path
#   ./scripts/gbr-diag.sh faults       # only failures / anomalies
set -euo pipefail

export PATH="${HOME}/.local/bin:${PATH}"
GBR_DIR="${HOME}/.gbr"
LOG_DIR="${GBR_DIR}/logs"
DEVICE="${GBR_DIR}/device.json"
TODAY="$(date -u +%Y-%m-%d)"
JSONL="${LOG_DIR}/agent-${TODAY}.jsonl"
DIAG="${LOG_DIR}/diag-$(date -u +%Y%m%d).log"
RELAY_DEFAULT="https://gbr-relay.ekobrott.workers.dev"

mkdir -p "$LOG_DIR"
ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts)] $*" | tee -a "$DIAG"; }

need_agent() {
  command -v gbr-agent >/dev/null || { echo "gbr-agent not on PATH"; exit 1; }
}

device_field() {
  python3 -c "import json; d=json.load(open('$DEVICE')); print(d.get('$1',''))" 2>/dev/null || true
}

cmd_status() {
  need_agent
  echo "======== GBR DIAG $(ts) ========"
  gbr-agent version 2>&1 || true
  echo
  gbr-agent status 2>&1 || true
  echo
  if [[ -f "$DEVICE" ]]; then
    python3 - <<'PY'
import json, os
p=os.path.expanduser("~/.gbr/device.json")
d=json.load(open(p))
print("device.json:")
print(f"  device_id:   {d.get('device_id')}")
print(f"  device_name: {d.get('device_name')}")
print(f"  mailbox:     {d.get('mailbox_conversation_id')}")
k=d.get("mailbox_key") or ""
print(f"  mailbox_key: {'set (%d chars)'%len(k) if k else 'MISSING'}")
PY
  else
    echo "device.json: MISSING — run: gbr-agent pair -code CODE"
  fi
  echo
  echo "log files:"
  ls -la "$LOG_DIR" 2>/dev/null || echo "  (none)"
  echo
  if [[ -f "$JSONL" ]]; then
    python3 - <<PY
import json
from collections import Counter
path="$JSONL"
hops=Counter(); fails=[]; injects=0; outs=0; recvs=0
with open(path) as f:
  for line in f:
    line=line.strip()
    if not line: continue
    try: e=json.loads(line)
    except: continue
    hop=e.get("hop") or "?"
    hops[hop]+=1
    if e.get("ok") is False:
      fails.append(e)
    if hop=="agent.inject": injects+=1
    if hop=="agent.push_output": outs+=1
    if hop=="agent.recv" and e.get("type")=="inject": recvs+=1
print("hop totals (today):")
for k,v in hops.most_common():
  print(f"  {k:24} {v}")
print(f"inject path: recv_inject={recvs} inject_ok={injects} push_output={outs}")
print(f"failures (ok=false): {len(fails)}")
for e in fails[-10:]:
  print(f"  FAIL {e.get('ts')} {e.get('hop')} type={e.get('type')} detail={e.get('detail')}")
# anomaly: inject without matching push_output for same command_id
cmds={}
with open(path) as f:
  for line in f:
    try: e=json.loads(line)
    except: continue
    cid=e.get("command_id")
    if not cid: continue
    cmds.setdefault(cid, []).append(e.get("hop"))
orphan=[]
for cid, hs in cmds.items():
  if "agent.inject" in hs and "agent.push_output" not in hs:
    orphan.append(cid)
print(f"inject without push_output: {len(orphan)}")
for cid in orphan[-5:]:
  print(f"  incomplete {cid} hops={cmds[cid]}")
PY
  else
    echo "no jsonl for $TODAY yet"
  fi
  echo
  echo "diag log: $DIAG"
}

cmd_faults() {
  need_agent
  echo "======== FAULTS $(ts) ========"
  if [[ ! -f "$JSONL" ]]; then echo "no jsonl"; exit 0; fi
  python3 - <<PY
import json, sys
path="$JSONL"
fails=[]; incomplete=[]
by={}
with open(path) as f:
  for line in f:
    try: e=json.loads(line)
    except: continue
    if e.get("ok") is False:
      fails.append(e)
    cid=e.get("command_id")
    if cid:
      by.setdefault(cid, []).append(e)
print(f"ok=false events: {len(fails)}")
for e in fails:
  print(json.dumps({"ts":e.get("ts"),"hop":e.get("hop"),"type":e.get("type"),"detail":e.get("detail"),"ms":e.get("ms")}))
print()
print("incomplete inject chains (no push_output):")
for cid, evs in by.items():
  hops=[e.get("hop") for e in evs]
  types=[e.get("type") for e in evs]
  if "agent.recv" in hops and ("inject" in types) and "agent.push_output" not in hops:
    print(cid, hops)
print()
print("slow injects (>15s agent.inject):")
for e in by.values():
  for x in e:
    if x.get("hop")=="agent.inject" and (x.get("ms") or 0) > 15000:
      print(x.get("command_id"), "ms=", x.get("ms"), x.get("detail"))
# auth rejects in detail text
print()
print("auth / 401 hints in details:")
with open(path) as f:
  for line in f:
    if "401" in line or "unauth" in line.lower() or "missing" in line:
      try:
        e=json.loads(line)
        if e.get("ok") is False or "401" in str(e.get("detail")):
          print(e.get("ts"), e.get("hop"), e.get("detail"))
      except: pass
PY
}

cmd_watch() {
  need_agent
  log "watch start jsonl=$JSONL"
  touch "$JSONL"
  echo "Tailing $JSONL (Ctrl-C to stop). Also writing $DIAG"
  tail -n 5 -F "$JSONL" | while read -r line; do
    python3 -c "
import json,sys
try:
  e=json.loads(sys.argv[1])
except Exception:
  print(sys.argv[1][:200]); raise SystemExit
flag='OK' if e.get('ok',True) else 'FAIL'
print(f\"{e.get('ts','')[:19]} [{flag}] {e.get('hop'):20} type={e.get('type') or '-':10} ms={e.get('ms') or '-'} { (e.get('detail') or '')[:80] }\")
" "$line" | tee -a "$DIAG"
  done
}

cmd_probe() {
  need_agent
  [[ -f "$DEVICE" ]] || { echo "pair first"; exit 1; }
  KEY="$(device_field mailbox_key)"
  MB="$(device_field mailbox_conversation_id)"
  [[ -n "$KEY" && -n "$MB" ]] || { echo "missing key/mailbox"; exit 1; }
  RELAY="$(python3 -c "import os; print(os.environ.get('GBR_RELAY_URL','$RELAY_DEFAULT'))")"
  CID="$(uuidgen | tr '[:upper:]' '[:lower:]')"
  TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  MARK="DIAG-PROBE-$(date +%H%M%S)"
  log "probe start mailbox=$MB cid=$CID mark=$MARK"
  # ensure agent running
  if ! gbr-agent status 2>/dev/null | grep -q "agent_lock:.*running"; then
    log "starting agent -log=debug"
    nohup gbr-agent -log=debug run >> "${LOG_DIR}/run-diag.log" 2>&1 &
    sleep 2
  fi
  HTTP=$(curl -sS -o /tmp/gbr-probe-push.json -w "%{http_code}" -X POST "${RELAY}/v1/mb/${MB}/push" \
    -H "Content-Type: application/json" -H "X-GBR-Key: $KEY" \
    -d "$(MARK="$MARK" CID="$CID" TS="$TS" python3 - <<'PY'
import json, os
print(json.dumps({
  "proto":"gbr/1","type":"inject","device_id":"diag-probe","session_id":"user",
  "command_id":os.environ["CID"],"ts":os.environ["TS"],
  "payload":{"mode":"text","text":"echo "+os.environ["MARK"],"submit":True}
}))
PY
)")
  log "push HTTP $HTTP body=$(head -c 120 /tmp/gbr-probe-push.json)"
  for i in $(seq 1 20); do
    sleep 1
    if [[ -f "$JSONL" ]] && grep -q "$CID" "$JSONL" && grep "$CID" "$JSONL" | grep -q push_output; then
      break
    fi
  done
  echo "---- hops for $CID ----"
  if [[ -f "$JSONL" ]]; then
    grep "$CID" "$JSONL" | python3 -c "
import sys,json
for line in sys.stdin:
  e=json.loads(line)
  print(e.get('hop'), e.get('ok'), e.get('detail','')[:100], 'ms=',e.get('ms'))
" | tee -a "$DIAG"
  fi
  curl -sS "${RELAY}/v1/mb/${MB}/poll?after=1970-01-01T00:00:00Z&for=diag-probe&role=mobile" \
    -H "X-GBR-Key: $KEY" | python3 -c "
import sys,json
j=json.load(sys.stdin)
mark='$MARK'
found=False
for e in reversed(j.get('envelopes') or []):
  if e.get('type')!='output': continue
  c=(e.get('payload') or {}).get('chunk') or ''
  if mark in c or e.get('command_id')=='$CID':
    found=True
    print('poll: output found, bytes', len(c), 'eof', (e.get('payload') or {}).get('eof'))
    for line in c.splitlines():
      if mark in line or 'echo' in line.lower():
        print(' ', line.strip()[:120])
    break
if not found:
  print('poll: NO matching output for mark', mark)
  print('envelope count', len(j.get('envelopes') or []))
" | tee -a "$DIAG"
  log "probe done"
}

case "${1:-status}" in
  status|"") cmd_status ;;
  watch) cmd_watch ;;
  probe) cmd_probe ;;
  faults) cmd_faults ;;
  *) echo "usage: $0 [status|watch|probe|faults]"; exit 2 ;;
esac
