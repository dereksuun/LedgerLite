#!/usr/bin/env bash
set -euo pipefail

DB="${DB_NAME:-ledgerlite}"
USER="${DB_USER:-postgres}"
SERVICE="${SERVICE:-postgres}"

shopt -s nullglob
FILES=(migrations/*.sql)

if (( ${#FILES[@]} == 0 )); then
  echo "[migrate] nenhuma migration encontrada em migrations/*.sql"
  exit 1
fi

echo "[migrate] rodando ${#FILES[@]} migration(s) em $DB..."

for f in "${FILES[@]}"; do
  echo "[migrate] -> $(basename "$f")"
  docker compose exec -T "$SERVICE" psql -U "$USER" -d "$DB" < "$f"
done

echo "[migrate] ok ✅"
