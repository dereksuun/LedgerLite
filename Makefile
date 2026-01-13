SHELL := /bin/bash

PROFILE_DEV := --profile dev

.PHONY: help up down ps logs wait migrate api dev health

help:
	@echo "Alvos:"
	@echo "  make up       -> sobe containers (db + perfil dev)"
	@echo "  make down     -> derruba containers (inclui perfil dev)"
	@echo "  make ps       -> status (inclui perfil dev)"
	@echo "  make logs     -> logs (inclui perfil dev)"
	@echo "  make wait     -> espera postgres ficar healthy"
	@echo "  make migrate  -> roda migrations"
	@echo "  make api      -> sobe API fora do docker (precisa DATABASE_URL)"
	@echo "  make dev      -> sobe tudo via scripts/dev.sh"
	@echo "  make health   -> testa /health (API precisa estar rodando)"

up:
	docker compose $(PROFILE_DEV) up -d

down:
	docker compose $(PROFILE_DEV) down --remove-orphans

ps:
	docker compose $(PROFILE_DEV) ps

logs:
	docker compose $(PROFILE_DEV) logs -f --tail=200 postgres api outbox-worker

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
