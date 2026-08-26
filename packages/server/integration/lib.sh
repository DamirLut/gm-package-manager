# shellcheck shell=bash
# Shared helpers for the integration suites: tiny TAP-ish assertions,
# npm/curl wrappers and fixture builders. Sourced by run.sh; every suite
# runs against $REG with the server process managed from run.sh.

REG="${REG:-http://127.0.0.1:8080}"
STORAGE="${STORAGE:-/tmp/gmpm-it/storage}"
WORK="${WORK:-/tmp/gmpm-it/work}"
FIXTURES="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/fixtures"

PASS=0
FAIL=0

suite() { printf '\n== %s ==\n' "$1"; }

ok()   { PASS=$((PASS + 1)); printf '    ok     %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf '    NOT OK %s\n' "$1"; [ $# -gt 1 ] && printf '           -> %s\n' "$2"; }

# eq <desc> <got> <want>
eq() {
	local desc="$1" got="$2" want="$3"
	if [ "$got" = "$want" ]; then ok "$desc"; else fail "$desc" "got '$got', want '$want'"; fi
}

# status <desc> <want-status> <method> <url> [curl opts...]
status() {
	local desc="$1" want="$2" method="$3" url="$4"
	shift 4
	eq "$desc" "$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url" "$@")" "$want"
}

# contains <desc> <haystack> <needle>
contains() {
	case "$2" in
	*"$3"*) ok "$1" ;;
	*) fail "$1" "missing '$3'; got: $(printf '%s' "$2" | head -c 160)" ;;
	esac
}

# npmrc_for <file> [token] — userconfig pinning npm to the test registry.
npmrc_for() {
	{
		echo "registry=$REG"
		[ -n "${2:-}" ] && echo "//${REG#http://}/:_authToken=$2"
	} >"$1"
}

NPM=(npm --no-audit --no-fund --loglevel=error)
# npm_as <npmrc-file> <npm args...>
npm_as() {
	NPM_CONFIG_USERCONFIG="$1" NPM_CONFIG_UPDATE_NOTIFIER=false "${NPM[@]}" "${@:2}"
}

# register <user> <password>; echoes the token, empty when refused.
register() {
	curl -s -X PUT "$REG/-/user/org.couchdb.user:$1" \
		-u "$1:$2" -H 'Content-Type: application/json' -d '{}' |
		jq -r '.token // ""'
}

# make_pkg <dir> <name> <version> — minimal publishable package: no dll,
# just package.json (with the mandatory gm block), README and one data file.
make_pkg() {
	mkdir -p "$1"
	cat >"$1/package.json" <<EOF
{
  "name": "$2",
  "version": "$3",
  "description": "gmpm integration fixture",
  "license": "MIT",
  "gm": {
    "destination": "packages/${2##*/}",
    "displayName": "IT ${2##*/}"
  }
}
EOF
	printf '# %s\n\nintegration fixture\n' "${2##*/}" >"$1/README.md"
	printf 'payload %s\n' "$3" >"$1/data.txt"
}

raw_status=""
raw_body=""

# raw_publish <token> <name> <body-json>; fills raw_status / raw_body.
# An empty token sends no Authorization header at all (true anonymous).
raw_publish() {
	local out
	if [ -n "$1" ]; then
		out=$(curl -s -w '\n%{http_code}' -X PUT "$REG/${2//\//%2F}" \
			-H "Authorization: Bearer $1" \
			-H 'Content-Type: application/json' --data-binary "$3")
	else
		out=$(curl -s -w '\n%{http_code}' -X PUT "$REG/${2//\//%2F}" \
			-H 'Content-Type: application/json' --data-binary "$3")
	fi
	raw_body=$(printf '%s' "$out" | sed '$d')
	raw_status=$(printf '%s' "$out" | tail -n 1)
	if [ "${DEBUG_RAW:-}" = 1 ]; then
		printf 'REQ url=%s body=%.160s\n' "${2//\//%2F}" "$3" >&2
		printf 'RES[%s] %s\n' "$raw_status" "$raw_body" >&2
	fi
	return 0
}

# raw_publish_from <token> <name> <body-file> — same, reading the body from
# a file so oversized payloads never hit the OS argv size limit.
raw_publish_from() {
	local out
	if [ -n "$1" ]; then
		out=$(curl -s -w '\n%{http_code}' -X PUT "$REG/${2//\//%2F}" \
			-H "Authorization: Bearer $1" \
			-H 'Content-Type: application/json' --data-binary @"$3")
	else
		out=$(curl -s -w '\n%{http_code}' -X PUT "$REG/${2//\//%2F}" \
			-H 'Content-Type: application/json' --data-binary @"$3")
	fi
	raw_body=$(printf '%s' "$out" | sed '$d')
	raw_status=$(printf '%s' "$out" | tail -n 1)
	[ "${DEBUG_RAW:-}" = 1 ] && printf 'RAW[%s] %.200s\n' "$raw_status" "$raw_body" >&2
	return 0
}
