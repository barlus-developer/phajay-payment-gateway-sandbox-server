# Development status

Recorded 2026-10-11. **The platform is implemented in the workspace but is not a fully validated delivery.** Frontend dependencies and the Bun lockfile cannot be completed without registry access. Browser/container acceptance checks and a new Git commit are blocked by the active environment.

## Scope and version control

The latest instruction requests one delivery and removes per-task commit gates. Work continued across all phases. The user's initial commit remains `0ec41c3` (`chore: initialize phajay sandbox project`). No remote push, destructive Git operation, or unrelated change was performed.

The session exposes `.git` read-only. Final command-line staging of the reviewed task files failed creating `.git/index.lock` with exit 128; current `git status`, diff inspection and diff whitespace checks work. No new commit hash exists. Git metadata was not relocated to bypass that restriction.

## Implemented source

- Go/Gin server, validated configuration, graceful shutdown, health/readiness, static SPA handling, API separation and streaming shutdown.
- Public-origin normalization matches browser hostname casing and default ports, preventing valid registration and CSRF-protected requests from being rejected by equivalent URL spellings; six regression cases pass.
- Domain/services/ports/adapters, exact fixed-point LAK amounts, raw `database/sql` SQLite repositories, WAL, per-connection foreign keys/busy timeout, atomic transactions and checksummed migrations.
- Argon2id registration/login, hashed sessions, cookies, CSRF/origin checks, bounded authentication rate limits, password changes and session revocation.
- Ownership-scoped projects/settings, one-time API secrets, hash verification/revocation/rotation, encrypted signing keys with rotation metadata.
- Six exact SDK sandbox routes, pending transaction creation, opaque QR URLs, checkout capability tokens, explicit atomic confirmation/failure/cancellation, idempotency, expiration and SSE state recovery.
- Durable webhook outbox, fenced leases, asynchronous worker, retries, restart recovery, sanitized responses, SSRF/DNS pinning and controlled local callback opt-in.
- Explicit managed SDK fork from commit `1ebf09402433d71a9b629b9573b3dce0818183c9`, sandbox-only HMAC verifier, unchanged production policy, original sources/tests/license and documented release requirements.
- React portal and simulator route source, real API data, project switching, management forms, QR rendering, camera/image/manual scanning, receipt and SSE/polling checkout. No mock production transaction data.
- OpenAPI 3.1, integration/signature/operations guides, pinned Bun/Go Docker stages, Compose persistent volume, CI, a Bun browser flow suite and Docker smoke script.

## Validation performed

| Check | Result |
| --- | --- |
| Main `go test ./... -race -cover -count=1` | Passed after actual-SDK QR/link, authorization, SSE, repository, service and security tests. |
| `go vet ./...`, `go build ./...` | Passed, including a fresh `CGO_ENABLED=0` production-server build. |
| Focused managed SDK signature/policy/decoder tests with race/coverage | Passed; production remains fail-closed. |
| Actual SDK all six sandbox operations | Passed through in-process Gin HTTP transport, including exact headers, response decoding, explicit confirmation and verified QR/link callbacks. |
| Concurrent confirmation | 32 confirmations return the completed state; one history completion and one durable delivery event. |
| Outbox rollback, migration checksum/rollback, recycled-connection pragmas, backup/reopen | Passed with real temporary SQLite files. |
| Webhook retry, stable event IDs, recovered leases and stale-worker fencing | Passed. |
| Payment expiration during blocked delivery; worker cancellation/shutdown | Passed. |
| Bun tests | Five tests pass for request cookies/CSRF/errors, exact amount formatting, token extraction, expiry calculations and image validation. These are utility tests, not browser coverage. |
| Frontend syntax formatting and cached dependency diagnostic | Passed parsing; type check now reports only missing TanStack Query and QR scanner modules. This is not a successful full build. |
| OpenAPI syntax, 344 schema references, six SDK paths | Checked; 39 documented paths. |
| Docker Compose configuration | Passed with a disposable test encryption key. |
| Bun Docker smoke script | Bundles successfully; not executed against containers. |
| Bun browser flow suite | Authored against a real Go server/SQLite; execution stops because Playwright is unavailable. No browser tests are claimed passed. |
| Git diff whitespace and generated/secret exclusions | Checked. |

Go checks use writable cache/temp paths under `/tmp` and offline module resolution. Available frontend packages were installed by Bun into a temporary diagnostic project using cached packages; required uncached libraries were not substituted or faked. The workspace's ignored `web/node_modules` points to that temporary diagnostic installation and is not deployable or committed. Browser test source also parses through Bun bundling with unresolved dependencies left external; that does not validate browser execution.

## Actual environment blockers

1. `bun install` reports `DNSResolveFailed` for registry manifests. Offline installation cannot resolve `@tanstack/react-query`, `qr-scanner` or the added `playwright` browser test dependency, which are absent from cache. Consequently no genuine `web/bun.lock` exists. `bun run build` stops at the missing application modules; `bun run test:e2e` stops at missing Playwright. The Dockerfile deliberately requires the lockfile and frozen installation.
2. Docker build fails connecting to `/var/run/docker.sock`: permission denied. Image/startup/persistence/recovery smoke checks cannot run here.
3. The original complete SDK suite panics creating its `httptest` TCP listener (`socket: operation not permitted`). Live application startup and browser tests likewise need local socket access. The main integration tests avoid listeners through an actual SDK HTTP transport.
4. `.git` metadata remains read-only under this session's filesystem policy; a new commit cannot be created here.
5. `go mod tidy` additionally attempts uncached dependency-test tools and fails on the read-only module cache. Main application tests/build/vet pass; a connected environment should rerun tidy and review any resulting dependency cleanup.

## Remaining acceptance work

Complete Bun installation, generate/review its only lockfile, run full lint/build, install Chromium, and run the authored browser flow suite for auth, switching projects, keys, webhooks, scanning, confirmation, transaction details and realtime recovery. Resolve actual build/browser failures and expand coverage as required. Run the full original SDK suite, CI and Docker smoke script, test real camera permissions/scanning on HTTPS/mobile, confirm container upgrades/backups, then commit the reviewed complete delivery using command-line Git. No remaining acceptance criterion is claimed as passed without that evidence.

## Skills and SDK investigation

Used the requested local find-skills workflow and read Superpowers using-superpowers, brainstorming, TDD and verification instructions from official obra sources; CLI skill installation was blocked by DNS. Supporting local Golang design-patterns/hexagonal and shadcn skills were read and applied. Vercel React guidance was discovered but not installed. Existing local shadcn-generated Base UI components were reused; registry component addition through Bun CLI was blocked by DNS.

SDK source is the primary compatibility reference. Documentation differences and selected behavior are recorded in SDK_COMPATIBILITY.md. The original sibling SDK repository was left unchanged. The managed fork is explicit in go.mod and sdk-fork/FORK.md; consumers need that fork or an upstream release for SDK-managed sandbox verification.
