// control-plane/graph-analyzer/graph/topology_test.go
package graph

import (
    "testing"
    "time"
)

// TestNewTopologyGraph memverifikasi inisialisasi graph kosong
func TestNewTopologyGraph(t *testing.T) {
    g := NewTopologyGraph()
    if g == nil {
        t.Fatal("NewTopologyGraph() returned nil")
    }
    if len(g.edges) != 0 {
        t.Errorf("expected 0 edges, got %d", len(g.edges))
    }
}

// TestRecordCall_NewEdge memverifikasi edge baru dibuat saat panggilan pertama
func TestRecordCall_NewEdge(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")

    key := "service-a->service-b"
    edge, exists := g.edges[key]
    if !exists {
        t.Fatalf("expected edge %q to exist", key)
    }
    if edge.CallCount != 1 {
        t.Errorf("expected CallCount=1, got %d", edge.CallCount)
    }
    if edge.Source != "service-a" {
        t.Errorf("expected Source=service-a, got %s", edge.Source)
    }
    if edge.Target != "service-b" {
        t.Errorf("expected Target=service-b, got %s", edge.Target)
    }
}

// TestRecordCall_Increment memverifikasi counter bertambah pada panggilan berulang
func TestRecordCall_Increment(t *testing.T) {
    g := NewTopologyGraph()

    for i := 0; i < 50; i++ {
        g.RecordCall("service-a", "service-b")
    }

    edge := g.edges["service-a->service-b"]
    if edge.CallCount != 50 {
        t.Errorf("expected CallCount=50, got %d", edge.CallCount)
    }
}

// TestRecordCall_DirectionalIsolation memverifikasi A→B dan B→A adalah edge terpisah
func TestRecordCall_DirectionalIsolation(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")
    g.RecordCall("service-b", "service-a")

    if len(g.edges) != 2 {
        t.Errorf("expected 2 separate edges, got %d", len(g.edges))
    }
}

// TestGetHighFrequencyPairs_AboveThreshold memverifikasi hanya edge di atas threshold yang dikembalikan
func TestGetHighFrequencyPairs_AboveThreshold(t *testing.T) {
    g := NewTopologyGraph()

    // Edge dengan frekuensi tinggi
    for i := 0; i < 20; i++ {
        g.RecordCall("service-a", "service-b")
    }

    // Edge dengan frekuensi rendah
    for i := 0; i < 5; i++ {
        g.RecordCall("service-c", "service-d")
    }

    candidates := g.GetHighFrequencyPairs(10)

    if len(candidates) != 1 {
        t.Errorf("expected 1 candidate, got %d", len(candidates))
    }
    if candidates[0].Source != "service-a" || candidates[0].Target != "service-b" {
        t.Errorf("unexpected candidate: %+v", candidates[0])
    }
}

// TestGetHighFrequencyPairs_StaleEdgeExcluded memverifikasi edge lama (>1 menit) dikecualikan
func TestGetHighFrequencyPairs_StaleEdgeExcluded(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")

    // Manipulasi LastSeen ke masa lalu
    edge := g.edges["service-a->service-b"]
    edge.CallCount = 100
    edge.LastSeen = time.Now().Add(-2 * time.Minute) // sudah stale

    candidates := g.GetHighFrequencyPairs(10)
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates (stale), got %d", len(candidates))
    }
}

// TestGetHighFrequencyPairs_Empty memverifikasi graph kosong mengembalikan slice kosong
func TestGetHighFrequencyPairs_Empty(t *testing.T) {
    g := NewTopologyGraph()
    candidates := g.GetHighFrequencyPairs(10)
    if candidates != nil && len(candidates) != 0 {
        t.Errorf("expected empty result, got %v", candidates)
    }
}

// TestRecordCall_Concurrent memverifikasi thread-safety saat concurrent writes
func TestRecordCall_Concurrent(t *testing.T) {
    g := NewTopologyGraph()
    done := make(chan struct{})

    for i := 0; i < 100; i++ {
        go func() {
            g.RecordCall("service-a", "service-b")
            done <- struct{}{}
        }()
    }

    for i := 0; i < 100; i++ {
        <-done
    }

    edge := g.edges["service-a->service-b"]
    if edge.CallCount != 100 {
        t.Errorf("concurrent writes: expected 100, got %d", edge.CallCount)
    }
}

// TestPruneStale_OrphanNodeCleanup memverifikasi edge stale dihapus DAN orphan nodes dibersihkan
func TestPruneStale_OrphanNodeCleanup(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("A", "B")
    g.RecordCall("C", "D")

    if len(g.edges) != 2 || len(g.nodes) != 4 {
        t.Fatalf("expected 2 edges and 4 nodes, got %d edges and %d nodes", len(g.edges), len(g.nodes))
    }

    // Set edge A->B ke 2 jam yang lalu
    g.edges["A->B"].LastSeen = time.Now().Add(-2 * time.Hour)

    // Prune edge > 1 jam
    g.PruneStale(1 * time.Hour)

    if len(g.edges) != 1 {
        t.Errorf("expected 1 edge remaining, got %d", len(g.edges))
    }
    if _, exists := g.edges["A->B"]; exists {
        t.Errorf("expected A->B to be pruned")
    }
    if _, exists := g.edges["C->D"]; !exists {
        t.Errorf("expected C->D to remain")
    }

    // Node A dan B harus dibersihkan karena tidak memiliki koneksi lagi
    if len(g.nodes) != 2 {
        t.Errorf("expected 2 active nodes after pruning orphans, got %d", len(g.nodes))
    }
    if _, ok := g.nodes["A"]; ok {
        t.Errorf("expected orphan node A to be removed from g.nodes")
    }
    if _, ok := g.nodes["B"]; ok {
        t.Errorf("expected orphan node B to be removed from g.nodes")
    }
    if _, ok := g.nodes["C"]; !ok {
        t.Errorf("expected node C to remain in g.nodes")
    }
    if _, ok := g.nodes["D"]; !ok {
        t.Errorf("expected node D to remain in g.nodes")
    }

    snap := g.Snapshot()
    if len(snap.Nodes) != 2 {
        t.Errorf("expected snapshot to have 2 nodes, got %d", len(snap.Nodes))
    }
}

// TestRecordCall_InverseDistanceWeight memverifikasi bobot edge Gonum adalah invers frekuensi (1 / CallsPerMin)
func TestRecordCall_InverseDistanceWeight(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("X", "Y")

    srcID := g.nodes["X"]
    tgtID := g.nodes["Y"]

    w1, ok1 := g.graph.Weight(srcID, tgtID)
    if !ok1 || w1 != 1.0 {
        t.Errorf("expected initial weight 1.0 (1 call), got %f (ok=%v)", w1, ok1)
    }

    // Record 4 panggilan tambahan (total 5 panggilan pada window yang sama)
    for i := 0; i < 4; i++ {
        g.RecordCall("X", "Y")
    }

    w5, ok5 := g.graph.Weight(srcID, tgtID)
    if !ok5 {
        t.Fatalf("expected edge to exist")
    }
    expectedW := 1.0 / 5.0
    if w5 < expectedW-0.001 || w5 > expectedW+0.001 {
        t.Errorf("expected weight ~%f (1/5), got %f", expectedW, w5)
    }
}
