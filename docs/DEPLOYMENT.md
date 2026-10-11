# Deployment and operations

## Required validation before hosting

Frontend dependency installation, the full browser build, and Docker execution could not be completed in the restricted development session. Generate `web/bun.lock` through a successful `bun install`, review it, and commit it before the frozen Docker build. Do not fabricate a lockfile or skip frozen installation. See DEVELOPMENT_STATUS.md for exact validation limits.

Use Bun 1.4.2 and Go 1.27.1. From `web/`, run `bun install`, `bun run lint`, `bun run test`, and `bun run build`. Then run `sh scripts/check.sh` in an environment that permits local listeners, registry downloads, and Docker access. The final image serves static React assets from a single Go process with no JavaScript runtime.

## Start

Copy `.env.example` to `.env`. Generate `MASTER_KEY` using `openssl rand -base64 32` and put its value in `.env`. Preserve this key across upgrades and restores. Changing it makes stored payment tokens and webhook secrets undecryptable. Set `PUBLIC_URL` to the exact externally visible origin.

Run `docker compose up --build -d`. The application runs as UID 65532 with a read-only root filesystem, writable SQLite named volume, and temporary filesystem. Do not use `docker compose down -v` when preserving data. Ordinary restarts, recreation and redeployment reuse `sqlite_data`. Use one application instance; shared network filesystems and multiple writers are unsupported.

Startup applies pending checksummed SQL migrations. Never edit applied files. Add new migrations for schema changes. `/healthz` checks the process; `/readyz` checks SQLite. SIGTERM cancels workers/SSE and drains HTTP requests before closing SQLite. Interrupted delivery is retried after its durable lease expires; consumers must be idempotent.

## HTTPS

Bind the container to localhost as provided. Put Caddy or Nginx on the host in front of port 8080. Example Caddyfile:

```caddy
sandbox.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Set `PUBLIC_URL=https://sandbox.example.com`; secure cookies and HSTS activate automatically. Trust only actual proxy IPs/CIDRs through `TRUSTED_PROXIES`. Proxy streaming must remain enabled for SSE. For Nginx, use `proxy_buffering off`, preserve Host, and set `X-Forwarded-Proto` appropriately. Mobile camera scanning requires HTTPS. Token URLs use `Referrer-Policy: no-referrer`; avoid recording query strings or simulator token paths in proxy access logs.

## Local callbacks

Production requires public HTTPS destinations. For controlled development set both `APP_ENV=development` and `ALLOW_LOCAL_WEBHOOKS=true`; HTTP, loopback and private-network destinations then become permitted. Metadata, link-local, multicast and reserved networks remain blocked. DNS answers are revalidated and pinned at connection time. Redirects and environment proxies are disabled. Container callback URLs must be reachable from inside the container; `localhost` refers to the container itself.

## Consistent SQLite backup

Use the built-in `VACUUM INTO` snapshot while the application runs:

```bash
docker compose exec app /app/server -backup /app/data/backup-2026-10-11.db
# Obtain the ID with docker compose ps -q app, then:
docker cp CONTAINER_ID:/app/data/backup-2026-10-11.db ./backup.db
```

The destination must be a new filename. Keep snapshots, stable MASTER_KEY, application version and migration history together in secure backup storage. Snapshots include committed data consistently in WAL mode. Copying only an active database file without its WAL is not a backup procedure.

Stop the application before restoration. Restore the snapshot as `phajay.db` in the named volume using an administrator-controlled helper container. Remove stale `phajay.db-wal`/`phajay.db-shm` only while all database processes are stopped. Ensure UID 65532 owns the database/directory. Restore the same MASTER_KEY. Start the matching application version, check readiness and existing project data, then upgrade through new migrations. Test restorations regularly.

## Docker acceptance checks

Build and check health, frontend assets, registration, SDK payments, signed callbacks, and camera/image scanning. Create a project before restarting and verify it remains after `docker compose restart` and after `down` followed by `up -d`. Stop during queued delivery and verify recovery with the same event ID. Repository tests exercise persistence/recovery in temporary SQLite databases; they do not substitute for container checks.

`bun run scripts/docker-smoke.ts` builds an isolated Compose project and checks
assets, startup migrations, authentication, explicit confirmation, persistence
after recreation, and retry recovery. It removes only its own disposable test
volume. It requires Docker access and a successfully generated Bun lockfile.
The CI workflow also pins Bun and runs backend, SDK and frontend checks before
that smoke test. Camera permissions and actual mobile scanning still need
browser/device validation.
