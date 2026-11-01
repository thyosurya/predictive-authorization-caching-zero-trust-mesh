package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/cache"
	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/graph"
	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/metrics"
)

// AnalyzerService mengorkestrasi semua sub-paket Graph Analyzer
type AnalyzerService struct {
	topology   *graph.TopologyGraph
	selector   *cache.CandidateSelector
	metrics    *metrics.Collector
	adminToken string
	opaURL     string
}

// RecordRequest adalah payload POST /record
type RecordRequest struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Method    string `json:"method"`
	Timestamp int64  `json:"timestamp"`
}

// CacheCandidateResponse adalah response GET /candidates
type CacheCandidateResponse struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	CallsPerMin     int64   `json:"calls_per_min"`
	SuggestedTTL    int64   `json:"suggested_ttl_ms"`
	Sensitivity     string  `json:"sensitivity"`
	CentralityScore float64 `json:"centrality_score"`
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	log.Println("Starting Graph Analyzer...")

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required (no default credential)")
	}
	opaURL := envOrDefault("OPA_URL", "")
	minCallsPerMin := envIntOrDefault("MIN_CALLS_PER_MIN", 10)
	maxCandidates := envIntOrDefault("MAX_CANDIDATES", 50)
	analysisInterval := envIntOrDefault("ANALYSIS_INTERVAL_S", 30)
	metricsPort := envOrDefault("METRICS_PORT", "2112")
	port := envOrDefault("PORT", "8080")

	modeStr := strings.ToLower(envOrDefault("PRIORITIZATION_MODE", "graph"))
	prioritizationMode := graph.ModeGraphAware
	if modeStr == "lfu" || modeStr == "frequency" || modeStr == "frequency-only" {
		prioritizationMode = graph.ModeFrequencyOnly
	}

	selector := cache.NewCandidateSelector(int64(minCallsPerMin), maxCandidates)
	selector.SetMode(prioritizationMode)

	svc := &AnalyzerService{
		topology:   graph.NewTopologyGraph(),
		selector:   selector,
		metrics:    metrics.NewCollector(),
		adminToken: adminToken,
		opaURL:     opaURL,
	}
	log.Printf("Graph Analyzer initialized with mode: %s, max candidates: %d", prioritizationMode, maxCandidates)

	// Background goroutine: analysisLoop setiap N detik
	go svc.analysisLoop(time.Duration(analysisInterval) * time.Second)

	// Metrics endpoint
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		log.Printf("Metrics listening on :%s", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
			log.Printf("Metrics listener warning: %v", err)
		}
	}()

	mux := svc.routes()

	log.Printf("Graph Analyzer API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (svc *AnalyzerService) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/record", svc.handleRecordCall)
	mux.HandleFunc("/candidates", svc.handleGetCandidates)
	mux.HandleFunc("/graph", svc.handleGetGraph)
	mux.HandleFunc("/graph/reset", svc.handleReset)
	mux.HandleFunc("/health", svc.handleHealth)

	return mux
}

// verifyAdminToken memverifikasi header X-Admin-Token atau Authorization: Bearer
// menggunakan perbandingan constant-time untuk mitigasi timing attack.
// Fail-closed: mengembalikan false jika adminToken kosong.
func (svc *AnalyzerService) verifyAdminToken(r *http.Request) bool {
	if svc.adminToken == "" {
		return false // fail-closed jika tidak terkonfigurasi
	}
	token := r.Header.Get("X-Admin-Token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(svc.adminToken)) == 1
}

// POST /record - mencatat panggilan antar service [butuh X-Admin-Token]
func (svc *AnalyzerService) handleRecordCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	var req RecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.Source == "" || req.Target == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "source and target are required"})
		return
	}

	svc.topology.RecordCall(req.Source, req.Target)
	svc.metrics.RecordCall(req.Source, req.Target)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]bool{"recorded": true})
}

// queryOPAPolicy mengambil allowed_actions dan sensitivity dari OPA data.service_rules,
// dengan fallback deterministik berbasis klasifikasi risiko layanan jika OPA tidak dikonfigurasi.
func (svc *AnalyzerService) queryOPAPolicy(source, target string) ([]string, cache.SensitivityLevel) {
	if svc.opaURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		reqBody, _ := json.Marshal(map[string]interface{}{
			"input": map[string]string{
				"source_identity": source,
				"target_identity": target,
			},
		})
		resp, err := client.Post(svc.opaURL+"/v1/data/service_rules", "application/json", bytes.NewReader(reqBody))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var result struct {
					Result []struct {
						Source         string   `json:"source"`
						Target         string   `json:"target"`
						SourceIdentity string   `json:"source_identity"`
						TargetIdentity string   `json:"target_identity"`
						AllowedActions []string `json:"allowed_actions"`
						Sensitivity    string   `json:"sensitivity"`
					} `json:"result"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
					for _, rule := range result.Result {
						rSrc := rule.Source
						if rSrc == "" {
							rSrc = rule.SourceIdentity
						}
						rTgt := rule.Target
						if rTgt == "" {
							rTgt = rule.TargetIdentity
						}
						if (rSrc == source || rSrc == "*") && (rTgt == target || rTgt == "*") && len(rule.AllowedActions) > 0 {
							return rule.AllowedActions, cache.SensitivityFromString(rule.Sensitivity)
						}
					}
				}
			}
		}
	}

	// Fallback deterministik sesuai klasifikasi data.json Online Boutique (Bab 3.3):
	switch {
	case strings.HasSuffix(target, "/productcatalogservice") || strings.HasSuffix(target, "/currencyservice"):
		return []string{"GET", "POST"}, cache.SensitivityLow
	case strings.HasSuffix(target, "/paymentservice"):
		return []string{"POST"}, cache.SensitivityHigh
	case strings.HasSuffix(target, "/cartservice"):
		return []string{"GET", "POST", "DELETE"}, cache.SensitivityMedium
	default:
		return []string{"GET", "POST", "PUT", "DELETE"}, cache.SensitivityMedium
	}
}

// GET /candidates - mengembalikan daftar kandidat cache [butuh X-Admin-Token]
func (svc *AnalyzerService) handleGetCandidates(w http.ResponseWriter, r *http.Request) {
	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	centrality := graph.ComputeBetweenness(svc.topology)

	// Default policyFn - query OPA data.service_rules dengan fallback deterministik
	policyFn := func(source, target string) ([]string, cache.SensitivityLevel) {
		return svc.queryOPAPolicy(source, target)
	}

	candidates := svc.selector.Select(svc.topology, centrality, policyFn)

	var resp []CacheCandidateResponse
	for _, c := range candidates {
		resp = append(resp, CacheCandidateResponse{
			Source:          c.Source,
			Target:          c.Target,
			CallsPerMin:     c.CallsPerMin,
			SuggestedTTL:    c.SuggestedTTLMs,
			Sensitivity:     c.Sensitivity.String(),
			CentralityScore: c.CentralityScore,
		})
	}

	if resp == nil {
		resp = []CacheCandidateResponse{}
	}

	w.Header().Set("X-Prioritization-Mode", string(svc.selector.Mode()))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GET /graph - mengembalikan snapshot seluruh graph [butuh X-Admin-Token]
func (svc *AnalyzerService) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	pairs := svc.topology.GetHighFrequencyPairs(0) // threshold 0 = semua edge
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pairs)
}

// DELETE /graph/reset - menghapus semua data graph [butuh X-Admin-Token]
func (svc *AnalyzerService) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	edgesCleared := len(svc.topology.GetHighFrequencyPairs(0))
	svc.topology.Reset()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reset":         true,
		"edges_cleared": edgesCleared,
	})
}

// GET /health - health check
func (svc *AnalyzerService) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"status":"ok","mode":"%s"}`, svc.selector.Mode())))
}

// analysisLoop menjalankan analisis periodik
func (svc *AnalyzerService) analysisLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		start := time.Now()
		svc.topology.PruneStale(2 * time.Minute)
		elapsed := time.Since(start)
		svc.metrics.ObserveAnalysisDuration(float64(elapsed.Milliseconds()))
	}
}
