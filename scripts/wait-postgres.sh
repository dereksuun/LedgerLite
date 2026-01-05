#!/usr/bin/env bash
set -euo pipefail

SERVICE="${1:-postgres}"
TIMEOUT="${TIMEOUT:-60}"

echo "[wait] esperando serviço '$SERVICE' ficar healthy (timeout: ${TIMEOUT}s)..."

start="$(date +%s)"
while true; do
  status="$(docker inspect --format='{{.State.Health.Status}}' "ledgerlite-${SERVICE}-1" 2>/dev/null || true)"
  if [[ "$status" == "healthy" ]]; then
    echo "[wait] postgres healthy ✅"
    exit 0
  fi

  now="$(date +%s)"
  elapsed=$((now - start))
  if (( elapsed > TIMEOUT )); then
    echo "[wait] timeout esperando postgres. Últimos logs:"
    docker compose logs --tail=50 "$SERVICE" || true
    exit 1
  fi

  sleep 1
done
