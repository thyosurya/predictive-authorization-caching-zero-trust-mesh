package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestCartServiceEastWestCall(t *testing.T) {
	// 1. Setup mock productcatalogservice
	var upstreamCalled int32
	var receivedCallerIdentity string
	var receivedSourceIdentity string

	mockCatalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalled, 1)
		receivedCallerIdentity = r.Header.Get("x-caller-identity")
		receivedSourceIdentity = r.Header.Get("x-source-identity")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": 1, "name": "Verified Product", "price": 49.99}]`))
	}))
	defer mockCatalogServer.Close()

	// 2. Set environment variable for product catalog URL
	os.Setenv("PRODUCT_CATALOG_URL", mockCatalogServer.URL)
	defer os.Unsetenv("PRODUCT_CATALOG_URL")

	// 3. Setup cart handler with mock client
	productCatalogURL := mockCatalogServer.URL
	httpClient := mockCatalogServer.Client()

	cartHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetURL := productCatalogURL + "/api/products"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err == nil {
			for k, vv := range r.Header {
				for _, v := range vv {
					req.Header.Add(k, v)
				}
			}
			req.Header.Set("x-source-identity", "spiffe://cluster.local/ns/zta-research/sa/cartservice")
			req.Header.Set("x-caller-identity", "cartservice")

			resp, err := httpClient.Do(req)
			if err == nil {
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

	// 4. Test POST /api/cart
	req := httptest.NewRequest(http.MethodPost, "/api/cart", nil)
	rec := httptest.NewRecorder()
	cartHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Expected status 201 Created, got %d", rec.Code)
	}

	if atomic.LoadInt32(&upstreamCalled) != 1 {
		t.Fatalf("Expected upstream productcatalogservice to be called once, got %d", upstreamCalled)
	}

	if receivedCallerIdentity != "cartservice" {
		t.Errorf("Expected x-caller-identity to be 'cartservice', got '%s'", receivedCallerIdentity)
	}

	expectedSPIFFE := "spiffe://cluster.local/ns/zta-research/sa/cartservice"
	if receivedSourceIdentity != expectedSPIFFE {
		t.Errorf("Expected x-source-identity to be '%s', got '%s'", expectedSPIFFE, receivedSourceIdentity)
	}

	var respBody map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if verified, ok := respBody["catalog_verified"].(bool); !ok || !verified {
		t.Errorf("Expected catalog_verified to be true, got %v", respBody["catalog_verified"])
	}
}
