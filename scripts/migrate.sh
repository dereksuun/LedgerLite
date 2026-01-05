#!/usr/bin/env bash
set -euo pipefail

DB="${DB_NAME:-ledgerlite}"
USER="${DB_USER:-postgres}"
SERVICE="${SERVICE:-postgres}"
SQL="${SQL_FILE:-migrations/001_init.sql}"

if [[ ! -f "$SQL" ]]; then
  echo "[migrate] arquivo não encontrado: $SQL"
  exit 1
fi

echo "[migrate] rodando migrations em $DB usando $SQL..."
docker compose exec -T "$SERVICE" psql -U "$USER" -d "$DB" < "$SQL"

echo "[migrate] ok ✅"
