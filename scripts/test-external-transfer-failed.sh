#!/usr/bin/env bash
# EXTERNAL transfer that must end FAILED (bank always_failure).
# Restores bank mode to always_success afterwards unless KEEP_BANK_MODE=1.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib-api.sh
source "${ROOT}/scripts/lib-api.sh"

RESTORE_MODE="${RESTORE_BANK_MODE:-always_success}"

cleanup() {
  if [[ "${KEEP_BANK_MODE:-0}" != "1" ]]; then
    echo "==> restore bank mode ${RESTORE_MODE}"
    "${ROOT}/scripts/set-bank-mode.sh" "$RESTORE_MODE" || true
  fi
}
trap cleanup EXIT

echo "==> set bank mode always_failure"
"${ROOT}/scripts/set-bank-mode.sh" always_failure

export EXPECTED_STATUS=FAILED
export AMOUNT="${AMOUNT:-3.00}"
"${ROOT}/scripts/test-external-transfer.sh"
