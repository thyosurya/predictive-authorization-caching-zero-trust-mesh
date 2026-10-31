# Arsitektur Detail Per File Program
## ZTA Predictive Authorization Caching

Dokumen ini mendefinisikan **batasan tanggung jawab**, **interface publik**, **dependensi**, dan **alur data** setiap file program. Tujuannya: developer baru dapat langsung tahu di mana harus mulai dan di mana harus berhenti saat mengerjakan satu komponen.

---

## Peta Ketergantungan Antar File

```
┌─────────────────────────────────────────────────────────────────────┐
│ CONTROL PLANE - graph-analyzer                                      │
│                                                                     │
│  main.go                                                            │
│   ├── graph/topology.go      ← menyimpan & memperbarui graph        │
│   │    └── graph/centrality.go ← menghitung skor node              │
│   ├── cache/candidate.go     ← menyeleksi kandidat cache           │
│   │    └── cache/ttl.go      ← menghitung TTL tiap kandidat        │
│   └── metrics/prometheus.go  ← ekspos Prometheus metrics           │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ CONTROL PLANE - cache-distributor                                   │
│                                                                     │
│  main.go                                                            │
│   ├── token/delegation.go    ← struktur & konstruksi token         │
│   │    └── token/signer.go   ← signing & verifikasi HMAC           │
│   ├── revocation/manager.go  ← registry nonce & proses revokasi    │
│   │    └── revocation/broadcaster.go ← push ke sidecar via gRPC   │
│   └── metrics/prometheus.go  ← ekspos Prometheus metrics           │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ DATA PLANE - wasm-filter                                            │
│                                                                     │
│  main.go  (entry point Envoy WASM)                                  │
│   ├── authz/decision.go      ← mesin keputusan otorisasi           │
│   │    └── token/validator.go ← validasi token (exp/nonce/action)  │
│   └── cache/local_cache.go   ← baca/tulis Envoy shared data        │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ SERVICE LAYER - services/                                           │
│  service-a/main.go  }                                               │
│  service-b/main.go  } ← microservice bisnis sederhana (testbed)    │
│  service-c/main.go  }                                               │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ PDP - control-plane/pdp/policies/                                   │
│  authz.rego  ← sumber kebenaran kebijakan otorisasi (OPA)          │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Alur Data Utama (End-to-End)

```
Saat service-a memanggil service-b (normal, cache hit):

service-a
  │ HTTP request
  ▼
[Envoy Sidecar A] ← wasm-filter/main.go intercept
  │ cek cache
  ▼
cache/local_cache.go → ada entri "service-a→service-b"
  │ ambil token
  ▼
authz/decision.go → token/validator.go
  │ exp OK, nonce OK, action OK
  ▼
→ ActionContinue (request diteruskan)
  │
  ▼
service-b


Saat cache miss (pertama kali atau setelah TTL habis):

service-a
  │
[Envoy Sidecar A] - cache miss
  │ forward ke OPA
  ▼
OPA (pdp/policies/authz.rego) → allow = true
  │ HTTP 200
  ▼
request diteruskan ke service-b
  │
  │ (paralel, Envoy log panggilan)
  ▼
graph-analyzer/main.go ← POST /record
  │
graph/topology.go → tambah/update edge
  │
cache/candidate.go → jika melewati threshold
  │
cache/ttl.go → hitung TTL optimal
  │
cache-distributor/main.go ← kandidat baru
  │
token/delegation.go → buat DelegationToken
  │
token/signer.go → tanda tangani token
  │
revocation/broadcaster.go → push via gRPC ke sidecar A dan B
  │
wasm-filter/cache/local_cache.go → simpan token


Saat revokasi:

Admin → POST /revoke ke cache-distributor
  │
revocation/manager.go → update nonce, blacklist nonce lama
  │
revocation/broadcaster.go → kirim RevocationEvent ke semua sidecar
  │
[Sidecar] wasm-filter/token/validator.go → nonce tidak cocok
  │
→ cache entry dihapus, fallback ke OPA
```

---

## 1. GRAPH ANALYZER

---

### `graph-analyzer/main.go`

**Peran:** Entry point dan HTTP server Graph Analyzer. Mengorkestrasi semua sub-paket dan mengekspos endpoint REST.

**Batasan (yang TIDAK boleh dilakukan file ini):**
- Tidak boleh mengandung logika graph atau algoritma TTL secara langsung
- Tidak boleh berkomunikasi langsung dengan Cache Distributor - hanya menyediakan data via endpoint
- Tidak boleh menulis ke Prometheus secara langsung (delegasikan ke `metrics/`)

**Struct utama:**
```go
type AnalyzerService struct {
    topology   *graph.TopologyGraph      // graph topologi aktif
    selector   *cache.CandidateSelector  // seleksi kandidat
    metrics    *metrics.Collector        // prometheus metrics
    adminToken string                    // token untuk endpoint admin
}
```

**Fungsi publik yang diekspos (HTTP handlers):**
```
POST /record        → handleRecordCall(w, r)
GET  /candidates    → handleGetCandidates(w, r)
GET  /graph         → handleGetGraph(w, r)
DELETE /graph/reset → handleReset(w, r)   [butuh X-Admin-Token]
GET  /health        → handleHealth(w, r)
GET  /metrics       → promhttp.Handler()
```

**Lifecycle:**
```
main()
  → baca config dari ENV / file YAML
  → inisialisasi TopologyGraph
  → inisialisasi CandidateSelector
  → inisialisasi metrics collector
  → jalankan background goroutine: analysisLoop() setiap 30 detik
  → http.ListenAndServe(":8080")
```

**Dependensi:**
```
import (
    "graph"    // TopologyGraph
    "cache"    // CandidateSelector, CalculateTTL
    "metrics"  // prometheus collector
)
```

**ENV yang dibaca:**
```
ADMIN_TOKEN          string  wajib
MIN_CALLS_PER_MIN    int     default: 10
ANALYSIS_INTERVAL_S  int     default: 30
MAX_EDGES            int     default: 500
PORT                 string  default: "8080"
```

---

### `graph/topology.go`

**Peran:** Menyimpan dan memperbarui directed weighted graph yang merepresentasikan pola panggilan antar-service. Satu-satunya sumber kebenaran untuk data edge antar service.

**Batasan:**
- Tidak boleh melakukan kalkulasi TTL
- Tidak boleh tahu tentang token atau otorisasi
- Tidak boleh melakukan I/O apapun (tidak ada HTTP, tidak ada DB)
- Harus thread-safe untuk concurrent read/write

**Struct yang diekspor:**
```go
// ServiceEdge merepresentasikan satu arah panggilan antar dua service
type ServiceEdge struct {
    Source    string    // SPIFFE ID pemanggil
    Target    string    // SPIFFE ID yang dipanggil
    CallCount int64     // jumlah panggilan dalam sampling window
    LastSeen  time.Time // waktu panggilan terakhir
}

// TopologyGraph adalah container utama graph
type TopologyGraph struct {
    mu    sync.RWMutex
    edges map[string]*ServiceEdge  // key: "source->target"
    nodes map[string]int64         // SPIFFE ID → node ID gonum
    graph *simple.WeightedDirectedGraph
}
```

**Fungsi yang diekspor:**
```go
func NewTopologyGraph() *TopologyGraph
// Membuat graph kosong. Dipanggil sekali saat startup.

func (g *TopologyGraph) RecordCall(source, target string)
// Menambah/mengincrement edge source→target.
// Thread-safe. Dipanggil dari HTTP handler /record.

func (g *TopologyGraph) GetHighFrequencyPairs(minCallsPerMin int64) []*ServiceEdge
// Mengembalikan edge dengan CallCount >= threshold DAN LastSeen dalam 1 menit terakhir.
// Read-only. Dipanggil oleh CandidateSelector.

func (g *TopologyGraph) Snapshot() GraphSnapshot
// Mengembalikan salinan seluruh graph untuk endpoint /graph.
// Read-only.

func (g *TopologyGraph) Reset()
// Menghapus semua edge dan node. Dipanggil oleh handler /graph/reset.
// Thread-safe (write lock).

func (g *TopologyGraph) PruneStale(olderThan time.Duration)
// Menghapus edge yang LastSeen lebih lama dari threshold.
// Dipanggil secara periodik oleh analysisLoop di main.go.
```

**Yang TIDAK ada di file ini:** TTL, token, OPA, HTTP, Prometheus.

---

### `graph/centrality.go`

**Peran:** Menghitung skor betweenness centrality untuk setiap node dalam graph. Skor ini digunakan oleh CandidateSelector untuk memprioritaskan pasangan service mana yang paling penting untuk di-cache.

**Batasan:**
- Hanya menerima `*TopologyGraph` sebagai input
- Tidak boleh memodifikasi graph
- Murni komputasional - tidak ada I/O

**Fungsi yang diekspor:**
```go
func ComputeBetweenness(g *TopologyGraph) map[string]float64
// Menghitung betweenness centrality semua node.
// Return: map[SPIFFE_ID]skor (0.0–1.0, sudah dinormalisasi).
// Menggunakan gonum/graph/network.Betweenness().
// Kompleksitas: O(VE) - acceptable untuk graph kecil (< 50 service).

func RankEdgesByImportance(g *TopologyGraph, centrality map[string]float64) []*ServiceEdge
// Mengurutkan edge berdasarkan gabungan: CallCount + centrality sumber.
// Formula: score = 0.7 * normalized_call_count + 0.3 * centrality[source]
// Return: slice ServiceEdge terurut descending.
```

**Dipanggil oleh:** `cache/candidate.go` → `ComputeBetweenness()` → `RankEdgesByImportance()`

---

### `cache/ttl.go`

**Peran:** Satu-satunya file yang mengandung formula perhitungan TTL. Semua komponen lain yang butuh nilai TTL harus memanggilnya dari sini.

**Batasan:**
- Tidak boleh tahu tentang graph, token, atau HTTP
- Murni fungsi matematika - input dan output saja
- Harus deterministic: input yang sama selalu menghasilkan output yang sama

**Konstanta yang diekspor:**
```go
const (
    BaseTTL = 30 * time.Second
    MaxTTL  = 60 * time.Second
    MinTTL  = 5  * time.Second
)

type SensitivityLevel int
const (
    SensitivityLow    SensitivityLevel = iota  // penalty = 0.0
    SensitivityMedium                           // penalty = 0.3
    SensitivityHigh                             // penalty = 0.7
)
```

**Fungsi yang diekspor:**
```go
func CalculateTTL(
    callsPerMin    int64,
    sensitivity    SensitivityLevel,
    certRemaining  time.Duration,
    certTotal      time.Duration,
) time.Duration
// Formula:
//   freqWeight      = min(callsPerMin / 100.0, 1.0)
//   sensitivityPenalty = {0.0, 0.3, 0.7}
//   certFactor      = certRemaining / certTotal (max 1.0)
//   raw             = BaseTTL × freqWeight × (1 - penalty) × certFactor
//   result          = clamp(raw, MinTTL, MaxTTL)

func SensitivityFromString(s string) SensitivityLevel
// Konversi string "low"/"medium"/"high" → SensitivityLevel.
// Return SensitivityMedium jika tidak dikenal (safe default).
```

---

### `cache/candidate.go`

**Peran:** Menggabungkan output TopologyGraph, centrality score, dan TTL calculator untuk menghasilkan daftar kandidat cache yang siap dikirim ke Cache Distributor.

**Batasan:**
- Tidak boleh memodifikasi graph
- Tidak boleh membuat token - hanya menyiapkan data kandidat
- Tidak boleh melakukan HTTP call

**Struct yang diekspor:**
```go
type CacheCandidate struct {
    Source          string
    Target          string
    AllowedActions  []string
    SuggestedTTLMs  int64
    Sensitivity     SensitivityLevel
    CentralityScore float64
    CallsPerMin     int64
}

type CandidateSelector struct {
    minCallsPerMin  int64
    maxCandidates   int
}
```

**Fungsi yang diekspor:**
```go
func NewCandidateSelector(minCallsPerMin int64, maxCandidates int) *CandidateSelector

func (s *CandidateSelector) Select(
    g           *graph.TopologyGraph,
    centrality  map[string]float64,
    policyFn    func(source, target string) ([]string, SensitivityLevel),
) []*CacheCandidate
// Alur internal:
//   1. GetHighFrequencyPairs(minCallsPerMin)
//   2. RankEdgesByImportance(centrality)
//   3. Untuk tiap edge teratas: panggil policyFn untuk dapat actions & sensitivity
//   4. CalculateTTL per kandidat
//   5. Return slice CacheCandidate, max maxCandidates entri
//
// policyFn adalah fungsi callback - dipasok dari main.go yang memanggil OPA
// untuk mendapat allowed_actions per pasangan. Ini memisahkan logika policy
// dari logika seleksi.
```

---

### `metrics/prometheus.go` (graph-analyzer)

**Peran:** Mendefinisikan dan mendaftarkan semua Prometheus metrics untuk Graph Analyzer.

**Batasan:**
- Tidak boleh mengandung logika bisnis
- Hanya boleh dipanggil dari main.go dan handler
- Semua metric harus didaftarkan dalam fungsi `init()` atau konstruktor

**Metrics yang didefinisikan:**
```go
var (
    EdgesTotal           prometheus.Gauge      // zta_graph_edges_total
    CallsRecordedTotal   *prometheus.CounterVec // zta_graph_calls_recorded_total {source,target}
    CandidatesSelected   prometheus.Counter     // zta_graph_candidates_selected_total
    AnalysisDurationMs   prometheus.Histogram   // zta_graph_analysis_duration_ms
)

type Collector struct {
    // wraps semua metric di atas
}

func NewCollector() *Collector
func (c *Collector) RecordCall(source, target string)
func (c *Collector) SetEdgeCount(n int)
func (c *Collector) ObserveAnalysisDuration(ms float64)
```

---

## 2. CACHE DISTRIBUTOR

---

### `cache-distributor/main.go`

**Peran:** Entry point Cache Distributor. Mengorkestrasi pembuatan token, gRPC server untuk sidecar, dan REST API untuk admin/monitoring.

**Batasan:**
- Tidak boleh mengandung logika signing atau validasi token secara inline
- Tidak boleh tahu tentang struktur graph
- Tidak boleh menyimpan state otorisasi - itu tugas OPA

**Struct utama:**
```go
type DistributorService struct {
    signer      *token.Signer
    revoker     *revocation.Manager
    broadcaster *revocation.Broadcaster
    metrics     *metrics.Collector
    opaPDPURL   string
    signingKey  []byte
}
```

**Endpoint REST:**
```
POST /distribute       → handleDistribute(w, r)
POST /revoke           → handleRevoke(w, r)
GET  /tokens/active    → handleActiveTokens(w, r)
GET  /health           → handleHealth(w, r)
GET  /metrics          → promhttp.Handler()
```

**gRPC server (port 50051):**
```
Implements proto/cache.proto CacheDistributorServer:
  PushToken(TokenPayload) → Ack
  RevokeToken(RevocationRequest) → Ack
  StreamRevocations(SubscribeRequest) → stream RevocationEvent
```

**Lifecycle:**
```
main()
  → baca config dari ENV
  → inisialisasi Signer(signingKey)
  → inisialisasi RevocationManager
  → inisialisasi Broadcaster
  → jalankan gRPC server di goroutine terpisah
  → jalankan background goroutine: distributeLoop() setiap 30 detik
      → query graph-analyzer /candidates
      → untuk tiap kandidat: buat token, push ke sidecar
  → http.ListenAndServe(":8081")
```

**ENV yang dibaca:**
```
SIGNING_KEY              string  wajib (base64, 32 bytes)
GRAPH_ANALYZER_URL       string  default: "http://graph-analyzer:8080"
OPA_URL                  string  default: "http://opa-service:8181"
GRPC_PORT                string  default: "50051"
REST_PORT                string  default: "8081"
DISTRIBUTION_INTERVAL_S  int     default: 30
```

---

### `token/delegation.go`

**Peran:** Mendefinisikan struktur DelegationToken dan menyediakan konstruktor yang memvalidasi semua field wajib.

**Batasan:**
- Tidak boleh melakukan signing - itu tugas `signer.go`
- Tidak boleh melakukan HTTP atau I/O
- Hanya bertanggung jawab atas struktur data dan validasi input

**Struct yang diekspor:**
```go
type DelegationToken struct {
    Issuer          string   `json:"iss"`
    SourceIdentity  string   `json:"sub"`
    TargetIdentity  string   `json:"aud"`
    AllowedActions  []string `json:"allowed_actions"`
    Sensitivity     string   `json:"sensitivity"`
    ExpiresAt       int64    `json:"exp"`
    IssuedAt        int64    `json:"iat"`
    RevocationNonce string   `json:"revocation_nonce"`
    CacheTTLMs      int64    `json:"cache_ttl_ms"`
}
```

**Fungsi yang diekspor:**
```go
func NewDelegationToken(
    source, target  string,
    actions         []string,
    sensitivity     string,
    ttl             time.Duration,
    nonce           string,
) (*DelegationToken, error)
// Validasi:
//   - source & target tidak boleh kosong
//   - actions tidak boleh kosong
//   - ttl tidak boleh <= 0
//   - nonce tidak boleh kosong
// Mengisi IssuedAt = now, ExpiresAt = now + ttl
// Mengisi Issuer = "pdp-central.spiffe.io" (konstan)

func (t *DelegationToken) ToJSON() ([]byte, error)
// Serialize ke JSON. Digunakan sebelum signing.

func (t *DelegationToken) IsExpired() bool
// Return true jika ExpiresAt < time.Now().Unix()
```

---

### `token/signer.go`

**Peran:** Menandatangani payload token dengan HMAC-SHA256 dan memverifikasi tanda tangan. Ini adalah satu-satunya file yang menyentuh kunci kriptografis.

**Batasan:**
- Tidak boleh menyimpan atau memodifikasi struktur token
- Tidak boleh tahu tentang field token - hanya menerima `[]byte` payload
- Kunci tidak boleh pernah di-log atau dikembalikan via API

**Struct yang diekspor:**
```go
type Signer struct {
    key []byte  // private - tidak pernah diekspor
}
```

**Fungsi yang diekspor:**
```go
func NewSigner(key []byte) (*Signer, error)
// Validasi: key harus minimal 32 byte.
// Return error jika key terlalu pendek.

func (s *Signer) Sign(payload []byte) (string, error)
// Hitung HMAC-SHA256(key, payload).
// Return base64url-encoded signature.
// Return error jika payload kosong.

func (s *Signer) Verify(payload []byte, signature string) error
// Decode signature dari base64url.
// Hitung HMAC ulang dan bandingkan secara constant-time (hmac.Equal).
// Return nil jika valid, error jika tidak cocok.
// Error tidak boleh mengungkap key atau payload.
```

**Catatan keamanan:** `Verify()` wajib menggunakan `hmac.Equal()` bukan `==` untuk mencegah timing attack.

---

### `revocation/manager.go`

**Peran:** Registry pusat untuk nonce aktif semua delegation token. Satu-satunya komponen yang berwenang mengubah state nonce.

**Batasan:**
- Tidak boleh mem-push ke sidecar - itu tugas `broadcaster.go`
- Tidak boleh membuat token baru
- Harus thread-safe untuk semua operasi
- Nonce lama harus masuk blacklist, bukan langsung dihapus

**Struct yang diekspor:**
```go
type NonceEntry struct {
    Nonce     string
    IssuedAt  time.Time
    RevokedAt *time.Time  // nil jika masih aktif
}

type Manager struct {
    mu        sync.RWMutex
    active    map[string]*NonceEntry  // key: "source->target"
    blacklist map[string]time.Time    // nonce → waktu revokasi
}
```

**Fungsi yang diekspor:**
```go
func NewManager() *Manager

func (m *Manager) RegisterNonce(source, target, nonce string) error
// Daftarkan nonce baru untuk pasangan service.
// Error jika source/target/nonce kosong.

func (m *Manager) ValidateNonce(source, target, nonce string) bool
// Return true HANYA jika:
//   1. Ada entri aktif untuk pasangan ini
//   2. Nonce cocok dengan entri aktif
//   3. Nonce tidak ada dalam blacklist
// Thread-safe (read lock).

func (m *Manager) Revoke(source, target string) (newNonce string, err error)
// Alur:
//   1. Cek pasangan ada dalam active map
//   2. Masukkan nonce lama ke blacklist dengan timestamp sekarang
//   3. Generate nonce baru (GenerateNonce())
//   4. Update active map dengan nonce baru
//   5. Return nonce baru
// Thread-safe (write lock - blokir semua read saat revokasi).

func (m *Manager) PruneBlacklist(olderThan time.Duration)
// Hapus entri blacklist yang sudah lebih tua dari threshold.
// Dipanggil periodik dari main.go. Threshold: 2 × MaxTTL = 120 detik.

func GenerateNonce() (string, error)
// Generate 8 byte random, encode hex. Return 16-char string.
// Fungsi level paket - bisa dipanggil dari luar manager.
```

---

### `revocation/broadcaster.go`

**Peran:** Mengelola koneksi gRPC stream ke semua sidecar dan mem-push RevocationEvent saat ada pencabutan.

**Batasan:**
- Tidak boleh membuat keputusan revokasi - hanya mengirimkan event yang sudah dibuat Manager
- Tidak boleh menyimpan state nonce
- Harus handle reconnect dari sisi sidecar dengan graceful

**Struct yang diekspor:**
```go
type StreamClient struct {
    sidecarID  string
    stream     cache.CacheDistributor_StreamRevocationsServer
    done       chan struct{}
}

type Broadcaster struct {
    mu      sync.RWMutex
    clients map[string]*StreamClient  // sidecarID → stream
}
```

**Fungsi yang diekspor:**
```go
func NewBroadcaster() *Broadcaster

func (b *Broadcaster) Register(sidecarID string, stream cache.CacheDistributor_StreamRevocationsServer)
// Daftarkan koneksi stream baru dari sidecar.
// Jika sidecarID sudah ada, ganti dengan koneksi baru (reconnect).

func (b *Broadcaster) Unregister(sidecarID string)
// Hapus koneksi saat stream terputus.

func (b *Broadcaster) Broadcast(event *cache.RevocationEvent) error
// Kirim event ke SEMUA sidecar yang terdaftar.
// Lanjutkan ke sidecar berikutnya meski satu gagal (best-effort).
// Log sidecar mana yang gagal menerima.
// Return error hanya jika SEMUA gagal.

func (b *Broadcaster) ConnectedCount() int
// Return jumlah sidecar yang sedang terkoneksi.
// Digunakan oleh /health endpoint.
```

**Pola penggunaan di main.go:**
```go
// Saat RevocationManager.Revoke() berhasil:
newNonce, _ := revoker.Revoke(source, target)
broadcaster.Broadcast(&cache.RevocationEvent{
    SourceIdentity: source,
    TargetIdentity: target,
    NewNonce:       newNonce,
    Timestamp:      time.Now().UnixMilli(),
})
```

---

## 3. WASM FILTER

---

### `wasm-filter/main.go`

**Peran:** Entry point Envoy WASM filter. Menginisialisasi plugin dan menghubungkan Envoy HTTP lifecycle (OnHttpRequestHeaders) ke DecisionEngine.

**Batasan:**
- Tidak boleh mengandung logika otorisasi
- Tidak boleh mengandung logika cache
- Hanya boleh mendelegasikan ke `authz/decision.go`
- Harus menggunakan proxy-wasm-go-sdk API

**Fungsi yang diimplementasikan (proxy-wasm interface):**
```go
func main()
// Set VMContext. Entry point WASM - dipanggil saat module dimuat Envoy.

type vmContext struct{
    types.DefaultVMContext
}
func (*vmContext) OnVMStart(vmConfigurationSize int) types.OnVMStartStatus
// Inisialisasi VM. Log info. Return OK.

type pluginContext struct {
    types.DefaultPluginContext
    engine *authz.DecisionEngine
    cache  *localcache.LocalCache
}
func (*pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus
// 1. Inisialisasi LocalCache (kapasitas 1000 entri).
// 2. Inisialisasi DecisionEngine(cache, nil).
// 3. Seed cache token awal (seedInitialTokens) untuk pasangan microservice kritis.
// 4. Set timer periodic polling (SetTickPeriodMilliSeconds: 2000 ms).
// Return OK.

func (p *pluginContext) OnTick()
// Dipanggil setiap 2 detik oleh Envoy runtime:
// Melakukan async HTTP callout (proxywasm.DispatchHttpCall) ke:
// GET http://cache-distributor.zta-research.svc.cluster.local:8081/tokens/active

func (p *pluginContext) onTokenResponse(numHeaders, bodySize, numTrailers int)
// Callback handler saat respons token diterima dari Cache Distributor:
// 1. Baca payload JSON (array of active tokens).
// 2. Daftarkan setiap revocation_nonce ke Validator (RegisterNonce).
// 3. Simpan token ke LocalCache dengan key "source->target".

func (*pluginContext) NewHttpContext(contextID uint32) types.HttpContext
// Buat httpContext baru per request.

type httpContext struct {
    types.DefaultHttpContext
    contextID uint32
    engine    *authz.DecisionEngine
}
func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action
// Ambil header x-forwarded-client-cert (SPIFFE ID source)
// Ambil header :authority (target)
// Ambil header :method
// Panggil engine.Decide(source, target, method)
// Jika Allow -> return ActionContinue (request diteruskan)
// Jika Deny/Revoked/Expired -> SendHttpResponse(403) + return ActionPause
```

**Alur kontrol:**
```
Envoy intercept request
  -> OnHttpRequestHeaders()
  -> RemoveHttpRequestHeader("x-zta-cache-verified") [Sanitasi anti-spoofing]
  -> extractSPIFFEID(rawSource) [Validasi skema spiffe:// & tolak delimiter |, ->]
  -> normalizeTargetIdentity(rawTarget) [Pemisahan port :3550 & domain K8s]
  -> engine.Decide(sourceID, targetID, method)
      -> local_cache.Get(key) -> O(1) *token.CachedToken dereference (zero-deserialization)
          hit  -> validator.Validate(tok, method)
          miss -> ActionContinue tanpa marker (diteruskan ke ext_authz OPA)
  -> Allow   -> AddHttpRequestHeader("x-zta-cache-verified", "true") -> ActionContinue (bypass OPA)
  -> Miss    -> ActionContinue tanpa header verified (Otorisasi dieksekusi ext_authz OPA)
  -> Expired -> ActionContinue tanpa header verified (Otorisasi dieksekusi ext_authz OPA)
  -> Revoked -> SendHttpResponse(403, "Forbidden: token revoked") -> ActionPause
  -> Deny    -> SendHttpResponse(403, "Forbidden") -> ActionPause
```

---

### `wasm-filter/authz/decision.go`

**Peran:** Mesin keputusan otorisasi utama di dalam sidecar. Memutuskan apakah sebuah request diizinkan berdasarkan local cache secara murni *zero-deserialization* tanpa alokasi heap berulang pada *hot-path*.

**Batasan:**
- Tidak boleh melakukan I/O jaringan (WASM sandbox)
- Tidak boleh memodifikasi cache saat evaluasi Allow (hanya membaca pointer struct)
- Penghapusan cache entri yang expired adalah satu-satunya mutasi yang diizinkan pada jalur keputusan
- Tidak boleh tahu tentang implementasi transport gRPC atau PDP

**Type yang diekspor:**
```go
type Decision int
const (
    Allow   Decision = iota  // token valid, akses diizinkan
    Deny                     // token ada tapi action tidak diizinkan
    Expired                  // token ada tapi sudah expired
    Revoked                  // nonce tidak cocok atau dicabut
    Miss                     // tidak ada token di cache (fallback ke PDP)
)

type DecisionResult struct {
    Decision  Decision
    FromCache bool
    Reason    string  // untuk logging debug
}
```

**Struct yang diekspor:**
```go
// Cache adalah interface untuk operasi cache lokal pointer struct
type Cache interface {
    Get(key string) (*token.CachedToken, bool)
    Set(key string, tok *token.CachedToken) error
    Delete(key string) error
    Keys() []string
}

// PDPClient adalah interface untuk Policy Decision Point fallback
type PDPClient interface {
    Evaluate(source, target, action string) bool
}

type DecisionEngine struct {
    cache     Cache
    validator *token.Validator
    pdp       PDPClient // fallback ke PDP saat cache miss (bisa nil)
}
```

**Fungsi yang diekspor:**
```go
func NewDecisionEngine(c Cache, pdp PDPClient) *DecisionEngine
// pdp bisa nil jika fallback ditangani secara transparan via ext_authz Envoy.

func (e *DecisionEngine) GetValidator() *token.Validator
// Mengembalikan validator yang digunakan oleh engine.

func (e *DecisionEngine) GetCache() Cache
// Mengembalikan interface cache lokal engine.

func (e *DecisionEngine) Decide(source, target, action string) DecisionResult
// Alur:
//   1. Buat cache key: source + "->" + target
//   2. cache.Get(key) -> *token.CachedToken pointer langsung (O(1), zero-deserialization)
//   3. Jika miss -> fallback ke PDP jika tersedia, atau return Miss
//   4. Jika ada -> validator.Validate(tok, action)
//   5. Jika Expired -> cache.Delete(key), fallback ke PDP atau return Expired
//   6. Jika Revoked -> cache.Delete(key), return Revoked
//   7. Jika Deny -> return Deny
//   8. Jika Allow -> return Allow
```

---

### `wasm-filter/token/validator.go`

**Peran:** Memvalidasi delegation token dari empat aspek: masa berlaku (exp), integritas tanda tangan kriptografis HMAC-SHA256, registri nonce aktif, dan action yang diizinkan.

**Batasan:**
- Tidak boleh memodifikasi cache
- Tidak boleh melakukan I/O sinkronus di data-plane
- Validasi harus dilakukan dalam urutan tetap: exp -> nonce -> action

**Struct yang diekspor:**
```go
type CachedToken struct {
    AllowedActions  []string `json:"allowed_actions"`
    ExpiresAt       int64    `json:"exp"`
    RevocationNonce string   `json:"revocation_nonce"`
    Signature       string   `json:"signature"`
}

type Validator struct {
    mu           sync.RWMutex
    activeNonces map[string]bool  // nonce yang masih valid
}
```

**Fungsi yang diekspor:**
```go
func NewValidator() *Validator

func (v *Validator) RegisterNonce(nonce string)
// Daftarkan nonce baru ke activeNonces.

func (v *Validator) InvalidateNonce(nonce string)
// Cabut nonce dari registry secara instan.

func (v *Validator) SyncActiveNonces(validNonces map[string]bool)
// Ganti registry nonce aktif secara atomik dengan daftar terbaru dari distributor,
// mencegah memory leak dan mencabut nonce lama yang sudah kedaluwarsa.

func (v *Validator) Validate(tok *CachedToken, action string) authz.Decision
// Urutan validasi (TIDAK BOLEH diubah):
//   1. Cek exp: jika time.Now().Unix() > tok.ExpiresAt -> return Expired
//   2. Cek nonce: jika tok.RevocationNonce tidak ada di activeNonces -> return Revoked
//   3. Cek action: jika action tidak ada di tok.AllowedActions -> return Deny
//   4. Semua valid -> return Allow

func CanonicalPayload(source, target string, actions []string, exp int64, nonce string) string
// Menghasilkan representasi string kanonikal deterministik untuk signing dan verification.

func VerifySignature(payload, signature string, key []byte) bool
// Memverifikasi tanda tangan HMAC-SHA256 dengan constant-time comparison (anti timing-attack).
```

---

### `wasm-filter/cache/local_cache.go`

**Peran:** In-memory, thread-isolated local cache per Envoy worker thread VM untuk menyimpan pointer token terverifikasi (`*token.CachedToken`). Mengeliminasi locking contention antar-thread dan overhead Garbage Collection.

**Batasan:**
- Kapasitas maksimal ditegakkan secara ketat (`maxEntries = 1000`)
- Mengembalikan `ErrCacheFull` jika kapasitas terlampaui
- Mendukung pembersihan proaktif stale keys via `Keys()` sebelum penyisipan entri baru (*stale-first eviction*)

**Struct yang diekspor:**
```go
var ErrCacheFull = errors.New("cache is full")

type LocalCache struct {
    mu         sync.RWMutex
    entries    map[string]*token.CachedToken
    maxEntries int
}
```

**Fungsi yang diekspor:**
```go
func NewLocalCache(maxEntries int) *LocalCache
// Inisialisasi cache lokal dengan batas kapasitas terdefinisi (default 1000).

func (c *LocalCache) Get(key string) (*token.CachedToken, bool)
// Mengambil pointer struct CachedToken secara langsung O(1).
// Key format: "source_spiffe_id->target_spiffe_id"

func (c *LocalCache) Set(key string, tok *token.CachedToken) error
// Menyimpan pointer struct token. Jika len(entries) >= maxEntries, kembalikan ErrCacheFull.

func (c *LocalCache) SetBytes(key string, raw []byte) error
// Helper parsing dan penyimpanan untuk kompatibilitas data mentah.

func (c *LocalCache) Delete(key string) error
// Menghapus entri key secara aman dari memori.

func (c *LocalCache) Keys() []string
// Mengembalikan daftar seluruh key aktif untuk mendukung urutan stale-first eviction.

func (c *LocalCache) MakeKey(source, target string) string
// Return: source + "->" + target
```
```

---

## 4. SERVICE LAYER

---

### `services/service-a/main.go` (Frontend API Gateway / Reverse Proxy)

**Peran:** Bertindak sebagai *API Gateway / Frontend* yang menerima request eksternal (dari load generator k6) dan melakukan reverse proxy ke microservice hilir (East-West traffic). Request diteruskan melalui Envoy sidecar `frontend` menuju `productcatalogservice` dan `cartservice`, sehingga mengaktifkan jalur otorisasi ZTA antar-service.

**Batasan:**
- Tidak boleh mengevaluasi otorisasi lokal: seluruh keputusan keamanan didelegasikan ke Envoy sidecar
- Harus meneruskan header konteks (`x-forwarded-client-cert`, trace ID, dll.) ke upstream service
- Menggunakan HTTP connection pooling (`MaxIdleConns: 1000`, `MaxIdleConnsPerHost: 500`) agar latensi proxy tidak mendistorsi pengukuran

**Endpoint yang diekspos:**
```
GET  /api/products  -> Reverse proxy ke http://productcatalogservice.../api/products
POST /api/cart      -> Reverse proxy ke http://cartservice.../api/cart
GET  /health        -> Health check lokal
GET  /metrics       -> Prometheus metrics (request count)
```

**ENV yang dibaca:**
```
SERVICE_NAME        string  default: "service-a"
PORT                string  default: "8080"
PRODUCT_CATALOG_URL string  default: "http://productcatalogservice.zta-research.svc.cluster.local:8080"
CART_SERVICE_URL    string  default: "http://cartservice.zta-research.svc.cluster.local:8080"
SIMULATED_LATENCY_MS int    default: 0
```

---

### `services/service-b/main.go` & `services/service-c/main.go` (Backend Microservices)

**Peran:** Microservice bisnis sederhana yang menjadi target panggilan East-West. Dilindungi oleh Envoy sidecar dengan policy otorisasi `authz-policy-zta.yaml` dan filter WASM `wasm-filter.yaml`.

**Batasan:**
- Tidak mengandung logika otorisasi di dalam kode aplikasi (semua diintersep oleh Envoy sidecar sebelum mencapai handler Go)
- Return JSON statis dengan komputasi minimal agar tidak menjadi bottleneck beban

**Endpoint yang diekspos:**
```
GET  /api/products  -> Response list produk statis (Product Catalog)
GET  /api/cart      -> Response status keranjang belanja (Cart Service)
POST /api/cart      -> Response penambahan item keranjang (Cart Service, HTTP 201 Created)
GET  /health        -> Health check
GET  /metrics       -> Prometheus metrics
```

**ENV yang dibaca:**
```
SERVICE_NAME         string  wajib (misal: "productcatalogservice" atau "cartservice")
PORT                 string  default: "8080"
SIMULATED_LATENCY_MS int     default: 0
```

---

## 5. PDP - OPA POLICY

---

### `pdp/policies/authz.rego`

**Peran:** Sumber kebenaran tunggal untuk seluruh kebijakan otorisasi. Mengevaluasi apakah `source_identity` boleh memanggil `target_identity` dengan `action` tertentu.

**Batasan:**
- Tidak boleh memanggil service eksternal dalam policy evaluation
- Data rules harus dipisahkan ke `data.json` - policy dan data adalah dua hal berbeda
- Default harus selalu `deny` - eksplisit lebih aman dari implisit

**Input yang diharapkan (per evaluasi):**
```json
{
  "input": {
    "source_identity": "spiffe://cluster.local/ns/zta-research/sa/<name>",
    "target_identity": "spiffe://cluster.local/ns/zta-research/sa/<name>",
    "action": "GET|POST|PUT|DELETE|PATCH"
  }
}
```

**Data eksternal (`data.json`):**
```json
{
  "service_rules": [
    {
      "source": "spiffe://...",
      "target": "spiffe://...",
      "allowed_actions": ["GET"],
      "sensitivity": "low|medium|high"
    }
  ]
}
```

**Policy rules:**
```rego
package authz

import future.keywords.if
import future.keywords.in

# Default deny - tidak ada allow tanpa aturan eksplisit
default allow := false

# Allow jika ada rule yang cocok
allow if {
    some rule in data.service_rules
    rule.source == input.source_identity
    rule.target == input.target_identity
    input.action in rule.allowed_actions
}

# Sensitivity level untuk kandidat cache
# (digunakan Cache Distributor saat query OPA untuk dapat sensitivity)
sensitivity := level if {
    some rule in data.service_rules
    rule.source == input.source_identity
    rule.target == input.target_identity
    level := rule.sensitivity
}

default sensitivity := "medium"
```

**Cara Cache Distributor menggunakan policy ini:**
```
1. Query /v1/data/authz/allow → dapat boolean
2. Query /v1/data/authz/sensitivity → dapat "low"/"medium"/"high"
3. Gabungkan untuk membangun DelegationToken
```

---

## Ringkasan Tanggung Jawab

| File | Tanggung Jawab | Tidak Boleh |
|---|---|---|
| `graph-analyzer/main.go` | Orkestrasi + HTTP server | Logika graph, TTL |
| `graph/topology.go` | Menyimpan edge + RecordCall | I/O, token, TTL |
| `graph/centrality.go` | Hitung centrality score | Memodifikasi graph |
| `cache/ttl.go` | Formula TTL | I/O, graph, token |
| `cache/candidate.go` | Seleksi kandidat | Buat token, HTTP |
| `metrics/prometheus.go` (analyzer) | Definisi metrics | Logika bisnis |
| `cache-distributor/main.go` | Orkestrasi + HTTP + gRPC server | Logika signing |
| `token/delegation.go` | Struktur + validasi token | Signing, I/O |
| `token/signer.go` | Sign + verify HMAC | Menyimpan token |
| `revocation/manager.go` | Registry nonce + revokasi | Push ke sidecar |
| `revocation/broadcaster.go` | Push event via gRPC stream | Keputusan revokasi |
| `wasm-filter/main.go` | Envoy lifecycle hooks | Logika otorisasi |
| `authz/decision.go` | Mesin keputusan | I/O, memodifikasi cache |
| `token/validator.go` | Validasi exp/nonce/action | Memodifikasi cache |
| `cache/local_cache.go` | CRUD Envoy shared data | Logika validasi |
| `services/*/main.go` | HTTP handler bisnis | Logika otorisasi |
| `pdp/policies/authz.rego` | Kebijakan otorisasi | Memanggil service luar |
