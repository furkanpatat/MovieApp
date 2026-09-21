#!/usr/bin/env bash
# Load test orchestration:
#   1. raise the gateway rate limits (.env.loadtest), verify they took effect
#   2. start the stack monitor
#   3. run k6 (in Docker, on the compose network, straight at the gateway)
#   4. wait for the async pipeline to drain, report how long it took
#   5. remove all test data and restore the gateway's normal limits
#
#   tests/load/run.sh            full profile: ramp to 5,000 VUs
#   SMOKE=1 tests/load/run.sh    small sanity run
#   VUS=1000 HOLD=1m tests/load/run.sh
set -uo pipefail
cd "$(dirname "$0")/../.."
set -a; . ./.env; set +a

if [ -n "${SMOKE:-}" ]; then
  : "${VUS:=50}" "${RAMP:=10s}" "${HOLD:=20s}" "${DOWN:=5s}" "${POOL_SIZE:=20}"
else
  : "${VUS:=5000}" "${RAMP:=2m}" "${HOLD:=3m}" "${DOWN:=30s}" "${POOL_SIZE:=200}"
fi
: "${MOVIES:=50}" "${BASE_URL:=http://gateway:8080}"
K6_IMAGE=${K6_IMAGE:-grafana/k6:latest}
NET=${COMPOSE_PROJECT_NAME:-movieapp}_movieapp
STAMP=$(date +%Y%m%d-%H%M%S)
OUT=tests/load/results/$STAMP
mkdir -p "$OUT"

psql_() { docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "$1"; }
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
die() { printf '\033[31m%s\033[0m\n' "$*"; exit 1; }

MON_PID=""
restore() {
  set +e
  [ -n "$MON_PID" ] && kill "$MON_PID" 2>/dev/null
  step "cleanup: remove test data, restore normal gateway limits"
  psql_ "delete from interaction.ratings where movie_id between 1 and $MOVIES;
         delete from interaction.movie_rating_stats where movie_id between 1 and $MOVIES;
         delete from interaction.comments where movie_id between 1 and $MOVIES;
         delete from interaction.outbox_events;
         delete from auth.users where email like '%@loadtest.local';" >/dev/null
  docker compose exec -T rabbitmq rabbitmqctl -q purge_queue interaction.ratings >/dev/null 2>&1
  docker compose exec -T rabbitmq rabbitmqctl -q purge_queue interaction.comments >/dev/null 2>&1
  docker compose exec -T rabbitmq rabbitmqctl -q purge_queue interaction.ratings.dlq >/dev/null 2>&1
  docker compose exec -T rabbitmq rabbitmqctl -q purge_queue interaction.comments.dlq >/dev/null 2>&1
  for pat in 'interaction:*' 'ratelimit:*'; do
    docker compose exec -T redis sh -c "redis-cli -a '$REDIS_PASSWORD' --no-auth-warning --scan --pattern '$pat' | xargs -r -n 500 redis-cli -a '$REDIS_PASSWORD' --no-auth-warning del >/dev/null"
  done
  docker compose up -d --no-deps --force-recreate gateway >/dev/null 2>&1
  echo "gateway restored to the normal rate limits (RATE_LIMIT_REQUESTS=${RATE_LIMIT_REQUESTS_NORMAL:-100}/min)"
}
trap restore EXIT

step "0. preflight"
for s in gateway auth interaction postgres redis rabbitmq; do
  docker compose ps "$s" 2>/dev/null | grep -q healthy || die "$s is not healthy (run: make up)"
done
dirty=$(psql_ "select (select count(*) from interaction.ratings)+(select count(*) from interaction.comments)+(select count(*) from interaction.outbox_events)")
[ "$dirty" = "0" ] || die "interaction tables are not empty ($dirty rows); the cleanup step deletes them, so refusing to run"
docker image inspect "$K6_IMAGE" >/dev/null 2>&1 || docker pull -q "$K6_IMAGE" >/dev/null

step "1. raise the gateway rate limits (.env.loadtest)"
docker compose --env-file .env --env-file .env.loadtest up -d --no-deps --force-recreate gateway >/dev/null 2>&1
for _ in $(seq 1 30); do docker compose ps gateway | grep -q healthy && break; sleep 1; done
docker compose exec -T gateway printenv RATE_LIMIT_REQUESTS AUTH_RATE_LIMIT_REQUESTS | paste -sd' ' - | sed 's/^/gateway limits now: /'
# prove it: 400 requests from one IP in a burst must all pass (production limit is 100/min)
blocked=$(for i in $(seq 1 400); do curl -s -o /dev/null -w '%{http_code}\n' "localhost:${GATEWAY_PORT:-8000}/api/v1/movies/1/interactions"; done | grep -c 429)
[ "$blocked" = "0" ] || die "rate limit override is not in effect ($blocked of 400 requests got 429)"
echo "verified: 400-request burst, 0 x 429"
docker compose exec -T redis sh -c "redis-cli -a '$REDIS_PASSWORD' --no-auth-warning --scan --pattern 'ratelimit:*' | xargs -r redis-cli -a '$REDIS_PASSWORD' --no-auth-warning del >/dev/null"

step "2. start monitor  ->  $OUT/monitor.csv"
python3 tests/load/monitor.py record "$OUT/monitor.csv" "${SAMPLE_EVERY:-5}" > "$OUT/monitor.log" 2>&1 &
MON_PID=$!

step "3. k6: $VUS VUs, ramp $RAMP / hold $HOLD / down $DOWN  (pool $POOL_SIZE users)"
docker run --rm --name movieapp-k6 --network "$NET" --ulimit nofile=1048576:1048576 \
  -v "$PWD/tests/load:/load" -w /load \
  -e BASE_URL="$BASE_URL" -e VUS="$VUS" -e RAMP="$RAMP" -e HOLD="$HOLD" -e DOWN="$DOWN" \
  -e POOL_SIZE="$POOL_SIZE" -e MOVIES="$MOVIES" -e RUN_ID="$STAMP" \
  ${EXTRA_K6_ENV:-} \
  "$K6_IMAGE" run --quiet --no-usage-report --summary-export="/load/results/$STAMP/summary.json" load_test.js 2>&1 | tee "$OUT/k6.log"
K6_RC=${PIPESTATUS[0]}

step "4. drain: wait for outbox + queues to empty"
DRAIN_START=$(date +%s)
for _ in $(seq 1 180); do
  pending=$(psql_ "select count(*) from interaction.outbox_events where status='pending'")
  q=$(docker compose exec -T rabbitmq rabbitmqctl -q list_queues name messages | awk '$1 ~ /^interaction\.(ratings|comments)$/ {s+=$2} END{print s+0}')
  [ "$pending" = "0" ] && [ "$q" = "0" ] && break
  printf '  outbox pending=%s queued=%s\n' "$pending" "$q"; sleep 5
done
DRAIN_S=$(( $(date +%s) - DRAIN_START ))
echo "pipeline drained $DRAIN_S s after k6 finished (outbox pending=$pending, queued=$q)"
sleep 6
kill "$MON_PID" 2>/dev/null; wait "$MON_PID" 2>/dev/null; MON_PID=""

step "5. consistency: every accepted (202) write must exist in Postgres"
python3 - "$OUT/summary.json" > "$OUT/accepted.env" <<'PY'
import json, sys
m = json.load(open(sys.argv[1]))["metrics"]
get = lambda k: int((m.get(k) or {}).get("count", (m.get(k) or {}).get("values", {}).get("count", 0)))
print(f"ACC_RATINGS={get('accepted_ratings')}\nACC_COMMENTS={get('accepted_comments')}")
PY
. "$OUT/accepted.env"
OUTBOX_TOTAL=$(psql_ "select count(*) from interaction.outbox_events")
OUTBOX_PUB=$(psql_ "select count(*) from interaction.outbox_events where status='published'")
COMMENT_ROWS=$(psql_ "select count(*) from interaction.comments")
RATING_ROWS=$(psql_ "select count(*) from interaction.ratings")
VOTES=$(psql_ "select coalesce(sum(vote_count),0) from interaction.movie_rating_stats")
DLQ=$(docker compose exec -T rabbitmq rabbitmqctl -q list_queues name messages | awk '$1 ~ /\.dlq$/ {s+=$2} END{print s+0}')
{
  echo "writes accepted by the API (202):   ratings=$ACC_RATINGS comments=$ACC_COMMENTS  total=$((ACC_RATINGS+ACC_COMMENTS))"
  echo "outbox rows (stored before 202):    total=$OUTBOX_TOTAL published=$OUTBOX_PUB"
  echo "projected into Postgres:            comments=$COMMENT_ROWS  rating rows=$RATING_ROWS (upserts: one per user+movie)  vote_count sum=$VOTES"
  echo "dead-lettered messages:             $DLQ"
  [ "$OUTBOX_TOTAL" = "$((ACC_RATINGS+ACC_COMMENTS))" ] && echo "CHECK outbox == accepted writes ........ OK" || echo "CHECK outbox == accepted writes ........ MISMATCH"
  [ "$COMMENT_ROWS" = "$ACC_COMMENTS" ] && echo "CHECK comments stored == accepted ...... OK" || echo "CHECK comments stored == accepted ...... MISMATCH"
  [ "$OUTBOX_PUB" = "$OUTBOX_TOTAL" ] && echo "CHECK all outbox rows published ........ OK" || echo "CHECK all outbox rows published ........ MISMATCH"
  [ "$VOTES" = "$RATING_ROWS" ] && echo "CHECK aggregate votes == rating rows ... OK" || echo "CHECK aggregate votes == rating rows ... MISMATCH"
  [ "$DLQ" = "0" ] && echo "CHECK nothing dead-lettered ............ OK" || echo "CHECK nothing dead-lettered ............ FAIL"
} | tee "$OUT/consistency.txt"

step "6. stack monitor summary"
python3 tests/load/monitor.py summary "$OUT/monitor.csv" | tee "$OUT/monitor-summary.txt"
echo; echo "artifacts in $OUT"
exit "$K6_RC"
