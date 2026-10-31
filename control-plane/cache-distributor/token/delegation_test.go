// control-plane/cache-distributor/token/delegation_test.go
package token

import (
    "testing"
    "time"
)

// TestNewDelegationToken_FieldsPopulated memverifikasi semua field terisi dengan benar
func TestNewDelegationToken_FieldsPopulated(t *testing.T) {
    nonce, _ := GenerateNonce()
    token, err := NewDelegationToken(
        "spiffe://cluster.local/ns/default/sa/service-a",
        "spiffe://cluster.local/ns/default/sa/service-b",
        []string{"GET", "POST"},
        "medium",
        30*time.Second,
        nonce,
    )

    if err != nil {
        t.Fatalf("NewDelegationToken() error: %v", err)
    }
    if token.SourceIdentity == "" {
        t.Error("SourceIdentity is empty")
    }
    if token.TargetIdentity == "" {
        t.Error("TargetIdentity is empty")
    }
    if len(token.AllowedActions) != 2 {
        t.Errorf("expected 2 actions, got %d", len(token.AllowedActions))
    }
    if token.RevocationNonce == "" {
        t.Error("RevocationNonce is empty")
    }
    if token.CacheTTLMs != 30000 {
        t.Errorf("expected CacheTTLMs=30000, got %d", token.CacheTTLMs)
    }
    if token.Sensitivity != "medium" {
        t.Errorf("expected Sensitivity=medium, got %s", token.Sensitivity)
    }
}

// TestNewDelegationToken_ExpiryInFuture memverifikasi ExpiresAt selalu di masa depan
func TestNewDelegationToken_ExpiryInFuture(t *testing.T) {
    nonce, _ := GenerateNonce()
    token, err := NewDelegationToken("a", "b", []string{"GET"}, "low", 30*time.Second, nonce)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    now := time.Now().Unix()
    if token.ExpiresAt <= now {
        t.Errorf("ExpiresAt %d is not in the future (now=%d)", token.ExpiresAt, now)
    }
}

// TestNewDelegationToken_UniqueNonces memverifikasi setiap token mendapat nonce unik
func TestNewDelegationToken_UniqueNonces(t *testing.T) {
    nonces := make(map[string]bool)

    for i := 0; i < 100; i++ {
        nonce, _ := GenerateNonce()
        tok, err := NewDelegationToken("a", "b", []string{"GET"}, "low", 30*time.Second, nonce)
        if err != nil {
            t.Fatalf("unexpected error at iteration %d: %v", i, err)
        }
        if nonces[tok.RevocationNonce] {
            t.Errorf("duplicate nonce found: %s", tok.RevocationNonce)
        }
        nonces[tok.RevocationNonce] = true
    }
}

// TestNewDelegationToken_ZeroTTL memverifikasi TTL nol menghasilkan error
func TestNewDelegationToken_ZeroTTL(t *testing.T) {
    _, err := NewDelegationToken("a", "b", []string{"GET"}, "low", 0, "nonce")
    if err == nil {
        t.Error("expected error for zero TTL")
    }
}

// TestNewDelegationToken_EmptyActions memverifikasi actions kosong menghasilkan error
func TestNewDelegationToken_EmptyActions(t *testing.T) {
    _, err := NewDelegationToken("a", "b", []string{}, "low", 30*time.Second, "nonce")
    if err == nil {
        t.Error("expected error for empty actions")
    }
}

// TestNewDelegationToken_EmptyIdentity memverifikasi identitas kosong menghasilkan error
func TestNewDelegationToken_EmptyIdentity(t *testing.T) {
    _, err := NewDelegationToken("", "service-b", []string{"GET"}, "low", 30*time.Second, "nonce")
    if err == nil {
        t.Error("expected error for empty source identity")
    }
}

// TestNewDelegationToken_EmptyNonce memverifikasi nonce kosong menghasilkan error
func TestNewDelegationToken_EmptyNonce(t *testing.T) {
    _, err := NewDelegationToken("a", "b", []string{"GET"}, "low", 30*time.Second, "")
    if err == nil {
        t.Error("expected error for empty nonce")
    }
}

// TestDelegationToken_ToJSON memverifikasi serialisasi ke JSON
func TestDelegationToken_ToJSON(t *testing.T) {
    nonce, _ := GenerateNonce()
    tok, _ := NewDelegationToken("a", "b", []string{"GET"}, "low", 30*time.Second, nonce)
    data, err := tok.ToJSON()
    if err != nil {
        t.Fatalf("ToJSON() failed: %v", err)
    }
    if len(data) == 0 {
        t.Error("ToJSON() returned empty bytes")
    }
}

// TestDelegationToken_IsExpired memverifikasi pengecekan expiry
func TestDelegationToken_IsExpired(t *testing.T) {
    nonce, _ := GenerateNonce()
    tok, _ := NewDelegationToken("a", "b", []string{"GET"}, "low", 30*time.Second, nonce)
    if tok.IsExpired() {
        t.Error("fresh token should not be expired")
    }
}

// TestNewDelegationToken_TTLClamp memverifikasi TTL > 30s di-clamp ke 30s (R4)
func TestNewDelegationToken_TTLClamp(t *testing.T) {
    nonce, _ := GenerateNonce()
    tok, err := NewDelegationToken("a", "b", []string{"GET"}, "low", 120*time.Second, nonce)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if tok.CacheTTLMs > 30000 {
        t.Errorf("expected CacheTTLMs <= 30000 (MaxTTL clamp), got %d", tok.CacheTTLMs)
    }
}
