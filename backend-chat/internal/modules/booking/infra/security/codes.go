package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
)

var ErrInvalidCode = errors.New("invalid appointment code")
var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

type Codes struct{}

func (Codes) New() (string, [32]byte, error) {
	var random [20]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", [32]byte{}, err
	}
	raw := codeEncoding.EncodeToString(random[:])
	code := raw[:8] + "-" + raw[8:16] + "-" + raw[16:24] + "-" + raw[24:]
	return code, sha256.Sum256([]byte(raw)), nil
}
func (Codes) Hash(code string) ([32]byte, error) {
	if len(code) > 80 {
		return [32]byte{}, ErrInvalidCode
	}
	raw := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "-", "")
	decoded, err := codeEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 20 || codeEncoding.EncodeToString(decoded) != raw {
		return [32]byte{}, ErrInvalidCode
	}
	return sha256.Sum256([]byte(raw)), nil
}

// The client generates a fresh 256-bit random key per final submission. It is a
// retry credential, NOT an ordinary user-selected identifier. Only its hash is
// stored. Domain-separated key derivation protects the original receipt without
// adding a server master secret or persisting the appointment capability in clear.
// Possession of this request key permits replay for 24 hours: never log it.
func receiptCipher(requestKey string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(requestKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid booking request key")
	}
	derived := sha256.Sum256(append([]byte("barber-chat/booking-receipt/v1/"), key...))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
func SealReceipt(requestKey string, plaintext, aad []byte) ([]byte, error) {
	aead, err := receiptCipher(requestKey)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nil, plaintext, aad), nil
}
func OpenReceipt(requestKey string, sealed, aad []byte) ([]byte, error) {
	aead, err := receiptCipher(requestKey)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nil, sealed, aad)
}
