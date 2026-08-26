# Persistence: the previous phase killed the server; this runs against a
# fresh process on the same storage. Files, SQLite users and tokens must
# all survive.
suite "persistence: storage"

[ -f "$STORAGE/metadata.db" ] && ok "sqlite database on disk" || fail "sqlite database on disk"
status "package survived restart" 200 GET "$REG/@it/minimal"
eq "versions intact" \
	"$(curl -sf "$REG/@it/minimal" | jq '.versions | length')" 3

suite "persistence: auth state"

eq "token still valid after restart" \
	"$(npm_as "$WORK/alice.npmrc" view @it/minimal version 2>/dev/null | tr -d '[:space:]')" "0.2.0"

tarball_url=$(curl -sf "$REG/@it/minimal" | jq -r '.versions["0.2.0"].dist.tarball')
status "tarball still downloadable" 200 GET "$tarball_url"
