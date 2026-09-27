# Deploying KinoCut ⇄ KinoShow

One Linux server runs everything with Docker Compose. Caddy is the only thing
exposed to the internet and gets HTTPS certificates automatically:

```
https://DOMAIN      ─┐            ┌─ web (Next.js)
                     ├─ Caddy ────┤
https://api.DOMAIN  ─┘            └─ gateway ─ auth · catalog · interaction · watchparty
                                                  └─ Postgres · Redis · RabbitMQ (private)
```

`docker-compose.prod.yml` layers the production settings over `docker-compose.yml`:
no published ports except Caddy's 80/443, `APP_ENV=production` (the services
refuse unsafe settings, e.g. a non-Secure session cookie), CORS and WebSocket
origins locked to your domain, the real client IP for rate limiting, and a
nightly database backup.

## 1. What you need

- A domain, e.g. `kinocut.com`.
- A server: Ubuntu 24.04, 2 vCPU / 4 GB RAM is plenty (e.g. Hetzner CX22).
- Your API keys: TMDB (required), OMDb and Groq (optional).

## 2. DNS

Point both names at the server's IPv4 address (A records), and wait until
`dig +short kinocut.com` and `dig +short api.kinocut.com` return it:

| Name  | Type | Value     |
|-------|------|-----------|
| `@`   | A    | server IP |
| `api` | A    | server IP |

## 3. Server setup (once)

```bash
ssh root@SERVER_IP
curl -fsSL https://get.docker.com | sh
ufw allow OpenSSH && ufw allow 80,443/tcp && ufw allow 443/udp && ufw --force enable
git clone https://github.com/furkanpatat/MovieApp.git /opt/kinocut && cd /opt/kinocut
scripts/prod-env.sh kinocut.com                   # strong random secrets -> .env (mode 600)
nano .env                                         # paste TMDB_API_KEY, OMDB_API_KEY, GROQ_API_KEY
```

The data stores publish no ports in production, so the firewall only has to
allow SSH and HTTP(S).

### Free: Oracle Cloud Always Free + DuckDNS

What the live demo runs on, at no cost:

- **Server:** an Always Free `VM.Standard.A1.Flex` (Arm, Ubuntu 24.04). Create
  the network first with *Networking → VCN Wizard → Create VCN with Internet
  Connectivity* (a plain "Create VCN" has no subnet or internet gateway), then
  pick its **public** subnet and *Automatically assign public IPv4 address*.
- **Ports:** add ingress rules for TCP 80 and 443 (source `0.0.0.0/0`) to the
  subnet's *Default Security List*. Oracle's Ubuntu image also has its own
  iptables `REJECT` rule, so skip `ufw` there and open the ports on the host:

  ```bash
  for rule in "-p tcp --dport 80" "-p tcp --dport 443" "-p udp --dport 443"; do
    sudo iptables -I INPUT 5 $rule -m state --state NEW -j ACCEPT
  done
  sudo netfilter-persistent save
  ```

- **Domain:** a free `NAME.duckdns.org` pointed at the server's public IP;
  `api.NAME.duckdns.org` resolves to it automatically. Log in as `ubuntu`
  (use `sudo`), then `scripts/prod-env.sh NAME.duckdns.org`.

## 4. Start

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps   # everything "healthy"
```

The database schema (`deployments/postgres/init.sql`) is applied on the first
boot. Caddy requests the certificates within a minute of DNS resolving.

Check it: `curl -I https://kinocut.com` and `curl https://api.kinocut.com/healthz`
return 200; sign up, and open a Watch Party from two devices.

Tip: `alias dc='docker compose -f docker-compose.yml -f docker-compose.prod.yml'`.

## 5. Updates

```bash
cd /opt/kinocut && git pull
dc up -d --build          # rebuilds what changed, restarts only those services
```

A schema change goes into `deployments/postgres/init.sql` (idempotent) and is
applied to the running database with
`dc exec -T postgres psql -U movieapp -d movieapp < deployments/postgres/init.sql`.

Rollback: `git checkout <previous commit> && dc up -d --build`.

## 6. Backups

The `backup` service writes a `pg_dump` to `./backups` every night and keeps 14
days (`BACKUP_KEEP_DAYS`). Copy them off the server as well, for example to your
own machine:

```bash
rsync -av root@SERVER_IP:/opt/kinocut/backups/ ./kinocut-backups/
```

Restore a dump:

```bash
dc exec -T postgres pg_restore -U movieapp -d movieapp --clean --if-exists < backups/movieapp-YYYYMMDD-HHMMSS.dump
```

## 7. Monitoring (optional)

```bash
dc --profile observability up -d
ssh -L 3000:127.0.0.1:3000 root@SERVER_IP    # then open http://localhost:3000 (Grafana)
```

Grafana and Prometheus listen on the server's loopback only; reach them
through the SSH tunnel. The Grafana admin password is `GRAFANA_ADMIN_PASSWORD`
in `.env`.

## Rehearse locally

The same stack runs on your machine with `DOMAIN=localhost` (Caddy issues
local certificates; your browser will warn once):

```bash
printf 'APP_ENV=production\nDOMAIN=localhost\n' > /tmp/rehearsal.env
docker compose -p kinoprod --env-file .env --env-file /tmp/rehearsal.env \
  -f docker-compose.yml -f docker-compose.prod.yml up -d --build
# https://localhost and https://api.localhost/healthz
docker compose -p kinoprod down -v    # clean up
```
