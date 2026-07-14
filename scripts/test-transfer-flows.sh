#!/usr/bin/env bash
# Run INTERNAL then EXTERNAL smoke tests against a live stack.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export BASE_URL="${BASE_URL:-http://localhost:4000}"

echo "======== INTERNAL ========"
"${ROOT}/scripts/test-internal-transfer.sh"
echo
echo "======== EXTERNAL ========"
"${ROOT}/scripts/test-external-transfer.sh"
echo
echo "All transfer flow smokes passed."
