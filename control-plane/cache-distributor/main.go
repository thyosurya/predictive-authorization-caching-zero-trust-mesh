package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/metrics"
	pb "github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/proto"
	"github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/revocation"
	"github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/token"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// DistributorService mengorkestrasi pembuatan token, gRPC server, dan REST API.
// Struct utama sesuai dokumentasi 06_arsitektur_detail_per_file.md.
// MaxTTL adalah batas atas TTL token delegasi (30 detik) sesuai jaminan AS-03.
const MaxTTL = 30 * time.Second

type DistributorService struct {
	signer      *token.Signer
	revoker     *revocation.Manager
	broadcaster *revocation.Broadcaster
	opaPDPURL   string
	graphURL    string
	adminToken  string
	enforceOPA  bool

	mu           sync.RWMutex
	activeTokens map[string]*token.DelegationToken // key: "source->target"
}

// DistributionRequest adalah payload POST /distribute
type DistributionRequest struct {
	Source          string   `json:"source"`
	Target          string   `json:"target"`
	AllowedActions  []string `json:"allowed_actions"`
	SuggestedTTLMs  int64    `json:"suggested_ttl_ms,omitempty"`
	Sensitivity     string   `json:"sensitivity,omitempty"`
}

// RevocationRequestPayload adalah payload POST /revoke
type RevocationRequestPayload struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// CandidateResponse dari graph-analyzer /candidates
type CandidateResponse struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	CallsPerMin     int64   `json:"calls_per_min"`
	SuggestedTTL    int64   `json:"suggested_ttl_ms"`
	Sensitivity     string  `json:"sensitivity"`
	CentralityScore float64 `json:"centrality_score"`
}

// gRPC server implementation
type grpcServer struct {
	pb.UnimplementedCacheDistributorServer
	svc *DistributorService
}

func (s *grpcServer) PushToken(ctx context.Context, payload *pb.TokenPayload) (*pb.Ack, error) {
	if payload == nil || payload.TokenJson == "" || payload.Signature == "" {
		return &pb.Ack{Success: false, Message: "missing token payload or signature"}, nil
	}

	var tok token.DelegationToken
	if err := json.Unmarshal([]byte(payload.TokenJson), &tok); err != nil {
		return &pb.Ack{Success: false, Message: "invalid token json: " + err.Error()}, nil
	}

	// Wajib verifikasi HMAC/Ed25519 signature terhadap CanonicalPayload sebelum diterima
	canonical := tok.CanonicalPayload()
	if err := s.svc.signer.Verify(canonical, payload.Signature); err != nil {
		return &pb.Ack{Success: false, Message: "cryptographic signature verification failed: " + err.Error()}, nil
	}
	tok.Signature = payload.Signature

	s.svc.mu.Lock()
	key := tok.SourceIdentity + "->" + tok.TargetIdentity
	s.svc.activeTokens[key] = &tok
	s.svc.mu.Unlock()
	_ = s.svc.revoker.RegisterNonce(tok.SourceIdentity, tok.TargetIdentity, tok.RevocationNonce)
	return &pb.Ack{Success: true, Message: "Token verified and pushed"}, nil
}

func (s *grpcServer) RevokeToken(ctx context.Context, req *pb.RevocationRequest) (*pb.Ack, error) {
	newNonce, err := s.svc.revoker.Revoke(req.SourceIdentity, req.TargetIdentity)
	if err != nil {
		return &pb.Ack{Success: false, Message: err.Error()}, nil
	}
	s.svc.broadcaster.Broadcast(req.SourceIdentity, req.TargetIdentity, newNonce)
	s.svc.mu.Lock()
	delete(s.svc.activeTokens, req.SourceIdentity+"->"+req.TargetIdentity)
	s.svc.mu.Unlock()
	return &pb.Ack{Success: true, Message: newNonce}, nil
}

func (s *grpcServer) StreamRevocations(req *pb.SubscribeRequest, stream pb.CacheDistributor_StreamRevocationsServer) error {
	if req == nil || req.SidecarId == "" {
		return fmt.Errorf("sidecar_id is required")
	}

	eventCh := s.svc.broadcaster.Register(req.SidecarId)
	metrics.GrpcStreamCount.Inc()
	defer func() {
		s.svc.broadcaster.Unregister(req.SidecarId)
		metrics.GrpcStreamCount.Dec()
	}()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			pbEvent := &pb.RevocationEvent{
				SourceIdentity: ev.SourceIdentity,
				TargetIdentity: ev.TargetIdentity,
				NewNonce:       ev.NewNonce,
				Timestamp:      ev.Timestamp,
			}
			if err := stream.Send(pbEvent); err != nil {
				log.Printf("StreamRevocations: failed to send to sidecar %s: %v", req.SidecarId, err)
				return err
			}
		}
	}
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
	log.Println("Starting Cache Distributor...")

	// Baca ENV config
	signingKeyB64 := os.Getenv("SIGNING_KEY")
	if signingKeyB64 == "" {
		log.Fatal("SIGNING_KEY environment variable is required")
	}
	signingKey, err := base64.StdEncoding.DecodeString(signingKeyB64)
	if err != nil {
		log.Fatalf("Invalid SIGNING_KEY (must be base64): %v", err)
	}

	signer, err := token.NewSigner(signingKey)
	if err != nil {
		log.Fatalf("Failed to create signer: %v", err)
	}

	graphURL := envOrDefault("GRAPH_ANALYZER_URL", "http://graph-analyzer:8080")
	opaPDPURL := envOrDefault("OPA_URL", "http://opa-service:8181")

	// ADMIN_TOKEN wajib diset secara eksplisit; tidak ada kredensial default.
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required (no default credential)")
	}
	enforceOPA := envOrDefault("ENFORCE_OPA", "true") == "true"
	if !enforceOPA {
		log.Println("WARNING: ENFORCE_OPA=false; OPA policy enforcement disabled (use only for local development)")
	}
	grpcPort := envOrDefault("GRPC_PORT", "50051")
	restPort := envOrDefault("REST_PORT", envOrDefault("PORT", "8081"))
	metricsPort := envOrDefault("METRICS_PORT", "2113")
	distributionInterval := envIntOrDefault("DISTRIBUTION_INTERVAL_S", 5)

	svc := &DistributorService{
		signer:       signer,
		revoker:      revocation.NewManager(),
		broadcaster:  revocation.NewBroadcaster(),
		opaPDPURL:    opaPDPURL,
		graphURL:     graphURL,
		adminToken:   adminToken,
		enforceOPA:   enforceOPA,
		activeTokens: make(map[string]*token.DelegationToken),
	}

	// Metrics endpoint
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		log.Printf("Metrics listening on :%s", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
			log.Printf("Metrics listener warning: %v", err)
		}
	}()

	// gRPC Server with admin token interceptors (unary & streaming)
	go func() {
		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("failed to listen on %s: %v", grpcPort, err)
		}
		s := grpc.NewServer(
			grpc.UnaryInterceptor(svc.grpcAdminAuthInterceptor),
			grpc.StreamInterceptor(svc.grpcAdminStreamAuthInterceptor),
		)
		pb.RegisterCacheDistributorServer(s, &grpcServer{svc: svc})
		log.Printf("gRPC Server listening on :%s", grpcPort)
		s.Serve(lis)
	}()

	// Background goroutine: distributeLoop
	go svc.distributeLoop(time.Duration(distributionInterval) * time.Second)

	// Background goroutine: pruneBlacklist setiap 2 menit
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			svc.revoker.PruneBlacklist(2 * time.Minute)
		}
	}()

	// Background goroutine: expired token cleanup setiap 10 detik
	go svc.expiredTokenCleanup()

	// REST API
	mux := svc.routes()

	log.Printf("Cache Distributor API listening on :%s", restPort)
	log.Fatal(http.ListenAndServe(":"+restPort, mux))
}

func (svc *DistributorService) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/distribute", svc.handleDistribute)
	mux.HandleFunc("/revoke", svc.handleRevoke)
	mux.HandleFunc("/tokens/active", svc.handleActiveTokens)
	mux.HandleFunc("/health", svc.handleHealth)
	return mux
}

// isValidIdentifier memvalidasi identifier SPIFFE, service name, atau action
// untuk mencegah Delimiter Injection (AS-06) pada CanonicalPayload dan cache key.
func isValidIdentifier(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	if strings.Contains(s, "->") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Tolak karakter kontrol ASCII (<= 0x20, 0x7F) serta delimiter payload ('|', ',', '"', ';')
		if c <= 0x20 || c == 0x7F || c == '|' || c == ',' || c == '"' || c == ';' {
			return false
		}
	}
	return true
}

// verifyAdminToken memverifikasi header X-Admin-Token atau Authorization: Bearer
// menggunakan perbandingan waktu-konstan (constant-time comparison) untuk mitigasi timing attack.
// Fail-closed: mengembalikan false jika adminToken kosong.
func (svc *DistributorService) verifyAdminToken(r *http.Request) bool {
	if svc.adminToken == "" {
		return false // fail-closed: tidak ada token terkonfigurasi = tolak semua
	}
	providedToken := r.Header.Get("X-Admin-Token")
	if providedToken == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			providedToken = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if providedToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(providedToken), []byte(svc.adminToken)) == 1
}

// grpcAdminAuthInterceptor memverifikasi metadata "x-admin-token" pada panggilan gRPC unary.
func (svc *DistributorService) grpcAdminAuthInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	tokens := md.Get("x-admin-token")
	if len(tokens) == 0 || tokens[0] == "" {
		return nil, status.Errorf(codes.Unauthenticated, "x-admin-token required")
	}
	if svc.adminToken == "" || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(svc.adminToken)) != 1 {
		return nil, status.Errorf(codes.PermissionDenied, "invalid admin token")
	}
	return handler(ctx, req)
}

// grpcAdminStreamAuthInterceptor memverifikasi metadata "x-admin-token" pada panggilan gRPC server streaming (StreamRevocations).
func (svc *DistributorService) grpcAdminStreamAuthInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	tokens := md.Get("x-admin-token")
	if len(tokens) == 0 || tokens[0] == "" {
		return status.Errorf(codes.Unauthenticated, "x-admin-token required")
	}
	if svc.adminToken == "" || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(svc.adminToken)) != 1 {
		return status.Errorf(codes.PermissionDenied, "invalid admin token")
	}
	return handler(srv, ss)
}

// POST /distribute - menerima kandidat dan membuat delegation token
func (svc *DistributorService) handleDistribute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Autentikasi Admin Token (fail-closed constant-time)
	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	var reqs []DistributionRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	var created int
	for _, req := range reqs {
		// Validasi input: tolak delimiter injection (AS-06)
		if !isValidIdentifier(req.Source) || !isValidIdentifier(req.Target) {
			continue
		}
		if len(req.AllowedActions) == 0 {
			continue
		}
		validActions := true
		for _, act := range req.AllowedActions {
			if !isValidIdentifier(act) {
				validActions = false
				break
			}
		}
		if !validActions {
			continue
		}

		// Validasi ke OPA PDP jika ditegakkan (Zero Trust Fail-Closed Policy Enforcement)
		allowedActions := req.AllowedActions
		if svc.enforceOPA && svc.opaPDPURL != "" {
			opaAllowed := svc.queryAllowedActions(client, req.Source, req.Target)
			if len(opaAllowed) == 0 {
				log.Printf("handleDistribute: rejected %s -> %s: no allowed actions from OPA (fail-closed)", req.Source, req.Target)
				continue
			}
			// Filter aksi yang diizinkan oleh OPA saja
			var filtered []string
			for _, act := range req.AllowedActions {
				for _, op := range opaAllowed {
					if op == "*" || op == act {
						filtered = append(filtered, act)
						break
					}
				}
			}
			if len(filtered) == 0 {
				log.Printf("handleDistribute: rejected %s -> %s: requested actions %v not permitted by OPA %v", req.Source, req.Target, req.AllowedActions, opaAllowed)
				continue
			}
			allowedActions = filtered
		}

		nonce, err := token.GenerateNonce()
		if err != nil {
			continue
		}

		// TTL dari request atau default 30s; di-clamp ke MaxTTL (defense-in-depth)
		ttl := 30 * time.Second
		if req.SuggestedTTLMs > 0 {
			ttl = time.Duration(req.SuggestedTTLMs) * time.Millisecond
		}
		if ttl <= 0 {
			continue
		}
		if ttl > MaxTTL {
			ttl = MaxTTL
		}

		// Sensitivity dari request atau default "medium"
		sensitivity := "medium"
		if req.Sensitivity != "" {
			sensitivity = req.Sensitivity
		}

		// Buat delegation token
		tok, err := token.NewDelegationToken(
			req.Source, req.Target,
			allowedActions,
			sensitivity,
			ttl,
			nonce,
		)
		if err != nil {
			continue
		}

		// Register nonce di revocation manager HANYA setelah token berhasil dibuat
		svc.revoker.RegisterNonce(req.Source, req.Target, nonce)

		// Sign token
		payload := tok.CanonicalPayload()
		sig, _ := svc.signer.Sign(payload)
		tok.Signature = sig

		// Simpan di active tokens (proteksi mutex lock terhadap pembacaan len)
		svc.mu.Lock()
		key := req.Source + "->" + req.Target
		svc.activeTokens[key] = tok
		totalActive := len(svc.activeTokens)
		svc.mu.Unlock()

		// Record metrics
		metrics.TokensIssuedTotal.Inc()
		metrics.TokensActive.Set(float64(totalActive))
		metrics.TokenTTL.WithLabelValues(req.Source, req.Target).Observe(float64(tok.CacheTTLMs))

		created++
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int{"tokens_created": created})
}

// POST /revoke - mencabut token untuk pasangan service
func (svc *DistributorService) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Autentikasi Admin Token (Remediasi C1)
	if !svc.verifyAdminToken(r) {
		http.Error(w, "Unauthorized: valid admin token required", http.StatusUnauthorized)
		return
	}

	var req RevocationRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("ERROR: JSON decode failed: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	start := time.Now()
	newNonce, err := svc.revoker.Revoke(req.Source, req.Target)
	if err != nil {
		http.Error(w, fmt.Sprintf("Revocation failed: %v", err), http.StatusNotFound)
		return
	}

	// Broadcast ke semua sidecar
	svc.broadcaster.Broadcast(req.Source, req.Target, newNonce)

	// Hapus dari active tokens
	svc.mu.Lock()
	delete(svc.activeTokens, req.Source+"->"+req.Target)
	metrics.TokensActive.Set(float64(len(svc.activeTokens)))
	svc.mu.Unlock()

	// Record revocation metrics
	elapsed := float64(time.Since(start).Milliseconds())
	metrics.RevocationLatency.Observe(elapsed)
	metrics.RevocationEventsTotal.WithLabelValues(req.Reason).Inc()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"revoked":   true,
		"new_nonce": newNonce,
	})
}

// GET /tokens/active - mengembalikan daftar token aktif lengkap dengan nonce & actions
func (svc *DistributorService) handleActiveTokens(w http.ResponseWriter, r *http.Request) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	type activeToken struct {
		Source          string   `json:"source"`
		Target          string   `json:"target"`
		AllowedActions  []string `json:"allowed_actions"`
		ExpiresAt       int64    `json:"exp"`
		TTLMs           int64    `json:"ttl_ms"`
		RevocationNonce string   `json:"revocation_nonce"`
		Signature       string   `json:"signature"`
	}

	var tokens []activeToken
	for _, tok := range svc.activeTokens {
		tokens = append(tokens, activeToken{
			Source:          tok.SourceIdentity,
			Target:          tok.TargetIdentity,
			AllowedActions:  tok.AllowedActions,
			ExpiresAt:       tok.ExpiresAt,
			TTLMs:           tok.CacheTTLMs,
			RevocationNonce: tok.RevocationNonce,
			Signature:       tok.Signature,
		})
	}

	if tokens == nil {
		tokens = []activeToken{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

// GET /health - health check
func (svc *DistributorService) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	svc.mu.RLock()
	totalActive := len(svc.activeTokens)
	svc.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"connected_sidecars": svc.broadcaster.ConnectedCount(),
		"active_tokens":     totalActive,
	})
}

// distributeLoop - query graph-analyzer setiap interval, buat token untuk kandidat
func (svc *DistributorService) distributeLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	for range ticker.C {
		// 1. Query graph-analyzer /candidates dengan otorisasi admin token
		req, err := http.NewRequest(http.MethodGet, svc.graphURL+"/candidates", nil)
		if err != nil {
			log.Printf("distributeLoop: failed to create request: %v", err)
			continue
		}
		if svc.adminToken != "" {
			req.Header.Set("X-Admin-Token", svc.adminToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("distributeLoop: failed to query graph-analyzer: %v", err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var candidates []CandidateResponse
		if err := json.Unmarshal(body, &candidates); err != nil {
			log.Printf("distributeLoop: failed to parse candidates: %v", err)
			continue
		}

		// 2. Untuk tiap kandidat: buat token
		for _, c := range candidates {
			// Validasi input: tolak delimiter injection (AS-06)
			if !isValidIdentifier(c.Source) || !isValidIdentifier(c.Target) {
				continue
			}

			nonce, err := token.GenerateNonce()
			if err != nil {
				continue
			}

			ttl := time.Duration(c.SuggestedTTL) * time.Millisecond
			if ttl <= 0 {
				continue // Jangan terbitkan token jika SuggestedTTL <= 0 (sertifikat kedaluwarsa atau invalid)
			}

			// Gunakan sensitivity dari graph-analyzer, default "medium"
			sensitivity := c.Sensitivity
			if sensitivity == "" {
				sensitivity = "medium"
			}

			// Query OPA untuk allowed actions per pasangan
			actions := svc.queryAllowedActions(client, c.Source, c.Target)
			if len(actions) == 0 {
				// ZERO TRUST FAIL-CLOSED: Jangan pernah menerbitkan token jika OPA menolak,
				// tidak memiliki aturan yang cocok, atau sedang mengalami gangguan jaringan.
				log.Printf("distributeLoop: no allowed actions from OPA for %s -> %s (fail-closed, skipping)", c.Source, c.Target)
				continue
			}

			// Validasi aksi dari OPA untuk mencegah Delimiter Injection (AS-06)
			var validActions []string
			for _, act := range actions {
				if isValidIdentifier(act) {
					validActions = append(validActions, act)
				}
			}
			if len(validActions) == 0 {
				log.Printf("distributeLoop: all actions from OPA failed identifier validation for %s -> %s", c.Source, c.Target)
				continue
			}

			tok, err := token.NewDelegationToken(
				c.Source, c.Target,
				validActions,
				sensitivity,
				ttl,
				nonce,
			)
			if err != nil {
				continue
			}

			// Daftarkan nonce ke revocation manager HANYA setelah token berhasil dibuat
			svc.revoker.RegisterNonce(c.Source, c.Target, nonce)

			payload := tok.CanonicalPayload()
			sig, _ := svc.signer.Sign(payload)
			tok.Signature = sig

			svc.mu.Lock()
			svc.activeTokens[c.Source+"->"+c.Target] = tok
			svc.mu.Unlock()

			metrics.TokensIssuedTotal.Inc()
			metrics.TokenTTL.WithLabelValues(c.Source, c.Target).Observe(float64(tok.CacheTTLMs))
		}

		svc.mu.RLock()
		metrics.TokensActive.Set(float64(len(svc.activeTokens)))
		svc.mu.RUnlock()

		if len(candidates) > 0 {
			log.Printf("distributeLoop: processed %d candidates, %d active tokens", len(candidates), len(svc.activeTokens))
		}
	}
}

// queryAllowedActions queries OPA untuk mendapat allowed actions per pasangan service.
// Zero Trust Fail-Closed: return nil jika OPA tidak terjangkau atau tidak ada rule cocok.
// Menghilangkan pencocokan wildcard implisit jika identifier kosong.
func (svc *DistributorService) queryAllowedActions(client *http.Client, source, target string) []string {
	type opaInput struct {
		SourceIdentity string `json:"source_identity"`
		TargetIdentity string `json:"target_identity"`
		Action         string `json:"action"`
	}
	type opaRequest struct {
		Input opaInput `json:"input"`
	}

	reqBytes, err := json.Marshal(opaRequest{
		Input: opaInput{
			SourceIdentity: source,
			TargetIdentity: target,
			Action:         "*",
		},
	})
	if err != nil {
		return nil
	}

	resp, err := client.Post(svc.opaPDPURL+"/v1/data/service_rules", "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var result struct {
		Result []struct {
			Source         string   `json:"source"`
			Target         string   `json:"target"`
			SourceIdentity string   `json:"source_identity"`
			TargetIdentity string   `json:"target_identity"`
			AllowedActions []string `json:"allowed_actions"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	// Cari rule yang cocok dengan source DAN target identity secara ketat (tanpa wildcard implisit pada string kosong)
	for _, rule := range result.Result {
		if len(rule.AllowedActions) == 0 {
			continue
		}
		ruleSource := rule.Source
		if ruleSource == "" {
			ruleSource = rule.SourceIdentity
		}
		ruleTarget := rule.Target
		if ruleTarget == "" {
			ruleTarget = rule.TargetIdentity
		}
		sourceMatch := (ruleSource != "" && (ruleSource == source || ruleSource == "*"))
		targetMatch := (ruleTarget != "" && (ruleTarget == target || ruleTarget == "*"))
		if sourceMatch && targetMatch {
			return rule.AllowedActions
		}
	}
	return nil
}

// expiredTokenCleanup menghapus token yang sudah expired dari activeTokens.
// Dijalankan sebagai goroutine setiap 10 detik.
func (svc *DistributorService) expiredTokenCleanup() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().Unix()
		svc.mu.Lock()
		for k, tok := range svc.activeTokens {
			if tok.ExpiresAt < now {
				delete(svc.activeTokens, k)
				metrics.TokensExpiredTotal.Inc()
			}
		}
		metrics.TokensActive.Set(float64(len(svc.activeTokens)))
		svc.mu.Unlock()
	}
}
