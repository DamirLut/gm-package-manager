#!/bin/bash
# Integration test entrypoint. Boots the server with its SQLite database,
# then walks through npm-protocol scenarios in phases:
#
#   phase A   default config            suites 01..04 (publish/download/update/unpublish)
#   restart   same storage              suite 05 (persistence: files + SQLite survive)
#   phase B   restricted rules,         suite 06 (permissions matrix)
#             DISABLE_SIGNUP=true       (same storage: alice/bob already exist)
#   phase C   MinIO + STORAGE_BACKEND   suite 09 (s3 publish/download;
#             =s3                       skipped when docker is unavailable)
#
# Exit status is non-zero when any assertion failed.

set -u
cd "$(dirname "$0")"
source lib.sh

SERVER_BIN="${SERVER_BIN:-server}"
LOG=/tmp/gmpm-it/server.log
SERVER_PID=""

start_server() {
	mkdir -p "$STORAGE"
	env STORAGE_PATH="$STORAGE" DATABASE_PATH="$STORAGE/metadata.db" \
		DATABASE_AUTO_MIGRATE=true "$@" \
		"$SERVER_BIN" >>"$LOG" 2>&1 &
	SERVER_PID=$!
	for _ in $(seq 1 50); do
		if curl -sf "$REG/-/ping" >/dev/null; then return 0; fi
		sleep 0.2
	done
	echo "FAIL: server did not become ready, log tail:" >&2
	tail -n 20 "$LOG" >&2
	exit 1
}

stop_server() {
	kill "$SERVER_PID" 2>/dev/null
	wait "$SERVER_PID" 2>/dev/null
}

# start_minio boots a disposable MinIO for the s3 phase; returns 1 (with
# a skip notice) when docker or the image is unavailable. Images default
# to Docker Hub and can be overridden (e.g. quay.io/minio/minio) when the
# Hub is unreachable.
MINIO_IMAGE="${MINIO_IMAGE:-minio/minio}"
MC_IMAGE="${MC_IMAGE:-minio/mc}"
MINIO_CONTAINER="gmpm-it-minio"
MINIO_UP=0
start_minio() {
	if ! command -v docker >/dev/null 2>&1; then
		echo "skip: docker unavailable, s3 phase skipped" >&2
		return 1
	fi
	docker rm -f "$MINIO_CONTAINER" >/dev/null 2>&1 || true
	if ! docker run -d --rm --name "$MINIO_CONTAINER" -p 9000:9000 \
		-e MINIO_ROOT_USER=gmpm -e MINIO_ROOT_PASSWORD=gmpm-secret \
		"$MINIO_IMAGE" server /data >/dev/null 2>&1; then
		echo "skip: minio container failed to start, s3 phase skipped" >&2
		return 1
	fi
	local up=0
	for _ in $(seq 1 60); do
		if curl -sf -o /dev/null http://127.0.0.1:9000/minio/health/live; then up=1; break; fi
		sleep 0.5
	done
	if [ "$up" != 1 ]; then
		echo "skip: minio not healthy, s3 phase skipped" >&2
		docker rm -f "$MINIO_CONTAINER" >/dev/null 2>&1 || true
		return 1
	fi
	if ! docker run --rm --add-host=host:host-gateway \
		-e MC_HOST_it=http://gmpm:gmpm-secret@host:9000 \
		"$MC_IMAGE" mb --ignore-existing it/gmpm-it >/dev/null 2>&1; then
		echo "skip: minio bucket creation failed, s3 phase skipped" >&2
		docker rm -f "$MINIO_CONTAINER" >/dev/null 2>&1 || true
		return 1
	fi
	MINIO_UP=1
	return 0
}

stop_minio() {
	[ "$MINIO_UP" = 1 ] || return 0
	docker rm -f "$MINIO_CONTAINER" >/dev/null 2>&1 || true
}
trap stop_minio EXIT

rm -rf /tmp/gmpm-it
mkdir -p "$WORK"
export NPM_CONFIG_CACHE=/tmp/gmpm-it/npm-cache

echo "gmpm integration tests against $REG"

start_server
source suites/01_publish.sh
source suites/02_download.sh
source suites/03_update.sh
source suites/04_unpublish.sh
stop_server

start_server # same storage — everything must survive the restart
source suites/05_persistence.sh
stop_server

start_server DISABLE_SIGNUP=true PACKAGES_CONFIG="$FIXTURES/packages.restricted.json"
source suites/06_perms.sh
stop_server

if start_minio; then
	STORAGE="/tmp/gmpm-it/storage-s3"
	start_server STORAGE_BACKEND=s3 S3_ENDPOINT=127.0.0.1:9000 S3_BUCKET=gmpm-it \
		S3_ACCESS_KEY=gmpm S3_SECRET_KEY=gmpm-secret S3_SECURE=false S3_PATH_STYLE=true
	source suites/09_s3.sh
	stop_server
	stop_minio
fi

printf '\n== summary ==\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
