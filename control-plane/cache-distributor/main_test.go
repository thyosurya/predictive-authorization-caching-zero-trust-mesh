package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/revocation"
	"github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/token"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testAdminToken = "test-admin-token"

func newTestDistributorService() *DistributorService {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	signer, _ := token.NewSigner(key)
	return &DistributorService{
		signer:       signer,
		revoker:      revocation.NewManager(),
		broadcaster:  revocation.NewBroadcaster(),
		opaPDPURL:    "http://localhost:8181",
		graphURL:     "http://localhost:8080",
		adminToken:   testAdminToken,
		enforceOPA:   false,
		activeTokens: make(map[string]*token.DelegationToken),
	}
}

func withAdminAuth(req *http.Request) *http.Request {
	req.Header.Set("X-Admin-Token", testAdminToken)
	return req
}

// TestHandleDistribute memverifikasi POST /distribute membuat tokens
func TestHandleDistribute(t *testing.T) {
	svc := newTestDistributorService()
	body := `[{"source":"svc-a","target":"svc-b","allowed_actions":["GET"]}]`
	req := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body)))
	w := httptest.NewRecorder()
	svc.handleDistribute(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp map[string]int
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["tokens_created"] != 1 {
		t.Errorf("expected 1 token created, got %d", resp["tokens_created"])
	}
}

// TestHandleDistribute_Unauthorized memverifikasi penolakan akses tanpa token admin valid (C1)
func TestHandleDistribute_Unauthorized(t *testing.T) {
	svc := newTestDistributorService()
	body := `[{"source":"svc-a","target":"svc-b","allowed_actions":["GET"]}]`

	// 1. Tanpa token
	req1 := httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body))
	w1 := httptest.NewRecorder()
	svc.handleDistribute(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w1.Code)
	}

	// 2. Token salah
	req2 := httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body))
	req2.Header.Set("X-Admin-Token", "wrong-secret")
	w2 := httptest.NewRecorder()
	svc.handleDistribute(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w2.Code)
	}

	// 3. Bearer token valid
	req3 := httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body))
	req3.Header.Set("Authorization", "Bearer "+testAdminToken)
	w3 := httptest.NewRecorder()
	svc.handleDistribute(w3, req3)
	if w3.Code != http.StatusAccepted {
		t.Errorf("expected 202 with Bearer token, got %d", w3.Code)
	}
}

// TestHandleDistribute_DelimiterInjection memverifikasi bahwa karakter injeksi ditolak
func TestHandleDistribute_DelimiterInjection(t *testing.T) {
	svc := newTestDistributorService()

	injections := []string{
		`[{"source":"svc|injected","target":"svc-b","allowed_actions":["GET"]}]`,
		`[{"source":"svc-a","target":"svc-b,injected","allowed_actions":["GET"]}]`,
		`[{"source":"svc-a->extra","target":"svc-b","allowed_actions":["GET"]}]`,
		`[{"source":"svc-a","target":"svc-b","allowed_actions":["GET|POST"]}]`,
		`[{"source":"svc-a","target":"svc-b","allowed_actions":["GET,POST"]}]`,
	}

	for i, payload := range injections {
		req := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(payload)))
		w := httptest.NewRecorder()
		svc.handleDistribute(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("[%d] expected 202, got %d", i, w.Code)
		}

		var resp map[string]int
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["tokens_created"] != 0 {
			t.Errorf("[%d] expected 0 tokens created for payload %s, got %d", i, payload, resp["tokens_created"])
		}
	}
}

// TestHandleDistribute_MethodNotAllowed memverifikasi GET ditolak
func TestHandleDistribute_MethodNotAllowed(t *testing.T) {
	svc := newTestDistributorService()
	req := withAdminAuth(httptest.NewRequest(http.MethodGet, "/distribute", nil))
	w := httptest.NewRecorder()
	svc.handleDistribute(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestHandleDistribute_OPAValidation memverifikasi filter dan fail-closed via OPA (C1)
func TestHandleDistribute_OPAValidation(t *testing.T) {
	// Mock OPA PDP server
	opaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input struct {
				SourceIdentity string `json:"source_identity"`
				TargetIdentity string `json:"target_identity"`
			} `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.Input.SourceIdentity == "svc-a" && req.Input.TargetIdentity == "svc-b" {
			// Izinkan hanya GET, dengan source/target identity (R5)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":[{"source_identity":"svc-a","target_identity":"svc-b","allowed_actions":["GET"]}]}`))
			return
		}
		// Pasangan lain tidak diizinkan
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":[]}`))
	}))
	defer opaServer.Close()

	svc := newTestDistributorService()
	svc.opaPDPURL = opaServer.URL
	svc.enforceOPA = true

	// 1. Request dengan aksi ["GET", "DELETE"] -> hanya "GET" yang lolos
	body1 := `[{"source":"svc-a","target":"svc-b","allowed_actions":["GET","DELETE"]}]`
	req1 := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body1)))
	w1 := httptest.NewRecorder()
	svc.handleDistribute(w1, req1)

	if w1.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w1.Code)
	}
	var resp1 map[string]int
	json.NewDecoder(w1.Body).Decode(&resp1)
	if resp1["tokens_created"] != 1 {
		t.Fatalf("expected 1 token created, got %d", resp1["tokens_created"])
	}

	// Verifikasi token yang tersimpan hanya memiliki aksi "GET"
	tok := svc.activeTokens["svc-a->svc-b"]
	if tok == nil || len(tok.AllowedActions) != 1 || tok.AllowedActions[0] != "GET" {
		t.Fatalf("expected token with only ['GET'], got %v", tok)
	}

	// 2. Request untuk pasangan yang ditolak oleh OPA -> 0 token dibuat (fail-closed)
	body2 := `[{"source":"svc-a","target":"svc-forbidden","allowed_actions":["GET"]}]`
	req2 := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body2)))
	w2 := httptest.NewRecorder()
	svc.handleDistribute(w2, req2)

	var resp2 map[string]int
	json.NewDecoder(w2.Body).Decode(&resp2)
	if resp2["tokens_created"] != 0 {
		t.Errorf("expected 0 token created for forbidden pair, got %d", resp2["tokens_created"])
	}
}

// TestHandleRevoke memverifikasi POST /revoke berhasil setelah distribute
func TestHandleRevoke(t *testing.T) {
	svc := newTestDistributorService()

	// Distribute dulu agar ada active token
	distBody := `[{"source":"svc-a","target":"svc-b","allowed_actions":["GET"]}]`
	distReq := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(distBody)))
	distW := httptest.NewRecorder()
	svc.handleDistribute(distW, distReq)

	// Revoke
	revokeBody := `{"source":"svc-a","target":"svc-b","reason":"policy_update"}`
	revokeReq := withAdminAuth(httptest.NewRequest(http.MethodPost, "/revoke", strings.NewReader(revokeBody)))
	revokeW := httptest.NewRecorder()
	svc.handleRevoke(revokeW, revokeReq)

	if revokeW.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", revokeW.Code)
	}
}

// TestHandleRevoke_Unauthorized memverifikasi penolakan revoke tanpa token admin (C1)
func TestHandleRevoke_Unauthorized(t *testing.T) {
	svc := newTestDistributorService()
	body := `{"source":"svc-a","target":"svc-b","reason":"policy_update"}`

	// Tanpa token
	req := httptest.NewRequest(http.MethodPost, "/revoke", strings.NewReader(body))
	w := httptest.NewRecorder()
	svc.handleRevoke(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestHandleRevoke_NotFound memverifikasi revoke gagal jika pair tidak ada
func TestHandleRevoke_NotFound(t *testing.T) {
	svc := newTestDistributorService()
	body := `{"source":"unknown","target":"svc","reason":"test"}`
	req := withAdminAuth(httptest.NewRequest(http.MethodPost, "/revoke", strings.NewReader(body)))
	w := httptest.NewRecorder()
	svc.handleRevoke(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestHandleActiveTokens memverifikasi GET /tokens/active mengembalikan array
func TestHandleActiveTokens(t *testing.T) {
	svc := newTestDistributorService()
	req := httptest.NewRequest(http.MethodGet, "/tokens/active", nil)
	w := httptest.NewRecorder()
	svc.handleActiveTokens(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestHandleHealth memverifikasi GET /health
func TestHandleHealth(t *testing.T) {
	svc := newTestDistributorService()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	svc.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

// TestRoutes memverifikasi semua route terdaftar
func TestRoutes(t *testing.T) {
	svc := newTestDistributorService()
	mux := svc.routes()
	if mux == nil {
		t.Fatal("routes() returned nil")
	}
}

// TestEnvOrDefault memverifikasi fungsi helper
func TestEnvOrDefault(t *testing.T) {
	result := envOrDefault("NON_EXISTENT_VAR_ZTA_TEST", "default-val")
	if result != "default-val" {
		t.Errorf("expected default-val, got %s", result)
	}
}

// TestEnvIntOrDefault memverifikasi fungsi helper int
func TestEnvIntOrDefault(t *testing.T) {
	result := envIntOrDefault("NON_EXISTENT_INT_ZTA_TEST", 42)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

// TestDistributeAndActiveTokensIntegration memverifikasi end-to-end distribute -> active tokens
func TestDistributeAndActiveTokensIntegration(t *testing.T) {
	svc := newTestDistributorService()

	// Distribute 2 tokens
	body := `[{"source":"A","target":"B","allowed_actions":["GET"]},{"source":"C","target":"D","allowed_actions":["POST"]}]`
	distReq := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body)))
	distW := httptest.NewRecorder()
	svc.handleDistribute(distW, distReq)

	// Check active
	actReq := httptest.NewRequest(http.MethodGet, "/tokens/active", nil)
	actW := httptest.NewRecorder()
	svc.handleActiveTokens(actW, actReq)

	var tokens []map[string]interface{}
	json.NewDecoder(actW.Body).Decode(&tokens)
	if len(tokens) != 2 {
		t.Errorf("expected 2 active tokens, got %d", len(tokens))
	}
}

// Ensure base64 import is used
var _ = base64.StdEncoding

// TestHandleDistribute_TTLClamp memverifikasi bahwa TTL > MaxTTL di-clamp ke MaxTTL (R4)
func TestHandleDistribute_TTLClamp(t *testing.T) {
	svc := newTestDistributorService()
	// Request dengan TTL 120000ms (120s) yang melebihi MaxTTL (30s)
	body := `[{"source":"svc-a","target":"svc-b","allowed_actions":["GET"],"suggested_ttl_ms":120000}]`
	req := withAdminAuth(httptest.NewRequest(http.MethodPost, "/distribute", strings.NewReader(body)))
	w := httptest.NewRecorder()
	svc.handleDistribute(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	tok := svc.activeTokens["svc-a->svc-b"]
	if tok == nil {
		t.Fatal("expected token to be created")
	}
	// CacheTTLMs harus di-clamp ke MaxTTL = 30000ms
	if tok.CacheTTLMs > 30000 {
		t.Errorf("expected CacheTTLMs <= 30000 (MaxTTL), got %d", tok.CacheTTLMs)
	}
}

// TestVerifyAdminToken_FailClosed memverifikasi bahwa verifyAdminToken menolak semua request
// jika adminToken kosong (R2: fail-closed)
func TestVerifyAdminToken_FailClosed(t *testing.T) {
	svc := newTestDistributorService()
	svc.adminToken = "" // simulasi adminToken kosong

	req := httptest.NewRequest(http.MethodPost, "/distribute", nil)
	req.Header.Set("X-Admin-Token", "anything")

	if svc.verifyAdminToken(req) {
		t.Error("expected verifyAdminToken to return false when adminToken is empty (fail-closed)")
	}
}

type testServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *testServerStream) Context() context.Context {
	return s.ctx
}

// TestGrpcAdminStreamAuthInterceptor memverifikasi bahwa StreamInterceptor menolak stream tanpa token valid
// dan mengizinkan jika token valid
func TestGrpcAdminStreamAuthInterceptor(t *testing.T) {
	svc := newTestDistributorService()

	handlerCalled := false
	dummyHandler := func(srv interface{}, stream grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	// 1. Tanpa metadata context
	ssNoMeta := &testServerStream{ctx: context.Background()}
	err := svc.grpcAdminStreamAuthInterceptor(nil, ssNoMeta, nil, dummyHandler)
	if err == nil || status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated for missing metadata, got %v", err)
	}

	// 2. Token kosong / metadata tidak ada x-admin-token
	ctxEmptyMeta := metadata.NewIncomingContext(context.Background(), metadata.Pairs("other-key", "value"))
	ssEmptyMeta := &testServerStream{ctx: ctxEmptyMeta}
	err = svc.grpcAdminStreamAuthInterceptor(nil, ssEmptyMeta, nil, dummyHandler)
	if err == nil || status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated for missing x-admin-token, got %v", err)
	}

	// 3. Token salah
	ctxWrongToken := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-admin-token", "wrong-token"))
	ssWrongToken := &testServerStream{ctx: ctxWrongToken}
	err = svc.grpcAdminStreamAuthInterceptor(nil, ssWrongToken, nil, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for wrong admin token, got %v", err)
	}

	// 4. Token valid
	ctxValid := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-admin-token", testAdminToken))
	ssValid := &testServerStream{ctx: ctxValid}
	err = svc.grpcAdminStreamAuthInterceptor(nil, ssValid, nil, dummyHandler)
	if err != nil {
		t.Errorf("expected nil error for valid admin token, got %v", err)
	}
	if !handlerCalled {
		t.Error("expected stream handler to be called for valid token")
	}
}

// TestQueryAllowedActions_RejectsImplicitWildcard memverifikasi rule dengan source_identity / target_identity kosong
// TIDAK bertindak sebagai wildcard implisit (fail-closed)
func TestQueryAllowedActions_RejectsImplicitWildcard(t *testing.T) {
	// Mock OPA PDP server yang mengembalikan rule dengan field kosong (potensi celah fail-open lama)
	opaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Rule pertama: source kosong (tidak boleh cocok ke svc-x)
		// Rule kedua: target kosong (tidak boleh cocok ke svc-y)
		// Rule ketiga: eksplisit wildcard "*" (hanya ini yang boleh cocok jika ada)
		respJSON := `{
			"result": [
				{"source_identity":"", "target_identity":"svc-b", "allowed_actions":["DELETE"]},
				{"source_identity":"svc-a", "target_identity":"", "allowed_actions":["PUT"]},
				{"source_identity":"*", "target_identity":"*", "allowed_actions":["GET"]}
			]
		}`
		w.Write([]byte(respJSON))
	}))
	defer opaServer.Close()

	svc := newTestDistributorService()
	svc.opaPDPURL = opaServer.URL
	client := &http.Client{Timeout: 2 * time.Second}

	// Panggil untuk svc-unknown -> svc-b: TIDAK boleh mencocokkan rule pertama (source kosong)
	// Hanya boleh mencocokkan rule ketiga ("*") -> ["GET"]
	actions := svc.queryAllowedActions(client, "svc-unknown", "svc-b")
	if len(actions) != 1 || actions[0] != "GET" {
		t.Errorf("expected only explicit wildcard rule ['GET'], got %v", actions)
	}

	// Panggil untuk svc-a -> svc-unknown: TIDAK boleh mencocokkan rule kedua (target kosong)
	actions2 := svc.queryAllowedActions(client, "svc-a", "svc-unknown")
	if len(actions2) != 1 || actions2[0] != "GET" {
		t.Errorf("expected only explicit wildcard rule ['GET'], got %v", actions2)
	}
}
