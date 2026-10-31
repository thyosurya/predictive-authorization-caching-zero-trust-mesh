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
		{"id": 1, "name": "Product D", "price": 39.99},
		{"id": 2, "name": "Product E", "price": 59.99},
	}
)

func main() {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "service-b"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	latencyMs, _ := strconv.Atoi(os.Getenv("SIMULATED_LATENCY_MS"))

	mux := http.NewServeMux()

	// GET /api/products - list produk (simulasi)
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(staticProductList)
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
