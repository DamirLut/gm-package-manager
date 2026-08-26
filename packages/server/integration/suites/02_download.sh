# Download: packument variants, caching headers, tarball bytes and a real
# npm install into an empty project.
suite "download: packument"

pkg_url="$REG/@it/minimal"

full=$(curl -s -H 'Accept: application/json' "$pkg_url")
eq "packument 0.1.0 present" "$(echo "$full" | jq -r '.versions["0.1.0"].version')" "0.1.0"
eq "dist-tags.latest" "$(echo "$full" | jq -r '."dist-tags".latest')" "0.1.0"
contains "gm.destination preserved" "$full" '"destination":"packages/minimal"'

abbrev_ct=$(curl -s -o /dev/null -w '%{content_type}' \
	-H 'Accept: application/vnd.npm.install-v1+json' "$pkg_url")
eq "abbreviated media type" "$abbrev_ct" "application/vnd.npm.install-v1+json"

# both spellings of a scoped path must converge (npm sends the encoded one)
status "decoded /@it/minimal -> 200" 200 GET "$REG/@it/minimal"
status "encoded /@it%2Fminimal -> 200" 200 GET "$REG/@it%2Fminimal"

suite "download: conditional request"

etag=$(curl -s -D - -o /dev/null "$pkg_url" | tr -d '\r' | awk 'tolower($1)=="etag:"{print $2}')
[ -n "$etag" ] && ok "ETag issued" || fail "ETag issued"
status "If-None-Match -> 304" 304 GET "$pkg_url" -H "If-None-Match: $etag"

suite "download: tarball"

tarball_url=$(echo "$full" | jq -r '.versions["0.1.0"].dist.tarball')
want_sha=$(echo "$full" | jq -r '.versions["0.1.0"].dist.shasum')
got_sha=$(curl -sf "$tarball_url" | sha1sum | cut -d' ' -f1)
eq "tarball sha1 matches dist.shasum" "$got_sha" "$want_sha"

status "unknown package -> 404" 404 GET "$REG/@it/missing"
status "wrong tarball name -> 404" 404 GET "$REG/@it/minimal/-/nope-0.1.0.tgz"

suite "download: npm install"

proj="$WORK/project"
mkdir -p "$proj"
(cd "$proj" && npm init -y >/dev/null 2>&1)
if out=$(cd "$proj" && npm_as "$WORK/alice.npmrc" install @it/minimal 2>&1); then
	ok "npm install @it/minimal"
else
	fail "npm install @it/minimal" "$out"
fi
eq "installed file in place" "$(cat "$proj/node_modules/@it/minimal/data.txt" 2>/dev/null)" "payload 0.1.0"
