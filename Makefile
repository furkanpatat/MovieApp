.PHONY: up down logs ps observability observability-down db-init load-test load-test-smoke load-restore
up:            ; docker compose up -d
down:          ; docker compose down
logs:          ; docker compose logs -f
ps:            ; docker compose ps
# Prometheus (http://localhost:9090) + Grafana (http://localhost:3000, admin / $GRAFANA_ADMIN_PASSWORD from .env)
# with their exporters. Only these services are started, plus what they depend on.
OBS_SERVICES = prometheus grafana redis-exporter postgres-exporter blackbox-exporter
observability: ; docker compose --profile observability up -d $(OBS_SERVICES)
observability-down: ; docker compose --profile observability stop $(OBS_SERVICES)

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
