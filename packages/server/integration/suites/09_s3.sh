# S3 backend: the full npm flow against a MinIO bucket (run.sh starts the
# container). Package objects are remote; the local disk holds only the
# database and the audit log.
suite "s3: setup"

ALICE_TOKEN=$(register alice s3-pw)
[ -n "$ALICE_TOKEN" ] && ok "alice registered" || fail "alice registered"
npmrc_for "$WORK/alice-s3.npmrc" "$ALICE_TOKEN"

suite "s3: publish and download"

make_pkg "$WORK/s3pkg" "@it/s3pkg" "1.0.0"
if out=$(npm_as "$WORK/alice-s3.npmrc" publish "$WORK/s3pkg" 2>&1); then
	ok "npm publish @it/s3pkg@1.0.0 to s3"
else
	fail "npm publish @it/s3pkg@1.0.0 to s3" "$out"
fi

eq "packument served from s3" \
	"$(curl -sf "$REG/@it/s3pkg" | jq -r '.versions | keys[0]')" "1.0.0"
status "tarball served from s3" 200 GET "$REG/@it/s3pkg/-/s3pkg-1.0.0.tgz"
[ -f "$STORAGE/@it/s3pkg/package.json" ] && fail "package file stayed on local disk" \
	|| ok "package file not on local disk"
