# Environment configuration

`internal/config.Load()` reads the process environment and returns validated,
typed application settings. It does not load `.env` files or start the server.
Docker Compose or the local shell must supply environment variables. The server
validates these settings before opening SQLite, running migrations and starting
the HTTP server and webhook workers.

Copy `.env.example` to `.env` for local configuration. Set `MASTER_KEY` to a
base64-encoded, cryptographically random 32-byte value, for example from
`openssl rand -base64 32`. There is no built-in key and startup configuration
fails when it is missing, malformed, or the wrong length. Keep the key outside
SQLite and retain it securely with backups; encrypted signing secrets will
require the same key after a restore. Do not log the configuration struct.

| Variable | Default | Rules |
| --- | --- | --- |
| `APP_ENV` | `production` | `production` or `development`; this controls operational safety, never production payment support. |
| `HTTP_ADDR` | `:8080` | Listening host and port; port must be 1–65535. |
| `PUBLIC_URL` | `http://localhost:8080` | Public HTTP(S) origin; no credentials, subpath, query, or fragment. Hostnames are lowercased and default ports omitted to match browser Origin headers. HTTPS derives Secure cookies even behind a reverse proxy. |
| `DATABASE_PATH` | `data/phajay.db` | Persistent file path; SQLite URIs and in-memory databases are rejected. Compose mounts the persistent volume at `/app/data` and uses `/app/data/phajay.db`. |
| `WEB_DIST_DIR` | `web/dist` | Compiled SPA directory. |
| `MIGRATIONS_DIR` | `migrations` | Manual SQL migration directory. |
| `MASTER_KEY` | Required | Base64 encoding of exactly 32 bytes; never returned in JSON serialization. |
| `TRUSTED_PROXIES` | Empty | Comma-separated IPs/CIDRs for actual reverse proxies. Wildcards, hostnames, and `/0` networks are rejected. |
| `ALLOW_LOCAL_WEBHOOKS` | `false` | Local callback opt-in accepted only with `APP_ENV=development`. The webhook adapter still validates destinations and pins resolved IPs during delivery. |
| `SESSION_TTL` | `24h` | Between 5 minutes and 30 days. |
| `PAYMENT_TTL` | `15m` | Between 1 minute and 24 hours. |
| `WEBHOOK_TIMEOUT` | `10s` | Between 1 and 30 seconds. |
| `WEBHOOK_MAX_ATTEMPTS` | `8` | Between 1 and 12. |
| `SHUTDOWN_TIMEOUT` | `15s` | Between 1 second and 1 minute. |

Unset or empty optional variables use defaults. The encryption key always
requires an explicit value. Configuration errors identify the offending
variable without echoing secret values. Invalid configuration returns no
partially usable settings. See [deployment instructions](DEPLOYMENT.md) for
reverse proxy setup, persistent storage, graceful shutdown and safe backups.
