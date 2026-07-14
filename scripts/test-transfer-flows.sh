#!/usr/bin/env bash
# Run INTERNAL then EXTERNAL smoke tests against a live stack.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export BASE_URL="${BASE_URL:-http://localhost:4000}"

echo "======== INTERNAL ========"
"${ROOT}/scripts/test-internal-transfer.sh"
echo
echo "======== EXTERNAL SUCCESS (always_success) ========"
"${ROOT}/scripts/set-bank-mode.sh" always_success
"${ROOT}/scripts/test-external-transfer.sh"
echo
echo "======== EXTERNAL FAILED (always_failure) ========"
"${ROOT}/scripts/test-external-transfer-failed.sh"
echo
echo "======== EXTERNAL RANDOM ========"
"${ROOT}/scripts/test-external-transfer-random.sh" "${RANDOM_N:-6}"
echo
echo "All transfer flow smokes passed."
