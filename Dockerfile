# Stage 1: Build frontend
FROM oven/bun:1 AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock* ./
RUN bun install
COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go backend
FROM golang:1.25-alpine AS backend-build
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY scripts/ ./scripts/
RUN CGO_ENABLED=1 go build -o /dashboard ./cmd/dashboard/
RUN CGO_ENABLED=1 go build -o /user-cli ./scripts/user-cli.go

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates fail2ban
WORKDIR /app

COPY --from=backend-build /dashboard /app/dashboard
COPY --from=backend-build /user-cli /app/user-cli
COPY --from=frontend-build /app/frontend/build /app/frontend/build

RUN mkdir -p /app/data

ENV HOST=0.0.0.0 \
    PORT=4200 \
    DB_PATH=/app/data/dashboard.sqlite \
    PROC_PATH=/host/proc \
    LOG_PATH=/host/log \
    DOCKER_SOCKET=/var/run/docker.sock \
    PUBLIC_DIR=/app/frontend/build

EXPOSE 4200
CMD ["/app/dashboard"]
