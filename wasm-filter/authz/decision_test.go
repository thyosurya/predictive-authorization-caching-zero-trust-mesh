// wasm-filter/authz/decision_test.go
package authz

import (
	"testing"
)

// TestDecisionEngine_CacheHit memverifikasi cache hit menghasilkan Allow tanpa ke PDP
func TestDecisionEngine_CacheHit(t *testing.T) {
	cache := NewMockLocalCache()
	cache.Set("service-a->service-b", validToken())

	engine := NewDecisionEngine(cache, nil) // nil = PDP tidak dipanggil
	engine.GetValidator().RegisterNonce("valid-nonce")
	result := engine.Decide("service-a", "service-b", "GET")

	if result.Decision != Allow {
		t.Errorf("expected cache hit Allow, got %v", result.Decision)
	}
	if !result.FromCache {
		t.Error("expected result to be from cache")
	}
}

// TestDecisionEngine_CacheMiss_FallbackToPDP memverifikasi cache miss meneruskan ke PDP
func TestDecisionEngine_CacheMiss_FallbackToPDP(t *testing.T) {
	cache  := NewMockLocalCache()         // cache kosong
	pdp    := NewMockPDP(true)            // PDP akan izinkan
	engine := NewDecisionEngine(cache, pdp)

	result := engine.Decide("service-a", "service-b", "GET")

	if result.Decision != Allow {
		t.Errorf("expected PDP fallback Allow, got %v", result.Decision)
	}
	if result.FromCache {
		t.Error("expected result NOT from cache (was a miss)")
	}
	if !pdp.WasCalled() {
		t.Error("expected PDP to be called on cache miss")
	}
}

// TestDecisionEngine_CacheMiss_PDPDeny memverifikasi PDP menolak diteruskan sebagai Deny
func TestDecisionEngine_CacheMiss_PDPDeny(t *testing.T) {
	cache  := NewMockLocalCache()
	pdp    := NewMockPDP(false)           // PDP akan tolak
	engine := NewDecisionEngine(cache, pdp)

	result := engine.Decide("service-x", "service-y", "DELETE")

	if result.Decision != Deny {
		t.Errorf("expected PDP Deny, got %v", result.Decision)
	}
}

// TestDecisionEngine_ExpiredCacheEntry memverifikasi entri cache kedaluwarsa dihapus dan ke PDP
func TestDecisionEngine_ExpiredCacheEntry(t *testing.T) {
	cache := NewMockLocalCache()
	cache.Set("service-a->service-b", expiredToken()) // token expired

	pdp    := NewMockPDP(true)
	engine := NewDecisionEngine(cache, pdp)

	result := engine.Decide("service-a", "service-b", "GET")

	if !pdp.WasCalled() {
		t.Error("expected PDP to be called after expired cache entry")
	}
	// Entri lama harus sudah dihapus dari cache
	if _, ok := cache.Get("service-a->service-b"); ok {
		t.Error("expected expired cache entry to be deleted")
	}
	_ = result
}
