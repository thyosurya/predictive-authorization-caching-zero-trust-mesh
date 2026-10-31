package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EdgesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "zta_graph_edges_total",
		Help: "Jumlah edge aktif dalam graph",
	})
	
	CallsRecordedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "zta_graph_calls_recorded_total",
		Help: "Total panggilan yang dicatat",
	}, []string{"source", "target"})

	CandidatesSelectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "zta_graph_candidates_selected_total",
		Help: "Total pasangan yang dipilih sebagai kandidat",
	})
	
	AnalysisDurationMs = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "zta_graph_analysis_duration_ms",
		Help:    "Durasi analisis graph per siklus",
		Buckets: []float64{10, 50, 100, 250, 500, 1000},
	})
)

// Collector wraps semua metric di atas
type Collector struct{}

// NewCollector membuat Collector baru
func NewCollector() *Collector {
	return &Collector{}
}

// RecordCall mencatat panggilan ke counter
func (c *Collector) RecordCall(source, target string) {
	CallsRecordedTotal.WithLabelValues(source, target).Inc()
}

// SetEdgeCount mengeset jumlah edge aktif
func (c *Collector) SetEdgeCount(n int) {
	EdgesTotal.Set(float64(n))
}

// ObserveAnalysisDuration mencatat durasi analisis
func (c *Collector) ObserveAnalysisDuration(ms float64) {
	AnalysisDurationMs.Observe(ms)
}
