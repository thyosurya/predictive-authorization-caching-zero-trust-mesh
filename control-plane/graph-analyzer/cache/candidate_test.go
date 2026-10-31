// control-plane/graph-analyzer/cache/candidate_test.go
package cache

import (
	"os"
	"testing"
	"time"

	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/graph"
)

func TestCandidateSelector_SelectWithCert_ExpiredCertSkipped(t *testing.T) {
	g := graph.NewTopologyGraph()
	// Add calls between svc-a -> svc-b (high frequency) and svc-c -> svc-d
	for i := 0; i < 20; i++ {
		g.RecordCall("svc-a", "svc-b")
		g.RecordCall("svc-c", "svc-d")
	}

	centrality := map[string]float64{
		"svc-a": 0.8,
		"svc-c": 0.5,
	}

	policyFn := func(src, dst string) ([]string, SensitivityLevel) {
		return []string{"GET"}, SensitivityLow
	}

	// Cert provider: svc-a has expired cert (0 remaining), svc-c has valid cert
	certFn := func(src string) (time.Duration, time.Duration) {
		if src == "svc-a" {
			return 0, 24 * time.Hour // expired
		}
		return 12 * time.Hour, 24 * time.Hour
	}

	selector := NewCandidateSelector(10, 1) // min 1 call/min, max 10 candidates
	candidates := selector.SelectWithCert(g, centrality, policyFn, certFn)

	if len(candidates) != 1 {
		t.Fatalf("expected exactly 1 candidate (svc-c -> svc-d), got %d", len(candidates))
	}

	if candidates[0].Source != "svc-c" || candidates[0].Target != "svc-d" {
		t.Errorf("expected candidate svc-c -> svc-d, got %s -> %s", candidates[0].Source, candidates[0].Target)
	}

	if candidates[0].SuggestedTTLMs <= 0 {
		t.Errorf("expected positive TTL, got %d", candidates[0].SuggestedTTLMs)
	}
}

func TestCandidateSelector_MaxCandidatesLimit(t *testing.T) {
	g := graph.NewTopologyGraph()
	// Buat 4 pasangan panggilan dengan frekuensi tinggi
	for i := 0; i < 20; i++ {
		g.RecordCall("svc-1", "svc-2")
		g.RecordCall("svc-2", "svc-3")
		g.RecordCall("svc-3", "svc-4")
		g.RecordCall("svc-4", "svc-5")
	}

	centrality := map[string]float64{
		"svc-1": 0.1,
		"svc-2": 0.8,
		"svc-3": 0.5,
		"svc-4": 0.2,
	}

	policyFn := func(src, dst string) ([]string, SensitivityLevel) {
		return []string{"GET"}, SensitivityLow
	}

	// Batasi hanya 2 kandidat teratas (kondisi cache contention)
	selector := NewCandidateSelector(1, 2)
	candidates := selector.Select(g, centrality, policyFn)

	if len(candidates) != 2 {
		t.Fatalf("expected exactly 2 candidates due to limit, got %d", len(candidates))
	}
}

func TestDefaultMaxCandidates(t *testing.T) {
	orig := os.Getenv("MAX_CANDIDATES")
	defer os.Setenv("MAX_CANDIDATES", orig)

	// Kasus default saat env kosong
	os.Unsetenv("MAX_CANDIDATES")
	if n := DefaultMaxCandidates(); n != 50 {
		t.Errorf("expected default 50, got %d", n)
	}

	// Kasus disetel ke nilai valid
	os.Setenv("MAX_CANDIDATES", "2")
	if n := DefaultMaxCandidates(); n != 2 {
		t.Errorf("expected configured 2, got %d", n)
	}

	// Kasus nilai tidak valid / non-positif
	os.Setenv("MAX_CANDIDATES", "-5")
	if n := DefaultMaxCandidates(); n != 50 {
		t.Errorf("expected fallback 50 for negative value, got %d", n)
	}
	os.Setenv("MAX_CANDIDATES", "invalid")
	if n := DefaultMaxCandidates(); n != 50 {
		t.Errorf("expected fallback 50 for non-numeric value, got %d", n)
	}
}

func TestCandidateSelector_ModeSwitching(t *testing.T) {
	g := graph.NewTopologyGraph()
	for i := 0; i < 100; i++ {
		g.RecordCall("frontend", "productcatalogservice")
	}
	for i := 0; i < 70; i++ {
		g.RecordCall("cartservice", "productcatalogservice")
	}

	centrality := map[string]float64{
		"frontend":    0.0,
		"cartservice": 0.8,
	}

	policyFn := func(src, dst string) ([]string, SensitivityLevel) {
		return []string{"GET"}, SensitivityLow
	}

	// Kuota hanya 1 kandidat (persaingan ketat)
	selector := NewCandidateSelector(1, 1)

	// Default: ModeGraphAware -> bridge edge (cartservice) menang
	if selector.Mode() != graph.ModeGraphAware {
		t.Errorf("expected default mode ModeGraphAware, got %s", selector.Mode())
	}
	candidatesGraph := selector.Select(g, centrality, policyFn)
	if len(candidatesGraph) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidatesGraph))
	}
	if candidatesGraph[0].Source != "cartservice" {
		t.Errorf("Graph-Aware mode must select 'cartservice' as top candidate, got '%s'", candidatesGraph[0].Source)
	}

	// Ganti ke ModeFrequencyOnly -> direct edge (frontend, 100 calls) menang
	selector.SetMode(graph.ModeFrequencyOnly)
	if selector.Mode() != graph.ModeFrequencyOnly {
		t.Errorf("expected mode ModeFrequencyOnly, got %s", selector.Mode())
	}
	candidatesLFU := selector.Select(g, centrality, policyFn)
	if len(candidatesLFU) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidatesLFU))
	}
	if candidatesLFU[0].Source != "frontend" {
		t.Errorf("LFU mode must select highest-frequency 'frontend' as top candidate, got '%s'", candidatesLFU[0].Source)
	}
}

