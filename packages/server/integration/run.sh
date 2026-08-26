#!/bin/bash
# Integration test entrypoint. Boots the server with its SQLite database,
# then walks through npm-protocol scenarios in phases:
#
#   phase A   default config            suites 01..04 (publish/download/update/unpublish)
#   restart   same storage              suite 05 (persistence: files + SQLite survive)
#   phase B   restricted rules,         suite 06 (permissions matrix)
#             DISABLE_SIGNUP=true       (same storage: alice/bob already exist)
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

printf '\n== summary ==\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
