// control-plane/graph-analyzer/cache/ttl.go
package cache

import "time"

// SensitivityLevel menentukan seberapa ketat TTL dikurangi
type SensitivityLevel int

const (
    SensitivityLow    SensitivityLevel = iota // penalty 0%
    SensitivityMedium                         // penalty 30%
    SensitivityHigh                           // penalty 70%
)

const (
    BaseTTL = 30 * time.Second
    MaxTTL  = 30 * time.Second // Selaras dengan Persamaan (1) di naskah [5s, 30s]
    MinTTL  = 5 * time.Second
)

// CalculateTTL menghitung TTL optimal untuk delegation token
// berdasarkan frekuensi panggilan, sensitivitas data, dan validitas sertifikat.
//
// Catatan Kebijakan Clamping (Design Trade-off Option B):
// Jika TTL_raw < MinTTL, nilai di-clamp ke MinTTL (5 detik).
// Rasional: Meskipun entitas berisiko tinggi secara teoritis memerlukan evaluasi ulang segera,
// memberikan TTL < 5 detik memicu cache thrashing parah dan antrean koneksi ke PDP eksternal.
// Batas bawah 5 detik mengamortisasi overhead jaringan sidecar pada konkurensi tinggi,
// dengan trade-off batas paparan maksimum 5 detik sebagai kompromi operasional terukur.
func CalculateTTL(
    callsPerMin    int64,
    sensitivity    SensitivityLevel,
    certRemaining  time.Duration,
    certTotal      time.Duration,
) time.Duration {
    // Guard terhadap pembagian nol atau sertifikat yang sudah kedaluwarsa
    if certTotal <= 0 || certRemaining <= 0 {
        return 0
    }

    // freq_weight: makin tinggi frekuensi, makin panjang TTL (max 1.0)
    // Sesuai Persamaan (1) di naskah: w_f = min(f_calls / 100, 1.0)
    freqWeight := float64(callsPerMin) / 100.0
    if freqWeight > 1.0 {
        freqWeight = 1.0
    }
    if freqWeight < 0.0 {
        freqWeight = 0.0
    }

    // sensitivity_penalty: data sensitif dapat TTL lebih pendek
    var sensitivityPenalty float64
    switch sensitivity {
    case SensitivityLow:
        sensitivityPenalty = 0.0
    case SensitivityMedium:
        sensitivityPenalty = 0.3
    case SensitivityHigh:
        sensitivityPenalty = 0.7
    default:
        sensitivityPenalty = 0.3
    }

    // cert_validity_factor: rasio sisa validitas sertifikat
    certFactor := float64(certRemaining) / float64(certTotal)
    if certFactor > 1.0 {
        certFactor = 1.0
    }
    if certFactor < 0.0 {
        certFactor = 0.0
    }

    ttl := float64(BaseTTL) * freqWeight * (1 - sensitivityPenalty) * certFactor

    result := time.Duration(ttl)
    if result > MaxTTL {
        result = MaxTTL
    }
    if result < MinTTL {
        result = MinTTL
    }
    // Hard ceiling: Token tidak boleh pernah melampaui sisa umur sertifikat mTLS
    if result > certRemaining {
        result = certRemaining
    }
    return result
}

// String mengembalikan representasi string "low", "medium", atau "high"
func (s SensitivityLevel) String() string {
	switch s {
	case SensitivityLow:
		return "low"
	case SensitivityHigh:
		return "high"
	default:
		return "medium"
	}
}

// SensitivityFromString konversi string "low"/"medium"/"high" -> SensitivityLevel.
// Return SensitivityMedium jika tidak dikenal (safe default).
func SensitivityFromString(s string) SensitivityLevel {
	switch s {
	case "low":
		return SensitivityLow
	case "medium":
		return SensitivityMedium
	case "high":
		return SensitivityHigh
	default:
		return SensitivityMedium
	}
}
