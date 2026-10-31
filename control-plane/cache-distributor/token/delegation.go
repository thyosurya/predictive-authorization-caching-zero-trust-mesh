// control-plane/cache-distributor/token/delegation.go
package token

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// DelegationToken adalah token yang dikirim ke sidecar
// berisi keputusan otorisasi yang sudah ditanda tangani PDP
type DelegationToken struct {
	Issuer          string   `json:"iss"`
	SourceIdentity  string   `json:"sub"`
	TargetIdentity  string   `json:"aud"`
	AllowedActions  []string `json:"allowed_actions"`
	Sensitivity     string   `json:"sensitivity"`
	ExpiresAt       int64    `json:"exp"`
	IssuedAt        int64    `json:"iat"`
	RevocationNonce string   `json:"revocation_nonce"`
	CacheTTLMs      int64    `json:"cache_ttl_ms"`
	Signature       string   `json:"signature,omitempty"`
}

// CanonicalPayload menghasilkan representasi deterministik token untuk penandatanganan dan verifikasi HMAC.
func CanonicalPayload(source, target string, actions []string, exp int64, nonce string) []byte {
	return []byte(source + "|" + target + "|" + strings.Join(actions, ",") + "|" + strconv.FormatInt(exp, 10) + "|" + nonce)
}

// CanonicalPayload memanggil fungsi CanonicalPayload dengan data dari DelegationToken.
func (t *DelegationToken) CanonicalPayload() []byte {
	return CanonicalPayload(t.SourceIdentity, t.TargetIdentity, t.AllowedActions, t.ExpiresAt, t.RevocationNonce)
}

// GenerateNonce membuat revocation nonce acak 8 byte.
// Return 16-char hex string.
// Fungsi level paket - bisa dipanggil dari luar.
func GenerateNonce() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewDelegationToken membuat token baru untuk pasangan service.
// Validasi:
//   - source & target tidak boleh kosong
//   - actions tidak boleh kosong
//   - ttl tidak boleh <= 0
//   - nonce tidak boleh kosong
//
// Mengisi IssuedAt = now, ExpiresAt = now + ttl
// Mengisi Issuer = "pdp-central.spiffe.io" (konstan)
func NewDelegationToken(
	source, target string,
	actions []string,
	sensitivity string,
	ttl time.Duration,
	nonce string,
) (*DelegationToken, error) {
	if source == "" {
		return nil, errors.New("empty source identity")
	}
	if target == "" {
		return nil, errors.New("empty target identity")
	}
	if len(actions) == 0 {
		return nil, errors.New("empty actions")
	}
	if ttl <= 0 {
		return nil, errors.New("TTL must be positive")
	}
	// Defense-in-depth: clamp TTL ke MaxTTL (30s) sesuai jaminan batas atas AS-03
	const maxTTL = 30 * time.Second
	if ttl > maxTTL {
		ttl = maxTTL
	}
	if nonce == "" {
		return nil, errors.New("empty nonce")
	}

	now := time.Now()
	return &DelegationToken{
		Issuer:          "pdp-central.spiffe.io",
		SourceIdentity:  source,
		TargetIdentity:  target,
		AllowedActions:  actions,
		Sensitivity:     sensitivity,
		ExpiresAt:       now.Add(ttl).Unix(),
		IssuedAt:        now.Unix(),
		RevocationNonce: nonce,
		CacheTTLMs:      ttl.Milliseconds(),
	}, nil
}

// ToJSON serializes token ke JSON. Digunakan sebelum signing.
func (t *DelegationToken) ToJSON() ([]byte, error) {
	return json.Marshal(t)
}

// IsExpired return true jika ExpiresAt <= time.Now().Unix() (penegakan batas ketat)
func (t *DelegationToken) IsExpired() bool {
	return t.ExpiresAt <= time.Now().Unix()
}
