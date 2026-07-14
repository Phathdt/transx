#!/usr/bin/env bash
# Shared helpers for API smoke scripts (login, accounts, transfers, poll).
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:4000}"
POLL_ATTEMPTS="${POLL_ATTEMPTS:-40}"
POLL_SLEEP_SEC="${POLL_SLEEP_SEC:-1}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd python3

api() {
  # api METHOD PATH [curl args...]
  local method="$1" path="$2"
  shift 2
  curl -sS -X "$method" "${BASE_URL}${path}" "$@"
}

json_get() {
  # json_get FIELD < json
  local field="$1"
  python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get(sys.argv[1],""))' "$field"
}

login() {
  local email="$1" password="${2:-password123}"
  local body
  body="$(api POST /api/v1/login \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${email}\",\"password\":\"${password}\"}")"
  local token
  token="$(printf '%s' "$body" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("accessToken") or d.get("token") or d.get("access_token") or "")')"
  if [[ -z "$token" ]]; then
    echo "login failed for ${email}: ${body}" >&2
    exit 1
  fi
  printf '%s' "$token"
}

# list_accounts TOKEN → JSON array of accounts (handles {data:[...]} or raw array)
list_accounts() {
  local token="$1"
  api GET /api/v1/accounts \
    -H "Authorization: Bearer ${token}" | python3 -c '
import json,sys
raw=sys.stdin.read()
d=json.loads(raw)
if isinstance(d, list):
    print(json.dumps(d))
elif isinstance(d, dict) and "data" in d:
    print(json.dumps(d["data"]))
elif isinstance(d, dict) and "items" in d:
    print(json.dumps(d["items"]))
else:
    print(raw)
'
}

# pick_account_ref TOKEN CURRENCY → accountRef
pick_account_ref() {
  local token="$1" currency="$2"
  list_accounts "$token" | python3 -c '
import json,sys
currency=sys.argv[1]
accounts=json.load(sys.stdin)
for a in accounts:
    cur=a.get("currency") or a.get("Currency") or ""
    if cur.upper()==currency.upper():
        ref=a.get("accountRef") or a.get("account_ref") or a.get("ref") or ""
        if ref:
            print(ref)
            sys.exit(0)
print("", end="")
sys.exit(1)
' "$currency"
}

uuid_v4() {
  python3 -c 'import uuid; print(uuid.uuid4())'
}

create_transfer() {
  local token="$1" body="$2" idem="${3:-}"
  if [[ -z "$idem" ]]; then
    idem="$(uuid_v4)"
  fi
  api POST /api/v1/transfers \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${idem}" \
    -d "$body"
}

get_transfer() {
  local token="$1" transfer_id="$2"
  api GET "/api/v1/transfers/${transfer_id}" \
    -H "Authorization: Bearer ${token}"
}

# poll_transfer_status TOKEN TRANSFER_ID EXPECTED_STATUS
poll_transfer_status() {
  local token="$1" transfer_id="$2" expected="$3"
  local i body status
  for ((i=1; i<=POLL_ATTEMPTS; i++)); do
    body="$(get_transfer "$token" "$transfer_id")"
    status="$(printf '%s' "$body" | json_get status)"
    echo "  poll ${i}/${POLL_ATTEMPTS}: status=${status}"
    if [[ "$status" == "$expected" ]]; then
      printf '%s' "$body"
      return 0
    fi
    if [[ "$status" == "FAILED" && "$expected" != "FAILED" ]]; then
      echo "transfer failed unexpectedly: ${body}" >&2
      return 1
    fi
    sleep "$POLL_SLEEP_SEC"
  done
  echo "timed out waiting for status=${expected}; last=${body:-}" >&2
  return 1
}

wait_api() {
  local i
  for ((i=1; i<=60; i++)); do
    if curl -sS -o /dev/null -w '%{http_code}' "${BASE_URL}/api/v1/login" \
      -H 'Content-Type: application/json' \
      -d '{"email":"x","password":"y"}' 2>/dev/null | grep -Eq '200|400|401|422'; then
      return 0
    fi
    sleep 1
  done
  echo "API not ready at ${BASE_URL}" >&2
  return 1
}
