#!/usr/bin/env bash
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing '$1'."; exit 1; }; }
need uuidgen

# Bring up env + seed + initial deposit.
RUN_SEED_MINT=1 RUN_INITIAL_DEPOSIT=1 source scripts/env-demo.sh
source .ledger_env

echo
echo "== Transfer A -> B =="
curl -s -X POST "$API_URL/transactions" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $(uuidgen)" \
  -d "{\"from_account_id\":\"$A\",\"to_account_id\":\"$B\",\"amount_cents\":123,\"description\":\"demo transfer A->B\"}" | jq

echo
echo "== Statement A =="
curl -s "$API_URL/accounts/$A/statement?limit=50" | jq

echo
echo "== Statement B =="
curl -s "$API_URL/accounts/$B/statement?limit=50" | jq

echo
echo "== Outbox (last events) =="
docker compose exec -T postgres psql -U postgres -d ledgerlite -c \
"SELECT event_type, created_at, published_at FROM outbox_events ORDER BY created_at DESC LIMIT 10;"
