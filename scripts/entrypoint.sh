#!/bin/sh
set -eu

APP_USER="${APP_USER:-dashboard}"
APP_GROUP="${APP_GROUP:-dashboard}"
APP_UID="${APP_UID:-1000}"
APP_GID="${APP_GID:-1000}"
SOCKET_PATH="${DOCKER_SOCKET:-/var/run/docker.sock}"
JOURNAL_GROUP_PATHS="${JOURNAL_GROUP_PATHS:-/run/log/journal,/var/log/journal}"
CRON_SPOOL_SOURCE_DIRS="${CRON_SPOOL_SOURCE_DIRS:-/host/var/spool/cron,/host/var/spool/cron/crontabs,/host/var/spool/cron/tabs}"
CRON_SPOOL_CACHE_DIR="${CRON_SPOOL_CACHE_DIR:-/app/data/cron-user-spool}"

group_name_by_gid() {
	getent group "$1" | cut -d: -f1
}

ensure_group() {
	gid="$1"
	name="$2"

	existing="$(group_name_by_gid "$gid" || true)"
	if [ -n "$existing" ]; then
		printf '%s\n' "$existing"
		return 0
	fi

	groupadd -g "$gid" "$name"
	printf '%s\n' "$name"
}

if ! id "$APP_USER" >/dev/null 2>&1; then
	PRIMARY_GROUP="$(ensure_group "$APP_GID" "$APP_GROUP")"
	useradd --no-create-home --home-dir /nonexistent --uid "$APP_UID" --gid "$PRIMARY_GROUP" "$APP_USER"
fi

if [ -S "$SOCKET_PATH" ]; then
	SOCKET_GID=$(stat -c %g "$SOCKET_PATH")
	SOCKET_GROUP=$(ensure_group "$SOCKET_GID" "dockersock")
	usermod -aG "$SOCKET_GROUP" "$APP_USER"
fi

OLD_IFS="$IFS"
IFS=','
for journal_path in $JOURNAL_GROUP_PATHS; do
	journal_path="$(printf '%s' "$journal_path" | xargs)"
	if [ -z "$journal_path" ] || [ ! -e "$journal_path" ]; then
		continue
	fi
	JOURNAL_GID=$(stat -c %g "$journal_path")
	JOURNAL_GROUP=$(ensure_group "$JOURNAL_GID" "journalaccess")
	usermod -aG "$JOURNAL_GROUP" "$APP_USER"
done
IFS="$OLD_IFS"

mkdir -p /app/data
chown "$APP_UID:$APP_GID" /app/data >/dev/null 2>&1 || true

sync_spool() {
	mkdir -p "$CRON_SPOOL_CACHE_DIR"
	_old_ifs="$IFS"
	IFS=','
	for spool_dir in $CRON_SPOOL_SOURCE_DIRS; do
		spool_dir="$(printf '%s' "$spool_dir" | xargs)"
		if [ -z "$spool_dir" ] || [ ! -d "$spool_dir" ]; then
			continue
		fi
		for spool_file in "$spool_dir"/*; do
			if [ ! -f "$spool_file" ]; then
				continue
			fi
			cp "$spool_file" "$CRON_SPOOL_CACHE_DIR/$(basename "$spool_file")"
		done
	done
	IFS="$_old_ifs"
	chown -R "$APP_UID:$APP_GID" "$CRON_SPOOL_CACHE_DIR" >/dev/null 2>&1 || true
}

mkdir -p "$CRON_SPOOL_CACHE_DIR"
find "$CRON_SPOOL_CACHE_DIR" -mindepth 1 -maxdepth 1 -type f -delete 2>/dev/null || true
sync_spool

# Background watcher: re-sync spool cache on host crontab changes.
# Without this, `crontab -e` edits would only show in dashboard after container restart.
(
	WATCH_DIRS=""
	IFS=','
	for spool_dir in $CRON_SPOOL_SOURCE_DIRS; do
		spool_dir="$(printf '%s' "$spool_dir" | xargs)"
		if [ -n "$spool_dir" ] && [ -d "$spool_dir" ]; then
			WATCH_DIRS="$WATCH_DIRS $spool_dir"
		fi
	done
	IFS="$OLD_IFS"
	if [ -z "$WATCH_DIRS" ]; then
		exit 0
	fi
	# close_write: editor saved a file. moved_to: atomic replace (crontab -e default).
	# delete: user removed their crontab. Loop restarts inotifywait if it ever exits.
	while true; do
		inotifywait -qq -e close_write,moved_to,delete,create $WATCH_DIRS || sleep 5
		sync_spool
	done
) &

exec gosu "$APP_USER" /app/dashboard
