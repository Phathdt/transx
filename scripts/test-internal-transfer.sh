#!/usr/bin/env bash
# End-to-end INTERNAL transfer via Traefik API (Temporal path).
# Usage: ./scripts/test-internal-transfer.sh
# Env: BASE_URL (default http://localhost:4000), AMOUNT (default 10.00)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib-api.sh
source "${ROOT}/scripts/lib-api.sh"

AMOUNT="${AMOUNT:-10.00}"
CURRENCY="${CURRENCY:-USD}"

echo "==> wait for API ${BASE_URL}"
wait_api

echo "==> login alice + bob"
ALICE_TOKEN="$(login alice@transx.dev)"
BOB_TOKEN="$(login bob@transx.dev)"

echo "==> resolve accounts (${CURRENCY})"
FROM_REF="$(pick_account_ref "$ALICE_TOKEN" "$CURRENCY")"
TO_REF="$(pick_account_ref "$BOB_TOKEN" "$CURRENCY")"
echo "  from=${FROM_REF}"
echo "  to=${TO_REF}"

BODY="$(python3 -c 'import json,sys; print(json.dumps({
  "fromAccountRef": sys.argv[1],
  "toAccountRef": sys.argv[2],
  "amount": sys.argv[3],
  "currency": sys.argv[4],
  "transferType": "INTERNAL",
  "message": "script internal smoke",
}))' "$FROM_REF" "$TO_REF" "$AMOUNT" "$CURRENCY")"

echo "==> create INTERNAL transfer amount=${AMOUNT} ${CURRENCY}"
CREATE_RESP="$(create_transfer "$ALICE_TOKEN" "$BODY")"
echo "  create: ${CREATE_RESP}"
TRANSFER_ID="$(printf '%s' "$CREATE_RESP" | json_get transferId)"
STATUS="$(printf '%s' "$CREATE_RESP" | json_get status)"
if [[ -z "$TRANSFER_ID" ]]; then
  echo "missing transferId in create response" >&2
  exit 1
fi
echo "  transferId=${TRANSFER_ID} status=${STATUS}"

echo "==> poll until SUCCEEDED"
FINAL="$(poll_transfer_status "$ALICE_TOKEN" "$TRANSFER_ID" SUCCEEDED)"
echo "  final: ${FINAL}"
echo "OK internal transfer ${TRANSFER_ID}"
