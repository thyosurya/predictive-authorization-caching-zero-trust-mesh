// control-plane/graph-analyzer/graph/topology.go
package graph

import (
    "sync"
    "time"

    "gonum.org/v1/gonum/graph/simple"
)

// ServiceEdge merepresentasikan panggilan antar dua service
type ServiceEdge struct {
    Source      string
    Target      string
    CallCount   int64     // Kumulatif total panggilan sejak awal (lifetime counter)
    LastSeen    time.Time
    buckets     [6]int64  // 6 bucket berukuran 10 detik untuk sliding window 60 detik
    bucketEpoch [6]int64  // Epoch interval (now.Unix() / 10) per bucket
}

// recordAt mencatat satu panggilan ke bucket sliding window yang sesuai.
func (e *ServiceEdge) recordAt(now time.Time) {
    e.CallCount++
    e.LastSeen = now
    epoch := now.Unix() / 10
    idx := epoch % 6
    if e.bucketEpoch[idx] != epoch {
        e.bucketEpoch[idx] = epoch
        e.buckets[idx] = 1
    } else {
        e.buckets[idx]++
    }
}

// CallsPerMinute mengembalikan estimasi laju panggilan per menit berdasarkan sliding window 60 detik.
func (e *ServiceEdge) CallsPerMinute() int64 {
    return e.callsPerMinuteAt(time.Now())
}

func (e *ServiceEdge) callsPerMinuteAt(now time.Time) int64 {
    curEpoch := now.Unix() / 10
    var total int64
    for i := 0; i < 6; i++ {
        diff := curEpoch - e.bucketEpoch[i]
        if diff >= 0 && diff < 6 {
            total += e.buckets[i]
        }
    }
    return total
}

// GraphSnapshot adalah salinan seluruh graph untuk endpoint /graph
type GraphSnapshot struct {
    Edges []*ServiceEdge `json:"edges"`
    Nodes []string       `json:"nodes"`
}

// TopologyGraph menyimpan struktur graph service mesh
type TopologyGraph struct {
    mu       sync.RWMutex
    edges    map[string]*ServiceEdge         // key: "source->target"
    graph    *simple.WeightedDirectedGraph
    nodes    map[string]int64               // serviceID -> nodeID
    nextNode int64
}

func NewTopologyGraph() *TopologyGraph {
    return &TopologyGraph{
        edges: make(map[string]*ServiceEdge),
        graph: simple.NewWeightedDirectedGraph(0, 0),
        nodes: make(map[string]int64),
    }
}

// RecordCall mencatat satu panggilan antar service.
// Thread-safe. Dipanggil dari HTTP handler /record.
func (g *TopologyGraph) RecordCall(source, target string) {
    g.mu.Lock()
    defer g.mu.Unlock()

    now := time.Now()
    key := source + "->" + target
    edge, exists := g.edges[key]
    if !exists {
        edge = &ServiceEdge{
            Source: source,
            Target: target,
        }
        g.edges[key] = edge
    }
    edge.recordAt(now)

    srcID := g.ensureNode(source)
    tgtID := g.ensureNode(target)

    callsPerMin := edge.callsPerMinuteAt(now)
    if callsPerMin < 1 {
        callsPerMin = 1
    }
    // Bobot jarak shortest path berbanding terbalik dengan frekuensi panggilan:
    // W = 1.0 / max(1, CallsPerMin) sehingga jalur yang lebih sering dipanggil memiliki jarak geodesik lebih pendek.
    weight := 1.0 / float64(callsPerMin)
    g.graph.SetWeightedEdge(simple.WeightedEdge{
        F: simple.Node(srcID),
        T: simple.Node(tgtID),
        W: weight,
    })
}

// nodeID returns the gonum node ID for a service, must be called with lock held
func (g *TopologyGraph) nodeID(service string) int64 {
    return g.nodes[service]
}

// ensureNode ensures a node exists in the gonum graph, must be called with lock held
func (g *TopologyGraph) ensureNode(service string) int64 {
    if id, ok := g.nodes[service]; ok {
        return id
    }
    id := g.nextNode
    g.nextNode++
    g.nodes[service] = id
    g.graph.AddNode(simple.Node(id))
    return id
}

func copyEdge(e *ServiceEdge) *ServiceEdge {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

// GetHighFrequencyPairs mengembalikan edge dengan CallCount/CallsPerMin >= threshold
// DAN LastSeen dalam 1 menit terakhir. Mengembalikan salinan pointer untuk mencegah data race.
// Read-only. Dipanggil oleh CandidateSelector.
func (g *TopologyGraph) GetHighFrequencyPairs(minCallsPerMin int64) []*ServiceEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	var result []*ServiceEdge

	for _, edge := range g.edges {
		if edge.LastSeen.After(cutoff) {
			calls := edge.callsPerMinuteAt(now)
			if calls <= 0 && edge.CallCount >= minCallsPerMin && edge.LastSeen.After(now.Add(-10*time.Second)) {
				// Hanya izinkan fallback CallCount pada fase cold-start (10 detik pertama sejak LastSeen)
				calls = edge.CallCount
			}
			if calls >= minCallsPerMin {
				result = append(result, copyEdge(edge))
			}
		}
	}
	return result
}

// Snapshot mengembalikan salinan seluruh graph untuk endpoint /graph.
// Mengembalikan salinan pointer untuk mencegah data race.
// Read-only.
func (g *TopologyGraph) Snapshot() GraphSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var edges []*ServiceEdge
	for _, e := range g.edges {
		edges = append(edges, copyEdge(e))
	}

	var nodes []string
	for n := range g.nodes {
		nodes = append(nodes, n)
	}

	return GraphSnapshot{Edges: edges, Nodes: nodes}
}

// Reset menghapus semua edge dan node.
// Dipanggil oleh handler /graph/reset.
// Thread-safe (write lock).
func (g *TopologyGraph) Reset() {
    g.mu.Lock()
    defer g.mu.Unlock()

    g.edges = make(map[string]*ServiceEdge)
    g.graph = simple.NewWeightedDirectedGraph(0, 0)
    g.nodes = make(map[string]int64)
    g.nextNode = 0
}

// PruneStale menghapus edge yang LastSeen lebih lama dari threshold,
// serta membersihkan orphan nodes dari g.nodes dan g.graph.
// Dipanggil secara periodik oleh analysisLoop di main.go.
func (g *TopologyGraph) PruneStale(olderThan time.Duration) {
    g.mu.Lock()
    defer g.mu.Unlock()

    cutoff := time.Now().Add(-olderThan)
    for key, edge := range g.edges {
        if edge.LastSeen.Before(cutoff) {
            // Remove from gonum graph
            srcID := g.nodes[edge.Source]
            tgtID := g.nodes[edge.Target]
            if g.graph.HasEdgeFromTo(srcID, tgtID) {
                g.graph.RemoveEdge(srcID, tgtID)
            }
            delete(g.edges, key)
        }
    }

    // Identifikasi dan bersihkan orphan nodes (simpul tanpa edge masuk maupun keluar)
    activeNodes := make(map[string]bool)
    for _, edge := range g.edges {
        activeNodes[edge.Source] = true
        activeNodes[edge.Target] = true
    }

    for service, nodeID := range g.nodes {
        if !activeNodes[service] {
            g.graph.RemoveNode(nodeID)
            delete(g.nodes, service)
        }
    }
}
