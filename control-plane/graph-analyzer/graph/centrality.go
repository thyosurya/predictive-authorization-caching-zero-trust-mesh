// control-plane/graph-analyzer/graph/centrality.go
package graph

import (
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/path"
)

// ComputeBetweenness menghitung betweenness centrality semua node.
// Return: map[SPIFFE_ID]skor (0.0-1.0, sudah dinormalisasi).
// Menggunakan gonum/graph/path.DijkstraAllPaths() dan gonum/graph/network.BetweennessWeighted().
// Kompleksitas: O(VE + V² log V) dengan Brandes berbobot.
func ComputeBetweenness(g *TopologyGraph) map[string]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return make(map[string]float64)
	}

	// Hitung all shortest paths berbasis pembobotan geodesik Dijkstra (bobot W = 1.0 / CallsPerMin)
	allPaths := path.DijkstraAllPaths(g.graph)
	rawScores := network.BetweennessWeighted(g.graph, allPaths)

	// Konversi nodeID → serviceID dan normalisasi
	scores := make(map[string]float64)
	maxScore := 0.0

	// Buat reverse map nodeID → serviceID
	reverseMap := make(map[int64]string)
	for serviceID, nodeID := range g.nodes {
		reverseMap[nodeID] = serviceID
	}

	for nodeID, score := range rawScores {
		if serviceID, ok := reverseMap[nodeID]; ok {
			scores[serviceID] = score
			if score > maxScore {
				maxScore = score
			}
		}
	}

	// Normalisasi ke 0.0–1.0
	if maxScore > 0 {
		for k := range scores {
			scores[k] /= maxScore
		}
	}

	return scores
}

// PrioritizationMode merepresentasikan algoritma pembobotan ranking edge
type PrioritizationMode string

const (
	// ModeGraphAware menggabungkan frekuensi panggilan (70%) dan Betweenness Centrality (30%)
	ModeGraphAware PrioritizationMode = "graph"
	// ModeFrequencyOnly (LFU) murni menggunakan frekuensi panggilan (100%) tanpa pengaruh graf
	ModeFrequencyOnly PrioritizationMode = "lfu"
)

// RankEdgesByImportanceWeighted mengurutkan edge berdasarkan formula parametrik:
// score = wFreq * normalized_call_count + wGraph * centrality[source]
// Tetap menegakkan 5-tier deterministic tie-breaking dengan kuantisasi integer 10^9:
// 1. Composite Score DESC (resolusi 10^-9)
// 2. Betweenness Centrality DESC (resolusi 10^-9)
// 3. CallsPerMin DESC
// 4. Source SPIFFE ID ASC
// 5. Target SPIFFE ID ASC
func RankEdgesByImportanceWeighted(g *TopologyGraph, centrality map[string]float64, wFreq, wGraph float64) []*ServiceEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Snapshot waktu konsisten untuk seluruh evaluasi ranking (m3: menghindari drift antar-loop)
	now := time.Now()
	coldStartCutoff := now.Add(-10 * time.Second)

	// Cari max call count untuk normalisasi
	var maxCalls int64
	for _, edge := range g.edges {
		effectiveCalls := edge.callsPerMinuteAt(now)
		if effectiveCalls <= 0 && edge.LastSeen.After(coldStartCutoff) {
			// Hanya izinkan fallback CallCount pada fase cold-start (10 detik pertama sejak LastSeen)
			effectiveCalls = edge.CallCount
		}
		if effectiveCalls > maxCalls {
			maxCalls = effectiveCalls
		}
	}

	// Buat slice dengan skor gabungan
	type scoredEdge struct {
		edge            *ServiceEdge
		score           float64
		centralityScore float64
		calls           int64
	}

	var scored []scoredEdge
	for _, edge := range g.edges {
		effectiveCalls := edge.callsPerMinuteAt(now)
		if effectiveCalls <= 0 && edge.LastSeen.After(coldStartCutoff) {
			// Hanya izinkan fallback CallCount pada fase cold-start (10 detik pertama sejak LastSeen)
			effectiveCalls = edge.CallCount
		}
		normalizedCalls := 0.0
		if maxCalls > 0 {
			normalizedCalls = float64(effectiveCalls) / float64(maxCalls)
		}
		centralityScore := centrality[edge.Source]
		combined := wFreq*normalizedCalls + wGraph*centralityScore

		scored = append(scored, scoredEdge{
			edge:            edge,
			score:           combined,
			centralityScore: centralityScore,
			calls:           effectiveCalls,
		})
	}

	// 5-tier deterministic sorting dengan kuantisasi integer untuk menjamin strict weak ordering transitif:
	sort.Slice(scored, func(i, j int) bool {
		qScoreI := math.Round(scored[i].score * 1e9)
		qScoreJ := math.Round(scored[j].score * 1e9)
		if qScoreI != qScoreJ {
			return qScoreI > qScoreJ
		}

		qCentI := math.Round(scored[i].centralityScore * 1e9)
		qCentJ := math.Round(scored[j].centralityScore * 1e9)
		if qCentI != qCentJ {
			return qCentI > qCentJ
		}

		if scored[i].calls != scored[j].calls {
			return scored[i].calls > scored[j].calls
		}

		if scored[i].edge.Source != scored[j].edge.Source {
			return scored[i].edge.Source < scored[j].edge.Source
		}

		return scored[i].edge.Target < scored[j].edge.Target
	})

	result := make([]*ServiceEdge, len(scored))
	for i, s := range scored {
		result[i] = copyEdge(s.edge)
	}
	return result
}

// RankEdgesByImportanceMode mengurutkan edge berdasarkan mode prioritisasi yang dipilih.
func RankEdgesByImportanceMode(g *TopologyGraph, centrality map[string]float64, mode PrioritizationMode) []*ServiceEdge {
	if mode == ModeFrequencyOnly {
		return RankEdgesByImportanceWeighted(g, centrality, 1.0, 0.0)
	}
	return RankEdgesByImportanceWeighted(g, centrality, 0.7, 0.3)
}

// RankEdgesByImportance mengurutkan edge berdasarkan gabungan: CallCount/CallsPerMin + centrality sumber.
// Formula: score = 0.7 * normalized_call_count + 0.3 * centrality[source]
// Menjaga kompatibilitas mundur dengan menggunakan ModeGraphAware secara default.
func RankEdgesByImportance(g *TopologyGraph, centrality map[string]float64) []*ServiceEdge {
	return RankEdgesByImportanceMode(g, centrality, ModeGraphAware)
}

