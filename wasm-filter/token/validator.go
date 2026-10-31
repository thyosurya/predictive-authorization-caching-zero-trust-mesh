// wasm-filter/token/validator.go
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Decision merepresentasikan hasil validasi token
type Decision int

const (
	DecisionAllow   Decision = iota // token valid, akses diizinkan
	DecisionDeny                    // token ada tapi action tidak diizinkan
	DecisionExpired                 // token ada tapi sudah expired
	DecisionRevoked                 // nonce tidak cocok (sudah dicabut)
)

// CachedToken adalah representasi token delegasi yang disimpan di cache lokal sidecar
type CachedToken struct {
	AllowedActions  []string `json:"allowed_actions"`
	ExpiresAt       int64    `json:"exp"`
	RevocationNonce string   `json:"revocation_nonce"`
	Signature       string   `json:"signature,omitempty"`
}

// CanonicalPayload menghasilkan representasi deterministik token untuk verifikasi HMAC.
func CanonicalPayload(source, target string, actions []string, exp int64, nonce string) []byte {
	return []byte(source + "|" + target + "|" + strings.Join(actions, ",") + "|" + strconv.FormatInt(exp, 10) + "|" + nonce)
}

// VerifySignature memverifikasi tanda tangan HMAC-SHA256 dari payload menggunakan secret key.
// Menggunakan perbandingan constant-time (hmac.Equal) untuk mencegah timing attack.
func VerifySignature(payload []byte, signature string, key []byte) bool {
	if len(payload) == 0 || signature == "" || len(key) == 0 {
		return false
	}
	sigBytes, err := base64.URLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}

// Validator memvalidasi delegation token dari tiga aspek:
// masa berlaku (exp), nonce aktif, dan action yang diizinkan.
// Tidak bertanggung jawab atas cache.
type Validator struct {
	mu           sync.RWMutex
	activeNonces map[string]bool // nonce yang masih valid
}

// NewValidator membuat Validator baru dengan registry nonce kosong.
// Parameter registryID adalah identifier opsional untuk registry ini.
func NewValidator(registryID string) *Validator {
	return &Validator{
		activeNonces: make(map[string]bool),
	}
}

// RegisterNonce mendaftarkan nonce baru saat token baru diterima dari Cache Distributor.
// Dipanggil dari WASM shared queue handler saat ada PushToken.
func (v *Validator) RegisterNonce(nonce string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.activeNonces[nonce] = true
}

// InvalidateNonce menghapus nonce dari registry saat RevocationEvent diterima.
// Dipanggil dari WASM shared queue handler saat ada RevocationEvent.
func (v *Validator) InvalidateNonce(nonce string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.activeNonces, nonce)
}

// SyncActiveNonces mengganti daftar nonce aktif secara atomik dengan daftar terbaru
// dari Cache Distributor, mencegah pertumbuhan memori tanpa batas (memory leak) dan
// memastikan nonce dari token yang dicabut langsung tidak valid.
func (v *Validator) SyncActiveNonces(validNonces map[string]bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.activeNonces = validNonces
}

// Validate memvalidasi token dengan urutan tetap: exp → nonce → action.
// Urutan ini TIDAK BOLEH diubah.
func (v *Validator) Validate(token *CachedToken, action string) Decision {
	// 1. Cek exp: jika sudah lewat atau tepat pada detik kedaluwarsa → Expired (m1: batas ketat >=)
	if time.Now().Unix() >= token.ExpiresAt {
		return DecisionExpired
	}

	// 2. Cek nonce: jika tidak ada di activeNonces → Revoked
	v.mu.RLock()
	nonceValid := v.activeNonces[token.RevocationNonce]
	v.mu.RUnlock()
	if !nonceValid {
		return DecisionRevoked
	}

	// 3. Cek action: jika action tidak ada di AllowedActions → Deny
	for _, allowed := range token.AllowedActions {
		if allowed == action {
			return DecisionAllow
		}
	}

	return DecisionDeny
}

// ParseToken melakukan unmarshal JSON ke CachedToken.
// Return error jika format tidak valid.
func ParseToken(raw []byte) (*CachedToken, error) {
	var token CachedToken
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, err
	}
	return &token, nil
}
