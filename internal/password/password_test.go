package password

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestArgon2idFixedParametersAndRandomSalt(t *testing.T) {
	h := Hasher{}
	first, err := h.Hash([]byte("a test password"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Hash([]byte("a test password"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("salt must be freshly generated")
	}
	for _, hash := range []string{first, second} {
		parts := strings.Split(hash, "$")
		if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=4" {
			t.Fatal("unexpected PHC parameters")
		}
		for i, size := range map[int]int{4: 16, 5: 32} {
			b, e := base64.RawStdEncoding.DecodeString(parts[i])
			if e != nil || len(b) != size {
				t.Fatalf("PHC field %d length", i)
			}
		}
	}
	if !h.Verify(first, []byte("a test password")) || h.Verify(first, []byte("wrong password")) {
		t.Fatal("password verification mismatch")
	}
}

func TestVerifyRejectsMalformedAndUnsupportedPHC(t *testing.T) {
	h := Hasher{}
	for _, hash := range []string{"", "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$a2V5", "$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$a2V5", "$argon2id$v=19$m=999999999,t=3,p=4$c2FsdA$a2V5", "$argon2id$v=19$m=65536,t=0,p=4$c2FsdA$a2V5", "$argon2id$v=19$m=65536,t=3,p=0$c2FsdA$a2V5", "$argon2id$v=19$m=65536,t=3,p=4$***$***", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$a2V5$extra"} {
		if h.Verify(hash, []byte("a test password")) {
			t.Fatal("accepted malformed PHC")
		}
	}
}
