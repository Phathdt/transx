#!/usr/bin/env bash
# Recreate bank-grpc with BANK__MODE.
# Usage: ./scripts/set-bank-mode.sh always_success|always_failure|always_timeout|random
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE="${1:-}"
if [[ -z "$MODE" ]]; then
  echo "usage: $0 always_success|always_failure|always_timeout|random" >&2
  exit 2
fi
case "$MODE" in
  always_success|always_failure|always_timeout|random) ;;
  *)
    echo "unknown mode: $MODE" >&2
    exit 2
    ;;
esac

cd "$ROOT"
echo "==> bank-grpc BANK__MODE=${MODE}"
export BANK__MODE="$MODE"
docker compose up -d --no-deps --force-recreate bank-grpc
for i in $(seq 1 30); do
  if docker compose ps bank-grpc 2>/dev/null | grep -q Up; then
    actual="$(docker inspect transx-bank-grpc --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^BANK__MODE=//p' | head -1)"
    echo "bank-grpc ready (requested=${MODE} actual=${actual:-unknown})"
    if [[ -n "$actual" && "$actual" != "$MODE" ]]; then
      echo "warning: container BANK__MODE=${actual} != requested ${MODE}" >&2
    fi
    exit 0
  fi
  sleep 0.5
done
echo "bank-grpc did not become ready" >&2
exit 1
