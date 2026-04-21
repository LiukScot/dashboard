#!/bin/sh
set -eu

APP_USER="${APP_USER:-dashboard}"
APP_GROUP="${APP_GROUP:-dashboard}"
APP_UID="${APP_UID:-1000}"
APP_GID="${APP_GID:-1000}"
SOCKET_PATH="${DOCKER_SOCKET:-/var/run/docker.sock}"

if ! awk -F: -v gid="$APP_GID" '$3 == gid { found = 1 } END { exit found ? 0 : 1 }' /etc/group; then
	addgroup -g "$APP_GID" -S "$APP_GROUP"
fi

if ! id "$APP_USER" >/dev/null 2>&1; then
	adduser -S -D -H -u "$APP_UID" -G "$APP_GROUP" "$APP_USER"
fi

if [ -S "$SOCKET_PATH" ]; then
	SOCKET_GID=$(stat -c %g "$SOCKET_PATH")
	SOCKET_GROUP=$(awk -F: -v gid="$SOCKET_GID" '$3 == gid { print $1; exit }' /etc/group)

	if [ -z "$SOCKET_GROUP" ]; then
		SOCKET_GROUP="dockersock"
		addgroup -g "$SOCKET_GID" -S "$SOCKET_GROUP"
	fi

	addgroup "$APP_USER" "$SOCKET_GROUP" >/dev/null 2>&1 || true
fi

mkdir -p /app/data
chown "$APP_UID:$APP_GID" /app/data >/dev/null 2>&1 || true

exec su-exec "$APP_USER" /app/dashboard