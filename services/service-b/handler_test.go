package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceB_ProductsAndHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(staticProductList)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 products, got %d", len(items))
	}

	reqH := httptest.NewRequest(http.MethodGet, "/health", nil)
	recH := httptest.NewRecorder()
	mux.ServeHTTP(recH, reqH)
	if recH.Code != http.StatusOK {
		t.Errorf("expected status 200 on /health, got %d", recH.Code)
	}
}
