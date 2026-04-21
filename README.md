# Dashboard

Dashboard is a self-hosted Linux server panel built with a Go backend, a Svelte frontend, SQLite for auth/session storage, and WebSockets for live metric updates. It focuses on a small set of operational views that are useful on a single host: system health, Docker containers, Fail2Ban status, and recent log activity.

## What It Does

- Shows live CPU, memory, disk, swap, uptime, and network activity.
- Lists Docker containers and streams runtime stats for running containers.
- Displays Fail2Ban jail state and recent ban/unban events.
- Surfaces recent system log entries with unit and priority filters.
- Uses cookie-based session auth backed by SQLite.

## Interface Overview

The current UI has two main views:

- `Overview`: system gauges, CPU history, network bandwidth history, and Docker container status.
- `Security`: Fail2Ban jail summaries, recent ban events, and filtered system logs.

## Architecture

```mermaid
flowchart LR
  Browser[Browser\nSvelteKit SPA] -->|REST + cookie auth| API[Go HTTP server]
  Browser -->|WebSocket /ws| API
  API --> SQLite[(SQLite\nusers + sessions)]
  API --> Proc[/proc]
  API --> DockerSock[/var/run/docker.sock]
  API --> Logs[/var/log]
  API --> F2B[fail2ban-client]
  API --> Static[Built frontend files]
```

## Tech Stack Rationale

- `Go`: small runtime, simple deployment, strong standard library, good fit for polling host resources and serving HTTP/WebSocket traffic from one binary.
- `Svelte 5 + SvelteKit`: very small frontend footprint, straightforward reactivity, easy static build output for embedding in the Go app.
- `SQLite`: enough for a local dashboard that only needs durable users and sessions, with almost no operational overhead.
- `Tailwind CSS 4`: quick UI iteration with a compact design token layer in `frontend/src/app.css`.
- `ECharts + custom SVG`: time-series charts use ECharts for richer tooltips and scaling, while simple gauge widgets stay custom SVG to keep them lightweight.
- `WebSockets`: avoids polling from the browser for live CPU/network/container updates.

## Requirements

- Linux host.
- Go `1.25+`.
- Bun `1.x` for frontend development/builds.
- Docker and Docker Compose for containerized usage.
- Optional but recommended for the security page: Fail2Ban installed on the host.

The container setup is Linux-oriented because it relies on:

- host networking
- `/proc` mounts
- `/var/log` mounts
- Docker socket access
- `fail2ban-client` inside the runtime container

## Quick Start

### 1. Configure environment

Copy the example environment file if you want Docker Compose to pick up overrides automatically:

```bash
cp .env.example .env
```

### 2. Start the app with Docker Compose

```bash
docker compose up --build
```

The app listens on `http://localhost:4200` by default.

### 3. Create the first user

In a second terminal:

```bash
docker exec -it dashboard /app/user-cli create
```

Then sign in through the browser.

## Local Development

### Backend

The Go app does not load `.env` files by itself. For local runs, export variables from `.env` first if you want something explicit and reproducible.

```bash
set -a
source .env
set +a
go run ./cmd/dashboard
```

Useful alternatives:

```bash
go build -o dashboard ./cmd/dashboard
./dashboard
```

To manage local users without Docker:

```bash
go run ./scripts/user-cli.go create
go run ./scripts/user-cli.go list
```

### Frontend

In another terminal:

```bash
cd frontend
bun install
bun run dev
```

The Vite dev server runs on `http://localhost:5173` and proxies `/api` and `/ws` to the Go backend on port `4200`.

### Production frontend build

```bash
cd frontend
bun install
bun run build
```

The static bundle is emitted into `frontend/build`, which the Go server serves via `PUBLIC_DIR`.

## Environment Variables

All current variables from `.env.example` are listed below.

| Variable | Default | Purpose |
| --- | --- | --- |
| `HOST` | `0.0.0.0` | HTTP listen host for the Go server. |
| `PORT` | `4200` | HTTP listen port for the Go server. |
| `DB_PATH` | `./data/dashboard.sqlite` locally, `/app/data/dashboard.sqlite` in Docker | SQLite database path. |
| `PROC_PATH` | `/proc` locally, `/host/proc` in Docker | Source for uptime, CPU, memory, and network metrics. |
| `LOG_PATH` | `/var/log` locally, `/host/log` in Docker | Source directory for security/system log parsing. |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Docker Engine Unix socket used for container listing and stats. |
| `ALLOWED_ORIGINS` | `http://localhost:4200,http://127.0.0.1:4200,http://localhost:5173,http://127.0.0.1:5173` | Comma-separated CORS and WebSocket origins. |
| `COOKIE_SECURE` | `false` | Marks the session cookie as `Secure` when set to `true`. |
| `SESSION_TTL` | `2592000` | Session lifetime in seconds. Default is 30 days. |
| `PUBLIC_DIR` | `./frontend/build` locally, `/app/frontend/build` in Docker | Directory served for the SPA bundle. |

## API Reference

The backend exposes REST endpoints under `/api/v1` and a cookie-authenticated WebSocket endpoint at `/ws`.

- Full API documentation: [docs/api.md](docs/api.md)
- Session cookie name: `DASHBOARD_SESSID`

## Testing and Validation

Backend checks:

```bash
go vet ./...
go test ./...
go build ./...
```

Frontend checks:

```bash
cd frontend
bun install
bun run build
```

Container build check:

```bash
docker build .
```

## Project Layout

```text
cmd/dashboard/           Application entrypoint
internal/auth/          Cookie auth and session validation
internal/collectors/    Host, Docker, Fail2Ban, and log collectors
internal/config/        Environment-driven config loading
internal/db/            SQLite setup and migrations
internal/server/        HTTP routes, auth middleware, WebSocket broadcast
frontend/src/           Svelte UI, API client, and live update handling
scripts/user-cli.go     Local/admin CLI for creating and listing users
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, structure, and workflow guidance.
