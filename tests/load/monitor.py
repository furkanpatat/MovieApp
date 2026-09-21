#!/usr/bin/env python3
"""Samples the running stack during a load test and writes a CSV.

  monitor.py record <out.csv> [interval_s]     run until SIGTERM
  monitor.py summary <in.csv>                  print peaks / averages

Signals collected every interval:
  - Postgres outbox: pending / published rows (=> insertion rate, relay backlog),
    projected ratings & comments (=> consumer progress), open connections
  - RabbitMQ: ready + unacked messages per queue (=> consumer lag), publish and
    deliver rates from the management API
  - Redis: ops/s, memory
  - CPU% and memory of every container (gateway, auth, interaction, ...)
"""
import base64, csv, json, os, signal, subprocess, sys, time, urllib.request

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
ENV = {}
for line in open(os.path.join(ROOT, ".env")):
    line = line.strip()
    if line and not line.startswith("#") and "=" in line:
        k, v = line.split("=", 1)
        ENV[k] = v

CONTAINERS = ["gateway", "auth", "interaction", "postgres", "rabbitmq", "redis", "k6"]
MQ = f"http://localhost:{ENV.get('RABBITMQ_UI_PORT', '15672')}"
QUEUES = {"ratings": "interaction.ratings", "comments": "interaction.comments"}
FIELDS = (
    ["t"]
    + [f"{c}_cpu" for c in CONTAINERS] + [f"{c}_mem_mb" for c in CONTAINERS]
    + ["outbox_pending", "outbox_published", "ratings_rows", "comments_rows", "pg_conns"]
    + [f"mq_{q}_{s}" for q in QUEUES for s in ("ready", "unacked", "dlq")]
    + ["mq_publish_rate", "mq_deliver_rate", "redis_ops", "redis_mem_mb"]
)


def sh(cmd, timeout=15):
    try:
        return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, timeout=timeout).stdout
    except Exception:
        return ""


def mb(s):
    s = s.strip().split("/")[0].strip()
    for unit, mul in (("GiB", 1024), ("MiB", 1), ("KiB", 1 / 1024), ("B", 1 / 1048576)):
        if s.endswith(unit):
            return float(s[: -len(unit)]) * mul
    return 0.0


def docker_stats():
    out = {}
    for line in sh(["docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}"], 20).splitlines():
        name, cpu, mem = line.split("|")
        for c in CONTAINERS:
            if name == (f"movieapp-{c}" if c == "k6" else f"movieapp-{c}-1"):
                out[f"{c}_cpu"] = float(cpu.strip("%") or 0)
                out[f"{c}_mem_mb"] = round(mb(mem), 1)
    return out


def postgres():
    q = ("select count(*) filter (where status='pending'), count(*) filter (where status='published'),"
         " (select count(*) from interaction.ratings), (select count(*) from interaction.comments),"
         " (select count(*) from pg_stat_activity where datname=current_database())"
         " from interaction.outbox_events")
    out = sh(["docker", "compose", "exec", "-T", "postgres", "psql", "-U", ENV["POSTGRES_USER"],
              "-d", ENV["POSTGRES_DB"], "-At", "-F,", "-c", q]).strip().split(",")
    if len(out) == 5:
        return dict(zip(["outbox_pending", "outbox_published", "ratings_rows", "comments_rows", "pg_conns"], out))
    return {}


def rabbit():
    auth = base64.b64encode(f"{ENV['RABBITMQ_USER']}:{ENV['RABBITMQ_PASSWORD']}".encode()).decode()

    def get(path):
        req = urllib.request.Request(MQ + path, headers={"Authorization": "Basic " + auth})
        return json.load(urllib.request.urlopen(req, timeout=5))

    out = {}
    try:
        for key, name in QUEUES.items():
            q = get(f"/api/queues/%2F/{name}")
            out[f"mq_{key}_ready"] = q.get("messages_ready", 0)
            out[f"mq_{key}_unacked"] = q.get("messages_unacknowledged", 0)
            out[f"mq_{key}_dlq"] = get(f"/api/queues/%2F/{name}.dlq").get("messages", 0)
        ov = get("/api/overview").get("message_stats", {})
        out["mq_publish_rate"] = round(ov.get("publish_details", {}).get("rate", 0), 1)
        out["mq_deliver_rate"] = round(ov.get("deliver_get_details", {}).get("rate", 0), 1)
    except Exception:
        pass
    return out


def redis():
    out = sh(["docker", "compose", "exec", "-T", "redis", "redis-cli", "-a", ENV["REDIS_PASSWORD"],
              "--no-auth-warning", "info"])
    kv = dict(l.strip().split(":", 1) for l in out.splitlines() if ":" in l)
    r = {}
    if "instantaneous_ops_per_sec" in kv:
        r["redis_ops"] = kv["instantaneous_ops_per_sec"]
    if "used_memory" in kv:
        r["redis_mem_mb"] = round(int(kv["used_memory"]) / 1048576, 1)
    return r


def record(path, interval):
    stop = []
    signal.signal(signal.SIGTERM, lambda *_: stop.append(1))
    signal.signal(signal.SIGINT, lambda *_: stop.append(1))
    t0 = time.time()
    with open(path, "w", newline="") as f:
        w = csv.DictWriter(f, FIELDS, restval="")
        w.writeheader()
        while not stop:
            started = time.time()
            row = {"t": round(started - t0)}
            for part in (docker_stats, postgres, rabbit, redis):
                row.update(part())
            w.writerow(row)
            f.flush()
            print(f"t={row['t']:>4}s gw_cpu={row.get('gateway_cpu','?'):>6} outbox_pending={row.get('outbox_pending','?'):>6} "
                  f"published={row.get('outbox_published','?'):>7} q_ratings={row.get('mq_ratings_ready','?')}+{row.get('mq_ratings_unacked','?')} "
                  f"q_comments={row.get('mq_comments_ready','?')}+{row.get('mq_comments_unacked','?')}", flush=True)
            time.sleep(max(0, interval - (time.time() - started)))


def summary(path):
    rows = list(csv.DictReader(open(path)))
    num = lambda r, k: float(r[k]) if r.get(k) not in (None, "") else None
    col = lambda k: [v for v in (num(r, k) for r in rows) if v is not None]
    t = col("t")
    print(f"samples: {len(rows)} over {int(t[-1]) if t else 0}s")
    print("\ncontainer            cpu% avg / peak     mem MB peak")
    for c in CONTAINERS:
        cpu, mem = col(f"{c}_cpu"), col(f"{c}_mem_mb")
        if cpu:
            print(f"  {c:<14} {sum(cpu)/len(cpu):>10.0f} / {max(cpu):<8.0f} {max(mem):>10.0f}")
    pend, pub = col("outbox_pending"), col("outbox_published")
    print("\nPostgres outbox")
    if pend:
        print(f"  pending rows        peak {max(pend):.0f}")
        # insertion rate = growth of (pending + published) between samples
        tot = [p + q for p, q in zip(pend, pub)]
        rates = [(tot[i] - tot[i - 1]) / (t[i] - t[i - 1]) for i in range(1, len(tot)) if t[i] > t[i - 1]]
        rel = [(pub[i] - pub[i - 1]) / (t[i] - t[i - 1]) for i in range(1, len(pub)) if t[i] > t[i - 1]]
        print(f"  insert rate         avg {sum(rates)/len(rates):.0f}/s  peak {max(rates):.0f}/s")
        print(f"  relay publish rate  avg {sum(rel)/len(rel):.0f}/s  peak {max(rel):.0f}/s")
        print(f"  total events        {tot[-1]:.0f}")
    print("\nRabbitMQ (consumer lag = ready + unacked)")
    for q in QUEUES:
        lag = [a + b for a, b in zip(col(f"mq_{q}_ready"), col(f"mq_{q}_unacked"))]
        dlq = col(f"mq_{q}_dlq")
        if lag:
            print(f"  {q:<10} lag peak {max(lag):.0f}  end {lag[-1]:.0f}   dead-lettered {dlq[-1] if dlq else 0:.0f}")
    for k, label in (("mq_publish_rate", "publish rate msg/s"), ("mq_deliver_rate", "deliver rate msg/s")):
        v = col(k)
        if v:
            print(f"  {label:<20} peak {max(v):.0f}")
    for k, label in (("redis_ops", "Redis ops/s"), ("redis_mem_mb", "Redis mem MB"), ("pg_conns", "PG connections")):
        v = col(k)
        if v:
            print(f"\n{label:<16} peak {max(v):.0f}")
    r, c = col("ratings_rows"), col("comments_rows")
    if r:
        print(f"projected into Postgres: {r[-1]:.0f} ratings, {c[-1]:.0f} comments")


if __name__ == "__main__":
    if len(sys.argv) >= 3 and sys.argv[1] == "record":
        record(sys.argv[2], float(sys.argv[3]) if len(sys.argv) > 3 else 5)
    elif len(sys.argv) == 3 and sys.argv[1] == "summary":
        summary(sys.argv[2])
    else:
        sys.exit(__doc__)
