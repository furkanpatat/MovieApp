#!/usr/bin/env bash
# Creates the server's .env for production: strong random secrets for
# Postgres, Redis, RabbitMQ and JWT, plus your domain. API keys (TMDB, OMDb,
# Groq) are left for you to paste in by hand. Never overwrites an existing .env.
#
#   scripts/prod-env.sh kinora.duckdns.org
set -euo pipefail
domain="${1:?usage: scripts/prod-env.sh <domain>}"
cd "$(dirname "$0")/.."
[ -e .env ] && { echo ".env already exists; not touching it." >&2; exit 1; }

secret() { openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c "$1"; }

umask 077
cat > .env <<ENV
# Production settings (generated $(date -u +%F)). Keep this file private.
APP_ENV=production
DOMAIN=$domain

POSTGRES_USER=movieapp
POSTGRES_DB=movieapp
POSTGRES_PASSWORD=$(secret 40)
REDIS_PASSWORD=$(secret 40)
RABBITMQ_USER=movieapp
RABBITMQ_PASSWORD=$(secret 40)
JWT_SECRET=$(secret 64)
GRAFANA_ADMIN_PASSWORD=$(secret 24)

# Paste your keys here (see .env.example for where to get them):
TMDB_API_KEY=
OMDB_API_KEY=
GROQ_API_KEY=
ENV
echo "Wrote .env (mode 600). Now add TMDB_API_KEY, OMDB_API_KEY and GROQ_API_KEY to it."
