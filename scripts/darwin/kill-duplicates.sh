#!/usr/bin/env bash
# List (default) or stop extra gbr-agent processes. Keep at most one.
# Does NOT start gbr-agent. Does NOT clear GBR_INJECT_HALT.
set -euo pipefail

KILL=0
[[ "${1:-}" == "--kill" ]] && KILL=1

PREFER="${HOME}/.local/bin/gbr-agent"
PIDS=()
while IFS= read -r pid; do
  [[ -n "$pid" ]] && PIDS+=("$pid")
done < <(pgrep -x gbr-agent || true)

echo "gbr-agent count: ${#PIDS[@]}"
for pid in "${PIDS[@]:-}"; do
  ps -p "$pid" -o pid=,command= || true
done

if [[ ${#PIDS[@]} -le 1 ]]; then
  echo "No duplicates."
  exit 0
fi

if [[ "$KILL" -eq 0 ]]; then
  echo "List only. Re-run with --kill to stop extras. Does not start the agent."
  exit 0
fi

KEEP=""
for pid in "${PIDS[@]}"; do
  cmd="$(ps -p "$pid" -o command= || true)"
  case "$cmd" in
    *"$PREFER"*) KEEP="$pid"; break ;;
  esac
done
KEEP="${KEEP:-${PIDS[0]}}"

for pid in "${PIDS[@]}"; do
  if [[ "$pid" != "$KEEP" ]]; then
    kill "$pid" 2>/dev/null || true
    echo "Stopped pid $pid"
  fi
done
echo "Kept pid $KEEP"
echo "Did not start gbr-agent. Did not clear GBR_INJECT_HALT."
