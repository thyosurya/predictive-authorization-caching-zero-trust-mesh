// control-plane/cache-distributor/metrics/metrics.go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    CacheHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "zta_cache_hits_total",
            Help: "Total authorization decisions served from local cache",
        },
        []string{"source", "target"},
    )

    CacheMisses = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "zta_cache_misses_total",
            Help: "Total authorization decisions that required PDP query",
        },
        []string{"source", "target"},
    )

    RevocationLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "zta_revocation_latency_ms",
            Help:    "Time from revocation event to sidecar rejection (ms)",
            Buckets: []float64{10, 25, 50, 75, 100, 150, 200, 500},
        },
    )

    TokenTTL = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "zta_token_ttl_ms",
            Help:    "Delegation token TTL distribution (ms)",
            Buckets: []float64{5000, 10000, 20000, 30000, 45000, 60000},
        },
        []string{"source", "target"},
    )

    TokensIssuedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "zta_tokens_issued_total",
			Help: "Total token yang pernah diterbitkan",
		},
	)
	
	TokensActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "zta_tokens_active",
			Help: "Jumlah token aktif terdistribusi",
		},
	)
	
	RevocationEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zta_revocation_events_total",
			Help: "Total event revokasi",
		},
		[]string{"reason"},
	)
	
	GrpcStreamCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "zta_grpc_stream_count",
			Help: "Jumlah sidecar yang terkoneksi via stream",
		},
	)

	TokensExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "zta_tokens_expired_total",
			Help: "Total token yang expired dan dibersihkan",
		},
	)
)

func init() {
    prometheus.MustRegister(CacheHits, CacheMisses, RevocationLatency, TokenTTL, TokensIssuedTotal, TokensActive, RevocationEventsTotal, GrpcStreamCount, TokensExpiredTotal)
}
