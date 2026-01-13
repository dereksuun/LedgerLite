#!/usr/bin/env bash
set -euo pipefail

# 0) Checagens básicas
if ! command -v docker >/dev/null 2>&1; then
  echo "[dev] docker não encontrado."
  exit 1
fi

# 1) Docker rodando?
if ! docker info >/dev/null 2>&1; then
  echo "[dev] docker não está acessível. Tentando iniciar via systemd (pode pedir sudo)..."
  sudo -n systemctl start docker 2>/dev/null || true
fi

if ! docker info >/dev/null 2>&1; then
  echo "[dev] ainda não consegui falar com o docker. Rode: sudo systemctl start docker"
  exit 1
fi

# 2) Subir postgres
echo "[dev] subindo postgres..."
docker compose up -d postgres

# 3) Esperar healthy
TIMEOUT="${TIMEOUT:-60}" scripts/wait-postgres.sh postgres

# 4) Migrations
scripts/migrate.sh

# 5) Porta 8080 livre?
PORT="${PORT:-8080}"
if ss -ltn 2>/dev/null | awk '{print $4}' | grep -q ":${PORT}$"; then
  echo "[dev] porta ${PORT} já está em uso."
  echo "      Se quiser matar automaticamente: KILL_PORT=1 ./scripts/dev.sh"
  if [[ "${KILL_PORT:-0}" == "1" ]]; then
    echo "[dev] matando processo na porta ${PORT}..."
    sudo fuser -k "${PORT}/tcp" || true
  else
    exit 1
  fi
fi

# 6) Subir API + worker via docker compose
echo "[dev] subindo API e worker..."
docker compose --profile dev up -d api outbox-worker

echo "[dev] ok. use 'make down' para parar tudo"
