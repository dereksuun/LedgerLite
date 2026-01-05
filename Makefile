SHELL := /bin/bash

.PHONY: help up down ps logs wait migrate api dev health

help:
	@echo "Alvos:"
	@echo "  make up       -> sobe containers"
	@echo "  make down     -> derruba containers"
	@echo "  make ps       -> status"
	@echo "  make logs     -> logs do postgres"
	@echo "  make migrate  -> roda migrations"
	@echo "  make api      -> sobe API (precisa DATABASE_URL)"
	@echo "  make dev      -> sobe tudo (db+migrate+api)"
	@echo "  make health   -> testa /health (API precisa estar rodando)"

up:
	docker compose up -d

down:
	docker compose down

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=200 postgres

wait:
	TIMEOUT=60 ./scripts/wait-postgres.sh postgres

migrate:
	./scripts/migrate.sh

api:
	@bash -lc 'set -a; [ -f .env ] && source .env; set +a; : "$${DATABASE_URL:?DATABASE_URL não definido}"; go run ./cmd/api'

dev:
	./scripts/dev.sh

health:
	curl -i http://localhost:8080/health
