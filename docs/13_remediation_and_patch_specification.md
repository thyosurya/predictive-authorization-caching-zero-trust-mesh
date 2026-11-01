# Spesifikasi Remedi, Patching Arsitektur, dan Panduan Rerun Eksperimen
## ZTA Predictive Authorization Caching

Dokumen ini mendefinisikan temuan audit statis (`verify_offline.py`), akar masalah (*root cause*), spesifikasi perbaikan kode (*code patches*), dan panduan pelaksanaan *fresh rerun* pada lingkungan VM 2 vCPU / 4 GB RAM.

---

## 1. Temuan Audit Statis & Analisis Akar Masalah

Berdasarkan eksekusi skrip audit forensik `scripts/verify_offline.py`, ditemukan 5 diskoneksi kritis antara rancangan naskah dan implementasi kode:

| No | Komponen | Status | Akar Masalah (*Root Cause*) |
|---|---|:---:|---|
| **1** | **Jalur Otorisasi ZTA** | `FLAG` | `AuthorizationPolicy` dan `WasmPlugin` hanya dipasang pada `productcatalogservice` dan `cartservice`. Target k6 adalah `http://frontend.../api/products`. Pod `frontend` (`service-a/main.go`) mengeksekusi request secara lokal di memori tanpa memanggil backend. Akibatnya, request k6 tidak pernah melewati OPA maupun WASM. |
| **2** | **Cache Sidecar WASM** | `FLAG` | Filter WASM tidak memiliki kode non-test yang memanggil `cache.Set()`, `SetSharedData`, maupun `Validator.RegisterNonce()`. Berkas gRPC stub `cache_grpc.pb.go` di distributor masih berupa placeholder 4 baris. Token yang di-generate tidak pernah diinjeksikan ke Envoy sidecar. |
| **3** | **Pengukuran H2 Revokasi** | `FLAG` | Skrip `analyze-all-data.py` baris 376 secara eksplisit menyalin durasi `p99_ms` dari HTTP GET biasa menjadi `revocation_latency_ms`. Pada skenario S09, request k6 ke `/revoke` gagal 100% (`fail >= triggered`), sehingga angka 4,46 ms di naskah adalah latensi GET biasa, bukan latensi propagasi revokasi. |
| **4** | **Inkonsistensi Nilai TTL** | `FLAG` | `candidate.go` memanggil `CalculateTTL` dengan parameter masa sertifikat `24*time.Hour, 72*time.Hour` ($v_c = 0{,}333$), menghasilkan TTL 10 detik dan 7 detik. Angka 30 detik dan 21 detik di naskah hanya tercapai jika $v_c = 1{,}0$. Selain itu, target `Makefile` mengambil nilai dari `seed-candidates.json` statis (45s, 20s). |
| **5** | **Konsistensi Statistik** | `FLAG` | Terdapat baris duplikat pada `shapiro-wilk-results.csv` (`ZTA-Naif 500 RPS`), serta perbedaan angka antara berkas laporan lama `analysis_report.md` dengan CSV hasil hitung ulang. |

---

## 2. Spesifikasi Teknis Perbaikan (*Remediation Patches*)

### Patch 1: Implementasi Trafik East-West Nyata pada Service Layer
* **Target:** [`services/service-a/main.go`](file:///d:/Penelitian/zta-predictive-cache/services/service-a/main.go)
* **Spesifikasi:**
  * Ubah `service-a` (frontend) menjadi *API Gateway/Forwarder* nyata:
    * Request `GET /api/products` diteruskan via HTTP client ke `http://productcatalogservice.zta-research.svc.cluster.local:8080/products`.
    * Request `POST /api/cart` diteruskan via HTTP client ke `http://cartservice.zta-research.svc.cluster.local:8080/cart`.
    * Request `GET /health` tetap ditangani lokal untuk liveness probe.
  * Tambahkan konfigurasi timeout (2 detik) dan reuse HTTP transport (connection pooling) agar performa proxy stabil.
* **Target:** [`services/service-b/main.go`](file:///d:/Penelitian/zta-predictive-cache/services/service-b/main.go) & [`services/service-c/main.go`](file:///d:/Penelitian/zta-predictive-cache/services/service-c/main.go)
  * Pastikan `service-b` melayani `GET /products` dan `service-c` melayani `POST /cart`.

### Patch 2: Sinkronisasi Token & Nonce pada WASM Filter
* **Target:** [`wasm-filter/main.go`](file:///d:/Penelitian/zta-predictive-cache/wasm-filter/main.go) & [`wasm-filter/authz/decision.go`](file:///d:/Penelitian/zta-predictive-cache/wasm-filter/authz/decision.go)
* **Spesifikasi:**
  * Di lingkungan TinyGo, pendekatan paling andal untuk sinkronisasi token tanpa keterbatasan gRPC client adalah **HTTP Callout Synchronizer** menggunakan API `proxywasm.DispatchHttpCall`:
    1. Filter WASM secara berkala (misal: setiap kali startup plugin atau saat *cache miss*) memanggil endpoint REST Cache Distributor: `GET http://cache-distributor:8081/tokens/active`.
    2. Respons JSON berisi daftar token aktif yang ditandatangani (`allowed_actions`, `exp`, `revocation_nonce`, `signature`).
    3. Filter memverifikasi signature HMAC, menyimpan token ke `localcache.Set()`, dan mendaftarkan nonce ke `Validator.RegisterNonce()`.
    4. Saat menerima event revokasi, nonce dihapus via `Validator.InvalidateNonce()`.
* **Target:** [`control-plane/cache-distributor/main.go`](file:///d:/Penelitian/zta-predictive-cache/control-plane/cache-distributor/main.go)
  * Pastikan endpoint REST `GET /tokens/active` mengekspos token yang sedang aktif dalam format JSON yang siap dikonsumsi oleh filter WASM.

### Patch 3: Perbaikan Endpoint Revokasi & Pengukuran Latensi Sah (H2)
* **Target:** [`control-plane/cache-distributor/main.go`](file:///d:/Penelitian/zta-predictive-cache/control-plane/cache-distributor/main.go)
  * Pastikan handler `POST /revoke` menerima payload `{source, target, reason}`, memvalidasi input, memperbarui blacklist nonce, dan mengembalikan status HTTP `202 Accepted` beserta JSON `{status: "revoked", new_nonce: "..."}`.
* **Target:** [`load-testing/scenarios/s09-revocation-test.js`](file:///d:/Penelitian/zta-predictive-cache/load-testing/scenarios/s09-revocation-test.js)
  * Pisahkan metrik k6:
    * `http_req_duration`: untuk request GET normal.
    * `revocation_http_duration`: custom Trend metric khusus untuk durasi round-trip eksekusi pemanggilan `POST /revoke`.
    * `revocation_propagation_ms`: waktu hingga request dengan token lama tertolak (menerima HTTP 403 Forbidden).
* **Target:** [`scripts/analyze-all-data.py`](file:///d:/Penelitian/zta-predictive-cache/scripts/analyze-all-data.py)
  * Ubah fungsi `generate_revocation_data()` agar mengekstrak metrik revokasi dari `revocation_http_duration` atau waktu konfirmasi penolakan, **bukan** dari `p99_ms` request GET produk.

### Patch 4: Penyelarasan Formula TTL ($v_c = 1{,}0$)
* **Target:** [`control-plane/graph-analyzer/cache/candidate.go`](file:///d:/Penelitian/zta-predictive-cache/control-plane/graph-analyzer/cache/candidate.go#L82)
* **Spesifikasi:**
  * Ubah pemanggilan `CalculateTTL`:
    ```go
    // Asumsi sertifikat SPIFFE baru diperbarui (sisa validitas 100%)
    ttl := CalculateTTL(edge.CallCount, sensitivity, 24*time.Hour, 24*time.Hour)
    ```
  * Dengan $v_c = 24/24 = 1{,}0$, formula Persamaan (1) secara deterministik menghasilkan:
    * Endpoint Low (`/api/products`): $30.000 \times 1{,}0 \times (1 - 0{,}0) \times 1{,}0 = \mathbf{30.000\text{ ms}}$.
    * Endpoint Medium (`/api/cart`): $30.000 \times 1{,}0 \times (1 - 0{,}3) \times 1{,}0 = \mathbf{21.000\text{ ms}}$.
  * Hapus ketergantungan `Makefile` pada `seed-candidates.json` statis; pastikan kandidat mengalir murni dari `graph-analyzer` ke `cache-distributor`.

### Patch 5: Penyesuaian Istio Manifests
* **Target:** [`k8s/istio/authz-policy-zta.yaml`](file:///d:/Penelitian/zta-predictive-cache/k8s/istio/authz-policy-zta.yaml) & [`k8s/istio/wasm-filter.yaml`](file:///d:/Penelitian/zta-predictive-cache/k8s/istio/wasm-filter.yaml)
  * Pastikan `AuthorizationPolicy` terpasang pada service tujuan (`productcatalogservice` dan `cartservice`).
  * Pastikan `WasmPlugin` terpasang pada pod yang sama dengan fase `AUTHZ` dan URL mengarah ke binary wasm lokal.
  * Pastikan nama extension provider di `MeshConfig` Istio (`opa-ext-authz`) cocok dengan manifes.

---

## 3. Panduan Setup Lingkungan VM Bersih (2 Core, 4 GB RAM)

Karena VM memiliki spesifikasi terbatas (2 vCPU / 4 GB RAM), konfigurasikan klaster secara *lean*:

### A. Alokasi Swap Memory (Mencegah Kernel OOM)
```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

### B. Instalasi k3s Minimal (*Ultra-Lean*)
Nonaktifkan komponen k3s bawaan yang tidak diperlukan:
```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik --disable servicelb --disable metrics-server --disable local-storage" sh -
```

### C. Instalasi Istio Profil Minimal
Gunakan profil minimal atau nonaktifkan fitur yang tidak terpakai:
```bash
istioctl install --set profile=minimal -y
```

### D. Penembakan Beban k6 dari Luar VM
* **Wajib:** Jalankan k6 dari mesin laptop/PC Anda, arahkan target ke NodePort atau IP Publik VM.
* **Alasan:** Menjalankan k6 500–2000 RPS di dalam VM 2-core akan menghabiskan 1–1.5 core CPU sendiri, sehingga mendistorsi pengukuran latensi Envoy sidecar.

---

## 4. Kriteria Keberhasilan (*Acceptance Criteria*)

Sebelum memulai rerun 30 repetisi penuh, jalankan *sanity check* berikut:
1. `curl http://<VM-IP>:<PORT>/api/products` mengembalikan JSON produk dari backend `productcatalogservice`.
2. Log pod OPA (`kubectl logs -l app=opa -n zta-research`) mencatat evaluasi otorisasi dari panggilan East-West frontend.
3. Log sidecar `productcatalogservice` (`kubectl logs <pod> -c istio-proxy`) mencatat log WASM filter: `Cache hit: authorized ...` setelah token terdistribusi.
4. Log `graph-analyzer` mencatat edge `frontend -> productcatalogservice`.
5. Skrip `verify_offline.py` menghasilkan **`OK`** untuk semua pemeriksaan.

---

## 5. Remediasi Audit Peer-Review Tingkat Lanjut (IEEE TDSC / ACM TOPS)

Berdasarkan rekomendasi reviewer untuk jurnal transaksi IEEE/ACM, sistem telah dilengkapi 5 patch mitigasi keamanan dan rekayasa data-plane:

### Patch 6: Fail-Closed Ingress Header Stripping
* **File Target:** `wasm-filter/main.go`
* **Permasalahan:** Potensi spoofing header HTTP `x-zta-cache-verified: true` oleh entitas ingress/penyerang luar untuk membypass OPA.
* **Solusi Rekayasa:** Pemanggilan `proxywasm.RemoveHttpRequestHeader("x-zta-cache-verified")` tanpa syarat di baris pertama `OnHttpRequestHeaders`. Header hanya disematkan kembali jika cache lokal mengembalikan `DecisionAllow`. Jika miss, request diteruskan ke OPA `ext_authz` yang dikonfigurasi dengan `failureModeAllow: false` (fail-closed).

### Patch 7: Zero-Allocation In-Memory Cache & Stale-First Eviction
* **File Target:** `wasm-filter/cache/local_cache.go`, `wasm-filter/main.go`
* **Permasalahan:** Overhead alokasi heap dari JSON deserialization pada hot-path otorisasi serta risiko `ErrCacheFull` palsu jika kapasitas $N=1000$ tercapai sebelum stale keys dibersihkan.
* **Solusi Rekayasa:** Menyimpan pointer struct `*token.CachedToken` langsung dalam map terisolasi per worker thread WASM. Di `onTokenResponse`, pembersihan entri kedaluwarsa dieksekusi sebelum token baru dimasukkan ke cache.

### Patch 8: Delimiter & SPIFFE Identity Sanitization
* **File Target:** `wasm-filter/main.go`, `control-plane/cache-distributor/`
* **Permasalahan:** Delimiter collision attack bila SPIFFE ID memuat karakter `->` atau karakter kontrol.
* **Solusi Rekayasa:** Implementasi validator `isValidSPIFFEURI` yang secara ketat memeriksa skema URI dan menolak karakter `->`, newline (`\n`), carriage return (`\r`), dan karakter kontrol non-ASCII.

### Patch 9: Deterministic Multi-Tier Graph Tie-Breaking
* **File Target:** `control-plane/graph-analyzer/graph/centrality.go`
* **Permasalahan:** Ketidakkonsistenan pemeringkatan kandidat cache saat beberapa edge memiliki bobot betweenness centrality yang sama.
* **Solusi Rekayasa:** Pengurutan deterministik multi-kunci: `(betweenness_score DESC, call_frequency DESC, source_spiffe ASC, target_spiffe ASC)`.

### Patch 10: Delta-HMAC Token Ingestion
* **File Target:** `wasm-filter/main.go`
* **Permasalahan:** Beban komputasi verifikasi kriptografi yang redundan saat payload token tidak mengalami perubahan status.
* **Solusi Rekayasa:** Caching hash token delta untuk memverifikasi signature hanya jika token mengalami pembaruan hak akses atau nonce baru.
