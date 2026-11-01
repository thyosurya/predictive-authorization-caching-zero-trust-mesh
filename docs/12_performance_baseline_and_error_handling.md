# Performance Baseline & Error Handling Specification
## ZTA Predictive Authorization Caching

Dokumen ini menggabungkan dua spesifikasi terkait: angka referensi baseline performa komponen dalam kondisi idle, dan definisi perilaku sistem pada setiap skenario kegagalan.

---

## Bagian A: Performance Baseline

### A.1 Tujuan

Baseline ini mendefinisikan penggunaan resource setiap komponen dalam kondisi **idle** (tidak ada beban load test). Angka ini digunakan sebagai:
- Referensi untuk menghitung overhead murni akibat mekanisme ZTA dan caching
- Deteksi anomali: jika resource usage jauh melebihi baseline saat idle, ada masalah
- Input untuk interpretasi metrik eksperimen (`cpu_overhead = cpu_total - cpu_baseline`)

### A.2 Cara Mengukur Baseline

```bash
# Pastikan tidak ada load test berjalan
# Tunggu 60 detik setelah last request
# Ukur selama 60 detik

# CPU sidecar baseline
kubectl top pod -n zta-research --containers | grep istio-proxy

# Memory sidecar baseline
kubectl exec -n zta-research <pod-name> -c istio-proxy -- \
  cat /sys/fs/cgroup/memory/memory.usage_in_bytes

# Atau via Prometheus (lebih akurat, rata-rata 60 detik):
# avg(container_memory_working_set_bytes{container="istio-proxy", namespace="zta-research"})[60s]
```

### A.3 Angka Baseline (Diukur pada Testbed Eksperimen)

**Spesifikasi hardware:** Testbed empiris utama menggunakan Single-Node VM Lean (2 vCPU AMD EPYC 7713, 4 GB DDR4, Debian 12, k3s v1.28.5+k3s1; Tabel 5 naskah mencatat rata-rata namespace ~11,15%–13,16% CPU dan ~257,62–290,01 MB RAM). Profil mikrobenchmark komponen idle per-container di bawah ini diukur pada node referensi (4 vCPU Intel Xeon E5-2680v4, 8 GB DDR4, k3s v1.28.5):

#### Envoy Sidecar (istio-proxy)

| Kondisi | CPU avg | CPU peak | Memory avg | Memory peak |
|---|---|---|---|---|
| Idle (tanpa Istio AuthzPolicy) | 2.1 m | 5 m | 38 MB | 42 MB |
| Idle + mTLS STRICT | 2.8 m | 7 m | 41 MB | 46 MB |
| Idle + mTLS + AuthzPolicy (ZTA-Naif) | 3.4 m | 9 m | 43 MB | 49 MB |
| Idle + mTLS + AuthzPolicy + WasmPlugin | 4.1 m | 11 m | 47 MB | 53 MB |

> `m` = millicore (1000 m = 1 vCPU)

**Overhead WasmPlugin vs baseline:** +1.3 mCPU, +9 MB RAM saat idle.

#### Graph Analyzer

| Kondisi | CPU avg | Memory avg | Goroutines |
|---|---|---|---|
| Idle (graph kosong) | 0.5 m | 18 MB | 8 |
| Idle (graph 100 edge) | 1.2 m | 22 MB | 8 |
| Idle (graph 500 edge, max) | 2.8 m | 31 MB | 8 |
| Saat analysis loop jalan | 15 m (spike 300ms) | 34 MB | 10 |

#### Cache Distributor

| Kondisi | CPU avg | Memory avg | gRPC connections |
|---|---|---|---|
| Idle (0 token) | 0.3 m | 15 MB | 0 |
| Idle (50 token aktif) | 0.8 m | 17 MB | 10 |
| Idle (100 token aktif, max) | 1.1 m | 19 MB | 10 |
| Saat distribute loop jalan | 8 m (spike 200ms) | 20 MB | 10 |

#### OPA (Open Policy Agent)

| Kondisi | CPU avg | Memory avg | Latency evaluasi |
|---|---|---|---|
| Idle | 0.2 m | 45 MB | - |
| Per evaluasi policy (cold) | 5 m (spike) | 48 MB | 2.1 ms |
| Per evaluasi policy (warm) | 3 m (spike) | 47 MB | 0.8 ms |

#### SPIRE Server & Agent

| Komponen | CPU avg | Memory avg |
|---|---|---|
| SPIRE Server | 1.5 m | 35 MB |
| SPIRE Agent (per node) | 0.8 m | 22 MB |

### A.4 Interpretasi Overhead

Untuk menghitung **overhead murni** mekanisme otorisasi:

```
overhead_cpu = cpu_during_load - cpu_baseline_idle
overhead_mem = mem_during_load - mem_baseline_idle

Contoh:
cpu_during_load (ZTA-Cache @ 2000 RPS) = 85 m
cpu_baseline_idle (PBS)                 = 2.8 m
overhead_cpu_zta_cache                  = 82.2 m

cpu_during_load (ZTA-Naif @ 2000 RPS)  = 142 m
overhead_cpu_zta_naive                  = 139.2 m

Reduksi overhead CPU:
(139.2 - 82.2) / 139.2 × 100 = 40.9%
```

---

## Bagian B: Error Handling Specification

### B.1 Filosofi

Sistem dirancang dengan prinsip **fail secure over fail open**:
- Jika komponen gagal, keputusan default adalah **menolak akses** (deny), bukan mengizinkan
- Tidak ada skenario kegagalan yang mengakibatkan akses tak terbatas
- Setiap skenario kegagalan memiliki fallback yang terdefinisi

### B.2 Skenario Kegagalan per Komponen

---

#### SKENARIO 1: OPA (PDP Central) Tidak Dapat Dijangkau

**Kondisi:** Envoy (ZTA-Naif) gagal menghubungi OPA via ext_authz.

**Perilaku yang diharapkan:**
```
Request masuk ke sidecar
  → ext_authz call ke OPA → timeout atau connection refused
  → Envoy: DENY request (default deny pada ext_authz failure)
  → HTTP 503 ke client
  → Log: "upstream connect error or disconnect/reset before headers"
```

**Konfigurasi Istio yang memastikan ini:**
```yaml
extensionProviders:
  - name: opa-ext-authz
    envoyExtAuthzHttp:
      service: opa-service.zta-research.svc.cluster.local
      port: 8181
      failureModeAllow: false  # KRITIS: false = fail secure (deny on failure)
```

**Pada ZTA-Cache:** Jika OPA down, sidecar masih dapat melayani request yang mengalami cache hit dari local in-memory cache selama token belum expired (ditandai dengan header `x-zta-cache-verified: true` yang membypass OPA). Namun, request yang mengalami cache miss akan diteruskan ke ext_authz OPA dan otomatis ditolak secara fail-closed (HTTP 503/403) karena `failureModeAllow: false`. Ini menjamin resiliensi terotorisasi tanpa menciptakan lubang keamanan.

**Recovery:** Otomatis saat OPA kembali tersedia.

---

#### SKENARIO 2: Graph Analyzer Tidak Dapat Dijangkau

**Kondisi:** Cache Distributor gagal mengambil `/candidates` dari Graph Analyzer.

**Perilaku yang diharapkan:**
```
distributeLoop() jalan
  → GET graph-analyzer:8080/candidates → connection refused
  → Log WARNING: "failed to fetch candidates: connection refused"
  → Skip distribusi untuk siklus ini
  → Token yang sudah terdistribusi tetap aktif sampai TTL habis
  → Retry pada siklus berikutnya (30 detik kemudian)
```

**Dampak pada eksperimen:** Cache hit ratio turun secara gradual karena tidak ada token baru yang didistribusikan. Token lama expired berdasarkan TTL. Bukan kegagalan kritis.

**Recovery:** Otomatis saat Graph Analyzer kembali tersedia: distributeLoop akan kembali mengambil kandidat.

---

#### SKENARIO 3: Cache Distributor Tidak Dapat Dijangkau oleh Sidecar

**Kondisi:** Koneksi gRPC stream dari sidecar ke Cache Distributor terputus.

**Perilaku yang diharapkan:**
```
Stream putus (network error / pod restart)
  → Sidecar: stream.Recv() return error
  → Log: "stream disconnected: <error>"
  → Sidecar: mulai reconnect dengan exponential backoff
    T+100ms: retry 1
    T+200ms: retry 2
    T+400ms: retry 3
    ...
    T+5000ms: retry N (steady state: retry setiap 5 detik)
  → Token yang ada di cache tetap valid sampai TTL habis
  → Revokasi yang terjadi selama disconnect TIDAK diterima
  → Setelah reconnect: Cache Distributor push full state
```

**Window vulnerability:** Selama disconnect, token yang di-revoke tetap valid di sidecar sampai TTL habis (max 30 detik sesuai MaxTTL). Ini terdokumentasi sebagai known limitation (lihat ADR-004).

**Recovery:** Otomatis via reconnect. Setelah reconnect, Cache Distributor melakukan full sync.

---

#### SKENARIO 4: SPIRE Agent Tidak Dapat Dijangkau

**Kondisi:** SPIRE Agent pod crash atau tidak merespons - workload tidak bisa mendapat SVID baru.

**Perilaku yang diharapkan:**
```
SVID yang ada masih valid sampai TTL-nya habis (default: 1 jam)
Selama SVID masih valid: semua operasi mTLS normal
Setelah SVID expired:
  → mTLS handshake gagal
  → Semua koneksi ke/dari pod yang terdampak ditolak
  → HTTP 503 untuk semua request
```

**Dampak pada eksperimen:** Jika terjadi selama pengukuran, skenario harus dihentikan dan di-reset. Tandai sebagai skenario invalid.

**Recovery:** Restart SPIRE Agent pod - workload akan mendapat SVID baru.

**Tanda awal masalah:** Log Envoy: `CERTIFICATE_VERIFY_FAILED` atau `SSL_ERROR_RX_UNEXPECTED_HELLO`

---

#### SKENARIO 5: WASM Filter Gagal Dimuat

**Kondisi:** Envoy gagal memuat WASM binary dari ConfigMap (binary corrupt, ukuran terlalu besar, dll).

**Perilaku yang diharapkan:**
```
Envoy startup / config reload
  → Load WasmPlugin dari ConfigMap
  → Gagal: error parsing WASM binary
  → Log: "wasm: failed to initialize"
  → Fallback: Envoy menonaktifkan filter ini
  → Request diproses tanpa WASM filter (cache bypass)
  → Semua request jatuh ke ext_authz (OPA) - berfungsi sebagai ZTA-Naif
```

**Konfigurasi yang memastikan ini:**
```yaml
spec:
  failStrategy: FAIL_OPEN  # atau FAIL_CLOSE - pilih sesuai kebutuhan
  # FAIL_OPEN: bypass filter jika gagal (request tetap berjalan via ext_authz)
  # FAIL_CLOSE: blokir semua request jika filter gagal (lebih aman)
```

**Untuk penelitian:** Gunakan `FAIL_OPEN` agar pengukuran tidak terganggu jika ada bug di WASM.

---

#### SKENARIO 6: Node Kubernetes Crash

**Kondisi:** Salah satu worker node mati mendadak.

**Perilaku yang diharapkan:**
```
Node unreachable terdeteksi oleh Kubernetes (< 40 detik)
  → Pod pada node yang crash: status Unknown/Terminating
  → Kubernetes reschedule pod ke node lain
  → Proses reschedule: ~2-5 menit (pull image, startup, health check)
Selama reschedule:
  → Load test: request ke pod yang hilang timeout
  → Error rate naik signifikan
  → Throughput turun
Setelah reschedule:
  → Pod baru startup
  → Sidecar subscribe ke Cache Distributor
  → Token didistribusikan ulang
  → Normal kembali
```

**Dampak pada eksperimen:** Skenario harus dihentikan dan dianggap invalid. Catat di incident log.

---

#### SKENARIO 7: Revocation Nonce Tidak Sinkron

**Kondisi:** Race condition - sidecar memiliki nonce yang berbeda dari yang didaftarkan di Manager (dapat terjadi saat concurrent push dan revoke).

**Perilaku yang diharapkan:**
```
Sidecar menerima request
  → Ambil token dari cache
  → Validator: cek nonce "abc" di registry
  → Nonce "abc" tidak ada (sudah di-blacklist atau belum terdaftar)
  → Return: DecisionRevoked
  → Delete cache entry
  → Request fallback ke OPA
  → OPA evaluasi berdasarkan kebijakan terkini
```

**Ini adalah perilaku yang benar** - nonce mismatch selalu diinterpretasikan sebagai revokasi, bukan sebagai error. Fail secure.

---

### B.3 Tabel Ringkasan

| Komponen gagal | Dampak langsung | Fallback | Recovery | Fail mode |
|---|---|---|---|---|
| OPA down | Request ditolak (503) | Tidak ada (ZTA-Naif) / Cache (ZTA-Cache) | Otomatis | Secure |
| Graph Analyzer down | Tidak ada token baru | Token lama tetap aktif | Otomatis | Graceful |
| Cache Distributor down | Tidak ada token baru, stream putus | Token lama tetap aktif | Otomatis + reconnect | Graceful |
| SPIRE Agent down | SVID expired → semua koneksi gagal | Tidak ada (hard failure) | Restart pod | Secure |
| WASM gagal load | Bypass cache | Jatuh ke OPA (ZTA-Naif mode) | Rebuild/reload WASM | Graceful |
| Node crash | Pod hilang, error rate naik | Kubernetes reschedule | 2–5 menit | Graceful |
| Nonce mismatch | Request fallback ke OPA | OPA evaluasi terkini | Otomatis | Secure |

---

### B.4 Log Patterns untuk Diagnosis Cepat

```bash
# OPA tidak terjangkau
kubectl logs -n zta-research <pod> -c istio-proxy | grep "upstream connect error"

# WASM filter gagal
kubectl logs -n zta-research <pod> -c istio-proxy | grep -i "wasm"

# mTLS handshake gagal
kubectl logs -n zta-research <pod> -c istio-proxy | grep "SSL_ERROR\|CERTIFICATE_VERIFY"

# Cache miss rate tinggi (token tidak terdistribusi)
kubectl logs -n zta-research deploy/cache-distributor | grep "cache miss"

# gRPC stream putus
kubectl logs -n zta-research deploy/cache-distributor | grep "stream disconnected"

# Revocation berhasil dikirim
kubectl logs -n zta-research deploy/cache-distributor | grep "revoked\|broadcast"
```
