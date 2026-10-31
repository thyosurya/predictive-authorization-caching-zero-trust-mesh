package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Signer menandatangani payload token dengan HMAC-SHA256 dan memverifikasi tanda tangan.
// Ini adalah satu-satunya file yang menyentuh kunci kriptografis.
// Kunci tidak boleh pernah di-log atau dikembalikan via API.
type Signer struct {
	key []byte // private - tidak pernah diekspor
}

// NewSigner membuat Signer baru dengan kunci HMAC.
// Validasi: key harus minimal 32 byte.
// Return error jika key terlalu pendek.
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < 32 {
		return nil, errors.New("signing key must be at least 32 bytes")
	}
	return &Signer{key: key}, nil
}

// Sign menghitung HMAC-SHA256(key, payload).
// Return base64url-encoded signature.
// Return error jika payload kosong.
func (s *Signer) Sign(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("empty payload")
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	return base64.URLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify memverifikasi tanda tangan payload.
// Decode signature dari base64url.
// Hitung HMAC ulang dan bandingkan secara constant-time (hmac.Equal).
// Return nil jika valid, error jika tidak cocok.
// Error tidak boleh mengungkap key atau payload.
func (s *Signer) Verify(payload []byte, signature string) error {
	if signature == "" {
		return errors.New("empty signature")
	}
	if len(payload) == 0 {
		return errors.New("empty payload")
	}

	// Decode signature dari base64url
	sigBytes, err := base64.URLEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("invalid signature encoding")
	}

	// Hitung HMAC ulang
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	expected := mac.Sum(nil)

	// Bandingkan secara constant-time untuk mencegah timing attack
	if !hmac.Equal(sigBytes, expected) {
		return errors.New("signature verification failed")
	}
	return nil
}
