#!/usr/bin/env bash
# End-to-end EXTERNAL transfer via Traefik API (Temporal + Bank gRPC).
# Usage: ./scripts/test-external-transfer.sh
# Env: BASE_URL, AMOUNT (default 5.00), BANK mode should be always_success for happy path.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib-api.sh
source "${ROOT}/scripts/lib-api.sh"

AMOUNT="${AMOUNT:-5.00}"
CURRENCY="${CURRENCY:-USD}"
EXPECTED_STATUS="${EXPECTED_STATUS:-SUCCEEDED}"

echo "==> wait for API ${BASE_URL}"
wait_api

echo "==> login alice"
ALICE_TOKEN="$(login alice@transx.dev)"

echo "==> resolve source account (${CURRENCY})"
FROM_REF="$(pick_account_ref "$ALICE_TOKEN" "$CURRENCY")"
echo "  from=${FROM_REF}"

BODY="$(python3 -c 'import json,sys; print(json.dumps({
  "fromAccountRef": sys.argv[1],
  "amount": sys.argv[2],
  "currency": sys.argv[3],
  "transferType": "EXTERNAL",
  "message": "script external smoke",
  "toAccountRef": "EXT-BENEFICIARY-DEMO",
}))' "$FROM_REF" "$AMOUNT" "$CURRENCY")"

echo "==> create EXTERNAL transfer amount=${AMOUNT} ${CURRENCY}"
CREATE_RESP="$(create_transfer "$ALICE_TOKEN" "$BODY")"
echo "  create: ${CREATE_RESP}"
TRANSFER_ID="$(printf '%s' "$CREATE_RESP" | json_get transferId)"
if [[ -z "$TRANSFER_ID" ]]; then
  echo "missing transferId in create response" >&2
  exit 1
fi
echo "  transferId=${TRANSFER_ID}"

echo "==> poll until ${EXPECTED_STATUS}"
FINAL="$(poll_transfer_status "$ALICE_TOKEN" "$TRANSFER_ID" "$EXPECTED_STATUS")"
echo "  final: ${FINAL}"
echo "OK external transfer ${TRANSFER_ID} -> ${EXPECTED_STATUS}"
