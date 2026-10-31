// wasm-filter/token/validator_test.go
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

// TestValidate_ValidToken memverifikasi token valid melewati semua pemeriksaan
func TestValidate_ValidToken(t *testing.T) {
    v := NewValidator("valid-nonce-registry")
    tok := &CachedToken{
        AllowedActions:  []string{"GET", "POST"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "valid-nonce",
    }
    v.RegisterNonce("valid-nonce")

    result := v.Validate(tok, "GET")
    if result != DecisionAllow {
        t.Errorf("expected Allow, got %v", result)
    }
}

// TestValidate_ExpiredToken memverifikasi token kedaluwarsa ditolak
func TestValidate_ExpiredToken(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},
        ExpiresAt:       time.Now().Add(-1 * time.Second).Unix(), // sudah expired
        RevocationNonce: "nonce-abc",
    }
    v.RegisterNonce("nonce-abc")

    result := v.Validate(tok, "GET")
    if result != DecisionExpired {
        t.Errorf("expected Expired, got %v", result)
    }
}

// TestValidate_ExactExpiryBoundary memverifikasi token tepat pada detik kedaluwarsa ditolak (m1 strict)
func TestValidate_ExactExpiryBoundary(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},
        ExpiresAt:       time.Now().Unix(), // tepat detik ini
        RevocationNonce: "nonce-exact",
    }
    v.RegisterNonce("nonce-exact")

    result := v.Validate(tok, "GET")
    if result != DecisionExpired {
        t.Errorf("expected Expired at exact boundary, got %v", result)
    }
}

// TestValidate_RevokedNonce memverifikasi nonce yang dicabut menghasilkan Deny
func TestValidate_RevokedNonce(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "old-nonce", // nonce ini tidak terdaftar = sudah dicabut
    }
    // Tidak register nonce - simulasi setelah revokasi

    result := v.Validate(tok, "GET")
    if result != DecisionRevoked {
        t.Errorf("expected Revoked, got %v", result)
    }
}

// TestValidate_DisallowedAction memverifikasi action di luar allowed_actions ditolak
func TestValidate_DisallowedAction(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},           // hanya GET
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "nonce-ok",
    }
    v.RegisterNonce("nonce-ok")

    result := v.Validate(tok, "DELETE")             // coba DELETE
    if result != DecisionDeny {
        t.Errorf("expected Deny for disallowed action, got %v", result)
    }
}

// TestValidate_MultipleActions memverifikasi semua action yang diizinkan diterima
func TestValidate_MultipleActions(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET", "POST", "PUT"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "nonce-multi",
    }
    v.RegisterNonce("nonce-multi")

    for _, action := range []string{"GET", "POST", "PUT"} {
        result := v.Validate(tok, action)
        if result != DecisionAllow {
            t.Errorf("expected Allow for action %s, got %v", action, result)
        }
    }
}

// TestVerifySignature_Valid memverifikasi signature HMAC yang sah lolos verifikasi
func TestVerifySignature_Valid(t *testing.T) {
	key := []byte("secret-key-32-bytes-at-least-ok!")
	payload := CanonicalPayload("svc-a", "svc-b", []string{"GET"}, 1234567890, "nonce-1")

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !VerifySignature(payload, sig, key) {
		t.Error("expected VerifySignature to succeed for valid signature")
	}
}

// TestVerifySignature_Tampered memverifikasi payload yang diubah ditolak
func TestVerifySignature_Tampered(t *testing.T) {
	key := []byte("secret-key-32-bytes-at-least-ok!")
	payload := CanonicalPayload("svc-a", "svc-b", []string{"GET"}, 1234567890, "nonce-1")
	tampered := CanonicalPayload("svc-a", "svc-b", []string{"DELETE"}, 1234567890, "nonce-1")

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if VerifySignature(tampered, sig, key) {
		t.Error("expected VerifySignature to fail for tampered payload")
	}
}

// TestVerifySignature_InvalidEncoding memverifikasi signature bukan base64 ditolak
func TestVerifySignature_InvalidEncoding(t *testing.T) {
	key := []byte("secret-key-32-bytes-at-least-ok!")
	payload := []byte("test")
	if VerifySignature(payload, "invalid-base64!@#$%", key) {
		t.Error("expected VerifySignature to fail for invalid base64")
	}
}

// TestVerifySignature_EmptyInputs memverifikasi input kosong ditolak
func TestVerifySignature_EmptyInputs(t *testing.T) {
	key := []byte("secret-key-32-bytes-at-least-ok!")
	if VerifySignature(nil, "sig", key) {
		t.Error("expected false for empty payload")
	}
	if VerifySignature([]byte("payload"), "", key) {
		t.Error("expected false for empty sig")
	}
	if VerifySignature([]byte("payload"), "sig", nil) {
		t.Error("expected false for empty key")
	}
}

// TestSyncActiveNonces memverifikasi penggantian atomik registry nonce
func TestSyncActiveNonces(t *testing.T) {
	v := NewValidator("")
	v.RegisterNonce("nonce-old")

	tokOld := &CachedToken{
		AllowedActions:  []string{"GET"},
		ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
		RevocationNonce: "nonce-old",
	}
	tokNew := &CachedToken{
		AllowedActions:  []string{"GET"},
		ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
		RevocationNonce: "nonce-new",
	}

	if v.Validate(tokOld, "GET") != DecisionAllow {
		t.Error("expected old token to be allowed initially")
	}

	// Sync dengan hanya nonce-new
	v.SyncActiveNonces(map[string]bool{"nonce-new": true})

	if v.Validate(tokOld, "GET") != DecisionRevoked {
		t.Error("expected old token to be revoked after sync")
	}
	if v.Validate(tokNew, "GET") != DecisionAllow {
		t.Error("expected new token to be allowed after sync")
	}
}

// BenchmarkTokenValidation mengukur latensi evaluasi validasi token dan pengecekan nonce
func BenchmarkTokenValidation(b *testing.B) {
	v := NewValidator("test-registry")
	tok := &CachedToken{
		AllowedActions:  []string{"GET", "POST"},
		ExpiresAt:       time.Now().Add(60 * time.Second).Unix(),
		RevocationNonce: "nonce-12345",
	}
	v.RegisterNonce("nonce-12345")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Validate(tok, "GET")
	}
}
