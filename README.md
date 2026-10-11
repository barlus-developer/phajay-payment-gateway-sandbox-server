# Phajay Sandbox Platform

A self-hosted payment simulator with Go/Gin, manually written SQLite SQL,
React/TypeScript, Bun, Vite, Tailwind CSS v4, shadcn/ui, React Router and
TanStack Query. All payments are simulations. It has no banking connections,
production payment routes, or real-money operations.

**Current validation limits:** backend race tests, actual-SDK integration tests
and Bun utility tests pass. Registry DNS, Docker daemon access and local TCP
listeners are restricted in the development session. Frontend dependencies,
`web/bun.lock`, browser build and container acceptance checks remain blocked.
This checkout is not yet a fully validated deployment. See
[development status](docs/DEVELOPMENT_STATUS.md) for evidence and remaining work.

## Run locally

Use Go 1.27.1 and Bun 1.4.2. Copy `.env.example` to `.env`, set a stable
32-byte base64 `MASTER_KEY` generated with `openssl rand -base64 32`, and load
the file into your shell environment. Never commit that file or change the key
across restarts. Build frontend assets:

```bash
cd web
bun install
bun run lint
bun run test
bun run build

# Browser suite (requires Chromium and local TCP listeners):
bun --bun playwright install chromium
bun run test:e2e
cd ..
go run ./cmd/server
```

Open `http://localhost:8080/register`, register, create a project, generate an
API key and signing key, and register your callback destination. For Vite
development use `bun run dev` in `web/` and run the backend with
`PUBLIC_URL=http://localhost:5173` so Origin and CSRF checks match the browser.
Use HTTPS on mobile for camera scanning. Scanner image upload and manual entry
work without camera access.

## Docker

After successful frontend installation creates the reviewed Bun lockfile:

```bash
docker compose up --build -d
docker compose logs -f app
```

The final non-root container runs only the Go binary. A named volume stores
SQLite. A reverse proxy terminates HTTPS. See [deployment and safe WAL backups](docs/DEPLOYMENT.md)
and [configuration](docs/CONFIGURATION.md).

## SDK usage

The upstream SDK's production signing specification is private and its current
verification fails closed. This platform keeps a clearly identified
[managed SDK fork](sdk-fork/FORK.md), with default-enabled sandbox verification
only when `WithSandbox()` is selected. The original sibling repository was
not modified. Use an explicit module replacement until the patch is released:

```go
// Consumer go.mod:
// replace github.com/barlus-developer/phajay-payment-gateway => /path/to/platform/sdk-fork

client := phajay.New(phajay.Config{
    APIKey: os.Getenv("PHAJAY_SANDBOX_API_KEY"),
    BaseURL: "http://localhost:8080",
    WebhookSignatureKey: os.Getenv("PHAJAY_SANDBOX_SIGNING_KEY"),
    BypassSignatureVerification: false,
}, phajay.WithSandbox())

qr, err := client.GenerateQR(ctx, phajay.BankJDB, phajay.PaymentQRRequest{
    Amount: 12500.50, Description: "Integration test",
})
// Handle err, encode qr.QRCode as a QR image, and scan it at /sandbox/scan.

link, err := client.CreatePaymentLink(ctx, phajay.PaymentLinkRequest{
    OrderNo: "order-001", Amount: 12500.50, Description: "Link payment",
})
// Handle err and open link.RedirectURL.
```

All five banks are supported: JDB, LDB, IB, BCEL and STB. Link checkout uses a
JDB simulator. `qrCode` contains only an opaque sandbox URL, never a bank-transfer
payload. Amounts are validated exactly as LAK with scale two and stored as
integer minor units; excess fractional precision is rejected, not rounded.
Orders are unique per project. Production paths return 404.

Scan → review → explicitly confirm. Scanning never completes a payment.
Confirmation, transaction history and the durable webhook outbox commit in
one SQLite transaction. Concurrent confirmation creates one completion event;
repeated successful confirmation returns the same completed state. Other
terminal states cannot be overwritten. Checkout uses token-authorized SSE and
current-state retrieval after reload/reconnection.

## Signed callbacks

Callbacks use HMAC-SHA256 over `{timestamp}.{rawRequestBody}` and three headers:
`X-Phajay-Sandbox-Timestamp`, `X-Phajay-Sandbox-Signature` and
`X-Phajay-Sandbox-Event-ID`. Signatures are lowercase hexadecimal prefixed by
`v1=`. Verification requires a separate project signing key and a five-minute
timestamp tolerance. Signing secrets are AES-GCM encrypted in SQLite using
the external master key. Rotation has a five-minute consumer overlap.

Use `client.DecodeWebhook(rawBody, r.Header, &callback)` before processing.
QR callbacks use `PaymentQRWebhookCallback`; link callbacks use
`SandboxPaymentLinkWebhookCallback`, preserving `descriptione` and `updateAt`.
Delivery is at least once: record event IDs and apply payment operations
idempotently, also deduplicating by transaction ID. Automatic retries use
exponential backoff and durable leases; manual retries preserve the event ID.
See [signature protocol](docs/SIGNATURE.md) and [SDK contracts](docs/SDK_COMPATIBILITY.md).

## Architecture and APIs

`internal/domain` contains entities, amounts and state rules; `internal/ports`
defines persistence/delivery dependencies; `internal/service` implements
business operations; SQLite, webhook transport and Gin live in adapters.
All queries are handwritten, parameterized `database/sql` calls. SQLite uses
WAL, per-connection foreign keys/busy timeout and a one-connection writer pool.
Checksummed migrations run automatically and never reset data.

Portal APIs use `/api/internal/v1`, cookie sessions and CSRF headers. Every
project operation validates ownership. SDK APIs expose only the six documented
sandbox paths. [OpenAPI](docs/openapi.yaml) describes both sets and the public
opaque-token simulator APIs. Portal pages are protected by server and React
authentication checks. API credentials and request bodies are excluded from logs.

## Checks

```bash
go test ./... -race -cover
go vet ./...
go build ./...
cd sdk-fork
go test ./... -race -cover
go vet ./...
cd ../web
bun run lint
bun run test
bun run build
```

The main integration tests run the real SDK against Gin using an in-process
HTTP transport. They cover exact headers/routes, decoding, QR/link callbacks,
explicit confirmation, CSRF, project authorization, and SSE. SQLite/service
tests cover migrations, backups, session revocation, encrypted secrets,
concurrency, terminal states, expiration, retries and restart recovery.
The Bun browser suite covers authentication, project switching, one-time key
reveal/rotation/revocation, webhook configuration/history, manual and image
scanning, explicit confirmation, transaction details and checkout recovery.
It uses a real Go server and disposable SQLite database. It is authored but
could not run here because Playwright is also absent from cache.
The original SDK listener tests and browser/container checks need an unrestricted
local test environment. Do not interpret utility tests as browser coverage.

No automatic push or destructive Git operation is performed. The latest user
instruction requests one delivery rather than per-task commit gates. Git history
currently contains the user's initial commit; this session cannot write `.git`.
