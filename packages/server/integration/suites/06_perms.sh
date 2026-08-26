# Permissions: runs on the hardened instance (PACKAGES_CONFIG with
# restricted patterns, DISABLE_SIGNUP=true). alice/bob exist since phase A.
suite "perms: public default pattern"

tgz="$WORK/perm.tgz"
tar -czf "$tgz" -C "$WORK/restricted" package.json 2>/dev/null ||
	{ mkdir -p "$WORK/restricted" && printf '{"name":"@x/y","version":"1.0.0"}\n' >"$WORK/restricted/package.json" && tar -czf "$tgz" -C "$WORK/restricted" package.json; }

status "anonymous reads public package" 200 GET "$REG/@it/minimal"
raw_publish "$BOB_TOKEN" "@it/pub" "$(node "$FIXTURES/mkpayload.js" --name @it/pub --tgz "$tgz")"
eq "authenticated publishes anywhere (**)" "$raw_status" 201

suite "perms: publish restricted to alice"

make_pkg "$WORK/secret" "@private/secret" "1.0.0"
if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/secret" 2>&1); then
	ok "alice publishes @private/secret"
else
	fail "alice publishes @private/secret" "$out"
fi

# bob's version differs from the published one, so only the access rule
# can deny him — no client-side or 409 ambiguity.
sed -i 's/"version": "1.0.0"/"version": "2.0.0"/' "$WORK/secret/package.json"
if out=$(npm_as "$WORK/bob.npmrc" publish "$WORK/secret" 2>&1); then
	fail "bob denied on @private/**"
else
	contains "bob denied on @private/** (E403)" "$out" "E403"
fi

suite "perms: read restricted to authenticated"

status "anonymous reads @private -> 401" 401 GET "$REG/@private/secret"
status "bob reads @private -> 200" 200 GET "$REG/@private/secret" \
	-H "Authorization: Bearer $BOB_TOKEN"
status "anonymous tarball -> 401" 401 GET "$REG/@private/secret/-/secret-1.0.0.tgz"

suite "perms: scoped publish allow-list"

make_pkg "$WORK/restricted" "@restricted/thing" "1.0.0"
if out=$(npm_as "$WORK/bob.npmrc" publish "$WORK/restricted" 2>&1); then
	fail "bob denied on @restricted/**"
else
	contains "bob denied on @restricted/** (E403)" "$out" "E403"
fi
sed -i 's/"version": "1.0.0"/"version": "1.0.1"/' "$WORK/restricted/package.json"
if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/restricted" 2>&1); then
	ok "alice allowed on @restricted/**"
else
	fail "alice allowed on @restricted/**" "$out"
fi

grep -q '"action":"package.access.denied"' "$STORAGE/audit.jsonl" &&
	ok "denied access audited" || fail "denied access audited"

suite "perms: signup disabled"

code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT \
	"$REG/-/user/org.couchdb.user:carol" -u carol:carol-pw \
	-H 'Content-Type: application/json' -d '{}')
eq "new user rejected when signup off" "$code" 401

code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT \
	"$REG/-/user/org.couchdb.user:alice" -u alice:alice-pw \
	-H 'Content-Type: application/json' -d '{}')
eq "existing user can still log in" "$code" 201
