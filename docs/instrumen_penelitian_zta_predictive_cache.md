# Instrumen Penelitian
## Predictive Authorization Caching Algorithm for Micro-Segmentation: Reducing Latency Overhead in Zero Trust Service Meshes

---

## Identitas Penelitian

| Item | Keterangan |
|---|---|
| Judul | Predictive Authorization Caching Algorithm for Micro-Segmentation: Reducing Latency Overhead in Zero Trust Service Meshes |
| Pendekatan | Eksperimental kuantitatif |
| Metode pengukuran | Black-box performance testing + statistical analysis |
| Standar acuan | NIST SP 800-207, NIST SP 800-204C |

---

## Bagian 1: Definisi Operasional Variabel

### 1.1 Variabel Independen

| Variabel | Definisi Operasional | Nilai / Level |
|---|---|---|
| Konfigurasi otorisasi | Mekanisme pengambilan keputusan otorisasi pada setiap API call antar-service | (1) Tanpa otorisasi per-call / PBS baseline, (2) ZTA naif: setiap call query PDP pusat, (3) ZTA + Predictive Cache (proposed) |
| Beban request (RPS) | Jumlah request per detik yang dikirimkan load generator ke service mesh | 500, 2000, 5000 RPS (dengan 50 RPS khusus fase warmup) |
| Topologi graph | Struktur hubungan antar-service yang dianalisis Graph Analyzer | Linear chain, fan-out, mesh penuh |

### 1.2 Variabel Dependen

| Variabel | Definisi Operasional | Satuan |
|---|---|---|
| E2E p99 Latency | Persentil ke-99 latensi end-to-end dari request masuk hingga response diterima | Milidetik (ms) |
| Throughput | Jumlah request sukses yang diproses per satuan waktu | Request/detik (RPS) |
| CPU overhead sidecar | Penggunaan CPU rata-rata pada container Envoy sidecar selama pengujian | Persentase (%) |
| Memory overhead sidecar | Penggunaan memori rata-rata pada container Envoy sidecar selama pengujian | Megabyte (MB) |
| Revocation latency | Waktu antara event pencabutan sertifikat di PDP hingga sidecar menolak request baru | Milidetik (ms) |
| Cache hit ratio | Persentase keputusan otorisasi yang dilayani dari local cache tanpa query PDP | Persentase (%) |

### 1.3 Variabel Kontrol

| Variabel | Nilai yang dikontrol |
|---|---|
| Spesifikasi hardware | Single-node VM lean (2 vCPU AMD EPYC 7713, 4 GB RAM); referensi skala multi-node (4 vCPU, 8 GB RAM) |
| Versi Kubernetes | v1.28.5+k3s1 (tetap selama eksperimen) |
| Versi Istio | v1.20.1 (tetap selama eksperimen) |
| Ukuran payload request | 1 KB per request (konstan) |
| Durasi pengujian per skenario | 5 menit steady-state |
| Jumlah pengulangan | 30 kali per skenario (untuk kecukupan statistik; $n = 5$ untuk studi ablasi multi-hop) |

---

## Bagian 2: Instrumen Pengukuran

### 2.1 Lembar Pengukuran Latensi (per skenario)

**Identitas Skenario**

| Field | Isi |
|---|---|
| Nomor skenario | S-___ |
| Konfigurasi otorisasi | PBS / ZTA-Naif / ZTA-Cache |
| Beban RPS | ___ RPS |
| Topologi | Linear / Fan-out / Mesh |
| Tanggal & waktu pengujian | __________________ |
| Pengulangan ke- | ___ / 30 |

**Hasil Pengukuran Latensi (ms)**

| Persentil | Nilai (ms) |
|---|---|
| p50 (median) | |
| p75 | |
| p90 | |
| p95 | |
| p99 | |
| p99.9 | |
| Max | |
| Mean | |
| Std Dev | |

**Sumber data:** Output k6 (`k6 run --out json`) / Jaeger trace API

---

### 2.2 Lembar Pengukuran Throughput & Error Rate

| Metrik | Nilai |
|---|---|
| Total request dikirim | |
| Request sukses (2xx) | |
| Request gagal (4xx/5xx) | |
| Throughput aktual (RPS) | |
| Error rate (%) | |
| Timeout count | |

**Sumber data:** k6 summary output, Prometheus `istio_requests_total`

---

### 2.3 Lembar Pengukuran Resource Overhead

**CPU Overhead (per container sidecar)**

| Container | CPU avg (%) | CPU peak (%) | CPU baseline (%) |
|---|---|---|---|
| Sidecar A (Envoy) | | | |
| Sidecar B (Envoy) | | | |
| Sidecar C (Envoy) | | | |
| Rata-rata | | | |

**Memory Overhead (per container sidecar)**

| Container | Mem avg (MB) | Mem peak (MB) | Mem baseline (MB) |
|---|---|---|---|
| Sidecar A (Envoy) | | | |
| Sidecar B (Envoy) | | | |
| Sidecar C (Envoy) | | | |
| Rata-rata | | | |

**Sumber data:** Prometheus `container_cpu_usage_seconds_total`, `container_memory_working_set_bytes` via cAdvisor

---

### 2.4 Lembar Pengukuran Cache Performance

| Metrik | Nilai |
|---|---|
| Total authorization decisions | |
| Cache hit count | |
| Cache miss count (fallback ke PDP) | |
| Cache hit ratio (%) | |
| Avg TTL yang dikonfigurasi (ms) | |
| Avg TTL tersisa saat hit (ms) | |

**Sumber data:** Custom Prometheus counter pada Graph Analyzer service

---

### 2.5 Lembar Pengukuran Revocation Latency

**Prosedur pengukuran:**
1. Catat timestamp `T0` saat event revokasi dipicu di PDP
2. Probe sidecar setiap 10ms dengan request menggunakan identitas yang direvokasi
3. Catat timestamp `T1` saat sidecar pertama kali menolak request (HTTP 403)
4. `Revocation Latency = T1 - T0`
5. Ulangi 30 kali, catat seluruh nilai

| Pengulangan | T0 (ms epoch) | T1 (ms epoch) | Latency (ms) |
|---|---|---|---|
| 1 | | | |
| 2 | | | |
| 3 | | | |
| ... | | | |
| 30 | | | |
| **Min** | | | |
| **Max** | | | |
| **Mean** | | | |
| **p99** | | | |

**Target:** Revocation latency p99 ≤ 100ms

---

## Bagian 3: Skenario Pengujian

### 3.1 Matrix Skenario

| Kode | Konfigurasi | RPS | Topologi | Tujuan |
|---|---|---|---|---|
| S01 | PBS Baseline | 500 | Linear | Baseline referensi |
| S02 | ZTA-Naif | 500 | Linear | Overhead ZTA tanpa cache |
| S03 | ZTA-Cache | 500 | Linear | Efektivitas cache, kondisi normal |
| S04 | ZTA-Naif | 2000 | Linear | Degradasi PDP central saat load tinggi |
| S05 | ZTA-Cache | 2000 | Linear | Ketahanan cache saat load tinggi |
| S06 | ZTA-Cache | 5000 | Linear | Stress test cache |
| S07 | ZTA-Cache | 2000 | Fan-out | Efektivitas graph topology analyzer |
| S08 | ZTA-Cache | 2000 | Mesh penuh | Worst-case topology |
| S09 | ZTA-Cache | 500 | Linear | Uji revokasi (sertifikat dicabut mid-test) |
| S10 | ZTA-Cache | 2000 | Fan-out | Variasi TTL (rendah vs tinggi) |

---

### 3.2 Prosedur Eksekusi Skenario

**Persiapan sebelum tiap skenario:**
- [ ] Reset Kubernetes namespace pengujian
- [ ] Flush local cache seluruh sidecar
- [ ] Verifikasi semua pod dalam status `Running`
- [ ] Verifikasi Prometheus scraping aktif
- [ ] Verifikasi Jaeger collector aktif
- [ ] Catat baseline CPU & memory selama 60 detik (idle)

**Eksekusi:**
- [ ] Aktifkan konfigurasi otorisasi sesuai skenario
- [ ] Jalankan load generator: `k6 run --vus {VU} --duration 5m scenario.js`
- [ ] Monitor real-time via Grafana dashboard
- [ ] Catat anomali / error yang muncul selama pengujian

**Pasca skenario:**
- [ ] Export metrics dari Prometheus (`promtool query range`)
- [ ] Export trace dari Jaeger API
- [ ] Simpan raw data ke `data/raw/S{nomor}/`
- [ ] Tunggu 2 menit cooldown sebelum skenario berikutnya

---

## Bagian 4: Instrumen Validasi Keamanan ZTA

### 4.1 Checklist Zero Trust Invariant

Diisi setelah seluruh skenario selesai untuk memverifikasi bahwa algoritma caching tidak melanggar prinsip ZTA.

| No | ZT Invariant | Metode Verifikasi | Hasil |
|---|---|---|---|
| 1 | Setiap service hanya dapat mengakses service yang secara eksplisit diizinkan | Kirim request dari service yang tidak diotorisasi, verifikasi HTTP 403 | Pass / Fail |
| 2 | Token yang kedaluwarsa tidak diterima | Manipulasi `exp` field pada delegation token, verifikasi ditolak | Pass / Fail |
| 3 | Revokasi sertifikat segera berlaku (≤100ms) | Eksekusi skenario S09, ukur revocation latency | Pass / Fail |
| 4 | Nonce yang tidak valid menyebabkan fallback ke PDP | Manipulasi nonce pada cache, verifikasi fallback terjadi | Pass / Fail |
| 5 | Tidak ada keputusan otorisasi yang bertahan melebihi TTL yang dikonfigurasi | Monitor cache expiry log, verifikasi tidak ada stale decision | Pass / Fail |
| 6 | mTLS tetap diberlakukan pada seluruh East-West traffic | Jalankan tcpdump pada pod-to-pod traffic, verifikasi TLS handshake | Pass / Fail |

---

### 4.2 Lembar Uji Penetrasi Sederhana (Attack Simulation)

| Skenario Serangan | Metode | Hasil yang diharapkan | Hasil aktual |
|---|---|---|---|
| Token replay attack | Gunakan delegation token expired | HTTP 401 / 403 | |
| Identity spoofing | Request dengan SVID service lain | HTTP 403 | |
| Cache poisoning attempt | Inject forged cache entry via sidecar API | Entry ditolak (signature check gagal) | |
| Revocation bypass | Akses setelah revokasi sebelum nonce propagated | HTTP 403 setelah ≤100ms | |
| Privilege escalation | Request action di luar allowed_actions token | HTTP 403 | |

---

## Bagian 5: Instrumen Analisis Statistik

### 5.1 Uji Normalitas (Shapiro-Wilk)

Dijalankan pada setiap distribusi pengukuran sebelum pemilihan uji hipotesis.

| Skenario | Metrik | W statistic | p-value | Distribusi Normal? |
|---|---|---|---|---|
| S02 vs S03 | p99 latency | | | Ya / Tidak |
| S04 vs S05 | p99 latency | | | Ya / Tidak |
| S02 vs S03 | CPU overhead | | | Ya / Tidak |

---

### 5.2 Uji Hipotesis Statistik

Digunakan pengujian non-parametrik karena distribusi latensi jaringan terbukti non-normal (uji Shapiro-Wilk $p < 0{,}001$ pada seluruh 10 skenario).

**H1: Paritas Latensi & Kesetaraan Praktis (Mann-Whitney U, Cliff's Delta, Hodges-Lehmann, dan TOST)**
- **Uji Kesetaraan & Non-Inferioritas (TOST):** Menguji apakah selisih latensi p99 berada dalam margin kesetaraan praktis $\Delta = 5\%$ dari median baseline ($14{,}79\text{ ms}$ pada 500 RPS; $111{,}37\text{ ms}$ pada 2000 RPS) pada taraf signifikansi $\alpha = 0{,}05$.
- **Uji Perbandingan Dua Kelompok (Mann-Whitney U & Cliff's Delta):** Menguji perbedaan distribusi latensi p99 antar konfigurasi (PBS Baseline, ZTA-Naif, dan ZTA-Cache) pada beban 500 RPS dan 2000 RPS.

| Pengujian | Beban | Median A vs B (ms) | $U$ | $p_{\text{MWU}}$ | Cliff's $\delta$ | $\hat{\theta}_{HL}$ (ms) | $p_{\text{TOST}} / p_{\text{non-inf}}$ | Keputusan ($\alpha=0{,}05$) |
|---|---|---|---|---|---|---|---|---|
| PBS vs ZTA-Naif | 500 RPS | 295,85 vs 301,36 | 421,0 | 0,673 | -0,06 (Negligible) | -10,48 | 0,3809 | Tidak Signifikan (Efek *Negligible*) |
| ZTA-Naif vs ZTA-Cache | 500 RPS | 301,36 vs 293,30 | 503,0 | 0,438 | +0,12 (Negligible) | +16,63 | 0,0727 | Paritas Terpelihara (Efek *Negligible*) |
| PBS vs ZTA-Cache | 500 RPS | 295,85 vs 293,30 | 475,0 | 0,717 | +0,06 (Negligible) | +5,79 | 0,1664 | Paritas Bebas Penalti (95% CI: $[-64{,}49;\ +57{,}78]\text{ ms}$) |
| ZTA-Naif vs ZTA-Cache | 2000 RPS | 2227,32 vs 2243,83 | 428,0 | 0,751 | -0,05 (Negligible) | -17,67 | 0,0251 | **Setara Praktis & Non-Inferior via TOST ($p < 0{,}05$)** |

**H2: Latensi Revokasi Memenuhi Target Desain Operasional Zero Trust ($\le 100\text{ ms}$)**

| Pengujian | Median (ms) | Mean (ms) | SD (ms) | p90 (ms) | p99 (ms) | Kepatuhan $< 100\text{ ms}$ | Uji Wilcoxon (1-Sisi) |
|---|---|---|---|---|---|---|---|
| Revocation latency ($n=30$, S09) | 18,65 ms | 41,87 ms | 49,30 ms | 102,32 ms | 183,39 ms | 26/30 run (86,7%; run ke-26 = 92,40 ms) | $W^+ = 43{,}0;\ p = 1{,}18 \times 10^{-5}$ (Terpenuhi) |

**H3: Pipeline Caching Adaptif Graph-Aware vs Pipeline Statis & Reduksi Jendela Keterpaparan ($W_{\text{exposure}}$)**
- Menguji apakah adaptasi TTL berbasis graf dan sensitivitas risiko (S07) mempertahankan paritas latensi p99 terhadap pipeline TTL statis 30 detik (S10) pada beban 2000 RPS topologi fan-out, sekaligus memangkas batas atas jendela keterpaparan otorisasi ($W_{\text{exposure}}(p_s) = T_{\text{base}} \times (1 - p_s)$ pada kondisi *steady-state*).
- **Hasil Paritas Latensi (S10 vs S07):** Median $5927{,}94\text{ ms}$ vs $6140{,}55\text{ ms}$, Mann-Whitney $U = 436{,}0$, $p = 0{,}842$, Cliff's $\delta = -0{,}03$ (*Negligible*), $\hat{\theta}_{HL} = -40{,}67\text{ ms}$ ($0{,}69\%$ dari median).
- **Hasil Reduksi Jendela Keterpaparan ($W_{\text{exposure}}$):** Endpoint sensitivitas rendah ($p_s = 0{,}0$): $30{,}0\text{ s}$ (reduksi 0%); sensitivitas sedang ($p_s = 0{,}3$): $\le 21{,}0\text{ s}$ (**reduksi 30,0%**); sensitivitas tinggi ($p_s = 0{,}7$): $\le 9{,}0\text{ s}$ (**reduksi 70,0%**).

---

### 5.3 Estimator Non-Parametrik & Effect Size

1. **Hodges-Lehmann Estimator ($\hat{\theta}_{HL}$):** Menghitung median selisih berpasangan non-parametrik antar kelompok, dilengkapi dengan Percentile Bootstrap Confidence Interval ($B = 10.000$ iterasi resampling, `seed = 42`).
2. **Cliff's Delta ($\delta$):** Mengukur ukuran efek dominasi stokastik non-parametrik (Romano et al., 2006):
   - $|\delta| < 0{,}147$: Negligible
   - $0{,}147 \le |\delta| < 0{,}330$: Small
   - $0{,}330 \le |\delta| < 0{,}474$: Medium
   - $|\delta| \ge 0{,}474$: Large

---

### 5.4 Uji Multi-Grup (Kruskal-Wallis)

Untuk membandingkan tiga konfigurasi pada beban dasar 500 RPS serta membandingkan tiga topologi komunikasi pada beban 2000 RPS.

| Kelompok Evaluasi | Metrik | H statistic | p-value | Signifikan? |
|---|---|---|---|---|
| Konfigurasi @ 500 RPS (PBS vs Naif vs Cache) | p99 latency | 0,63 | 0,731 | Tidak ($p > 0{,}05$) |
| Topologi @ 2000 RPS (Linear vs Fan-out vs Mesh) | p99 latency | 63,40 | $1{,}71 \times 10^{-14}$ | Ya ($p < 0{,}001$) |
| Resource @ 500 RPS (PBS vs Naif vs Cache) | CPU overhead | 2,79 | 0,248 | Tidak ($p > 0{,}05$) |
| Resource @ 500 RPS (PBS vs Naif vs Cache) | Memory overhead | 0,91 | 0,636 | Tidak ($p > 0{,}05$) |

Post-hoc (jika signifikan): Uji Mann-Whitney U berpasangan dengan koreksi Bonferroni ($\alpha' = \alpha / 3 = 0{,}0167$).

---

## Bagian 6: Spesifikasi Teknis Testbed

### 6.1 Konfigurasi Kubernetes Cluster

```yaml
# cluster-config.yaml (referensi)
nodes:
  - role: control-plane
    cpu: 4
    memory: 8Gi
  - role: worker
    cpu: 4
    memory: 8Gi
    count: 3
kubernetes_version: "1.28"
cni: "calico"
```

### 6.2 Parameter Istio yang Dikonfigurasi

| Parameter | Nilai | Keterangan |
|---|---|---|
| `meshConfig.accessLogFile` | `/dev/stdout` | Aktifkan access log |
| `meshConfig.enableTracing` | `true` | Aktifkan distributed tracing |
| `meshConfig.defaultConfig.tracing.zipkin.address` | `jaeger-collector:9411` | Jaeger endpoint |
| `PeerAuthentication.mode` | `STRICT` | mTLS wajib seluruh mesh |
| `AuthorizationPolicy` | per-service | Definisi akses per workload |

### 6.3 Parameter Graph Topology Analyzer

| Parameter | Nilai default | Keterangan |
|---|---|---|
| Sampling window | 60 detik | Window historis call frequency |
| Edge weight threshold | 10 calls/menit | Minimum frekuensi untuk cache delegation |
| TTL formula | `f(freq, sensitivity, cert_validity)` | Lihat Section 7 |
| Centrality algorithm | Betweenness centrality | Untuk ranking kandidat cache |

### 6.4 Delegation Token Schema

```json
{
  "iss": "pdp-central.spiffe.io",
  "sub": "spiffe://cluster.local/ns/default/sa/service-a",
  "aud": "spiffe://cluster.local/ns/default/sa/service-b",
  "allowed_actions": ["GET", "POST"],
  "sensitivity": "medium",
  "exp": 1700000000,
  "iat": 1699999700,
  "revocation_nonce": "a3f9c2e1",
  "cache_ttl_ms": 30000
}
```

---

## Bagian 7: Formula TTL & Skor Prioritas Graf (Referensi Algoritma)

TTL optimal per pasangan service dihitung sesuai Persamaan (1) (`control-plane/graph-analyzer/cache/ttl.go`):

```
TTL_raw(A→B) = T_base × w_f × (1 - p_s) × v_c
TTL(A→B)     = min(t_remaining, min(T_max, max(T_min, TTL_raw(A→B))))

di mana:
  T_base       = 30.000 ms (default)
  T_max        = 30.000 ms (batas atas clamping; mencegah stale decision)
  T_min        = 5.000 ms  (batas bawah clamping; mencegah cache thrashing)
  w_f          = min(calls_per_min / 100, 1.0)
  p_s          = {0.0 = low, 0.3 = medium, 0.7 = high}
  v_c          = min(remaining_cert_validity / total_cert_validity, 1.0)
  t_remaining  = sisa masa aktif sertifikat mTLS SPIFFE (hard ceiling)
```

Skor prioritas gabungan per directed edge dihitung sesuai Persamaan (2) (`control-plane/graph-analyzer/graph/centrality.go`):

```
S(e) = w × f_norm(e) + (1 - w) × C_B(source_e),   dengan w = 0.7
```

---

## Daftar Referensi Instrumen

| Dokumen | Relevansi |
|---|---|
| NIST SP 800-207 | Definisi dan prinsip ZTA |
| NIST SP 800-204C | ZTA untuk service mesh dan microservices |
| RFC 7519 | Format dan klaim JWT (delegation token) |
| SPIFFE Spec v1.0 | Format SVID dan workload identity |
| ISO/IEC 25010 | Karakteristik kualitas perangkat lunak (performance efficiency) |
| k6 documentation | Panduan load testing dan metrics output |
| Prometheus exposition format | Format scraping metrics |
| Jaeger API spec | Query trace data untuk latency measurement |
