// control-plane/graph-analyzer/cache/candidate.go
package cache

import (
	"os"
	"strconv"
	"time"

	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/graph"
)

// DefaultMaxCandidates membaca batas maksimum kandidat dari environment variable MAX_CANDIDATES.
// Jika tidak disetel atau tidak valid, fungsi mengembalikan nilai default 50.
func DefaultMaxCandidates() int {
	if v := os.Getenv("MAX_CANDIDATES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 50
}

// CacheCandidate merepresentasikan pasangan service yang layak mendapat delegation token
type CacheCandidate struct {
	Source          string
	Target          string
	AllowedActions  []string
	SuggestedTTLMs  int64
	Sensitivity     SensitivityLevel
	CentralityScore float64
	CallsPerMin     int64
}

// CandidateSelector menyeleksi pasangan service untuk cache delegation
type CandidateSelector struct {
	minCallsPerMin int64
	maxCandidates  int
	mode           graph.PrioritizationMode
}

// NewCandidateSelector membuat CandidateSelector baru dengan mode default ModeGraphAware
func NewCandidateSelector(minCallsPerMin int64, maxCandidates int) *CandidateSelector {
	return &CandidateSelector{
		minCallsPerMin: minCallsPerMin,
		maxCandidates:  maxCandidates,
		mode:           graph.ModeGraphAware,
	}
}

// SetMode mengatur mode algoritma prioritisasi (ModeGraphAware atau ModeFrequencyOnly)
func (s *CandidateSelector) SetMode(mode graph.PrioritizationMode) {
	s.mode = mode
}

// Mode mengembalikan mode algoritma prioritisasi yang sedang aktif
func (s *CandidateSelector) Mode() graph.PrioritizationMode {
	if s.mode == "" {
		return graph.ModeGraphAware
	}
	return s.mode
}

// Select menyeleksi kandidat cache dari graph berdasarkan frekuensi dan centrality.
// Alur internal:
//  1. GetHighFrequencyPairs(minCallsPerMin)
//  2. RankEdgesByImportanceMode(centrality, mode)
//  3. Untuk tiap edge teratas: panggil policyFn untuk dapat actions & sensitivity
//  4. CalculateTTL per kandidat
//  5. Return slice CacheCandidate, max maxCandidates entri
//
// policyFn adalah fungsi callback - dipasok dari main.go yang memanggil OPA
// untuk mendapat allowed_actions per pasangan. Ini memisahkan logika policy
// dari logika seleksi.
// CertValidityProvider menyediakan sisa dan total durasi sertifikat mTLS untuk sebuah service.
type CertValidityProvider func(source string) (remaining, total time.Duration)

// DefaultCertProvider merepresentasikan durasi rotasi sertifikat SPIFFE/mTLS standar Istio (24 jam)
// pada kondisi operasional steady-state saat metadata SDS tidak dipolling secara dinamis oleh analyzer.
// Pada prototipe testbed fisik, faktor v_c bernilai 1.0. Perilaku degradasi batas (v_c -> 0)
// divalidasi secara independen menggunakan SelectWithCert dan suite pengujian unit ttl_test.go.
func DefaultCertProvider(source string) (time.Duration, time.Duration) {
	return 24 * time.Hour, 24 * time.Hour
}

// Select menyeleksi kandidat cache dari graph berdasarkan frekuensi dan centrality menggunakan DefaultCertProvider.
func (s *CandidateSelector) Select(
	g *graph.TopologyGraph,
	centrality map[string]float64,
	policyFn func(source, target string) ([]string, SensitivityLevel),
) []*CacheCandidate {
	return s.SelectWithCert(g, centrality, policyFn, DefaultCertProvider)
}

// SelectWithCert menyeleksi kandidat cache dengan penyedia validitas sertifikat mTLS dinamis.
func (s *CandidateSelector) SelectWithCert(
	g *graph.TopologyGraph,
	centrality map[string]float64,
	policyFn func(source, target string) ([]string, SensitivityLevel),
	certFn CertValidityProvider,
) []*CacheCandidate {
	// 1. Ambil edge dengan frekuensi tinggi
	pairs := g.GetHighFrequencyPairs(s.minCallsPerMin)
	if len(pairs) == 0 {
		return nil
	}

	// 2. Rank berdasarkan importance sesuai mode yang aktif (sudah terurut descending)
	ranked := graph.RankEdgesByImportanceMode(g, centrality, s.Mode())

	// Filter: hanya ambil edge yang ada di high-frequency pairs
	highFreqSet := make(map[string]bool)
	for _, edge := range pairs {
		key := edge.Source + "->" + edge.Target
		highFreqSet[key] = true
	}

	var candidates []*CacheCandidate
	coldStartCutoff := time.Now().Add(-10 * time.Second)
	for _, edge := range ranked {
		key := edge.Source + "->" + edge.Target
		if !highFreqSet[key] {
			continue
		}

		// 3. Query policy untuk actions & sensitivity
		actions, sensitivity := policyFn(edge.Source, edge.Target)
		if len(actions) == 0 {
			continue
		}

		// 4. Hitung TTL berdasarkan frekuensi, sensitivitas, dan sisa umur sertifikat mTLS
		certRemaining, certTotal := DefaultCertProvider(edge.Source)
		if certFn != nil {
			certRemaining, certTotal = certFn(edge.Source)
		}
		callsPerMin := edge.CallsPerMinute()
		if callsPerMin <= 0 && edge.LastSeen.After(coldStartCutoff) {
			// Hanya izinkan fallback CallCount pada fase cold-start (10 detik pertama sejak LastSeen)
			callsPerMin = edge.CallCount
		}
		ttl := CalculateTTL(callsPerMin, sensitivity, certRemaining, certTotal)
		if ttl < time.Millisecond {
			continue // Abaikan edge jika sertifikat kedaluwarsa atau sisa < 1ms agar tidak memicu truncation ke 0ms
		}

		candidates = append(candidates, &CacheCandidate{
			Source:          edge.Source,
			Target:          edge.Target,
			AllowedActions:  actions,
			SuggestedTTLMs:  ttl.Milliseconds(),
			Sensitivity:     sensitivity,
			CentralityScore: centrality[edge.Source],
			CallsPerMin:     callsPerMin,
		})

		// 5. Limit ke maxCandidates
		if len(candidates) >= s.maxCandidates {
			break
		}
	}

	// Candidates sudah terurut sesuai composite priority score S(e) dari RankEdgesByImportance
	return candidates
}
