# Struktur Proyek & Unit Test
## ZTA Predictive Authorization Caching

---

## Daftar Isi

1. [Struktur Direktori Lengkap](#1-struktur-direktori-lengkap)
2. [Penjelasan Tiap Modul](#2-penjelasan-tiap-modul)
3. [Unit Test: Graph Topology Analyzer](#3-unit-test-graph-topology-analyzer)
4. [Unit Test: TTL Calculator](#4-unit-test-ttl-calculator)
5. [Unit Test: Cache Policy Distributor](#5-unit-test-cache-policy-distributor)
6. [Unit Test: Delegation Token](#6-unit-test-delegation-token)
7. [Unit Test: Revocation Manager](#7-unit-test-revocation-manager)
8. [Unit Test: WASM Filter Logic](#8-unit-test-wasm-filter-logic)
9. [Integration Test](#9-integration-test)
10. [Menjalankan Semua Test](#10-menjalankan-semua-test)

---

## 1. Struktur Direktori Lengkap

```
zta-predictive-cache/
│
├── control-plane/
│   │
│   ├── graph-analyzer/
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── main.go
│   │   ├── main_test.go
│   │   ├── graph/
│   │   │   ├── topology.go           # Core graph data structure
│   │   │   ├── topology_test.go      # Unit test topology
│   │   │   ├── centrality.go         # Betweenness centrality
│   │   │   └── centrality_test.go    # Unit test centrality
│   │   ├── cache/
│   │   │   ├── ttl.go                # TTL calculation algorithm
│   │   │   ├── ttl_test.go           # Unit test TTL formula
│   │   │   └── candidate.go          # Cache candidate selector
│   │   └── metrics/
│   │       ├── prometheus.go         # Prometheus metrics definitions
│   │       └── prometheus_test.go
│   │
│   ├── cache-distributor/
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── main.go
│   │   ├── main_test.go
│   │   ├── token/
│   │   │   ├── delegation.go         # DelegationToken struct & constructor
│   │   │   ├── delegation_test.go    # Unit test token creation & validation
│   │   │   ├── signer.go             # Token signing (HMAC-SHA256)
│   │   │   └── signer_test.go        # Unit test signing & verification
│   │   ├── revocation/
│   │   │   ├── manager.go            # Revocation nonce registry
│   │   │   ├── manager_test.go       # Unit test revocation
│   │   │   └── broadcaster.go        # gRPC push broadcaster
│   │   ├── proto/
│   │   │   ├── cache.proto
│   │   │   ├── cache.pb.go           # generated
│   │   │   └── cache_grpc.pb.go      # generated
│   │   └── metrics/
│   │       ├── prometheus.go
│   │       └── prometheus_test.go
│   │
│   └── pdp/
│       ├── policies/
│       │   ├── authz.rego            # OPA policy
│       │   ├── authz_test.rego       # OPA policy unit test
│       │   └── data.json             # Service rules seed data
│       └── scripts/
│           └── load_policies.sh
│
├── wasm-filter/
│   ├── go.mod
│   ├── go.sum
│   ├── main.go                       # Envoy WASM filter entry point
│   ├── cache/
│   │   ├── local_cache.go            # Local cache logic (shared data)
│   │   └── local_cache_test.go       # Unit test cache logic
│   ├── authz/
│   │   ├── decision.go               # Authorization decision engine
│   │   └── decision_test.go          # Unit test authorization logic
│   └── token/
│       ├── validator.go              # Token validation (exp, nonce, actions)
│       └── validator_test.go         # Unit test token validation
│
├── services/
│   ├── service-a/
│   │   ├── go.mod
│   │   ├── main.go
│   │   ├── handler_test.go
│   │   └── Dockerfile
│   ├── service-b/
│   │   ├── go.mod
│   │   ├── main.go
│   │   ├── handler_test.go
│   │   └── Dockerfile
│   └── service-c/
│       ├── go.mod
│       ├── main.go
│       ├── handler_test.go
│       └── Dockerfile
│
├── k8s/
│   ├── cluster/
│   │   ├── namespace.yaml
│   │   ├── resource-quota.yaml
│   │   ├── secrets.yaml              # ServiceAccounts + Secrets
│   │   └── network-policy-pbs.yaml   # PBS baseline NetworkPolicy
│   ├── istio/
│   │   ├── peer-auth-strict.yaml     # mTLS STRICT mode
│   │   ├── authz-policy-zta.yaml     # ZTA ext_authz policy
│   │   ├── wasm-filter.yaml          # WasmPlugin config
│   │   └── telemetry.yaml            # Tracing config
│   ├── spire/
│   │   ├── server-configmap.yaml
│   │   ├── server-statefulset.yaml
│   │   └── agent-daemonset.yaml
│   └── apps/
│       ├── graph-analyzer.yaml
│       ├── cache-distributor.yaml
│       ├── opa-deployment.yaml
│       └── testbed-boutique.yaml
│
├── load-testing/
│   ├── scenarios/
│   │   ├── s01-pbs-baseline-500rps.js
│   │   ├── s02-zta-naive-500rps.js
│   │   ├── s03-zta-cache-500rps.js
│   │   ├── s04-zta-naive-2000rps.js
│   │   ├── s05-zta-cache-2000rps.js
│   │   ├── s06-zta-cache-5000rps.js
│   │   ├── s07-zta-cache-fanout.js
│   │   ├── s08-zta-cache-mesh.js
│   │   ├── s09-revocation-test.js
│   │   └── s10-ttl-comparison.js
│   ├── results/                      # Output JSON dari k6 (git-ignored)
│   └── warmup.js
│
├── observability/
│   ├── grafana/
│   │   ├── dashboard-zta-research.json
│   │   └── provisioning/             # Datasource + dashboard provisioning
│   ├── prometheus/
│   │   ├── prometheus-local.yml      # Scrape config untuk docker-compose
│   │   └── custom-rules.yaml         # Recording rules + alerts
│   └── jaeger/
│       └── jaeger-deploy.yaml
│
├── scripts/
│   ├── run-all-scenarios.sh          # Jalankan S01–S10 dengan repetisi
│   ├── setup_vm_2core_4gb.sh         # Setup otomatis lean testbed VM 2-Core 4GB
│   ├── verify_offline.py             # Verifikasi integritas arsitektur & offline audit
│   ├── analyze-all-data.py           # Pipeline analisis lengkap (k6 merge, statistik, figures)
│   ├── export-metrics.py             # Export Prometheus → CSV
│   ├── validate-state.sh             # Validasi konfigurasi sebelum test
│   ├── reset-light.sh                # Reset ringan (flush cache)
│   ├── reset-full.sh                 # Reset penuh (restart semua pod)
│   ├── verify-versions.sh            # Cek versi tools
│   ├── snapshot-config.sh            # Simpan snapshot config
│   ├── register-spire-entries.sh     # Daftar workload ke SPIRE
│   └── shutdown.sh                   # Shutdown bersih
│
├── docs/                             # 18 dokumen arsitektur, formalisme, & evaluasi
│
├── .gitignore
├── docker-compose.yml                # Local dev environment
├── requirements.txt                  # Python dependencies
├── Makefile                          # 40+ targets: build, test, deploy
└── README.md
```

---

## 2. Penjelasan Tiap Modul

| Modul | Path | Tanggung Jawab |
|---|---|---|
| `graph/topology` | `graph-analyzer/graph/topology.go` | Menyimpan dan memperbarui directed weighted graph panggilan antar-service |
| `graph/centrality` | `graph-analyzer/graph/centrality.go` | Menghitung betweenness centrality untuk ranking kandidat cache |
| `cache/ttl` | `graph-analyzer/cache/ttl.go` | Menghitung TTL optimal berdasarkan frekuensi, sensitivitas, dan validitas sertifikat |
| `cache/candidate` | `graph-analyzer/cache/candidate.go` | Menyeleksi pasangan service yang layak mendapat delegation |
| `token/delegation` | `cache-distributor/token/delegation.go` | Membuat dan memvalidasi struktur delegation token |
| `token/signer` | `cache-distributor/token/signer.go` | Menandatangani dan memverifikasi token dengan HMAC-SHA256 |
| `revocation/manager` | `cache-distributor/revocation/manager.go` | Menyimpan registry nonce aktif dan memproses pencabutan |
| `revocation/broadcaster` | `cache-distributor/revocation/broadcaster.go` | Push event revokasi ke semua sidecar via gRPC stream |
| `wasm-filter/cache` | `wasm-filter/cache/local_cache.go` | Operasi baca/tulis/hapus pada shared data Envoy |
| `wasm-filter/authz` | `wasm-filter/authz/decision.go` | Logika keputusan otorisasi (cache hit/miss/deny) |
| `wasm-filter/token` | `wasm-filter/token/validator.go` | Validasi expiry, nonce, dan action pada delegation token |

---

## 3. Unit Test: Graph Topology Analyzer

```go
// control-plane/graph-analyzer/graph/topology_test.go
package graph

import (
    "testing"
    "time"
)

// TestNewTopologyGraph memverifikasi inisialisasi graph kosong
func TestNewTopologyGraph(t *testing.T) {
    g := NewTopologyGraph()
    if g == nil {
        t.Fatal("NewTopologyGraph() returned nil")
    }
    if len(g.edges) != 0 {
        t.Errorf("expected 0 edges, got %d", len(g.edges))
    }
}

// TestRecordCall_NewEdge memverifikasi edge baru dibuat saat panggilan pertama
func TestRecordCall_NewEdge(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")

    key := "service-a->service-b"
    edge, exists := g.edges[key]
    if !exists {
        t.Fatalf("expected edge %q to exist", key)
    }
    if edge.CallCount != 1 {
        t.Errorf("expected CallCount=1, got %d", edge.CallCount)
    }
    if edge.Source != "service-a" {
        t.Errorf("expected Source=service-a, got %s", edge.Source)
    }
    if edge.Target != "service-b" {
        t.Errorf("expected Target=service-b, got %s", edge.Target)
    }
}

// TestRecordCall_Increment memverifikasi counter bertambah pada panggilan berulang
func TestRecordCall_Increment(t *testing.T) {
    g := NewTopologyGraph()

    for i := 0; i < 50; i++ {
        g.RecordCall("service-a", "service-b")
    }

    edge := g.edges["service-a->service-b"]
    if edge.CallCount != 50 {
        t.Errorf("expected CallCount=50, got %d", edge.CallCount)
    }
}

// TestRecordCall_DirectionalIsolation memverifikasi A→B dan B→A adalah edge terpisah
func TestRecordCall_DirectionalIsolation(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")
    g.RecordCall("service-b", "service-a")

    if len(g.edges) != 2 {
        t.Errorf("expected 2 separate edges, got %d", len(g.edges))
    }
}

// TestGetHighFrequencyPairs_AboveThreshold memverifikasi hanya edge di atas threshold yang dikembalikan
func TestGetHighFrequencyPairs_AboveThreshold(t *testing.T) {
    g := NewTopologyGraph()

    // Edge dengan frekuensi tinggi
    for i := 0; i < 20; i++ {
        g.RecordCall("service-a", "service-b")
    }

    // Edge dengan frekuensi rendah
    for i := 0; i < 5; i++ {
        g.RecordCall("service-c", "service-d")
    }

    candidates := g.GetHighFrequencyPairs(10)

    if len(candidates) != 1 {
        t.Errorf("expected 1 candidate, got %d", len(candidates))
    }
    if candidates[0].Source != "service-a" || candidates[0].Target != "service-b" {
        t.Errorf("unexpected candidate: %+v", candidates[0])
    }
}

// TestGetHighFrequencyPairs_StaleEdgeExcluded memverifikasi edge lama (>1 menit) dikecualikan
func TestGetHighFrequencyPairs_StaleEdgeExcluded(t *testing.T) {
    g := NewTopologyGraph()
    g.RecordCall("service-a", "service-b")

    // Manipulasi LastSeen ke masa lalu
    edge := g.edges["service-a->service-b"]
    edge.CallCount = 100
    edge.LastSeen = time.Now().Add(-2 * time.Minute) // sudah stale

    candidates := g.GetHighFrequencyPairs(10)
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates (stale), got %d", len(candidates))
    }
}

// TestGetHighFrequencyPairs_Empty memverifikasi graph kosong mengembalikan slice kosong
func TestGetHighFrequencyPairs_Empty(t *testing.T) {
    g := NewTopologyGraph()
    candidates := g.GetHighFrequencyPairs(10)
    if candidates != nil && len(candidates) != 0 {
        t.Errorf("expected empty result, got %v", candidates)
    }
}

// TestRecordCall_Concurrent memverifikasi thread-safety saat concurrent writes
func TestRecordCall_Concurrent(t *testing.T) {
    g := NewTopologyGraph()
    done := make(chan struct{})

    for i := 0; i < 100; i++ {
        go func() {
            g.RecordCall("service-a", "service-b")
            done <- struct{}{}
        }()
    }

    for i := 0; i < 100; i++ {
        <-done
    }

    edge := g.edges["service-a->service-b"]
    if edge.CallCount != 100 {
        t.Errorf("concurrent writes: expected 100, got %d", edge.CallCount)
    }
}
```

---

## 4. Unit Test: TTL Calculator

```go
// control-plane/graph-analyzer/cache/ttl_test.go
package cache

import (
    "testing"
    "time"
)

// TestCalculateTTL_NominalCase memverifikasi output TTL dalam rentang valid
func TestCalculateTTL_NominalCase(t *testing.T) {
    ttl := CalculateTTL(50, SensitivityMedium, 24*time.Hour, 72*time.Hour)

    if ttl < MinTTL {
        t.Errorf("TTL %v below MinTTL %v", ttl, MinTTL)
    }
    if ttl > MaxTTL {
        t.Errorf("TTL %v above MaxTTL %v", ttl, MaxTTL)
    }
}

// TestCalculateTTL_HighFrequencyIncreaseTTL memverifikasi frekuensi tinggi menghasilkan TTL lebih panjang
func TestCalculateTTL_HighFrequencyIncreaseTTL(t *testing.T) {
    ttlLow  := CalculateTTL(10,  SensitivityLow, 24*time.Hour, 72*time.Hour)
    ttlHigh := CalculateTTL(100, SensitivityLow, 24*time.Hour, 72*time.Hour)

    if ttlHigh <= ttlLow {
        t.Errorf("expected higher freq to produce longer TTL: low=%v high=%v", ttlLow, ttlHigh)
    }
}

// TestCalculateTTL_HighSensitivityReducesTTL memverifikasi sensitivitas tinggi mengurangi TTL
func TestCalculateTTL_HighSensitivityReducesTTL(t *testing.T) {
    ttlLow  := CalculateTTL(50, SensitivityLow,  24*time.Hour, 72*time.Hour)
    ttlHigh := CalculateTTL(50, SensitivityHigh, 24*time.Hour, 72*time.Hour)

    if ttlHigh >= ttlLow {
        t.Errorf("expected high sensitivity to produce shorter TTL: low=%v high=%v", ttlLow, ttlHigh)
    }
}

// TestCalculateTTL_NeverExceedsMaxTTL memverifikasi batas atas selalu dihormati
func TestCalculateTTL_NeverExceedsMaxTTL(t *testing.T) {
    // Kondisi terbaik: frekuensi sangat tinggi, sensitivitas rendah, sertifikat masih panjang
    ttl := CalculateTTL(10000, SensitivityLow, 72*time.Hour, 72*time.Hour)

    if ttl > MaxTTL {
        t.Errorf("TTL %v exceeded MaxTTL %v", ttl, MaxTTL)
    }
}

// TestCalculateTTL_NeverBelowMinTTL memverifikasi batas bawah selalu dihormati
func TestCalculateTTL_NeverBelowMinTTL(t *testing.T) {
    // Kondisi terburuk: frekuensi sangat rendah, sensitivitas tinggi, sertifikat hampir habis
    ttl := CalculateTTL(1, SensitivityHigh, 1*time.Minute, 72*time.Hour)

    if ttl < MinTTL {
        t.Errorf("TTL %v below MinTTL %v", ttl, MinTTL)
    }
}

// TestCalculateTTL_CertExpiryReducesTTL memverifikasi sertifikat hampir habis mengurangi TTL
func TestCalculateTTL_CertExpiryReducesTTL(t *testing.T) {
    ttlFresh  := CalculateTTL(50, SensitivityLow, 72*time.Hour, 72*time.Hour)
    ttlExpiry := CalculateTTL(50, SensitivityLow, 1*time.Hour,  72*time.Hour)

    if ttlExpiry >= ttlFresh {
        t.Errorf("expected near-expiry cert to reduce TTL: fresh=%v expiry=%v", ttlFresh, ttlExpiry)
    }
}

// TestCalculateTTL_ZeroFrequency memverifikasi frekuensi nol menghasilkan MinTTL
func TestCalculateTTL_ZeroFrequency(t *testing.T) {
    ttl := CalculateTTL(0, SensitivityLow, 24*time.Hour, 72*time.Hour)
    if ttl != MinTTL {
        t.Errorf("expected MinTTL for zero frequency, got %v", ttl)
    }
}

// TestSensitivityLevels_Ordering memverifikasi urutan: Low > Medium > High TTL
func TestSensitivityLevels_Ordering(t *testing.T) {
    low    := CalculateTTL(50, SensitivityLow,    24*time.Hour, 72*time.Hour)
    medium := CalculateTTL(50, SensitivityMedium, 24*time.Hour, 72*time.Hour)
    high   := CalculateTTL(50, SensitivityHigh,   24*time.Hour, 72*time.Hour)

    if !(low >= medium && medium >= high) {
        t.Errorf("TTL ordering wrong: low=%v medium=%v high=%v", low, medium, high)
    }
}
```

---

## 5. Unit Test: Cache Policy Distributor

```go
// control-plane/cache-distributor/token/signer_test.go
package token

import (
    "testing"
)

// TestSignAndVerify memverifikasi token yang ditandatangani dapat diverifikasi kembali
func TestSignAndVerify(t *testing.T) {
    signer := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    payload := []byte(`{"sub":"service-a","aud":"service-b","exp":9999999999}`)

    sig, err := signer.Sign(payload)
    if err != nil {
        t.Fatalf("Sign() failed: %v", err)
    }

    if err := signer.Verify(payload, sig); err != nil {
        t.Errorf("Verify() failed for valid signature: %v", err)
    }
}

// TestVerify_TamperedPayload memverifikasi payload yang diubah gagal verifikasi
func TestVerify_TamperedPayload(t *testing.T) {
    signer := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    original := []byte(`{"sub":"service-a","exp":9999999999}`)
    tampered := []byte(`{"sub":"service-EVIL","exp":9999999999}`)

    sig, _ := signer.Sign(original)
    err := signer.Verify(tampered, sig)

    if err == nil {
        t.Error("expected Verify() to fail for tampered payload, but it succeeded")
    }
}

// TestVerify_WrongKey memverifikasi kunci berbeda gagal verifikasi
func TestVerify_WrongKey(t *testing.T) {
    signerA := NewSigner([]byte("key-a-32-bytes-padded-xxxxxxxxxx!"))
    signerB := NewSigner([]byte("key-b-32-bytes-padded-yyyyyyyyyy!"))

    payload := []byte(`{"sub":"service-a"}`)
    sig, _ := signerA.Sign(payload)

    err := signerB.Verify(payload, sig)
    if err == nil {
        t.Error("expected Verify() to fail with wrong key")
    }
}

// TestVerify_EmptySignature memverifikasi signature kosong selalu ditolak
func TestVerify_EmptySignature(t *testing.T) {
    signer := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    payload := []byte(`{"sub":"service-a"}`)

    err := signer.Verify(payload, "")
    if err == nil {
        t.Error("expected Verify() to fail for empty signature")
    }
}
```

---

## 6. Unit Test: Delegation Token

```go
// control-plane/cache-distributor/token/delegation_test.go
package token

import (
    "testing"
    "time"
)

// TestNewDelegationToken_FieldsPopulated memverifikasi semua field terisi dengan benar
func TestNewDelegationToken_FieldsPopulated(t *testing.T) {
    token, err := NewDelegationToken(
        "spiffe://cluster.local/ns/default/sa/service-a",
        "spiffe://cluster.local/ns/default/sa/service-b",
        []string{"GET", "POST"},
        30*time.Second,
    )

    if err != nil {
        t.Fatalf("NewDelegationToken() error: %v", err)
    }
    if token.SourceIdentity == "" {
        t.Error("SourceIdentity is empty")
    }
    if token.TargetIdentity == "" {
        t.Error("TargetIdentity is empty")
    }
    if len(token.AllowedActions) != 2 {
        t.Errorf("expected 2 actions, got %d", len(token.AllowedActions))
    }
    if token.RevocationNonce == "" {
        t.Error("RevocationNonce is empty")
    }
    if token.CacheTTLMs != 30000 {
        t.Errorf("expected CacheTTLMs=30000, got %d", token.CacheTTLMs)
    }
}

// TestNewDelegationToken_ExpiryInFuture memverifikasi ExpiresAt selalu di masa depan
func TestNewDelegationToken_ExpiryInFuture(t *testing.T) {
    token, err := NewDelegationToken("a", "b", []string{"GET"}, 30*time.Second)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    now := time.Now().Unix()
    if token.ExpiresAt <= now {
        t.Errorf("ExpiresAt %d is not in the future (now=%d)", token.ExpiresAt, now)
    }
}

// TestNewDelegationToken_UniqueNonces memverifikasi setiap token mendapat nonce unik
func TestNewDelegationToken_UniqueNonces(t *testing.T) {
    nonces := make(map[string]bool)

    for i := 0; i < 100; i++ {
        tok, err := NewDelegationToken("a", "b", []string{"GET"}, 30*time.Second)
        if err != nil {
            t.Fatalf("unexpected error at iteration %d: %v", i, err)
        }
        if nonces[tok.RevocationNonce] {
            t.Errorf("duplicate nonce found: %s", tok.RevocationNonce)
        }
        nonces[tok.RevocationNonce] = true
    }
}

// TestNewDelegationToken_ZeroTTL memverifikasi TTL nol menghasilkan error
func TestNewDelegationToken_ZeroTTL(t *testing.T) {
    _, err := NewDelegationToken("a", "b", []string{"GET"}, 0)
    if err == nil {
        t.Error("expected error for zero TTL")
    }
}

// TestNewDelegationToken_EmptyActions memverifikasi actions kosong menghasilkan error
func TestNewDelegationToken_EmptyActions(t *testing.T) {
    _, err := NewDelegationToken("a", "b", []string{}, 30*time.Second)
    if err == nil {
        t.Error("expected error for empty actions")
    }
}

// TestNewDelegationToken_EmptyIdentity memverifikasi identitas kosong menghasilkan error
func TestNewDelegationToken_EmptyIdentity(t *testing.T) {
    _, err := NewDelegationToken("", "service-b", []string{"GET"}, 30*time.Second)
    if err == nil {
        t.Error("expected error for empty source identity")
    }
}
```

---

## 7. Unit Test: Revocation Manager

```go
// control-plane/cache-distributor/revocation/manager_test.go
package revocation

import (
    "testing"
    "time"
)

// TestRegisterNonce_Success memverifikasi nonce berhasil didaftarkan
func TestRegisterNonce_Success(t *testing.T) {
    m := NewManager()
    err := m.RegisterNonce("service-a", "service-b", "nonce-abc")
    if err != nil {
        t.Errorf("RegisterNonce() unexpected error: %v", err)
    }
}

// TestValidateNonce_Valid memverifikasi nonce yang terdaftar dinyatakan valid
func TestValidateNonce_Valid(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-xyz")

    if !m.ValidateNonce("service-a", "service-b", "nonce-xyz") {
        t.Error("expected nonce to be valid")
    }
}

// TestValidateNonce_Invalid memverifikasi nonce yang salah dinyatakan tidak valid
func TestValidateNonce_Invalid(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-correct")

    if m.ValidateNonce("service-a", "service-b", "nonce-wrong") {
        t.Error("expected wrong nonce to be invalid")
    }
}

// TestRevokeAndReissue_UpdatesNonce memverifikasi nonce lama tidak valid setelah revokasi
func TestRevokeAndReissue_UpdatesNonce(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-old")

    newNonce, err := m.Revoke("service-a", "service-b")
    if err != nil {
        t.Fatalf("Revoke() unexpected error: %v", err)
    }

    // Nonce lama harus tidak valid
    if m.ValidateNonce("service-a", "service-b", "nonce-old") {
        t.Error("old nonce should be invalid after revocation")
    }

    // Nonce baru harus valid
    if !m.ValidateNonce("service-a", "service-b", newNonce) {
        t.Errorf("new nonce %q should be valid", newNonce)
    }
}

// TestRevoke_UnknownPair memverifikasi revokasi pada pasangan tidak terdaftar menghasilkan error
func TestRevoke_UnknownPair(t *testing.T) {
    m := NewManager()
    _, err := m.Revoke("service-x", "service-y")
    if err == nil {
        t.Error("expected error when revoking unknown service pair")
    }
}

// TestRevocationLatency memverifikasi operasi revokasi selesai dalam batas waktu yang wajar
func TestRevocationLatency(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-init")

    start := time.Now()
    _, err := m.Revoke("service-a", "service-b")
    elapsed := time.Since(start)

    if err != nil {
        t.Fatalf("Revoke() failed: %v", err)
    }

    // Operasi revokasi lokal harus sangat cepat (jauh di bawah 1ms)
    if elapsed > 1*time.Millisecond {
        t.Errorf("Revoke() took %v, expected < 1ms", elapsed)
    }
}

// TestConcurrentRevocations memverifikasi thread-safety saat revokasi bersamaan
func TestConcurrentRevocations(t *testing.T) {
    m := NewManager()

    // Daftarkan 50 pasangan service
    for i := 0; i < 50; i++ {
        src := fmt.Sprintf("service-%d", i)
        m.RegisterNonce(src, "service-target", "nonce-init")
    }

    done := make(chan error, 50)
    for i := 0; i < 50; i++ {
        src := fmt.Sprintf("service-%d", i)
        go func(source string) {
            _, err := m.Revoke(source, "service-target")
            done <- err
        }(src)
    }

    for i := 0; i < 50; i++ {
        if err := <-done; err != nil {
            t.Errorf("concurrent Revoke() failed: %v", err)
        }
    }
}
```

---

## 8. Unit Test: WASM Filter Logic

Karena WASM filter berjalan di dalam Envoy, logika inti diisolasi ke paket `authz` dan `token` yang dapat diuji secara mandiri di luar runtime Envoy.

```go
// wasm-filter/token/validator_test.go
package token

import (
    "testing"
    "time"
)

// TestValidate_ValidToken memverifikasi token valid melewati semua pemeriksaan
func TestValidate_ValidToken(t *testing.T) {
    v := NewValidator("valid-nonce-registry")
    tok := &CachedToken{
        AllowedActions:  []string{"GET", "POST"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "valid-nonce",
    }
    v.RegisterNonce("valid-nonce")

    result := v.Validate(tok, "GET")
    if result != DecisionAllow {
        t.Errorf("expected Allow, got %v", result)
    }
}

// TestValidate_ExpiredToken memverifikasi token kedaluwarsa ditolak
func TestValidate_ExpiredToken(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},
        ExpiresAt:       time.Now().Add(-1 * time.Second).Unix(), // sudah expired
        RevocationNonce: "nonce-abc",
    }
    v.RegisterNonce("nonce-abc")

    result := v.Validate(tok, "GET")
    if result != DecisionExpired {
        t.Errorf("expected Expired, got %v", result)
    }
}

// TestValidate_RevokedNonce memverifikasi nonce yang dicabut menghasilkan Deny
func TestValidate_RevokedNonce(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "old-nonce", // nonce ini tidak terdaftar = sudah dicabut
    }
    // Tidak register nonce - simulasi setelah revokasi

    result := v.Validate(tok, "GET")
    if result != DecisionRevoked {
        t.Errorf("expected Revoked, got %v", result)
    }
}

// TestValidate_DisallowedAction memverifikasi action di luar allowed_actions ditolak
func TestValidate_DisallowedAction(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET"},           // hanya GET
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "nonce-ok",
    }
    v.RegisterNonce("nonce-ok")

    result := v.Validate(tok, "DELETE")             // coba DELETE
    if result != DecisionDeny {
        t.Errorf("expected Deny for disallowed action, got %v", result)
    }
}

// TestValidate_MultipleActions memverifikasi semua action yang diizinkan diterima
func TestValidate_MultipleActions(t *testing.T) {
    v := NewValidator("")
    tok := &CachedToken{
        AllowedActions:  []string{"GET", "POST", "PUT"},
        ExpiresAt:       time.Now().Add(30 * time.Second).Unix(),
        RevocationNonce: "nonce-multi",
    }
    v.RegisterNonce("nonce-multi")

    for _, action := range []string{"GET", "POST", "PUT"} {
        result := v.Validate(tok, action)
        if result != DecisionAllow {
            t.Errorf("expected Allow for action %s, got %v", action, result)
        }
    }
}
```

```go
// wasm-filter/authz/decision_test.go
package authz

import (
    "testing"
)

// TestDecisionEngine_CacheHit memverifikasi cache hit menghasilkan Allow tanpa ke PDP
func TestDecisionEngine_CacheHit(t *testing.T) {
    cache := NewMockLocalCache()
    cache.Set("service-a->service-b", validToken())

    engine := NewDecisionEngine(cache, nil) // nil = PDP tidak dipanggil
    engine.GetValidator().RegisterNonce("valid-nonce")
    result := engine.Decide("service-a", "service-b", "GET")

    if result.Decision != Allow {
        t.Errorf("expected cache hit Allow, got %v", result.Decision)
    }
    if !result.FromCache {
        t.Error("expected result to be from cache")
    }
}

// TestDecisionEngine_CacheMiss_FallbackToPDP memverifikasi cache miss meneruskan ke PDP
func TestDecisionEngine_CacheMiss_FallbackToPDP(t *testing.T) {
    cache  := NewMockLocalCache()         // cache kosong
    pdp    := NewMockPDP(true)            // PDP akan izinkan
    engine := NewDecisionEngine(cache, pdp)

    result := engine.Decide("service-a", "service-b", "GET")

    if result.Decision != Allow {
        t.Errorf("expected PDP fallback Allow, got %v", result.Decision)
    }
    if result.FromCache {
        t.Error("expected result NOT from cache (was a miss)")
    }
    if !pdp.WasCalled() {
        t.Error("expected PDP to be called on cache miss")
    }
}

// TestDecisionEngine_CacheMiss_PDPDeny memverifikasi PDP menolak diteruskan sebagai Deny
func TestDecisionEngine_CacheMiss_PDPDeny(t *testing.T) {
    cache  := NewMockLocalCache()
    pdp    := NewMockPDP(false)           // PDP akan tolak
    engine := NewDecisionEngine(cache, pdp)

    result := engine.Decide("service-x", "service-y", "DELETE")

    if result.Decision != Deny {
        t.Errorf("expected PDP Deny, got %v", result.Decision)
    }
}

// TestDecisionEngine_ExpiredCacheEntry memverifikasi entri cache kedaluwarsa dihapus dan ke PDP
func TestDecisionEngine_ExpiredCacheEntry(t *testing.T) {
    cache := NewMockLocalCache()
    cache.Set("service-a->service-b", expiredToken()) // token expired

    pdp    := NewMockPDP(true)
    engine := NewDecisionEngine(cache, pdp)

    result := engine.Decide("service-a", "service-b", "GET")

    if !pdp.WasCalled() {
        t.Error("expected PDP to be called after expired cache entry")
    }
    // Entri lama harus sudah dihapus dari cache
    if _, ok := cache.Get("service-a->service-b"); ok {
        t.Error("expected expired cache entry to be deleted")
    }
    _ = result
}
```

---

## 9. Integration Test

Integration test memverifikasi interaksi antar komponen secara end-to-end dalam lingkungan yang lebih nyata.

```go
// control-plane/graph-analyzer/main_test.go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

// TestIntegration_RecordAndRetrieveCandidates memverifikasi alur lengkap:
// record call → graph update → retrieve candidates
func TestIntegration_RecordAndRetrieveCandidates(t *testing.T) {
    svc := newAnalyzerService()
    server := httptest.NewServer(svc.routes())
    defer server.Close()

    // 1. Kirim 20 panggilan service-a → service-b
    for i := 0; i < 20; i++ {
        body, _ := json.Marshal(map[string]string{
            "source": "spiffe://cluster.local/ns/default/sa/service-a",
            "target": "spiffe://cluster.local/ns/default/sa/service-b",
        })
        resp, err := http.Post(server.URL+"/record", "application/json", bytes.NewReader(body))
        if err != nil || resp.StatusCode != 202 {
            t.Fatalf("record call failed: %v %v", err, resp.StatusCode)
        }
    }

    // 2. Query candidates
    resp, err := http.Get(server.URL + "/candidates")
    if err != nil {
        t.Fatalf("GET /candidates failed: %v", err)
    }
    defer resp.Body.Close()

    var candidates []CacheCandidateResponse
    json.NewDecoder(resp.Body).Decode(&candidates)

    if len(candidates) == 0 {
        t.Fatal("expected at least one candidate")
    }
    if candidates[0].SuggestedTTL <= 0 {
        t.Errorf("expected positive TTL, got %v", candidates[0].SuggestedTTL)
    }
}

// TestIntegration_LowFrequencyNotCandidate memverifikasi pasangan frekuensi rendah
// tidak masuk daftar kandidat cache
func TestIntegration_LowFrequencyNotCandidate(t *testing.T) {
    svc := newAnalyzerService()
    server := httptest.NewServer(svc.routes())
    defer server.Close()

    // Hanya 3 panggilan - di bawah threshold
    for i := 0; i < 3; i++ {
        body, _ := json.Marshal(map[string]string{
            "source": "spiffe://cluster.local/ns/default/sa/service-c",
            "target": "spiffe://cluster.local/ns/default/sa/service-d",
        })
        http.Post(server.URL+"/record", "application/json", bytes.NewReader(body))
    }

    resp, _ := http.Get(server.URL + "/candidates")
    var candidates []CacheCandidateResponse
    json.NewDecoder(resp.Body).Decode(&candidates)

    for _, c := range candidates {
        if c.Source == "spiffe://cluster.local/ns/default/sa/service-c" {
            t.Error("low-frequency pair should not appear as candidate")
        }
    }
}
```

### OPA Policy Test (Rego)

```rego
# control-plane/pdp/policies/authz_test.rego
package authz_test

import data.authz

# Test: service yang terdaftar diizinkan GET
test_allowed_service_get {
    authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "GET"
    }
}

# Test: service yang terdaftar diizinkan POST
test_allowed_service_post {
    authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/cartservice",
        "action": "POST"
    }
}

# Test: service tidak terdaftar ditolak
test_denied_unknown_source {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/unknown-service",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "GET"
    }
}

# Test: action tidak diizinkan ditolak
test_denied_disallowed_action {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "DELETE"   # hanya GET yang diizinkan untuk pasangan ini
    }
}

# Test: target tidak diizinkan ditolak
test_denied_wrong_target {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/paymentservice",
        "action": "GET"      # frontend tidak punya akses ke paymentservice
    }
}

# Test: default deny (tanpa input)
test_default_deny {
    not authz.allow with input as {}
}
```

Jalankan OPA test:
```bash
opa test control-plane/pdp/policies/ -v
```

---

## 10. Menjalankan Semua Test

### Makefile (shortcut)

```makefile
# Makefile

.PHONY: test test-graph test-distributor test-wasm test-opa lint coverage

## Jalankan semua unit test
test:
	@echo "Running graph-analyzer tests..."
	cd control-plane/graph-analyzer && go test ./... -v -count=1

	@echo "Running cache-distributor tests..."
	cd control-plane/cache-distributor && go test ./... -v -count=1

	@echo "Running wasm-filter tests..."
	cd wasm-filter && go test ./... -v -count=1

	@echo "Running OPA policy tests..."
	opa test control-plane/pdp/policies/ -v

## Test per komponen
test-graph:
	cd control-plane/graph-analyzer && go test ./... -v -run TestTopology

test-ttl:
	cd control-plane/graph-analyzer && go test ./cache/... -v -run TestCalculateTTL

test-distributor:
	cd control-plane/cache-distributor && go test ./... -v

test-wasm:
	cd wasm-filter && go test ./... -v

test-opa:
	opa test control-plane/pdp/policies/ -v

## Test dengan race detector (penting untuk concurrent tests)
test-race:
	cd control-plane/graph-analyzer && go test ./... -race -count=1
	cd control-plane/cache-distributor && go test ./... -race -count=1

## Coverage report
coverage:
	cd control-plane/graph-analyzer && \
	  go test ./... -coverprofile=coverage.out && \
	  go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: control-plane/graph-analyzer/coverage.html"

## Lint
lint:
	golangci-lint run ./control-plane/...
	golangci-lint run ./wasm-filter/...

## Build semua komponen
build:
	cd control-plane/graph-analyzer  && go build -o bin/graph-analyzer .
	cd control-plane/cache-distributor && go build -o bin/cache-distributor .
	cd wasm-filter && tinygo build -o authz-cache.wasm -scheduler=none -target=wasi .
```

### Perintah langsung

```bash
# Semua test sekaligus
make test

# Hanya graph analyzer
make test-graph

# Hanya TTL formula
make test-ttl

# Dengan race detector (wajib sebelum deploy)
make test-race

# Coverage report (buka di browser)
make coverage

# OPA policy test
make test-opa
```

### Target coverage minimal

| Modul | Coverage minimal |
|---|---|
| `graph/topology` | ≥ 90% |
| `cache/ttl` | ≥ 95% (formula kritis) |
| `token/delegation` | ≥ 90% |
| `token/signer` | ≥ 90% |
| `revocation/manager` | ≥ 85% |
| `wasm-filter/authz` | ≥ 85% |
| `wasm-filter/token` | ≥ 90% |

### Output test yang diharapkan

```
--- PASS: TestNewTopologyGraph (0.00s)
--- PASS: TestRecordCall_NewEdge (0.00s)
--- PASS: TestRecordCall_Increment (0.00s)
--- PASS: TestRecordCall_DirectionalIsolation (0.00s)
--- PASS: TestGetHighFrequencyPairs_AboveThreshold (0.00s)
--- PASS: TestGetHighFrequencyPairs_StaleEdgeExcluded (0.00s)
--- PASS: TestGetHighFrequencyPairs_Empty (0.00s)
--- PASS: TestRecordCall_Concurrent (0.02s)
--- PASS: TestCalculateTTL_NominalCase (0.00s)
--- PASS: TestCalculateTTL_HighFrequencyIncreaseTTL (0.00s)
--- PASS: TestCalculateTTL_HighSensitivityReducesTTL (0.00s)
--- PASS: TestCalculateTTL_NeverExceedsMaxTTL (0.00s)
--- PASS: TestCalculateTTL_NeverBelowMinTTL (0.00s)
--- PASS: TestCalculateTTL_CertExpiryReducesTTL (0.00s)
--- PASS: TestCalculateTTL_ZeroFrequency (0.00s)
--- PASS: TestSensitivityLevels_Ordering (0.00s)
PASS
coverage: 96.2% of statements
```
