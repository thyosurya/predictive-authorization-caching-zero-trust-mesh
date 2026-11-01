# State Persistence Specification
## ZTA Predictive Authorization Caching - Desain State & In-Memory vs Persistent

Dokumen ini mendefinisikan keputusan desain tentang persistensi state, apa yang boleh hilang saat pod restart, dan apa yang tidak.

---

## Daftar Isi
1. [Keputusan Desain: In-Memory by Design](#1-keputusan-desain-in-memory-by-design)
2. [State Inventory per Komponen](#2-state-inventory-per-komponen)
3. [Behavior saat Pod Restart](#3-behavior-saat-pod-restart)
4. [State Schema (In-Memory)](#4-state-schema-in-memory)
5. [Jika Persistent Dibutuhkan (Ekstensi Masa Depan)](#5-jika-persistent-dibutuhkan-ekstensi-masa-depan)

---

## 1. Keputusan Desain: In-Memory by Design

Seluruh state pada Graph Analyzer dan Cache Distributor **sengaja disimpan in-memory** tanpa persistensi ke disk atau database. Ini adalah keputusan desain yang disengaja, bukan keterbatasan.

**Alasan:**

**Konsistensi dengan ZTA invariant:** Zero Trust tidak mempercayai state yang tersimpan lama. Token otorisasi bersifat time-bound dan harus diperbarui secara aktif. State lama yang di-load dari disk berpotensi menjadi stale dan melanggar prinsip "never trust, always verify".

**Recovery time cepat dan aman:** Saat pod restart, Graph Analyzer memulai dengan graph kosong dan membangun ulang topologi dalam 1–2 menit dari traffic aktif. Ini lebih aman daripada me-load state lama yang mungkin sudah tidak merepresentasikan kondisi mesh terkini.

**Kesederhanaan eksperimen:** Untuk testbed penelitian, state yang bisa di-reset sempurna antar skenario sangat dibutuhkan. Database persisten akan mempersulit prosedur reset.

**Konsekuensi yang diterima:**
- Setelah pod restart, ada window 1–2 menit di mana cache hit ratio rendah (graph belum terisi)
- Semua token aktif hilang dan harus di-distribusikan ulang
- Ini **dapat diterima** karena fallback ke PDP tetap berjalan - tidak ada request yang ditolak salah

---

## 2. State Inventory per Komponen

### Graph Analyzer

| State | Tipe | Ukuran estimasi | Hilang saat restart? | Dampak |
|---|---|---|---|---|
| `TopologyGraph.edges` | `map[string]*ServiceEdge` | ~500 entry × 200 byte = ~100 KB | **Ya** | Cache hit ratio turun sementara |
| `TopologyGraph.nodes` | `map[string]int64` | ~50 node × 100 byte = ~5 KB | **Ya** | Tidak ada dampak fungsional |
| Prometheus counters | In-process | Minimal | **Ya** | Hilang dari Prometheus (gap dalam time-series) |

### Cache Distributor

| State | Tipe | Ukuran estimasi | Hilang saat restart? | Dampak |
|---|---|---|---|---|
| `RevocationManager.active` | `map[string]*NonceEntry` | ~100 entry × 150 byte = ~15 KB | **Ya** | Sidecar kehilangan referensi nonce - fallback ke PDP |
| `RevocationManager.blacklist` | `map[string]time.Time` | ~200 entry × 50 byte = ~10 KB | **Ya** | Nonce lama tidak lagi di-blacklist - window kecil vulnerability (< MaxTTL = 30 detik) |
| `Broadcaster.clients` | `map[string]*StreamClient` | ~10 entry | **Ya** | Sidecar reconnect otomatis dalam < 5 detik |
| Token inventory (untuk /tokens/active) | `map[string]*ActiveTokenSummary` | ~100 entry × 300 byte = ~30 KB | **Ya** | Endpoint monitoring menunjukkan 0 sementara |

### Envoy Sidecar (WASM shared data)

| State | Tipe | Hilang saat restart? | Dampak |
|---|---|---|---|
| Local cache (delegation tokens) | Envoy shared memory | **Ya (saat Envoy restart)** | Cache miss sampai token baru di-push |
| Nonce registry (validator) | WASM in-process memory | **Ya** | Semua request fallback ke PDP |

---

## 3. Behavior saat Pod Restart

### Graph Analyzer restart

```
T=0   Pod restart
T=0   graph = {} (kosong)
T=0   TopologyGraph.edges = empty map
T=5s  Pod Running, menerima /record calls
T=30s analysis loop jalan pertama - graph masih sedikit edge
T=60s mulai ada kandidat jika traffic cukup
T=90s cache hit ratio kembali normal (graph terisi dari traffic aktual)
```

**Yang terjadi pada Cache Distributor:** Cache Distributor polling `/candidates` setiap 30 detik. Selama graph kosong, tidak ada kandidat baru yang didistribusikan - tapi token yang sudah ada di sidecar tetap berlaku sampai expired.

### Cache Distributor restart

```
T=0   Pod restart
T=0   active nonces = {} (kosong)
T=0   broadcaster clients = {} (kosong)
T=5s  Pod Running
T=5s  semua gRPC stream dari sidecar terputus
T=5-15s  sidecar reconnect dengan exponential backoff
T=15s distributeLoop() jalan pertama
T=15s ambil kandidat dari graph-analyzer
T=15s buat token baru, push ke sidecar
T=15s sidecar memiliki token baru, cache aktif kembali

! WINDOW VULNERABILITY: T=0 hingga T=15s
  - nonce blacklist hilang
  - jika ada token yang di-revoke sebelum restart,
    nonce lamanya tidak lagi di-blacklist
  - MITIGASI: saat Pod startup, Cache Distributor
    revoke semua token aktif dan issue ulang dengan nonce baru
    (implementasi: startup hook di main.go)
```

### Envoy sidecar restart

```
T=0   Envoy restart (sidecar crash atau pod restart)
T=0   shared data = {} (kosong)
T=0   nonce registry = {} (kosong)
T=2s  Envoy Ready
T=2s  WASM filter aktif
T=2s  semua request: cache miss → fallback ke OPA
T=2s  sidecar subscribe ke StreamRevocations
T=5s  Cache Distributor mendeteksi reconnect
T=5s  Cache Distributor push semua token aktif via PushToken
T=5s  sidecar memiliki cache terisi kembali
```

---

## 4. State Schema (In-Memory)

### TopologyGraph

```go
// Representasi internal graph
type TopologyGraph struct {
    mu    sync.RWMutex

    // edges: map dari "source->target" ke ServiceEdge
    // Key format: "<source_spiffe_id>-><target_spiffe_id>"
    // Contoh: "spiffe://cluster.local/ns/zta-research/sa/frontend->spiffe://cluster.local/ns/zta-research/sa/productcatalogservice"
    edges map[string]*ServiceEdge

    // nodes: map dari SPIFFE ID ke node ID gonum internal
    nodes map[string]int64

    // graph: directed weighted graph gonum untuk komputasi centrality
    graph *simple.WeightedDirectedGraph

    // nextNodeID: auto-increment ID untuk node baru
    nextNodeID int64
}

type ServiceEdge struct {
    Source    string    // SPIFFE ID
    Target    string    // SPIFFE ID
    CallCount int64     // atomic counter
    LastSeen  time.Time // diupdate setiap RecordCall
    Method    string    // HTTP method terakhir yang dicatat (opsional)
}

// Ukuran maksimum: MAX_EDGES = 500 entry
// Pruning: edge dengan LastSeen > 2 menit dihapus setiap 30 detik
```

### RevocationManager

```go
type Manager struct {
    mu sync.RWMutex

    // active: nonce aktif per pasangan service
    // Key: "source->target" (format sama dengan edges)
    active map[string]*NonceEntry

    // blacklist: nonce yang sudah dicabut
    // Key: nonce string (16 char hex)
    // Value: waktu revokasi (untuk pruning)
    blacklist map[string]time.Time
}

type NonceEntry struct {
    Nonce     string
    IssuedAt  time.Time
    PairKey   string // "source->target" - untuk lookup balik
}

// Ukuran aktif: ~100 entry (sama dengan jumlah pasangan service)
// Ukuran blacklist: ~200 entry (2× active, karena nonce lama tetap 120 detik)
// Pruning blacklist: setiap 120 detik, hapus entry > 120 detik
```

### Broadcaster

```go
type Broadcaster struct {
    mu sync.RWMutex

    // clients: semua sidecar yang sedang terkoneksi via stream
    // Key: sidecarID
    clients map[string]*StreamClient
}

type StreamClient struct {
    sidecarID string
    namespace string
    stream    cache.CacheDistributor_StreamRevocationsServer
    connectedAt time.Time
    done      chan struct{} // ditutup saat stream berakhir
}

// Ukuran maksimum: ~100 entry (satu per pod dengan sidecar)
```

### Local Cache (Thread-Isolated In-Memory Pointer Cache)

```
// Local cache diimplementasikan sebagai Go map thread-isolated dalam pluginContext WASM
// Menghindari shared data overhead dan zero-deserialization pada critical path

Key format  : "<source_spiffe_id>-><target_spiffe_id>" (tersanitasi dari delimiter)
Value format: Pointer langsung *token.CachedToken (in-memory struct)
Max entries : 1000 (konstanta maxCacheEntries)
Eviction    : Proactive stale-first eviction pada onTokenResponse jika len >= 1000;
              lazy eviction pada Decide() jika token kedaluwarsa.

CachedToken Struct:
{
  "Source": "spiffe://cluster.local/ns/default/sa/frontend",
  "Target": "spiffe://cluster.local/ns/default/sa/productcatalog",
  "AllowedActions": ["GET", "POST"],
  "ExpiresAt": 1700045000,
  "RevocationNonce": "a3f9c2e1b4d70812",
  "Signature": "base64_ed25519_sig"
}

TTL Bounds  : Diklem secara closed-form pada interval [5.000 ms, 30.000 ms].
Estimasi ukuran per entry: ~256 bytes
Total ukuran max: 1000 x 256 bytes = ~256 KB (ringan untuk WASM linear memory)
```

---

## 5. Jika Persistent Dibutuhkan (Ekstensi Masa Depan)

Jika penelitian diperluas ke lingkungan produksi, berikut skema database yang direkomendasikan. Ini **bukan bagian dari implementasi saat ini**.

### Opsi: Redis (recommended untuk production)

```
# Graph Analyzer state
HSET graph:edges "frontend->productcatalog" '{"call_count":342,"last_seen":"2024-01-15T10:30:00Z"}'
EXPIRE graph:edges:frontend->productcatalog 120  # auto-expire 2 menit

# Cache Distributor: active nonces
HSET nonces:active "frontend->productcatalog" "a3f9c2e1b4d70812"

# Cache Distributor: nonce blacklist
SET nonce:blacklist:a3f9c2e1b4d70812 1 EX 120  # TTL = 2× MaxTTL
```

**Alasan Redis:** TTL native per key, in-memory tapi bisa persist ke RDB, pub/sub untuk menggantikan gRPC stream.

### Opsi: SQLite (untuk deployment single-node)

```sql
-- Graph Analyzer
CREATE TABLE service_edges (
    source_identity TEXT NOT NULL,
    target_identity TEXT NOT NULL,
    call_count      INTEGER NOT NULL DEFAULT 0,
    last_seen       DATETIME NOT NULL,
    PRIMARY KEY (source_identity, target_identity)
);
CREATE INDEX idx_edges_last_seen ON service_edges(last_seen);

-- Cache Distributor: active nonces
CREATE TABLE active_nonces (
    pair_key        TEXT PRIMARY KEY,  -- "source->target"
    nonce           TEXT NOT NULL,
    issued_at       DATETIME NOT NULL
);

-- Cache Distributor: blacklist
CREATE TABLE nonce_blacklist (
    nonce           TEXT PRIMARY KEY,
    revoked_at      DATETIME NOT NULL
);
-- Cleanup: DELETE FROM nonce_blacklist WHERE revoked_at < datetime('now', '-120 seconds');
```

**Kapan memilih SQLite:** Deployment kecil (< 10 service), tidak butuh pub/sub, cukup dengan periodic polling.
