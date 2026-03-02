#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:18080}"
TOTAL="${TOTAL:-50}"
CONCURRENCY="${CONCURRENCY:-10}"
CUSTOMER_PREFIX="${CUSTOMER_PREFIX:-cust}"

usage() {
  cat <<USAGE
Load-test style ticket producer for rabbitamq-queuecraft.

Usage:
  $(basename "$0") [-u api_url] [-n total] [-c concurrency] [-p customer_prefix]

Options:
  -u  API base URL (default: ${API_URL})
  -n  Number of tickets to create (default: ${TOTAL})
  -c  Parallel workers (default: ${CONCURRENCY})
  -p  Customer ID prefix (default: ${CUSTOMER_PREFIX})

Examples:
  $(basename "$0")
  $(basename "$0") -n 200 -c 25
  API_URL=http://localhost:8080 $(basename "$0") -n 100
USAGE
}

while getopts ":u:n:c:p:h" opt; do
  case "$opt" in
    u) API_URL="$OPTARG" ;;
    n) TOTAL="$OPTARG" ;;
    c) CONCURRENCY="$OPTARG" ;;
    p) CUSTOMER_PREFIX="$OPTARG" ;;
    h) usage; exit 0 ;;
    :) echo "error: -$OPTARG requires a value" >&2; usage; exit 1 ;;
    \?) echo "error: invalid option -$OPTARG" >&2; usage; exit 1 ;;
  esac
done

if ! [[ "$TOTAL" =~ ^[0-9]+$ ]] || [ "$TOTAL" -lt 1 ]; then
  echo "error: total must be a positive integer" >&2
  exit 1
fi

if ! [[ "$CONCURRENCY" =~ ^[0-9]+$ ]] || [ "$CONCURRENCY" -lt 1 ]; then
  echo "error: concurrency must be a positive integer" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required" >&2
  exit 1
fi

start_ts="$(date +%s)"
out_file="$(mktemp)"

send_ticket() {
  local i="$1"
  local sev subject body

  case $((i % 3)) in
    0)
      sev="high"
      subject="Urgent: payment failed during checkout #${i}"
      body="Multiple users report timeout and potential outage for order flow #${i}."
      ;;
    1)
      sev="medium"
      subject="API bug report #${i}"
      body="Intermittent error response observed in support dashboard #${i}."
      ;;
    *)
      sev="low"
      subject="Need invoice copy #${i}"
      body="Please share invoice PDF for order #${i}."
      ;;
  esac

  local payload
  payload=$(printf '{"customer_id":"%s_%04d","subject":"%s","body":"%s"}' "$CUSTOMER_PREFIX" "$i" "$subject" "$body")

  local response id
  response=$(curl -sS -X POST "${API_URL}/v1/tickets" -H 'Content-Type: application/json' -d "$payload") || {
    echo "ERR index=${i} request_failed" >>"$out_file"
    return
  }

  id=$(echo "$response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  if [ -z "$id" ]; then
    echo "ERR index=${i} bad_response=${response}" >>"$out_file"
    return
  fi

  echo "OK index=${i} severity=${sev} ticket_id=${id}" >>"$out_file"
}

export API_URL CUSTOMER_PREFIX out_file
export -f send_ticket

seq 1 "$TOTAL" | xargs -I{} -P "$CONCURRENCY" bash -c 'send_ticket "$@"' _ {}

ok_count=$(grep -c '^OK ' "$out_file" || true)
err_count=$(grep -c '^ERR ' "$out_file" || true)
end_ts="$(date +%s)"
duration=$((end_ts - start_ts))

printf '\nSent %s tickets with concurrency=%s in %ss\n' "$TOTAL" "$CONCURRENCY" "$duration"
printf 'Success: %s  Errors: %s\n' "$ok_count" "$err_count"

if [ "$ok_count" -gt 0 ]; then
  echo "Sample successful ticket IDs:"
  grep '^OK ' "$out_file" | head -n 10 | sed -n 's/.*ticket_id=\([^ ]*\).*/ - \1/p'
fi

if [ "$err_count" -gt 0 ]; then
  echo "Errors:"
  grep '^ERR ' "$out_file"
  rm -f "$out_file"
  exit 1
fi

rm -f "$out_file"
