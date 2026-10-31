package metrics

import (
	"testing"
)

// TestMetricsRegistration memverifikasi semua metrics terdaftar tanpa panic
func TestMetricsRegistration(t *testing.T) {
	// Metrics terdaftar via init() - jika sampai sini tanpa panic, berhasil
	if CacheHits == nil {
		t.Fatal("CacheHits is nil")
	}
	if CacheMisses == nil {
		t.Fatal("CacheMisses is nil")
	}
	if RevocationLatency == nil {
		t.Fatal("RevocationLatency is nil")
	}
	if TokenTTL == nil {
		t.Fatal("TokenTTL is nil")
	}
	if TokensIssuedTotal == nil {
		t.Fatal("TokensIssuedTotal is nil")
	}
	if TokensActive == nil {
		t.Fatal("TokensActive is nil")
	}
	if RevocationEventsTotal == nil {
		t.Fatal("RevocationEventsTotal is nil")
	}
	if GrpcStreamCount == nil {
		t.Fatal("GrpcStreamCount is nil")
	}
}

// TestCacheHitsIncrement memverifikasi CacheHits counter bisa di-increment
func TestCacheHitsIncrement(t *testing.T) {
	CacheHits.WithLabelValues("source-a", "target-b").Inc()
	// Tidak panic = berhasil
}

// TestRevocationLatencyObserve memverifikasi histogram bisa menerima observasi
func TestRevocationLatencyObserve(t *testing.T) {
	RevocationLatency.Observe(42.0)
}

// TestTokenTTLObserve memverifikasi TokenTTL histogram bisa menerima observasi
func TestTokenTTLObserve(t *testing.T) {
	TokenTTL.WithLabelValues("source", "target").Observe(30000)
}
