# Stage 1: Build frontend
FROM oven/bun:1 AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock* ./
RUN bun install
COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go backend
FROM golang:1.26-bookworm AS backend-build
RUN apt-get update \
	&& apt-get install -y --no-install-recommends gcc libc6-dev \
	&& rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY scripts/ ./scripts/
RUN CGO_ENABLED=1 go build -o /dashboard ./cmd/dashboard/
RUN CGO_ENABLED=1 go build -o /user-cli ./scripts/user-cli.go

# Stage 3: Runtime
FROM debian:bookworm-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl fail2ban gosu systemd inotify-tools \
	&& rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY --from=backend-build /dashboard /app/dashboard
COPY --from=backend-build /user-cli /app/user-cli
COPY --from=frontend-build /app/frontend/build /app/frontend/build
COPY scripts/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh && mkdir -p /app/data

ENV HOST=0.0.0.0 \
    PORT=4200 \
    DB_PATH=/app/data/dashboard.sqlite \
    PROC_PATH=/host/proc \
    LOG_PATH=/host/log \
    DOCKER_SOCKET=/var/run/docker.sock \
    PUBLIC_DIR=/app/frontend/build

EXPOSE 4200
ENTRYPOINT ["/app/entrypoint.sh"]
