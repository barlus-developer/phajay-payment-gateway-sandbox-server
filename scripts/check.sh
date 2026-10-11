#!/bin/sh
set -eu
go test ./... -race -cover
go vet ./...
go build ./...
(cd sdk-fork && go test ./... -race -cover && go vet ./...)
(cd web && bun install --frozen-lockfile && bun run lint && bun run test && bun run build)
(cd web && bun --bun playwright install chromium && bun run test:e2e)
docker compose build
