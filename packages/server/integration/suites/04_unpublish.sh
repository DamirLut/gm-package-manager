# Unpublish: revision conflicts, full removal (manifest + tarball),
# republishing after removal, anonymous denial.
suite "unpublish: setup"

make_pkg "$WORK/shortlived" "@it/shortlived" "1.0.0"
if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/shortlived" 2>&1); then
	ok "@it/shortlived@1.0.0 published"
else
	fail "@it/shortlived@1.0.0 published" "$out"
fi

rev=$(curl -sf "$REG/@it/shortlived" | jq -r '._rev')

suite "unpublish: deletion"

status "stale revision -> 409" 409 DELETE "$REG/@it/shortlived/-rev/999-beef" \
	-H "Authorization: Bearer $ALICE_TOKEN"

resp=$(curl -s -w '\n%{http_code}' -X DELETE "$REG/@it/shortlived/-rev/$rev" \
	-H "Authorization: Bearer $ALICE_TOKEN")
eq "correct revision -> 200" "${resp##*$'\n'}" 200
contains "response reports removal" "$resp" '"package removed"'

status "packument gone -> 404" 404 GET "$REG/@it/shortlived"
status "tarball gone -> 404" 404 GET "$REG/@it/shortlived/-/shortlived-1.0.0.tgz"

suite "unpublish: republish and denial"

if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/shortlived" 2>&1); then
	ok "republish after removal works"
else
	fail "republish after removal works" "$out"
fi
eq "republished version served" \
	"$(curl -sf "$REG/@it/shortlived" | jq -r '.versions["1.0.0"].version')" "1.0.0"

status "anonymous unpublish -> 401" 401 DELETE "$REG/@it/shortlived/-rev/$rev"

grep -q '"action":"package.unpublish"' "$STORAGE/audit.jsonl" &&
	ok "unpublish event audited" || fail "unpublish event audited"
