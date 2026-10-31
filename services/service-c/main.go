package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	requestCount  int64
	staticProductList = []map[string]interface{}{
		{"id": 1, "name": "Product F", "price": 99.99},
	}
)

func main() {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "service-c"
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

	// High-performance HTTP client for downstream East-West calls
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 500,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	mux := http.NewServeMux()

	// GET /api/products - list produk (simulasi lokal)
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(staticProductList)
	})

	// /api/cart - cart operations dengan East-West call ke productcatalogservice
	mux.HandleFunc("/api/cart", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}

		// Rantai Multi-Hop East-West: Validasi metadata produk ke productcatalogservice
		targetURL := productCatalogURL + "/api/products"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err == nil {
			// Forward headers dan propagasi identitas SPIFFE service-c
			for k, vv := range r.Header {
				for _, v := range vv {
					req.Header.Add(k, v)
				}
			}
			req.Header.Set("x-source-identity", "spiffe://cluster.local/ns/zta-research/sa/cartservice")
			req.Header.Set("x-caller-identity", "cartservice")

			resp, err := httpClient.Do(req)
			if err != nil {
				log.Printf("[cartservice] East-West upstream call to productcatalogservice failed: %v", err)
			} else {
				resp.Body.Close()
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":           "item_added",
				"cart_id":          "cart-user-default",
				"items":            1,
				"catalog_verified": true,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cart_id":          "cart-user-default",
			"items":            []map[string]interface{}{{"product_id": 1, "quantity": 1}},
			"catalog_verified": true,
		})
	})

	// GET /health - health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("%s listening on :%s", serviceName, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
