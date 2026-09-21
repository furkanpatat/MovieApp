#!/usr/bin/env bash
# End-to-end proof of the outbox guarantee against the running compose stack:
#   RabbitMQ stopped -> writes still return 202 -> events wait in the outbox
#   -> RabbitMQ restarted -> relay delivers -> read model is correct.
# Usage: scripts/outbox-outage-test.sh      (stack must be up: make up)
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; . ./.env; set +a

GW="localhost:${GATEWAY_PORT:-8000}"
API="$GW/api/v1/movies"
MOVIE=888001
psql_() { docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "$1"; }
pending() { psql_ "select count(*) from interaction.outbox_events where status='pending'"; }
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
fail() { printf '\033[31mFAIL: %s\033[0m\n' "$*"; exit 1; }
wait_for() { # wait_for <seconds> <description> <command...>
  local t=$1 d=$2; shift 2
  for ((i = 0; i < t; i++)); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done
  fail "timed out waiting for: $d"
}
cleanup() {
  docker compose start rabbitmq >/dev/null 2>&1 || true
  psql_ "delete from auth.users where email like '%@outage.test'" >/dev/null 2>&1 || true
  psql_ "delete from interaction.ratings where movie_id=$MOVIE; delete from interaction.movie_rating_stats where movie_id=$MOVIE; delete from interaction.comments where movie_id=$MOVIE; delete from interaction.outbox_events where payload->>'movie_id'='$MOVIE'" >/dev/null 2>&1 || true
  docker compose exec -T redis redis-cli -a "$REDIS_PASSWORD" --no-auth-warning del "interaction:$MOVIE:rating" "interaction:$MOVIE:comments" >/dev/null 2>&1 || true
}
trap cleanup EXIT

step "0. baseline: stack healthy, outbox empty"
wait_for 30 "gateway healthy" bash -c "docker compose ps gateway | grep -q healthy"
SUFFIX=$(date +%s)$RANDOM
signup() { # signup <name>  -> prints an access token (real register + login through the gateway)
  curl -sf -XPOST "$GW/api/v1/auth/register" -d "{\"username\":\"$1_$SUFFIX\",\"email\":\"$1$SUFFIX@outage.test\",\"password\":\"outage-test-pw\"}" >/dev/null || fail "register $1 failed (auth rate limit? wait a minute)"
  curl -sf -XPOST "$GW/api/v1/auth/login" -d "{\"login\":\"$1_$SUFFIX\",\"password\":\"outage-test-pw\"}" | sed 's/.*"access_token":"\([^"]*\)".*/\1/'
}
TOKEN_ALICE=$(signup alice)
TOKEN_BOB=$(signup bob)
[ -n "$TOKEN_ALICE" ] || fail "could not get a token from the auth service"
wait_for 30 "interaction healthy" bash -c "docker compose ps interaction | grep -q healthy"
[ "$(pending)" = "0" ] || fail "outbox not empty at start"

step "1. stop RabbitMQ"
docker compose stop rabbitmq >/dev/null
echo "rabbitmq stopped"

step "2. restart the interaction service WHILE the broker is down (API must not depend on it)"
docker compose restart interaction >/dev/null
wait_for 60 "interaction healthy without a broker" bash -c "docker compose ps interaction | grep -q healthy"
echo "interaction is healthy with no broker"
[ "$(curl -s -o /dev/null -w '%{http_code}' "localhost:${INTERACTION_API_PORT:-8081}/readyz")" = "200" ] || fail "/readyz should be 200 during a broker outage"

step "3. writes during the outage must return 202"
for pair in "$TOKEN_ALICE:9" "$TOKEN_BOB:5"; do
  code=$(curl -s -o /dev/null -w '%{http_code}' -XPOST "$API/$MOVIE/rate" -H "Authorization: Bearer ${pair%%:*}" -d "{\"score\":${pair##*:}}"); echo "rate    -> $code"; [ "$code" = 202 ] || fail "rate returned $code"
done
code=$(curl -s -o /dev/null -w '%{http_code}' -XPOST "$API/$MOVIE/comment" -H "Authorization: Bearer $TOKEN_ALICE" -d '{"text":"posted while RabbitMQ was down"}'); echo "comment -> $code"; [ "$code" = 202 ] || fail "comment returned $code"

step "4. events are safely in the outbox, not yet projected"
sleep 3
[ "$(pending)" = "3" ] || fail "expected 3 pending outbox rows, got $(pending)"
echo "pending outbox rows: 3"
psql_ "select event_type, status, attempts from interaction.outbox_events where payload->>'movie_id'='$MOVIE' order by created_at"
curl -s "$API/$MOVIE/interactions" | grep -q '"total_votes":0' || fail "read model should be empty during the outage"
echo "read model still empty (eventual consistency)"

step "5. start RabbitMQ again"
docker compose start rabbitmq >/dev/null
wait_for 120 "outbox to drain" bash -c "[ \"\$(docker compose exec -T postgres psql -U $POSTGRES_USER -d $POSTGRES_DB -At -c \"select count(*) from interaction.outbox_events where status='pending'\")\" = 0 ]"
echo "outbox drained"

step "6. read model converges"
wait_for 30 "read model" bash -c "curl -s $API/$MOVIE/interactions | grep -q '\"total_votes\":2'"
curl -s "$API/$MOVIE/interactions" | tee /dev/stderr | grep -q 'posted while RabbitMQ was down' || fail "comment missing"
echo
psql_ "select status, count(*) from interaction.outbox_events where payload->>'movie_id'='$MOVIE' group by 1"
printf '\n\033[32mPASS: no write was lost or rejected during the broker outage\033[0m\n'
