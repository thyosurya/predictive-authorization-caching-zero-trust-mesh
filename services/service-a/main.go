package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	requestCount      int64
	staticProductList = []map[string]interface{}{
		{"id": 1, "name": "Product A", "price": 19.99},
		{"id": 2, "name": "Product B", "price": 29.99},
	}
)

func newFrontendMux(serviceName, productCatalogURL, cartServiceURL string, latencyMs int, localBenchmarkMode bool, httpClient *http.Client) *http.ServeMux {
	mux := http.NewServeMux()

	// GET /api/products - Forward to productcatalogservice (East-West) or handle locally (LOCAL_BENCHMARK_MODE=true)
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}

		if localBenchmarkMode {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(staticProductList)
			return
		}

		targetURL := productCatalogURL + "/api/products"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			http.Error(w, "Failed to create upstream request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Forward headers
		for k, vv := range r.Header {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("x-source-identity", "spiffe://cluster.local/ns/zta-research/sa/frontend")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[frontend] upstream call to productcatalogservice failed: %v", err)
			http.Error(w, "Upstream productcatalogservice error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	// /api/cart - Forward to cartservice (East-West) or handle locally (LOCAL_BENCHMARK_MODE=true)
	mux.HandleFunc("/api/cart", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}

		if localBenchmarkMode {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"status":  "item_added",
					"cart_id": "cart-user-default",
					"items":   1,
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cart_id": "cart-user-default",
				"items":   []map[string]interface{}{{"product_id": 1, "quantity": 1}},
			})
			return
		}

		targetURL := cartServiceURL + "/api/cart"
		req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, "Failed to create upstream request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for k, vv := range r.Header {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("x-source-identity", "spiffe://cluster.local/ns/zta-research/sa/frontend")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[frontend] upstream call to cartservice failed: %v", err)
			http.Error(w, "Upstream cartservice error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	// GET /health - local health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// GET /metrics - Prometheus-compatible text format
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		count := atomic.LoadInt64(&requestCount)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP http_requests_total Total HTTP requests\n")
		fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
		fmt.Fprintf(w, "http_requests_total{service=\"%s\"} %d\n", serviceName, count)
	})

	// Root handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": serviceName,
			"status":  "ok",
		})
	})

	return mux
}

func main() {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "service-a"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	latencyMs, _ := strconv.Atoi(os.Getenv("SIMULATED_LATENCY_MS"))
	localBenchmarkMode := os.Getenv("LOCAL_BENCHMARK_MODE") == "true"

	productCatalogURL := os.Getenv("PRODUCT_CATALOG_URL")
	if productCatalogURL == "" {
		productCatalogURL = "http://productcatalogservice.zta-research.svc.cluster.local:8080"
	}
	cartServiceURL := os.Getenv("CART_SERVICE_URL")
	if cartServiceURL == "" {
		cartServiceURL = "http://cartservice.zta-research.svc.cluster.local:8080"
	}

	// High-performance HTTP client for reverse proxying East-West calls
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        2000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	mux := newFrontendMux(serviceName, productCatalogURL, cartServiceURL, latencyMs, localBenchmarkMode, httpClient)

	log.Printf("%s listening on :%s (East-West routing enabled: %v)", serviceName, port, !localBenchmarkMode)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
