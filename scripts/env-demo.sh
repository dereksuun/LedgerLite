#!/usr/bin/env bash
set -euo pipefail

export API_URL="${API_URL:-http://localhost:8080}"
export MINT_BALANCE="${MINT_BALANCE:-1000000000}" # 10,000,000.00 cents (adjust)

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing '$1'."; exit 1; }; }
need curl
need jq
need docker

psql_exec() {
  docker compose exec -T postgres psql -U postgres -d ledgerlite -v ON_ERROR_STOP=1 -c "$1" >/dev/null
}

create_account() {
  local name="$1"
  curl -sf -X POST "$API_URL/accounts" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"$name\"}" | jq -r '.id'
}

# Reuse if already in env; otherwise create.
if [[ -z "${MINT:-}" ]]; then
  export MINT="$(create_account "MINT")"
fi

echo "MINT=$MINT"

# Demo seed: set balance_cents directly (idempotent).
if [[ "${RUN_SEED_MINT:-0}" == "1" ]]; then
  psql_exec "UPDATE accounts SET balance_cents = GREATEST(balance_cents, $MINT_BALANCE) WHERE id = '$MINT';"
  echo "Seed OK: MINT balance_cents >= $MINT_BALANCE"
fi

# Create A and B (new each run by default).
export A="$(create_account "Conta A")"
export B="$(create_account "Conta B")"
echo "A=$A"
echo "B=$B"

# Initial deposit: works because MINT has balance.
if [[ "${RUN_INITIAL_DEPOSIT:-0}" == "1" ]]; then
  need uuidgen
  curl -sf -X POST "$API_URL/transactions" \
    -H 'Content-Type: application/json' \
    -H "Idempotency-Key: $(uuidgen)" \
    -d "{\"from_account_id\":\"$MINT\",\"to_account_id\":\"$A\",\"amount_cents\":100000,\"description\":\"initial deposit\"}" >/dev/null
  echo "Initial deposit OK: MINT -> A (100000 cents)"
fi

# Save for reuse.
cat > .ledger_env <<EOF
export API_URL="$API_URL"
export MINT="$MINT"
export A="$A"
export B="$B"
EOF

echo "Env saved to .ledger_env. Run: source .ledger_env"
