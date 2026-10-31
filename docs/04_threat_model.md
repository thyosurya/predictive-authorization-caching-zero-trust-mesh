# Threat Model Document
## ZTA Predictive Authorization Caching - Analisis Ancaman Keamanan

Dokumen ini menganalisis ancaman keamanan spesifik yang muncul akibat penambahan lapisan caching pada arsitektur ZTA. Analisis menggunakan metodologi STRIDE.

---

## Daftar Isi
1. [Scope dan Asumsi](#1-scope-dan-asumsi)
2. [Asset yang Dilindungi](#2-asset-yang-dilindungi)
3. [Trust Boundaries](#3-trust-boundaries)
4. [Analisis Ancaman per Komponen (STRIDE)](#4-analisis-ancaman-per-komponen-stride)
5. [Attack Scenarios Detail](#5-attack-scenarios-detail)
6. [Mitigasi yang Diimplementasikan](#6-mitigasi-yang-diimplementasikan)
7. [Residual Risk](#7-residual-risk)
8. [ZT Invariant Verification](#8-zt-invariant-verification)

---

## 1. Scope dan Asumsi

**Dalam scope:**
- Cache Distributor dan mekanisme distribusi token
- Local cache di Envoy sidecar
- Mekanisme revokasi (push-based via gRPC)
- Delegation token (format, signing, validasi)
- Graph Topology Analyzer (data poisoning)

**Di luar scope:**
- Keamanan Kubernetes control plane
- Keamanan fisik node
- Keamanan aplikasi bisnis (Online Boutique)
- Keamanan Istio core (sudah diuji oleh komunitas)

**Asumsi:**
- mTLS diberlakukan ketat (STRICT mode) - tidak ada plaintext traffic
- SPIRE server tidak dikompromikan
- Kubernetes RBAC dikonfigurasi dengan benar
- Node tidak dalam kondisi dikompromikan (in-scope hanya ancaman network)

---

## 2. Asset yang Dilindungi

| Asset | Nilai | Keterangan |
|---|---|---|
| Delegation token | **Sangat Tinggi** | Berisi keputusan otorisasi - jika dipalsukan, bisa bypass PDP |
| Revocation nonce | **Sangat Tinggi** | Jika dimanipulasi, pencabutan akses bisa gagal |
| Local cache (Envoy shared data) | **Tinggi** | Berisi token aktif - target cache poisoning |
| Graph topology data | **Sedang** | Data palsu bisa mempengaruhi keputusan TTL |
| gRPC stream (revocation channel) | **Tinggi** | Jika diblokir, revokasi tidak sampai ke sidecar |
| OPA policy | **Sangat Tinggi** | Sumber kebenaran seluruh otorisasi |

---

## 3. Trust Boundaries

```
[Internet / Utara-Selatan]
        │
        ▼ (Ingress Gateway - TLS termination)
┌─────────────────────────────────────┐
│ NAMESPACE: zta-research             │ ← Trust Boundary 1
│                                     │   (Kubernetes namespace isolation)
│  ┌──────────────┐                   │
│  │ Control Plane│ ← Trust Boundary 2│
│  │  (OPA, Graph │   (Pod-to-pod mTLS│
│  │   Analyzer,  │    + SPIFFE ID)   │
│  │   Cache Dist)│                   │
│  └──────┬───────┘                   │
│         │ gRPC (mTLS)               │
│  ┌──────▼───────┐                   │
│  │ Sidecar Cache│ ← Trust Boundary 3│
│  │  (Envoy)     │   (shared data    │
│  │              │    in-process)    │
│  └──────┬───────┘                   │
│         │ localhost                 │
│  ┌──────▼───────┐                   │
│  │ Service App  │                   │
│  └──────────────┘                   │
└─────────────────────────────────────┘
```

---

## 4. Analisis Ancaman per Komponen (STRIDE)

### 4.1 Delegation Token

| STRIDE | Ancaman | Tingkat | Ada? |
|---|---|---|---|
| **S**poofing | Penyerang membuat token palsu yang tidak ditandatangani PDP | Kritis | Ya - dimitigasi via HMAC-SHA256 |
| **T**ampering | Penyerang mengubah `allowed_actions` atau `exp` di token yang valid | Kritis | Ya - dimitigasi via signature check |
| **R**epudiation | Tidak ada log siapa yang menggunakan token tertentu | Rendah | Ya - dimitigasi via access log Envoy |
| **I**nformation Disclosure | Token bocor ke service yang tidak berhak | Sedang | Parsial - token hanya ada di memory sidecar |
| **D**enial of Service | Flood pembuatan token untuk exhausted cache | Rendah | Ya - dimitigasi via rate limiting |
| **E**levation of Privilege | Memalsukan `sub`/`aud` untuk mendapat akses lebih tinggi | Kritis | Ya - dimitigasi via SPIFFE ID verification |

### 4.2 Local Cache (Thread-Isolated In-Memory Cache)

| STRIDE | Ancaman | Tingkat | Ada? |
|---|---|---|---|
| **S**poofing | Service lain mengklaim identitas SPIFFE yang berbeda | Tinggi | Ya - dimitigasi via mTLS cert |
| **T**ampering | Modifikasi entri cache langsung di memori Envoy | Sedang | Parsial - membutuhkan akses proses |
| **R**epudiation | Cache decision tidak ter-log | Rendah | Ya - audit log dapat diaktifkan |
| **I**nformation Disclosure | Policy otorisasi bocor via cache enumeration | Rendah | Kecil - hanya via admin API |
| **D**enial of Service | Cache flooding - mengisi cache dengan entri palsu | Sedang | Ya - dimitigasi via ukuran cache max |
| **E**levation of Privilege | Stale cache mempertahankan akses setelah revokasi | **Kritis** | Ya - **ancaman utama**, dimitigasi via nonce |

### 4.3 gRPC Revocation Stream

| STRIDE | Ancaman | Tingkat | Ada? |
|---|---|---|---|
| **S**poofing | Penyerang menyamar sebagai Cache Distributor | Tinggi | Ya - dimitigasi via mTLS server cert |
| **T**ampering | Modifikasi pesan revokasi dalam transit | Tinggi | Ya - dimitigasi via mTLS encryption |
| **R**epudiation | - | Rendah | - |
| **I**nformation Disclosure | Isi revocation event bocor | Rendah | Kecil - hanya berisi ID dan nonce |
| **D**enial of Service | Stream diputus sehingga revokasi tidak sampai | **Kritis** | Ya - **ancaman utama**, dimitigasi via reconnect + TTL |
| **E**levation of Privilege | Replay revocation event lama untuk flood sidecar | Rendah | Ya - event idempotent |

### 4.4 Graph Topology Analyzer

| STRIDE | Ancaman | Tingkat | Ada? |
|---|---|---|---|
| **S**poofing | Service palsu mengirim `/record` call palsu | Sedang | Ya - endpoint `/record` harus dibatasi |
| **T**ampering | Data poisoning: memalsukan frekuensi panggilan tinggi untuk memaksa TTL panjang | Sedang | Ya - dimitigasi via input validation + threshold |
| **I**nformation Disclosure | Graph topologi mengungkap arsitektur internal | Rendah | Ya - `/graph` endpoint harus dilindungi |
| **D**enial of Service | Flood `/record` untuk membuat graph sangat besar | Sedang | Ya - rate limit dan max edges |

---

## 5. Attack Scenarios Detail

### AS-01: Token Replay Attack

**Deskripsi:** Penyerang mencegat delegation token yang valid (meski mTLS membuatnya sulit) dan mencoba menggunakannya dari service berbeda atau setelah masa berlaku.

**Alur serangan:**
```
1. Penyerang mendapat salinan token (misal: via memory dump atau log)
2. Penyerang mengirim request dengan token tersebut setelah exp
3. Sidecar memvalidasi exp field
```

**Dampak:** Akses tidak sah ke service target.

**Mitigasi:**
- Validasi `exp` dilakukan di setiap request, bukan hanya saat token diterima
- TTL pendek (5-30 detik) membatasi window serangan
- `sub` (source identity) diverifikasi terhadap sertifikat mTLS pengirim: token dari service A tidak bisa digunakan service B

**Residual risk:** Sangat rendah: window serangan <= 30 detik dan memerlukan akses memori proses.

---

### AS-02: Cache Poisoning via Forged Token

**Deskripsi:** Penyerang mencoba menyuntikkan token palsu ke dalam local cache Envoy.

**Alur serangan:**
```
1. Penyerang membuat token JSON dengan isi yang diinginkan
2. Penyerang memanggil endpoint admin Cache Distributor (PushToken gRPC)
3. Sidecar menyimpan token tanpa verifikasi
```

**Dampak:** Sidecar membuat keputusan otorisasi berdasarkan token palsu.

**Mitigasi:**
- Setiap token ditandatangani dengan HMAC-SHA256 menggunakan kunci yang hanya diketahui PDP dan Cache Distributor
- Sidecar **selalu** memverifikasi signature sebelum menyimpan ke cache
- gRPC endpoint PushToken hanya dapat diakses dari Cache Distributor (mTLS + SPIFFE ID check)

**Residual risk:** Rendah - memerlukan kunci signing yang seharusnya tidak dapat diakses dari luar.

---

### AS-03: Revocation Bypass via Stream Disruption

**Deskripsi:** Penyerang memutus koneksi gRPC antara Cache Distributor dan sidecar, sehingga event revokasi tidak diterima.

**Alur serangan:**
```
1. Penyerang memutus koneksi gRPC (network partition atau DoS)
2. Admin mencabut akses service A ke service B
3. Cache Distributor mengirim RevocationEvent, tapi stream terputus
4. Sidecar masih menggunakan token lama dengan nonce yang belum di-update
5. Service A masih bisa mengakses service B
```

**Dampak:** Revokasi akses terlambat selama stream putus - melanggar ZT invariant "instant revocation".

**Mitigasi pertama - TTL sebagai safety net:**
- Token memiliki TTL maksimum 60 detik
- Meski stream putus, token akan kedaluwarsa dalam ≤ 60 detik
- Setelah expired, sidecar fallback ke PDP yang sudah memiliki kebijakan revokasi

**Mitigasi kedua - Reconnect agresif:**
```go
// Cache Distributor: reconnect dengan exponential backoff
func (s *Sidecar) subscribeWithRetry(ctx context.Context) {
    backoff := 100 * time.Millisecond
    for {
        err := s.subscribe(ctx)
        if err == nil {
            return
        }
        log.Printf("stream disconnected, retry in %v", backoff)
        time.Sleep(backoff)
        backoff = min(backoff*2, 5*time.Second)
    }
}
```

**Mitigasi ketiga - Revocation pada fallback PDP:**
- Saat cache miss (setelah token expired), sidecar query PDP
- PDP sudah mencerminkan kebijakan terbaru (akses dicabut)
- Request ditolak di PDP

**Residual risk:** Sedang: window kerentanan maksimum = TTL token (30 detik). Dapat dikurangi dengan menurunkan MaxTTL untuk service sensitif tinggi.

---

### AS-04: Nonce Staleness Attack

**Deskripsi:** Race condition antara token distribution dan revocation mengakibatkan sidecar memvalidasi nonce yang sudah tidak berlaku.

**Alur serangan:**
```
Timeline:
T=0: Cache Distributor push token dengan nonce "abc" ke Sidecar A
T=1: Admin revoke - nonce baru "xyz" di-push
T=2: Token baru dengan nonce "abc" dikirim (karena race condition di distributor)
T=3: Sidecar A menerima token baru dengan nonce "abc" - valid karena tidak tahu "xyz"
```

**Mitigasi:**
- Revocation Manager menggunakan mutex - operasi revoke dan distribute tidak bisa bersamaan untuk pasangan yang sama
- Setiap revokasi mengincrement versi nonce secara monotonic - token lama selalu ditolak meski ada race condition
- Nonce lama dimasukkan ke blacklist selama 2× MaxTTL setelah revokasi

**Residual risk:** Sangat rendah - mutex dan blacklist mencegah race condition.

---

### AS-05: Graph Data Poisoning

**Deskripsi:** Penyerang membanjiri endpoint `/record` dengan data palsu untuk memaksa TTL yang sangat panjang pada pasangan service tertentu.

**Alur serangan:**
```
1. Penyerang mengirim ribuan request POST /record dengan source/target palsu
2. Graph Analyzer mencatat edge palsu dengan call_count sangat tinggi
3. TTL dihitung sangat panjang (mendekati MaxTTL)
4. Jika policy kemudian berubah, cache bertahan terlalu lama
```

**Mitigasi:**
- Endpoint `/record` hanya dapat diakses dari Envoy sidecar dalam namespace yang sama (NetworkPolicy)
- Rate limit: maksimum 1000 record/menit per source identity
- TTL tetap dibatasi oleh MaxTTL (30 detik): data poisoning hanya bisa memaksimalkan TTL, tidak melewatinya
- Sertifikat revokasi tetap bekerja via nonce: tidak terpengaruh TTL

**Residual risk:** Rendah: dampak terbatas pada TTL yang lebih panjang dari optimal, tidak mengubah keputusan otorisasi.

---

### AS-06: Delimiter Injection Attack

**Deskripsi:** Penyerang mendaftarkan SPIFFE ID yang secara sengaja menyematkan karakter delimiter `->` atau pemisah kontrol (misal: `spiffe://cluster.local/ns/default/sa/alice->bob`) untuk memanipulasi pemetaan key-value cache atau memalsukan routing otorisasi.

**Alur serangan:**
```
1. Penyerang menyusupkan identity yang memuat substring "->"
2. Komponen cache menggabungkan source + "->" + target secara naif
3. Key collisions terjadi, memicu cache injection atau bypass otorisasi service lain
```

**Mitigasi:**
- Fungsi sanitasi `isValidSPIFFEURI` memvalidasi URI secara ketat, menolak karakter `->`, newline (`\n`), carriage return (`\r`), spasi, serta karakter kontrol non-ASCII.
- Pengecekan dilakukan sebelum key cache dibentuk maupun dicari, baik pada distributor maupun wasm-filter.

**Residual risk:** Negligible: input tidak valid langsung digugurkan dengan error `ErrInvalidSPIFFEURI`.

---

### AS-07: Ingress Bypass Header Spoofing (x-zta-cache-verified)

**Deskripsi:** Klien jahat atau workload tanpa otorisasi mengirimkan request HTTP dengan menyuntikkan header buatan `x-zta-cache-verified: true` dengan tujuan membypass evaluasi policy OPA di Envoy `ext_authz`.

**Alur serangan:**
```
1. Penyerang mengirim HTTP request langsung dengan header "x-zta-cache-verified: true"
2. Jika filter tidak membersihkan header, filter downstream (OPA ext_authz) mengira request telah diverifikasi
3. Terjadi otorisasi tanpa evaluasi policy sesungguhnya
```

**Mitigasi:**
- WASM filter secara tanpa syarat menghapus header `x-zta-cache-verified` pada ingress (`proxywasm.RemoveHttpRequestHeader`) sebelum evaluasi local cache dilakukan.
- Header tersebut hanya diinjeksi ulang (`AddHttpRequestHeader`) jika dan hanya jika local cache menemukan entry valid (`DecisionAllow`) dengan signature Ed25519/HMAC yang terverifikasi dan belum kedaluwarsa.

**Residual risk:** Sangat rendah: spoofing dari luar secara otomatis dieliminasi pada langkah pertama pemrosesan filter.

---

## 6. Mitigasi yang Diimplementasikan

### Ringkasan kontrol keamanan

| Kontrol | Ancaman yang Dimitigasi | Implementasi |
|---|---|---|
| HMAC-SHA256 / Ed25519 token signing | AS-02 (cache poisoning) | `token/signer.go` |
| mTLS STRICT pada semua traffic | AS-01 (token replay via network) | Istio PeerAuthentication |
| SPIFFE ID verification pada `sub`/`aud` | AS-01, AS-02 | WASM filter validator |
| `exp` validation per request | AS-01 | `wasm-filter/token/validator.go` |
| Nonce-based revocation | AS-03 (revocation bypass via cache) | `revocation/manager.go` |
| Push-based gRPC stream + reconnect | AS-03 | `revocation/broadcaster.go` |
| TTL MaxTTL sebagai safety net (30s) | AS-03 | `cache/ttl.go` |
| Rate limit pada `/record` | AS-05 | Middleware Graph Analyzer |
| NetworkPolicy namespace isolation | AS-04, AS-05 | `k8s/cluster/network-policy.yaml` |
| Nonce blacklist 2xMaxTTL | AS-04 (nonce staleness) | `revocation/manager.go` |
| Cache max size (1000) + stale-first eviction | STRIDE DoS pada cache | WASM in-memory cache |
| SPIFFE Delimiter Sanitization | AS-06 (delimiter injection) | `isValidSPIFFEURI` di wasm-filter & distributor |
| Ingress Header Sanitization (x-zta-cache-verified) | AS-07 (header spoofing bypass) | `OnHttpRequestHeaders` header stripping |

---

## 7. Residual Risk

| Skenario | Kemungkinan | Dampak | Risk Level |
|---|---|---|---|
| Node compromise + memory dump token | Sangat Rendah | Tinggi | **Rendah** |
| gRPC stream putus > 30 detik | Rendah | Sedang | **Rendah** |
| Kunci HMAC/Ed25519 bocor | Sangat Rendah | Kritis | **Sedang** |
| Zero-day pada Envoy WASM runtime | Sangat Rendah | Tinggi | **Rendah** |
| Insider threat (admin mencabut lalu restore nonce) | Sangat Rendah | Tinggi | **Rendah** |

**Kesimpulan residual risk:** Semua risiko residual berada pada level Rendah atau Sedang. Risiko Sedang (kunci kriptografi bocor) dimitigasi via rotasi kunci berkala dan isolasi secret via Kubernetes Secret.

---

## 8. ZT Invariant Verification

Berikut verifikasi formal bahwa algoritma caching tidak melanggar prinsip-prinsip Zero Trust.

| ZT Principle (NIST 800-207) | Pemenuhan dalam Algoritma |
|---|---|
| **Semua sumber daya dianggap tidak aman** | Terpenuhi: Setiap request tetap divalidasi identity via mTLS; sidecar menolak request tanpa sertifikat valid |
| **Least privilege diberlakukan** | Terpenuhi: Delegation token hanya berisi `allowed_actions` yang didefinisikan di OPA; tidak ada eskalasi via token |
| **Akses diberikan per-session** | Terpenuhi: Token memiliki TTL yang terbatas; setiap sesi baru memerlukan token baru atau validasi nonce |
| **Verifikasi identitas dinamis** | Terpenuhi: SPIFFE SVID diperbarui otomatis sebelum expiry; identitas selalu segar |
| **Kebijakan diberlakukan secara terpusat** | Terpenuhi: Sumber kebenaran tetap di OPA; sidecar hanya mengevaluasi cache delegasi yang telah disahkan PDP |
| **Telemetri dikumpulkan dan dianalisis** | Terpenuhi: Prometheus metrics + Jaeger traces pada setiap keputusan otorisasi |
| **Revokasi segera berlaku** | Terpenuhi: Push-based nonce invalidation (revokasi <= 100 ms normal, empiris 1.72 ms); fallback via TTL expiry dalam <= 30 detik jika stream terputus |
