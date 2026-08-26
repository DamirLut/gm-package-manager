# Update: publishing a new version moves dist-tags, is visible to npm view
# and lands on install; custom tags must not move "latest".
suite "update: new version"

sed -i 's/"version": "0.1.0"/"version": "0.2.0"/' "$WORK/minimal/package.json"
printf 'payload 0.2.0\n' >"$WORK/minimal/data.txt"
if out=$(npm_as "$WORK/alice.npmrc" publish "$WORK/minimal" 2>&1); then
	ok "npm publish @it/minimal@0.2.0"
else
	fail "npm publish @it/minimal@0.2.0" "$out"
fi

packument=$(curl -sf "$REG/@it/minimal")
eq "latest moved to 0.2.0" "$(echo "$packument" | jq -r '."dist-tags".latest')" "0.2.0"
eq "two versions stored" "$(echo "$packument" | jq '.versions | length')" 2
eq "old version still served" "$(echo "$packument" | jq -r '.versions["0.1.0"].version')" "0.1.0"
[ -n "$(echo "$packument" | jq -r '.time.created // ""')" ] &&
	[ -n "$(echo "$packument" | jq -r '.time.modified // ""')" ] &&
	ok "time.created/modified tracked" || fail "time.created/modified tracked"

suite "update: visible to clients"

eq "npm view shows latest" \
	"$(npm_as "$WORK/alice.npmrc" view @it/minimal version 2>/dev/null | tr -d '[:space:]')" "0.2.0"

proj="$WORK/project-update"
mkdir -p "$proj"
(cd "$proj" && npm init -y >/dev/null 2>&1)
if out=$(cd "$proj" && npm_as "$WORK/alice.npmrc" install @it/minimal 2>&1); then
	ok "install resolves to latest"
else
	fail "install resolves to latest" "$out"
fi
eq "installed content updated" \
	"$(cat "$proj/node_modules/@it/minimal/data.txt" 2>/dev/null)" "payload 0.2.0"

suite "update: custom dist-tag"

sed -i 's/"version": "0.2.0"/"version": "0.3.0"/' "$WORK/minimal/package.json"
if out=$(npm_as "$WORK/alice.npmrc" publish --tag next "$WORK/minimal" 2>&1); then
	ok "npm publish --tag next"
else
	fail "npm publish --tag next" "$out"
fi

packument=$(curl -sf "$REG/@it/minimal")
eq "next tag points to 0.3.0" "$(echo "$packument" | jq -r '."dist-tags".next')" "0.3.0"
eq "latest untouched by next" "$(echo "$packument" | jq -r '."dist-tags".latest')" "0.2.0"
eq "npm view @next resolves" \
	"$(npm_as "$WORK/alice.npmrc" view @it/minimal@next version 2>/dev/null | tr -d '[:space:]')" "0.3.0"
