package auth

import (
	"strings"
	"testing"
)

func TestPasswordRoundtrip(t *testing.T) {
	h, err := hashPassword("s3cret!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("hash not in expected PHC form: %q", h)
	}
	if h2, _ := hashPassword("s3cret!"); h == h2 {
		t.Error("salt must be unique per hash")
	}
	if !verifyPassword(h, "s3cret!") {
		t.Error("correct password rejected")
	}
	if verifyPassword(h, "wrong") {
		t.Error("wrong password accepted")
	}
}

func TestVerifyMalformedHashes(t *testing.T) {
	for _, h := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=65536,t=3,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAA",
		"$argon2id$v=99$m=65536,t=3,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=65536,t=3,p=1$!!!notbase64!!!$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=65536,t=3,p=1$AAAAAAAAAAAAAAAAAAAAAA$",
	} {
		if verifyPassword(h, "x") {
			t.Errorf("verify accepted malformed hash %q", h)
		}
	}
}
