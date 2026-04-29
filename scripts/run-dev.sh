#!/usr/bin/env bash
set -euo pipefail

set -a
source .env 2>/dev/null || source .env.example
set +a

cd frontend
bun install
cd ..

go run ./cmd/dashboard &
backend_pid=$!

cleanup() {
	kill "$backend_pid" 2>/dev/null || true
}

trap cleanup EXIT

for _ in $(seq 1 60); do
	if curl --silent --fail http://127.0.0.1:4200/api/v1/auth/session >/dev/null; then
		cd frontend
		bun run dev --host 0.0.0.0
		exit $?
	fi

	sleep 1
done

echo "Backend did not become ready on http://127.0.0.1:4200 within 60s" >&2
exit 1
