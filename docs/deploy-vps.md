# Deploying to a VPS (demo)

Target: `https://schoolsys.orisystems.site`, on a VPS that already runs Nginx on ports 80/443.

## How it works

```
merge to main ──► GitHub Actions
                   1. CI: Go vet + tests (Postgres service), web type-check + build
                   2. build images ──► ghcr.io/putodoho/nova-vida-sms-api:<sha>
                                       ghcr.io/putodoho/nova-vida-sms-web:<sha>
                   3. SSH to the VPS: copy compose file + Caddyfile to /opt/nova-vida,
                      docker compose pull && up -d, seed (idempotent), health check
Internet ──► Nginx :443 (TLS, certbot) ──► 127.0.0.1:8088 (web container: static app + /api proxy)
                                              └─► api ──► postgres, gotenberg (internal network only)
```

Nothing but `127.0.0.1:8088` is published on the host; Postgres is never exposed. Pull requests only run CI.

## One-time setup on the VPS

Run as root (or with sudo).

1. **Docker** with the compose plugin (`docker compose version` must work). If missing: `curl -fsSL https://get.docker.com | sh`.
2. **Deploy user** with SSH key login, allowed to use Docker:
   ```sh
   adduser --disabled-password --gecos "" deploy
   usermod -aG docker deploy
   mkdir -p /opt/nova-vida && chown deploy:deploy /opt/nova-vida
   # key pair used only by GitHub Actions
   sudo -u deploy ssh-keygen -t ed25519 -N "" -f /home/deploy/.ssh/gh_actions -C gh-actions
   sudo -u deploy sh -c 'cat ~/.ssh/gh_actions.pub >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys'
   cat /home/deploy/.ssh/gh_actions        # private key -> GitHub secret VPS_SSH_KEY
   ```
3. **Environment file** `/opt/nova-vida/.env` (see `deploy/.env.vps.example`):
   ```sh
   sudo -u deploy tee /opt/nova-vida/.env >/dev/null <<EOT
   POSTGRES_PASSWORD=$(openssl rand -hex 24)
   APP_ENV=demo
   FAKE_TODAY=2027-05-12
   APP_PORT=8088
   SESSION_TTL=12h
   IMAGE_TAG=latest
   EOT
   chmod 600 /opt/nova-vida/.env
   ```
   Do not change `POSTGRES_PASSWORD` after the first start: the database volume keeps the original one.
   If port 8088 is taken, pick another and use it in the Nginx config too.
4. **Nginx site** — copy `deploy/nginx-schoolsys.conf` to `/etc/nginx/sites-available/schoolsys.orisystems.site`, then:
   ```sh
   ln -s /etc/nginx/sites-available/schoolsys.orisystems.site /etc/nginx/sites-enabled/
   nginx -t && systemctl reload nginx
   certbot --nginx -d schoolsys.orisystems.site     # adds the 443 block and the http->https redirect
   ```

## One-time setup on GitHub

Repository → Settings → Secrets and variables → Actions → New repository secret:

| Secret | Value |
| --- | --- |
| `VPS_HOST` | VPS IP or host name |
| `VPS_USER` | `deploy` |
| `VPS_SSH_KEY` | contents of `/home/deploy/.ssh/gh_actions` (the private key, including the BEGIN/END lines) |
| `VPS_PORT` | only if SSH is not on 22 |

Settings → Environments: the `demo` environment is created on the first run. Add required reviewers there if deploys should wait for approval.

## First deploy

1. Merge to main (or Actions → Deploy → Run workflow).
2. On the VPS, create the first user:
   ```sh
   cd /opt/nova-vida
   docker compose -f docker-compose.vps.yml run --rm --entrypoint /app/cli api \
     create-admin -login admin@novavida.co.mz -name "Administrador" -password '<strong password>'
   ```
3. Optional demo data, from any machine with Python 3:
   ```sh
   DEMO_LOGIN=admin@novavida.co.mz DEMO_PASSWORD='<password>' \
     python3 docs/samples/demo_data.py https://schoolsys.orisystems.site/api/v1
   ```

## Day to day

```sh
cd /opt/nova-vida
C="docker compose -f docker-compose.vps.yml"
$C ps                       # status
$C logs -f api              # API logs
$C restart api              # restart (keeps the deployed IMAGE_TAG)
```

- **Roll back**: Actions → Deploy → pick an older successful run → Re-run all jobs (re-deploys that commit's images), or set `IMAGE_TAG=<older sha>` in `.env` and `$C up -d`.
- **Reset the demo** (deletes all data): `$C down -v`, then the next deploy (or `$C up -d` + seed) recreates an empty database; repeat create-admin and the demo data.
- **Backup**: `$C exec -T postgres pg_dump -U novavida novavida | gzip > nova-vida-$(date +%F).sql.gz`.

## Moving from demo to real use

Set `APP_ENV=prod`, remove `FAKE_TODAY` (the API refuses to start with it in prod), reset the database, and set up scheduled backups (milestone 8).
