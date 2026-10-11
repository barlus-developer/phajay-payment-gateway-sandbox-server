# syntax=docker/dockerfile:1
FROM oven/bun:1.4.2 AS frontend
WORKDIR /build/web
COPY web/package.json web/bun.lock* ./
RUN test -f bun.lock || (echo 'Generate web/bun.lock with bun install before building' >&2; exit 1)
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run test && bun run build

FROM golang:1.27.1-bookworm AS backend
WORKDIR /build
COPY go.mod go.sum ./
COPY sdk-fork/ ./sdk-fork/
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot AS production
WORKDIR /app
COPY --from=backend --chown=65532:65532 /out/server /app/server
COPY --from=frontend --chown=65532:65532 /build/web/dist /app/web/dist
COPY --chown=65532:65532 migrations/ /app/migrations/
COPY --chown=65532:65532 docs/openapi.yaml /app/docs/openapi.yaml
# Gives a fresh Docker named volume its non-root ownership.
COPY --chown=65532:65532 docker/data/ /app/data/
ENV HTTP_ADDR=:8080 DATABASE_PATH=/app/data/phajay.db WEB_DIST_DIR=/app/web/dist MIGRATIONS_DIR=/app/migrations
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=20s --timeout=5s --start-period=15s --retries=3 CMD ["/app/server", "-healthcheck"]
ENTRYPOINT ["/app/server"]
