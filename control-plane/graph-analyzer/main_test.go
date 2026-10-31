package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/cache"
	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/graph"
	"github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/metrics"
)

func newTestService() *AnalyzerService {
	return &AnalyzerService{
		topology:   graph.NewTopologyGraph(),
		selector:   cache.NewCandidateSelector(1, 50),
		metrics:    metrics.NewCollector(),
		adminToken: "test-admin-token",
	}
}

func withAdminAuth(req *http.Request) *http.Request {
	req.Header.Set("X-Admin-Token", "test-admin-token")
	return req
}

// TestHandleRecordCall memverifikasi POST /record mencatat panggilan dengan token valid
func TestHandleRecordCall(t *testing.T) {
	svc := newTestService()
	body := `{"source":"A","target":"B"}`
	req := withAdminAuth(httptest.NewRequest(http.MethodPost, "/record", strings.NewReader(body)))
	w := httptest.NewRecorder()
	svc.handleRecordCall(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// TestHandleRecordCall_Unauthorized memverifikasi penolakan /record tanpa token admin valid
func TestHandleRecordCall_Unauthorized(t *testing.T) {
	svc := newTestService()
	body := `{"source":"A","target":"B"}`

	// Tanpa token
	req1 := httptest.NewRequest(http.MethodPost, "/record", strings.NewReader(body))
	w1 := httptest.NewRecorder()
	svc.handleRecordCall(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w1.Code)
	}

	// Token salah
	req2 := httptest.NewRequest(http.MethodPost, "/record", strings.NewReader(body))
	req2.Header.Set("X-Admin-Token", "wrong-token")
	w2 := httptest.NewRecorder()
	svc.handleRecordCall(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", w2.Code)
	}
}

// TestHandleRecordCall_MethodNotAllowed memverifikasi GET /record ditolak
func TestHandleRecordCall_MethodNotAllowed(t *testing.T) {
	svc := newTestService()
	req := withAdminAuth(httptest.NewRequest(http.MethodGet, "/record", nil))
	w := httptest.NewRecorder()
	svc.handleRecordCall(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleGetCandidates memverifikasi GET /candidates mengembalikan array dan header mode dengan token valid
func TestHandleGetCandidates(t *testing.T) {
	svc := newTestService()
	req := withAdminAuth(httptest.NewRequest(http.MethodGet, "/candidates", nil))
	w := httptest.NewRecorder()
	svc.handleGetCandidates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if modeHeader := w.Header().Get("X-Prioritization-Mode"); modeHeader != "graph" {
		t.Errorf("expected header X-Prioritization-Mode 'graph', got '%s'", modeHeader)
	}

	var result []CacheCandidateResponse
	json.NewDecoder(w.Body).Decode(&result)
	if result == nil {
		t.Error("expected non-nil array response")
	}

	// Uji pergantian header saat mode diubah ke LFU
	svc.selector.SetMode(graph.ModeFrequencyOnly)
	w2 := httptest.NewRecorder()
	req2 := withAdminAuth(httptest.NewRequest(http.MethodGet, "/candidates", nil))
	svc.handleGetCandidates(w2, req2)
	if modeHeader := w2.Header().Get("X-Prioritization-Mode"); modeHeader != "lfu" {
		t.Errorf("expected header X-Prioritization-Mode 'lfu', got '%s'", modeHeader)
	}
}

// TestHandleGetCandidates_Unauthorized memverifikasi penolakan /candidates tanpa token
func TestHandleGetCandidates_Unauthorized(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/candidates", nil)
	w := httptest.NewRecorder()
	svc.handleGetCandidates(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

// TestHandleHealth memverifikasi GET /health mengembalikan status ok dan mode aktif tanpa token (public probe)
func TestHandleHealth(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	svc.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
	if resp["mode"] != "graph" {
		t.Errorf("expected mode 'graph', got '%s'", resp["mode"])
	}
}

// TestHandleReset_WithValidToken memverifikasi DELETE /graph/reset berhasil dengan token valid
func TestHandleReset_WithValidToken(t *testing.T) {
	svc := newTestService()
	// Tambah data dulu
	svc.topology.RecordCall("A", "B")

	req := withAdminAuth(httptest.NewRequest(http.MethodDelete, "/graph/reset", nil))
	w := httptest.NewRecorder()
	svc.handleReset(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestHandleReset_Unauthorized memverifikasi DELETE /graph/reset ditolak tanpa token
func TestHandleReset_Unauthorized(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodDelete, "/graph/reset", nil)
	req.Header.Set("X-Admin-Token", "wrong-token")
	w := httptest.NewRecorder()
	svc.handleReset(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestHandleGetGraph memverifikasi GET /graph mengembalikan snapshot dengan token valid
func TestHandleGetGraph(t *testing.T) {
	svc := newTestService()
	svc.topology.RecordCall("X", "Y")

	req := withAdminAuth(httptest.NewRequest(http.MethodGet, "/graph", nil))
	w := httptest.NewRecorder()
	svc.handleGetGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestHandleGetGraph_Unauthorized memverifikasi penolakan /graph tanpa token valid
func TestHandleGetGraph_Unauthorized(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	w := httptest.NewRecorder()
	svc.handleGetGraph(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

// TestVerifyAdminToken_FailClosed memverifikasi fail-closed saat adminToken kosong
func TestVerifyAdminToken_FailClosed(t *testing.T) {
	svc := newTestService()
	svc.adminToken = "" // kosong

	req := httptest.NewRequest(http.MethodGet, "/candidates", nil)
	req.Header.Set("X-Admin-Token", "any-token")

	if svc.verifyAdminToken(req) {
		t.Error("expected verifyAdminToken to return false when adminToken is empty (fail-closed)")
	}
}
