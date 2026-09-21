.PHONY: up down logs ps observability db-init load-test load-test-smoke load-restore
up:            ; docker compose up -d
down:          ; docker compose down
logs:          ; docker compose logs -f
ps:            ; docker compose ps
observability: ; docker compose --profile observability up -d

# Apply deployments/postgres/init.sql to an already-initialised database.
db-init:
	docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < deployments/postgres/init.sql

# --- load testing (k6, see tests/load/) ---
# Raises the gateway rate limits for the run (.env.loadtest), then restores them.
load-test:
	tests/load/run.sh
load-test-smoke:
	SMOKE=1 tests/load/run.sh
# If a run was interrupted: put the gateway back on the normal limits.
load-restore:
	docker compose up -d --no-deps --force-recreate gateway
