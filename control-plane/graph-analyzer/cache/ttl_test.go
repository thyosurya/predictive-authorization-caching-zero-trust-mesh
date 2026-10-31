// control-plane/graph-analyzer/cache/ttl_test.go
package cache

import (
    "testing"
    "time"
)

// TestCalculateTTL_NominalCase memverifikasi output TTL dalam rentang valid
func TestCalculateTTL_NominalCase(t *testing.T) {
    ttl := CalculateTTL(50, SensitivityMedium, 72*time.Hour, 72*time.Hour)

    if ttl < MinTTL {
        t.Errorf("TTL %v below MinTTL %v", ttl, MinTTL)
    }
    if ttl > MaxTTL {
        t.Errorf("TTL %v above MaxTTL %v", ttl, MaxTTL)
    }
}

// TestCalculateTTL_HighFrequencyIncreaseTTL memverifikasi frekuensi tinggi menghasilkan TTL lebih panjang
func TestCalculateTTL_HighFrequencyIncreaseTTL(t *testing.T) {
    ttlLow  := CalculateTTL(10,  SensitivityLow, 72*time.Hour, 72*time.Hour)
    ttlHigh := CalculateTTL(100, SensitivityLow, 72*time.Hour, 72*time.Hour)

    if ttlHigh <= ttlLow {
        t.Errorf("expected higher freq to produce longer TTL: low=%v high=%v", ttlLow, ttlHigh)
    }
}

// TestCalculateTTL_HighSensitivityReducesTTL memverifikasi sensitivitas tinggi mengurangi TTL
func TestCalculateTTL_HighSensitivityReducesTTL(t *testing.T) {
    ttlLow  := CalculateTTL(50, SensitivityLow,  72*time.Hour, 72*time.Hour)
    ttlHigh := CalculateTTL(50, SensitivityHigh, 72*time.Hour, 72*time.Hour)

    if ttlHigh >= ttlLow {
        t.Errorf("expected high sensitivity to produce shorter TTL: low=%v high=%v", ttlLow, ttlHigh)
    }
}

// TestCalculateTTL_NeverExceedsMaxTTL memverifikasi batas atas selalu dihormati
func TestCalculateTTL_NeverExceedsMaxTTL(t *testing.T) {
    // Kondisi terbaik: frekuensi sangat tinggi, sensitivitas rendah, sertifikat masih panjang
    ttl := CalculateTTL(10000, SensitivityLow, 72*time.Hour, 72*time.Hour)

    if ttl > MaxTTL {
        t.Errorf("TTL %v exceeded MaxTTL %v", ttl, MaxTTL)
    }
}

// TestCalculateTTL_NeverBelowMinTTL memverifikasi batas bawah selalu dihormati
func TestCalculateTTL_NeverBelowMinTTL(t *testing.T) {
    // Kondisi terburuk: frekuensi sangat rendah, sensitivitas tinggi, sertifikat hampir habis
    ttl := CalculateTTL(1, SensitivityHigh, 1*time.Minute, 72*time.Hour)

    if ttl < MinTTL {
        t.Errorf("TTL %v below MinTTL %v", ttl, MinTTL)
    }
}

// TestCalculateTTL_CertExpiryReducesTTL memverifikasi sertifikat hampir habis mengurangi TTL
func TestCalculateTTL_CertExpiryReducesTTL(t *testing.T) {
    ttlFresh  := CalculateTTL(50, SensitivityLow, 72*time.Hour, 72*time.Hour)
    ttlExpiry := CalculateTTL(50, SensitivityLow, 1*time.Hour,  72*time.Hour)

    if ttlExpiry >= ttlFresh {
        t.Errorf("expected near-expiry cert to reduce TTL: fresh=%v expiry=%v", ttlFresh, ttlExpiry)
    }
}

// TestCalculateTTL_ZeroFrequency memverifikasi frekuensi nol menghasilkan MinTTL
func TestCalculateTTL_ZeroFrequency(t *testing.T) {
    ttl := CalculateTTL(0, SensitivityLow, 72*time.Hour, 72*time.Hour)
    if ttl != MinTTL {
        t.Errorf("expected MinTTL for zero frequency, got %v", ttl)
    }
}

// TestSensitivityLevels_Ordering memverifikasi urutan: Low > Medium > High TTL
func TestSensitivityLevels_Ordering(t *testing.T) {
    low    := CalculateTTL(50, SensitivityLow,    72*time.Hour, 72*time.Hour)
    medium := CalculateTTL(50, SensitivityMedium, 72*time.Hour, 72*time.Hour)
    high   := CalculateTTL(50, SensitivityHigh,   72*time.Hour, 72*time.Hour)

    if !(low >= medium && medium >= high) {
        t.Errorf("TTL ordering wrong: low=%v medium=%v high=%v", low, medium, high)
    }
}

// TestCalculateTTL_ExpiredCertReturnsZero memverifikasi sertifikat kedaluwarsa mengembalikan TTL 0
func TestCalculateTTL_ExpiredCertReturnsZero(t *testing.T) {
    ttlExpired := CalculateTTL(100, SensitivityLow, 0, 72*time.Hour)
    if ttlExpired != 0 {
        t.Errorf("expected 0 for expired cert, got %v", ttlExpired)
    }

    ttlNegative := CalculateTTL(100, SensitivityLow, -5*time.Second, 72*time.Hour)
    if ttlNegative != 0 {
        t.Errorf("expected 0 for negative cert remaining, got %v", ttlNegative)
    }

    ttlZeroTotal := CalculateTTL(100, SensitivityLow, 10*time.Second, 0)
    if ttlZeroTotal != 0 {
        t.Errorf("expected 0 for zero cert total, got %v", ttlZeroTotal)
    }
}

// TestCalculateTTL_HardCeilingNeverExceedsCertRemaining memverifikasi TTL tidak pernah melebihi sisa sertifikat
func TestCalculateTTL_HardCeilingNeverExceedsCertRemaining(t *testing.T) {
    // Sisa sertifikat hanya 2 detik (di bawah MinTTL 5 detik)
    certRemaining := 2 * time.Second
    ttl := CalculateTTL(100, SensitivityLow, certRemaining, 72*time.Hour)

    if ttl > certRemaining {
        t.Errorf("TTL %v exceeded certRemaining %v", ttl, certRemaining)
    }
}

// BenchmarkCalculateTTL mengukur efisiensi perhitungan adaptive TTL
func BenchmarkCalculateTTL(b *testing.B) {
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = CalculateTTL(50, SensitivityMedium, 12*time.Hour, 24*time.Hour)
    }
}


