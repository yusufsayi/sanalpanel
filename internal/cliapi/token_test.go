package cliapi

import "testing"

func TestHashTokenDeterministic(t *testing.T) {
	a := hashToken("abc123")
	b := hashToken("abc123")
	if a != b {
		t.Fatalf("hashToken aynı girdi için farklı çıktı üretti: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("hashToken 64 hex karakter (sha256) döndürmeli, uzunluk=%d", len(a))
	}
	if hashToken("abc123") == hashToken("xyz789") {
		t.Fatalf("farklı girdiler aynı hash üretti")
	}
}
