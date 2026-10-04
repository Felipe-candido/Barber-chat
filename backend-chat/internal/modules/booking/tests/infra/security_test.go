package infra_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
)

func TestCodesAreRandomAndCanonical(t *testing.T) {
	codes := security.Codes{}
	seen := make(map[string]bool)
	for range 100 {
		code, digest, err := codes.New()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 35 || seen[code] {
			t.Fatal("invalid or repeated code")
		}
		seen[code] = true
		hash, err := codes.Hash("  " + strings.ToLower(strings.ReplaceAll(code, "-", "")) + "  ")
		if err != nil || hash != digest {
			t.Fatal("canonical code mismatch")
		}
	}
	for _, code := range []string{"", "123456", "11111111-11111111-11111111-11111111", strings.Repeat("A", 81)} {
		if _, err := codes.Hash(code); err == nil {
			t.Fatal("invalid code accepted")
		}
	}
}

func TestReceiptEncryptionAuthenticatesKeyAndTenant(t *testing.T) {
	key := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)
	plaintext := []byte("secret appointment code")
	aad := []byte("tenant and request hash")
	first, err := security.SealReceipt(key, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}
	second, err := security.SealReceipt(key, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) || bytes.Contains(first, plaintext) {
		t.Fatal("nonce reused or plaintext exposed")
	}
	opened, err := security.OpenReceipt(key, first, aad)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatal("receipt cannot be recovered")
	}
	if _, err := security.OpenReceipt(other, first, aad); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := security.OpenReceipt(key, first, []byte("other tenant")); err == nil {
		t.Fatal("wrong tenant accepted")
	}
	first[len(first)-1] ^= 1
	if _, err := security.OpenReceipt(key, first, aad); err == nil {
		t.Fatal("tampered receipt accepted")
	}
}
