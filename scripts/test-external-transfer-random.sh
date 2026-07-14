#!/usr/bin/env bash
# Run N EXTERNAL transfers under BANK__MODE=random; each must terminate
# SUCCEEDED or FAILED. Prints the distribution.
# Usage: ./scripts/test-external-transfer-random.sh [N]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib-api.sh
source "${ROOT}/scripts/lib-api.sh"

N="${1:-8}"
AMOUNT="${AMOUNT:-2.00}"
CURRENCY="${CURRENCY:-USD}"
RESTORE_MODE="${RESTORE_BANK_MODE:-always_success}"

cleanup() {
  if [[ "${KEEP_BANK_MODE:-0}" != "1" ]]; then
    echo "==> restore bank mode ${RESTORE_MODE}"
    "${ROOT}/scripts/set-bank-mode.sh" "$RESTORE_MODE" || true
  fi
}
trap cleanup EXIT

echo "==> set bank mode random"
"${ROOT}/scripts/set-bank-mode.sh" random

echo "==> wait for API ${BASE_URL}"
wait_api
ALICE_TOKEN="$(login alice@transx.dev)"
FROM_REF="$(pick_account_ref "$ALICE_TOKEN" "$CURRENCY")"
echo "  from=${FROM_REF}"

success=0
failure=0
other=0

for ((i=1; i<=N; i++)); do
  BODY="$(python3 -c 'import json,sys; print(json.dumps({
    "fromAccountRef": sys.argv[1],
    "amount": sys.argv[2],
    "currency": sys.argv[3],
    "transferType": "EXTERNAL",
    "message": "script external random "+sys.argv[4],
    "toAccountRef": "EXT-BENEFICIARY-DEMO",
  }))' "$FROM_REF" "$AMOUNT" "$CURRENCY" "$i")"

  echo "-- run ${i}/${N}"
  CREATE_RESP="$(create_transfer "$ALICE_TOKEN" "$BODY")"
  TRANSFER_ID="$(printf '%s' "$CREATE_RESP" | json_get transferId)"
  if [[ -z "$TRANSFER_ID" ]]; then
    echo "create failed: ${CREATE_RESP}" >&2
    exit 1
  fi

  # Poll until terminal (SUCCEEDED or FAILED)
  body=""
  status=""
  for ((p=1; p<=POLL_ATTEMPTS; p++)); do
    body="$(get_transfer "$ALICE_TOKEN" "$TRANSFER_ID")"
    status="$(printf '%s' "$body" | json_get status)"
    echo "  poll ${p}: status=${status}"
    if [[ "$status" == "SUCCEEDED" || "$status" == "FAILED" ]]; then
      break
    fi
    sleep "$POLL_SLEEP_SEC"
  done

  case "$status" in
    SUCCEEDED) success=$((success+1)); echo "  -> SUCCESS ${TRANSFER_ID}" ;;
    FAILED)    failure=$((failure+1)); echo "  -> FAILURE ${TRANSFER_ID}" ;;
    *)
      other=$((other+1))
      echo "  -> unexpected status=${status} body=${body}" >&2
      exit 1
      ;;
  esac
done

echo "random external results: success=${success} failure=${failure} total=${N}"
if [[ $((success+failure)) -ne "$N" ]]; then
  echo "not all transfers terminal" >&2
  exit 1
fi
# With N large enough we expect both sides; for small N allow all-one-side but warn.
if [[ "$N" -ge 6 && ( "$success" -eq 0 || "$failure" -eq 0 ) ]]; then
  echo "warning: expected both SUCCESS and FAILURE for N=${N} (got success=${success} failure=${failure})" >&2
  # still exit 0 — hash distribution can theoretically skew; increase N if needed
fi
echo "OK external random suite"
