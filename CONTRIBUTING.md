# Contributing

This project is small enough to move quickly, but it touches host-level resources, Docker, auth, and a live UI. The safest way to contribute is to keep changes narrow and verify them from a clean start.

## Prerequisites

- Linux host
- Go `1.25+`
- Bun `1.x`
- Docker and Docker Compose
- Optional for security features: Fail2Ban on the host

## Development Setup

### Docker-first workflow

```bash
cp .env.example .env
docker compose up --build
docker exec -it dashboard /app/user-cli create
```

Use this when you want the app to run close to production, with host mounts and Docker socket access.

### Local split workflow

Terminal 1:

```bash
set -a
source .env
set +a
go run ./cmd/dashboard
```

Terminal 2:

```bash
cd frontend
bun install
bun run dev
```

Notes:

- The Go app does not auto-load `.env` files.
- The Vite dev server proxies `/api` and `/ws` to `http://localhost:4200`.
- The local split workflow is better for frontend iteration.

## Verification Commands

Run these before opening a PR:

```bash
go vet ./...
go test ./...
go build ./...
cd frontend && bun install && bun run build
docker build .
```

## Project Structure

```text
cmd/dashboard/           Main application entrypoint
internal/auth/          Session auth, cookie validation, user context helpers
internal/collectors/    System, Docker, Fail2Ban, and log data sources
internal/config/        Environment-backed runtime configuration
internal/db/            SQLite connection setup and schema migrations
internal/server/        HTTP handlers, auth wrapper, CORS, WebSocket hub
frontend/src/           Svelte routes, components, API client, live stores
scripts/user-cli.go     User administration CLI
```

## How To Add a New Collector

1. Create a new collector in `internal/collectors/`.
2. Keep host interactions isolated inside that collector.
3. Return strongly shaped structs with JSON tags if the data will be exposed to the frontend.
4. Wire the collector into `cmd/dashboard/main.go`.
5. Add a handler in `internal/server/server.go` if the data needs a REST endpoint.
6. If it belongs in live updates, include it in the WebSocket broadcast payload in `internal/server/ws.go`.
7. Add focused Go tests in `internal/collectors/<name>_test.go`.

Mental model: collectors gather data, server handlers expose it, frontend consumes it.

## How To Add a New Frontend Page

1. Add a route under `frontend/src/routes/`.
2. Use the shared API client in `frontend/src/lib/api.ts` for REST calls.
3. Use `frontend/src/lib/ws.ts` only when the page needs live updates.
4. Reuse existing components where possible, especially table and metric display patterns.
5. Add the page to the `navItems` array in `frontend/src/routes/+layout.svelte` if it should be visible in the sidebar.
6. Run `bun run build` to catch SvelteKit or TypeScript issues.

## Commit and PR Conventions

Recommended workflow:

1. One issue per branch
2. One issue per PR
3. As many commits as needed while building
4. Squash on merge if you want a clean `main` history

Suggested branch naming:

```text
issue-10-docs-ci
```

Suggested commit style:

```text
docs: expand project documentation
ci: add GitHub Actions validation workflow
```

When possible, reference the issue in the PR description with a closing keyword such as:

```text
Closes #10
```

## Pull Request Checklist

- Change is scoped to one issue
- Docs updated if behavior or setup changed
- `go test ./...` passes
- `bun run build` passes
- `docker build .` still succeeds
- Any new route or API shape is documented in [docs/api.md](docs/api.md)
