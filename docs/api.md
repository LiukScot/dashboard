# API Documentation

This document describes the current public API exposed by the dashboard backend.

Base URL examples assume a local server running on:

```text
http://localhost:4200
```

## Authentication Model

- Authentication is session-based.
- Successful login sets the `DASHBOARD_SESSID` cookie.
- Protected REST endpoints require that cookie.
- The WebSocket handshake on `/ws` also requires the same cookie.

When calling the API from `curl`, use a cookie jar so login and subsequent requests share the same session.

Example login flow:

```bash
curl -c cookies.txt \
  -H 'Content-Type: application/json' \
  -X POST http://localhost:4200/api/v1/auth/login \
  -d '{"email":"admin@example.com","password":"secret"}'
```

Then reuse the session:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/auth/me
```

## Auth

### POST `/api/v1/auth/login`

- Auth required: `No`
- Request body:

```json
{
  "email": "admin@example.com",
  "password": "secret"
}
```

- Success response: `200 OK`

```json
{
  "status": "ok"
}
```

- Failure responses:
  - `400 Bad Request` for malformed JSON or oversized request body
  - `401 Unauthorized` for invalid credentials

Example:

```bash
curl -c cookies.txt \
  -H 'Content-Type: application/json' \
  -X POST http://localhost:4200/api/v1/auth/login \
  -d '{"email":"admin@example.com","password":"secret"}'
```

### POST `/api/v1/auth/logout`

- Auth required: `No`
- Clears the session cookie if present.

- Success response: `200 OK`

```json
{
  "status": "ok"
}
```

Example:

```bash
curl -b cookies.txt -c cookies.txt -X POST http://localhost:4200/api/v1/auth/logout
```

### GET `/api/v1/auth/session`

- Auth required: `No`
- Returns whether the request currently has a valid session.

- Success response without a valid session:

```json
{
  "authenticated": false
}
```

- Success response with a valid session:

```json
{
  "authenticated": true,
  "user": {
    "id": 1,
    "email": "admin@example.com"
  }
}
```

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/auth/session
```

### GET `/api/v1/auth/me`

- Auth required: `Yes`
- Returns the authenticated user.

- Success response: `200 OK`

```json
{
  "id": 1,
  "email": "admin@example.com"
}
```

- Failure response: `401 Unauthorized`

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/auth/me
```

## System

### GET `/api/v1/system/overview`

- Auth required: `Yes`
- Returns a snapshot of system metrics.

- Success response: `200 OK`

```json
{
  "hostname": "server01",
  "uptime": 82134.57,
  "loadAvg": [0.31, 0.42, 0.55],
  "cpuPercent": 19.2,
  "cpuCores": 8,
  "memTotal": 16777216000,
  "memUsed": 6442450944,
  "memPercent": 38.4,
  "swapTotal": 2147483648,
  "swapUsed": 0,
  "diskTotal": 512110190592,
  "diskUsed": 128849018880,
  "diskPercent": 25.2,
  "timestamp": "2026-04-21T17:40:12.123456Z"
}
```

- Failure response: `500 Internal Server Error`

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/system/overview
```

### GET `/api/v1/system/cpu-history`

- Auth required: `Yes`
- Returns the in-memory history collected by the system collector.

- Success response: `200 OK`

```json
[
  {
    "hostname": "server01",
    "uptime": 82134.57,
    "loadAvg": [0.31, 0.42, 0.55],
    "cpuPercent": 19.2,
    "cpuCores": 8,
    "memTotal": 16777216000,
    "memUsed": 6442450944,
    "memPercent": 38.4,
    "swapTotal": 2147483648,
    "swapUsed": 0,
    "diskTotal": 512110190592,
    "diskUsed": 128849018880,
    "diskPercent": 25.2,
    "timestamp": "2026-04-21T17:40:12.123456Z"
  }
]
```

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/system/cpu-history
```

### GET `/api/v1/system/network`

- Auth required: `Yes`
- Returns per-interface network counters and computed rates.

- Success response: `200 OK`

```json
[
  {
    "interface": "eth0",
    "rxBytes": 6543210,
    "txBytes": 1234567,
    "rxRate": 10240.5,
    "txRate": 2048.8
  }
]
```

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/system/network
```

## Docker

### GET `/api/v1/docker/containers`

- Auth required: `Yes`
- Returns all containers from the local Docker daemon.

- Success response: `200 OK`

```json
[
  {
    "id": "aaaaaaaaaaaa",
    "name": "dashboard",
    "image": "dashboard:local",
    "state": "running",
    "status": "Up 2 minutes",
    "created": 1713710000,
    "ports": [
      {
        "ip": "0.0.0.0",
        "privatePort": 4200,
        "publicPort": 4200,
        "type": "tcp"
      }
    ],
    "labels": {
      "com.docker.compose.service": "dashboard"
    }
  }
]
```

- Failure response: `500 Internal Server Error`

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/docker/containers
```

## Security

### GET `/api/v1/security/fail2ban`

- Auth required: `Yes`
- Returns current Fail2Ban jail status.

- Success response: `200 OK`

```json
{
  "jails": [
    {
      "name": "sshd",
      "bannedIPs": ["1.2.3.4"],
      "banCount": 1,
      "totalBans": 12,
      "totalFails": 40
    }
  ],
  "totalBans": 1,
  "totalJails": 1
}
```

- Failure response: `500 Internal Server Error`

Example:

```bash
curl -b cookies.txt http://localhost:4200/api/v1/security/fail2ban
```

### GET `/api/v1/security/fail2ban/bans`

- Auth required: `Yes`
- Query parameters:
  - `limit`: optional integer, default `50`, capped at `1000`

- Success response: `200 OK`

```json
[
  {
    "timestamp": "2026-04-20T12:34:56.789Z",
    "jail": "sshd",
    "ip": "1.2.3.4",
    "action": "ban"
  }
]
```

Example:

```bash
curl -b cookies.txt 'http://localhost:4200/api/v1/security/fail2ban/bans?limit=25'
```

### GET `/api/v1/security/logs`

- Auth required: `Yes`
- Query parameters:
  - `unit`: optional substring filter against the log source bucket used by the collector (`syslog`, `system`, `auth`, `kernel`, `daemon`)
  - `priority`: optional integer from `0` to `7`
  - `limit`: optional integer, default `100`, capped at `1000`

- Success response: `200 OK`

```json
[
  {
    "timestamp": "1713729112123456",
    "unit": "auth",
    "message": "Failed password for invalid user root from 1.2.3.4 port 2222 ssh2",
    "priority": 3,
    "priorityLabel": "err",
    "hostname": "server01",
    "pid": "1234"
  }
]
```

Example:

```bash
curl -b cookies.txt 'http://localhost:4200/api/v1/security/logs?unit=auth&priority=4&limit=50'
```

## Cron

### GET `/api/v1/cron/week`

- Auth required: `Yes`
- Query parameters:
  - `start`: optional date in `YYYY-MM-DD` format. The server returns the 7-day window starting at this date.
- Hidden cron jobs are excluded from `jobs` and `occurrences`.

- Success response: `200 OK`

```json
{
  "start": "2026-04-27",
  "end": "2026-05-03",
  "historyCoverage": "partial",
  "jobs": [
    {
      "fingerprint": "1a2b3c4d5e6f7890",
      "source": "/etc/cron.d/0hourly",
      "line": 2,
      "schedule": "01 * * * *",
      "user": "root",
      "command": "run-parts /etc/cron.hourly"
    }
  ],
  "occurrences": [
    {
      "id": "1a2b3c4d5e6f7890-202604271001",
      "jobId": "1a2b3c4d5e6f7890",
      "scheduledAt": "2026-04-27T10:01:00+02:00",
      "status": "scheduled",
      "source": "/etc/cron.d/0hourly",
      "user": "root",
      "command": "run-parts /etc/cron.hourly"
    }
  ],
  "history": [],
  "warnings": []
}
```

Example:

```bash
curl -b cookies.txt 'http://localhost:4200/api/v1/cron/week?start=2026-04-27'
```

### POST `/api/v1/cron/jobs/{fingerprint}/hide`

- Auth required: `Yes`
- Hides a cron job from weekly calendar responses.

- Success response: `200 OK`

```json
{
  "status": "ok"
}
```

### DELETE `/api/v1/cron/hidden`

- Auth required: `Yes`
- Resets all hidden cron jobs.

- Success response: `200 OK`

```json
{
  "status": "ok"
}
```

### GET `/api/v1/cron/hidden/count`

- Auth required: `Yes`
- Returns how many cron jobs are currently hidden.

- Success response: `200 OK`

```json
{
  "count": 1
}
```

## WebSocket Protocol

### GET `/ws`

- Auth required: `Yes`
- Transport: WebSocket upgrade on the same host as the REST API
- Browser auth model: reuse the existing session cookie set by `/api/v1/auth/login`

The frontend computes the URL as:

```text
ws://<host>/ws
```

or `wss://` when the page is served over HTTPS.

The server broadcasts every 3 seconds when at least one client is connected.

Current message shape:

```json
{
  "type": "metrics",
  "system": {
    "hostname": "server01",
    "uptime": 82134.57,
    "loadAvg": [0.31, 0.42, 0.55],
    "cpuPercent": 19.2,
    "cpuCores": 8,
    "memTotal": 16777216000,
    "memUsed": 6442450944,
    "memPercent": 38.4,
    "swapTotal": 2147483648,
    "swapUsed": 0,
    "diskTotal": 512110190592,
    "diskUsed": 128849018880,
    "diskPercent": 25.2,
    "timestamp": "2026-04-21T17:40:12.123456Z"
  },
  "network": [
    {
      "interface": "eth0",
      "rxBytes": 6543210,
      "txBytes": 1234567,
      "rxRate": 10240.5,
      "txRate": 2048.8
    }
  ],
  "docker": [
    {
      "id": "aaaaaaaaaaaa",
      "name": "dashboard",
      "cpuPercent": 3.5,
      "memUsage": 104857600,
      "memLimit": 2147483648,
      "memPercent": 4.8,
      "netRx": 1024,
      "netTx": 2048
    }
  ]
}
```

## Error Format

Protected and collector-backed endpoints return small JSON errors like:

```json
{
  "error": "unauthorized"
}
```

or:

```json
{
  "error": "failed to collect system metrics"
}
```

The exact string varies by handler, but the shape is consistently:

```json
{
  "error": "..."
}
```
