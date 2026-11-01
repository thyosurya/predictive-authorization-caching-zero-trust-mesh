package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestServiceA_EastWestForwarding(t *testing.T) {
	var catalogCalls int32
	var cartCalls int32
	var gotSourceID string

	mockCatalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&catalogCalls, 1)
		gotSourceID = r.Header.Get("x-source-identity")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"Catalog Item","price":10.0}]`))
	}))
	defer mockCatalog.Close()

	mockCart := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cartCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"item_added"}`))
	}))
	defer mockCart.Close()

	mux := newFrontendMux("frontend", mockCatalog.URL, mockCart.URL, 0, false, mockCatalog.Client())

	// 1. Test /api/products East-West forwarding
	req1 := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec1.Code)
	}
	if atomic.LoadInt32(&catalogCalls) != 1 {
		t.Fatalf("expected 1 upstream call to catalog, got %d", catalogCalls)
	}
	if gotSourceID != "spiffe://cluster.local/ns/zta-research/sa/frontend" {
		t.Errorf("unexpected source identity header: %s", gotSourceID)
	}

	// 2. Test POST /api/cart East-West forwarding
	req2 := httptest.NewRequest(http.MethodPost, "/api/cart", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec2.Code)
	}
	if atomic.LoadInt32(&cartCalls) != 1 {
		t.Fatalf("expected 1 upstream call to cart, got %d", cartCalls)
	}
}

func TestServiceA_LocalBenchmarkMode(t *testing.T) {
	mux := newFrontendMux("frontend", "http://127.0.0.1:1", "http://127.0.0.1:1", 0, true, http.DefaultClient)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK in local benchmark mode, got %d", rec.Code)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items) == 0 {
		t.Fatalf("expected valid product list in local mode, got err=%v", err)
	}

	// Health check
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	mux.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /health, got %d", recHealth.Code)
	}
}
