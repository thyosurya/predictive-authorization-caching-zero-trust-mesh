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
	requestCount int64
)

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

	mux := http.NewServeMux()

	// GET /api/products - Forward to productcatalogservice (East-West traffic)
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
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

	// POST /api/cart - Forward to cartservice (East-West traffic)
	mux.HandleFunc("/api/cart", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
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

	log.Printf("%s listening on :%s (East-West routing enabled)", serviceName, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
