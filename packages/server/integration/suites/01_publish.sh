# Publish: happy path through the real npm CLI, then server-side
# validation cases driven with raw protocol bodies.
suite "publish: registration"

ALICE_TOKEN=$(register alice alice-pw)
BOB_TOKEN=$(register bob bob-pw)
[ -n "$ALICE_TOKEN" ] && ok "alice registered" || fail "alice registered"
[ -n "$BOB_TOKEN" ] && ok "bob registered" || fail "bob registered"

npmrc_for "$WORK/alice.npmrc" "$ALICE_TOKEN"
npmrc_for "$WORK/bob.npmrc" "$BOB_TOKEN"
npmrc_for "$WORK/anon.npmrc"

suite "publish: npm happy path"

make_pkg "$WORK/minimal" "@it/minimal" "0.1.0"
if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/minimal" 2>&1); then
	ok "npm publish @it/minimal@0.1.0"
else
	fail "npm publish @it/minimal@0.1.0" "$out"
fi

suite "publish: validation cases (raw protocol)"

# raw bodies hit a dedicated package so version-space pollution cannot
# affect the CLI flows below.
tgz="$WORK/dummy.tgz"
tar -czf "$tgz" -C "$WORK/minimal" package.json README.md data.txt

badsha=$(sha1sum "$WORK/minimal/README.md" | cut -d' ' -f1)
payload() { node "$FIXTURES/mkpayload.js" --name @it/validations "$@"; }

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --tgz "$tgz")"
eq "first raw publish -> 201" "$raw_status" 201

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --tgz "$tgz")"
eq "duplicate version -> 409" "$raw_status" 409

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --key not.semver)"
eq "non-semver version key -> 400" "$raw_status" 400

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --key 0.2.0 --version 0.2.1)"
eq "inner version mismatch -> 400" "$raw_status" 400

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --tgz "$tgz" --shasum "$badsha")"
eq "wrong shasum -> 400" "$raw_status" 400

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --data-b64 '')"
eq "zero-length attachment -> 422" "$raw_status" 422

raw_publish "$ALICE_TOKEN" "@it/validations" "$(payload --no-gm)"
eq "missing gm block -> 422" "$raw_status" 422

raw_publish "" "@it/validations" "$(payload --tgz "$tgz" --key 9.9.9 --version 9.9.9)"
eq "anonymous publish -> 403" "$raw_status" 403

raw_publish "gmpm_$(printf 'beef%.0s' $(seq 16))" "@it/validations" "$(payload --tgz "$tgz" --key 9.9.9 --version 9.9.9)"
eq "garbage token -> 401" "$raw_status" 401

big=$(head -c 8000000 /dev/zero | base64 | tr -d '\n')
node "$FIXTURES/mkpayload.js" --name @it/validations --key 8.0.0 --version 8.0.0 \
	--data-b64 "$big" --out "$WORK/big.json"
# the server answers 413 or kills the upload mid-flight (empty/000 status);
# either way the oversized version must not end up in storage.
raw_publish_from "$ALICE_TOKEN" "@it/validations" "$WORK/big.json"
case "$raw_status" in
413 | 000 | "") ok "body over 10MiB rejected" ;;
*) fail "body over 10MiB rejected" "got '$raw_status', want 413" ;;
esac
eq "oversized version not stored" \
	"$(curl -s "$REG/@it/validations" | jq -r '.versions["8.0.0"] // "absent"')" "absent"

suite "publish: audit trail"

grep -q '"action":"package.publish"' "$STORAGE/audit.jsonl" &&
	grep -q '"actor":"alice"' "$STORAGE/audit.jsonl" &&
	ok "publish event audited" || fail "publish event audited"
