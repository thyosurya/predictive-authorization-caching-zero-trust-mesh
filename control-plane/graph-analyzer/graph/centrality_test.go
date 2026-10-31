package graph

import (
	"testing"
)

// TestComputeBetweenness_LinearChain memverifikasi node tengah dalam chain memiliki
// skor betweenness tertinggi: A → B → C (B adalah hub)
func TestComputeBetweenness_LinearChain(t *testing.T) {
	g := NewTopologyGraph()
	g.RecordCall("A", "B")
	g.RecordCall("B", "C")

	scores := ComputeBetweenness(g)

	// B harus memiliki skor tertinggi (hub antara A dan C)
	if len(scores) == 0 {
		t.Fatal("expected non-empty centrality scores")
	}
	if scores["B"] < scores["A"] || scores["B"] < scores["C"] {
		t.Errorf("expected B to have highest centrality: A=%.2f B=%.2f C=%.2f",
			scores["A"], scores["B"], scores["C"])
	}
}

// TestComputeBetweenness_EmptyGraph memverifikasi graph kosong menghasilkan map kosong
func TestComputeBetweenness_EmptyGraph(t *testing.T) {
	g := NewTopologyGraph()
	scores := ComputeBetweenness(g)
	if len(scores) != 0 {
		t.Errorf("expected empty scores, got %d entries", len(scores))
	}
}

// TestComputeBetweenness_SingleEdge memverifikasi graph satu edge menghasilkan skor 0
func TestComputeBetweenness_SingleEdge(t *testing.T) {
	g := NewTopologyGraph()
	g.RecordCall("X", "Y")
	scores := ComputeBetweenness(g)
	// Dengan hanya 2 node, betweenness harus 0 untuk keduanya
	for node, score := range scores {
		if score != 0.0 {
			t.Errorf("expected 0 betweenness for %s with single edge, got %.2f", node, score)
		}
	}
}

// TestComputeBetweenness_Normalized memverifikasi skor dinormalisasi ke 0.0-1.0
func TestComputeBetweenness_Normalized(t *testing.T) {
	g := NewTopologyGraph()
	g.RecordCall("A", "B")
	g.RecordCall("B", "C")
	g.RecordCall("C", "D")
	g.RecordCall("A", "C")

	scores := ComputeBetweenness(g)
	for node, score := range scores {
		if score < 0.0 || score > 1.0 {
			t.Errorf("score for %s out of range [0,1]: %.4f", node, score)
		}
	}
}

// TestRankEdgesByImportance memverifikasi edge diurutkan descending berdasarkan skor gabungan
func TestRankEdgesByImportance(t *testing.T) {
	g := NewTopologyGraph()
	// Edge dengan call count berbeda
	for i := 0; i < 50; i++ {
		g.RecordCall("hot-source", "hot-target")
	}
	for i := 0; i < 5; i++ {
		g.RecordCall("cold-source", "cold-target")
	}

	centrality := map[string]float64{
		"hot-source":  0.9,
		"cold-source": 0.1,
	}

	ranked := RankEdgesByImportance(g, centrality)

	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked edges, got %d", len(ranked))
	}
	// Hot edge harus di posisi pertama
	if ranked[0].Source != "hot-source" {
		t.Errorf("expected hot-source first, got %s", ranked[0].Source)
	}
}

// TestRankEdgesByImportance_EmptyGraph memverifikasi graph kosong menghasilkan slice kosong
func TestRankEdgesByImportance_EmptyGraph(t *testing.T) {
	g := NewTopologyGraph()
	ranked := RankEdgesByImportance(g, map[string]float64{})
	if len(ranked) != 0 {
		t.Errorf("expected empty result, got %d edges", len(ranked))
	}
}

// TestRankEdgesByImportance_Formula memverifikasi formula 0.7*calls + 0.3*centrality
func TestRankEdgesByImportance_Formula(t *testing.T) {
	g := NewTopologyGraph()
	// Edge A: banyak call, rendah centrality
	for i := 0; i < 100; i++ {
		g.RecordCall("A", "B")
	}
	// Edge C: sedikit call, tinggi centrality
	for i := 0; i < 10; i++ {
		g.RecordCall("C", "D")
	}

	centrality := map[string]float64{
		"A": 0.1, // rendah
		"C": 1.0, // tinggi
	}

	ranked := RankEdgesByImportance(g, centrality)

	if len(ranked) < 2 {
		t.Fatalf("expected at least 2 edges, got %d", len(ranked))
	}
	// Edge A harus menang karena 0.7*1.0 + 0.3*0.1 = 0.73 > 0.7*0.1 + 0.3*1.0 = 0.37
	if ranked[0].Source != "A" {
		t.Errorf("expected A first (high call weight dominates), got %s", ranked[0].Source)
	}
}

// TestRankEdgesByImportance_AfterReset memverifikasi Reset membersihkan graph sehingga tidak ada edge
func TestRankEdgesByImportance_AfterReset(t *testing.T) {
	g := NewTopologyGraph()
	g.RecordCall("A", "B")

	// Reset menghapus semua edge dan node
	g.Reset()

	ranked := RankEdgesByImportance(g, map[string]float64{"A": 0.5})
	if len(ranked) != 0 {
		t.Errorf("expected empty after reset, got %d", len(ranked))
	}
}

// TestRankEdgesByImportance_DeterministicTieBreaking memverifikasi 5-tier deterministic tie-breaking
func TestRankEdgesByImportance_DeterministicTieBreaking(t *testing.T) {
	g := NewTopologyGraph()
	// Insert edges in non-sorted order
	g.RecordCall("svc-b", "svc-z")
	g.RecordCall("svc-b", "svc-a")
	g.RecordCall("svc-a", "svc-y")
	g.RecordCall("svc-a", "svc-x")

	// Centrality identik untuk semua source
	centrality := map[string]float64{
		"svc-a": 0.5,
		"svc-b": 0.5,
	}

	ranked := RankEdgesByImportance(g, centrality)
	if len(ranked) != 4 {
		t.Fatalf("expected 4 ranked edges, got %d", len(ranked))
	}

	expected := []struct {
		src string
		tgt string
	}{
		{"svc-a", "svc-x"},
		{"svc-a", "svc-y"},
		{"svc-b", "svc-a"},
		{"svc-b", "svc-z"},
	}

	for i, exp := range expected {
		if ranked[i].Source != exp.src || ranked[i].Target != exp.tgt {
			t.Errorf("pos %d: expected %s->%s, got %s->%s",
				i, exp.src, exp.tgt, ranked[i].Source, ranked[i].Target)
		}
	}
}

// TestComputeBetweenness_BoutiqueMultiHopTopology memverifikasi bahwa rantai multi-hop
// (frontend -> cartservice -> productcatalogservice) menjadikan cartservice sebagai bridge node
// dengan betweenness centrality > 0 (CB(cartservice) > 0)
func TestComputeBetweenness_BoutiqueMultiHopTopology(t *testing.T) {
	g := NewTopologyGraph()
	// Frontend calls cartservice (hop 1)
	for i := 0; i < 50; i++ {
		g.RecordCall("frontend", "cartservice")
	}
	// East-West call: cartservice calls productcatalogservice (hop 2)
	for i := 0; i < 50; i++ {
		g.RecordCall("cartservice", "productcatalogservice")
	}

	scores := ComputeBetweenness(g)

	if scores["cartservice"] <= 0.0 {
		t.Errorf("expected cartservice betweenness centrality > 0, got %.4f", scores["cartservice"])
	}

	ranked := RankEdgesByImportance(g, scores)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked edges, got %d", len(ranked))
	}

	// Edge cartservice -> productcatalogservice memiliki source centrality tertinggi
	if ranked[0].Source != "cartservice" {
		t.Errorf("expected edge with highest combined score to have source 'cartservice', got '%s'", ranked[0].Source)
	}
}

// TestRankEdgesByImportanceMode_LFU_vs_GraphAware memverifikasi divergensi ranking antara Mode LFU vs Mode Graph-Aware:
// - Pada Mode LFU: Edge dengan frekuensi murni lebih tinggi (edge direct) menang (#1).
// - Pada Mode Graph-Aware: Edge jembatan dengan sentralitas tinggi (edge bridge) diangkat menjadi #1.
func TestRankEdgesByImportanceMode_LFU_vs_GraphAware(t *testing.T) {
	g := NewTopologyGraph()

	// Edge 1: frontend -> productcatalogservice (Direct call, frekuensi tinggi: 100 calls)
	for i := 0; i < 100; i++ {
		g.RecordCall("frontend", "productcatalogservice")
	}
	// Edge 2: cartservice -> productcatalogservice (Bridge call, frekuensi sedang: 70 calls)
	for i := 0; i < 70; i++ {
		g.RecordCall("cartservice", "productcatalogservice")
	}

	centrality := map[string]float64{
		"frontend":    0.0, // Direct leaf node source, no shortest paths traverse through it
		"cartservice": 0.8, // Critical bridge node, high betweenness centrality
	}

	// 1. Uji Mode Frequency-Only (LFU)
	rankedLFU := RankEdgesByImportanceMode(g, centrality, ModeFrequencyOnly)
	if len(rankedLFU) != 2 {
		t.Fatalf("expected 2 edges in LFU ranking, got %d", len(rankedLFU))
	}
	// Di LFU, Edge 1 (100 calls) WAJIB berada di peringkat #1 mengalahkan Edge 2 (70 calls)
	if rankedLFU[0].Source != "frontend" {
		t.Errorf("LFU mode must rank highest-frequency edge first (source 'frontend'), got '%s'", rankedLFU[0].Source)
	}
	if rankedLFU[1].Source != "cartservice" {
		t.Errorf("LFU mode must rank lower-frequency edge second (source 'cartservice'), got '%s'", rankedLFU[1].Source)
	}

	// 2. Uji Mode Graph-Aware
	rankedGraph := RankEdgesByImportanceMode(g, centrality, ModeGraphAware)
	if len(rankedGraph) != 2 {
		t.Fatalf("expected 2 edges in Graph-Aware ranking, got %d", len(rankedGraph))
	}
	// Di Graph-Aware:
	// Skor Edge 1 = 0.7 * (100/100) + 0.3 * 0.0 = 0.70
	// Skor Edge 2 = 0.7 * (70/100) + 0.3 * 0.8 = 0.49 + 0.24 = 0.73
	// Edge 2 (cartservice bridge) WAJIB mengalahkan Edge 1 dan berada di peringkat #1!
	if rankedGraph[0].Source != "cartservice" {
		t.Errorf("Graph-Aware mode must rank high-centrality bridge edge first (source 'cartservice'), got '%s'", rankedGraph[0].Source)
	}
	if rankedGraph[1].Source != "frontend" {
		t.Errorf("Graph-Aware mode must rank low-centrality edge second (source 'frontend'), got '%s'", rankedGraph[1].Source)
	}
}

// BenchmarkComputeBetweenness mengukur latensi kalkulasi Brandes betweenness centrality
func BenchmarkComputeBetweenness(b *testing.B) {
	g := NewTopologyGraph()
	g.RecordCall("frontend", "cartservice")
	g.RecordCall("cartservice", "productcatalogservice")
	g.RecordCall("frontend", "productcatalogservice")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeBetweenness(g)
	}
}

// BenchmarkRankEdgesByImportance mengukur latensi perangkingan komposit edge
func BenchmarkRankEdgesByImportance(b *testing.B) {
	g := NewTopologyGraph()
	for i := 0; i < 100; i++ {
		g.RecordCall("frontend", "cartservice")
		g.RecordCall("cartservice", "productcatalogservice")
	}
	scores := map[string]float64{
		"frontend":              0.0,
		"cartservice":           0.5,
		"productcatalogservice": 0.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RankEdgesByImportance(g, scores)
	}
}

// TestComputeBetweenness_WeightedGeodesicProbe membuktikan secara empiris bahwa bobot panggilan aktif
// memengaruhi rute geodesik: A -> B -> C (bobot kecil karena trafik tinggi) mengalahkan A -> C (bobot besar karena trafik rendah)
func TestComputeBetweenness_WeightedGeodesicProbe(t *testing.T) {
	g := NewTopologyGraph()

	// A -> B: 50 panggilan -> W = 1/50 = 0.02
	for i := 0; i < 50; i++ {
		g.RecordCall("A", "B")
	}
	// B -> C: 50 panggilan -> W = 1/50 = 0.02
	for i := 0; i < 50; i++ {
		g.RecordCall("B", "C")
	}
	// A -> C: 1 panggilan -> W = 1/1 = 1.00
	g.RecordCall("A", "C")

	scores := ComputeBetweenness(g)

	// Pada graf tak-berbobot hop murni, A -> C adalah 1 hop, sehingga B tidak berada di shortest path (betweenness 0).
	// Pada graf berbobot (BetweennessWeighted), jarak A->B->C (0.04) jauh lebih pendek daripada direct A->C (1.00),
	// sehingga jalur geodesik melalui B, dan B WAJIB memiliki betweenness centrality tertinggi (> 0).
	if scores["B"] <= 0.0 {
		t.Fatalf("expected node B to have non-zero centrality due to weighted geodesics, got %.4f (scores: %v)", scores["B"], scores)
	}
	if scores["B"] <= scores["A"] || scores["B"] <= scores["C"] {
		t.Errorf("expected node B to have strictly highest centrality, got B=%.4f A=%.4f C=%.4f", scores["B"], scores["A"], scores["C"])
	}
}



