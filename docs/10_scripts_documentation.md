# Scripts Documentation
## ZTA Predictive Authorization Caching - Panduan Lengkap Semua Script

Semua script ada di direktori `scripts/`. Dokumen ini menjelaskan setiap script: tujuan, parameter, output, error handling, dan kapan digunakan.

---

## Daftar Script

| Script | Bahasa | Kapan digunakan |
|---|---|---|
| `verify_offline.py` | Python | Audit integritas arsitektur & verifikasi statis kelayakan ZTA |
| `setup_vm_2core_4gb.sh` | Bash | Otomasi setup & deployment lean testbed pada VM 2 vCPU / 4 GB RAM |
| `verify-versions.sh` | Bash | Sebelum eksperimen - cek versi tools |
| `register-spire-entries.sh` | Bash | Setup awal - daftarkan workload ke SPIRE |
| `validate-state.sh` | Bash | Sebelum tiap skenario - validasi konfigurasi |
| `run-all-scenarios.sh` | Bash | Eksperimen penuh - jalankan S01–S10 |
| `reset-light.sh` / `reset-full.sh` | Bash | Antar skenario: reset state sistem |
| `snapshot-config.sh` | Bash | Sebelum eksperimen - simpan state konfigurasi |
| `export-metrics.py` | Python | Setelah skenario - export Prometheus ke CSV |
| `analyze-all-data.py` | Python | Pipeline gabungan: ingest JSON/CSV k6 & Prometheus + uji statistik |
| `statistical-analysis.py` | Python | Analisis statistik mendalam & kalkulasi effect size |
| `generate-figures.py` / `generate-figures-en.py` | Python | Pembuatan visualisasi grafik publikasi ilmiah (Gambar 1–10 + suplemen, ID & EN) |
| `analyze_ablation_results.py` | Python | Analisis statistik inferensial studi ablasi multi-hop ($K_{\text{local}} = 2$) |
| `generate_ablation_figure.py` | Python | Pembuatan grafik boxplot studi ablasi multi-hop (Gambar 10, ID & EN) |
| `scalability-analysis.py` | Python | Analisis model kompleksitas algoritmik $O(1)$ data plane & $O(V^2)$ control plane |
| `validate-data.py` | Python | Audit integritas 600 berkas data mentah k6 JSON & Prometheus CSV |
| `verify_traceability.py` | Python | Verifikasi keterlacakan 119 klaim angka & rumus naskah terhadap CSV |
| `verify_en_draft.py` | Python | Audit kesetaraan naskah Bahasa Indonesia vs Bahasa Inggris |

---

## 1. `verify-versions.sh`

**Tujuan:** Memverifikasi semua tools terpasang dengan versi yang benar sesuai `docs/05_experiment_config_reproducibility.md`.

**Penggunaan:**
```bash
bash scripts/verify-versions.sh
# atau via Makefile:
make verify-env
```

**Parameter:** Tidak ada.

**Output:**
```
=== Verifikasi Versi Komponen ===
✅ k3s: v1.28.5+k3s1
✅ Istio: 1.20.1
✅ OPA: 0.60.0
✅ k6: v0.48.0
✅ Go: go1.22.0
✅ TinyGo: tinygo version 0.31.0
ℹ️  Python: 3.11.7
⚠️  Docker: 23.0.1 (expected 24.0+)

[RESULT] 1 komponen tidak sesuai versi. Perbaiki sebelum eksperimen.
```

**Exit code:**
- `0` - semua versi sesuai
- `1` - ada versi yang tidak sesuai

**Kapan digunakan:** Wajib dijalankan sekali sebelum setiap sesi eksperimen, terutama setelah update sistem atau ketika mereplikasi di lingkungan baru.

---

## 2. `register-spire-entries.sh`

**Tujuan:** Mendaftarkan semua workload service ke SPIRE server sehingga mendapat SVID (SPIFFE Verifiable Identity Document).

**Penggunaan:**
```bash
bash scripts/register-spire-entries.sh
```

**Parameter:** Tidak ada (membaca dari konstanta dalam script).

**Dependensi:**
- SPIRE server pod harus dalam status `Running`
- `kubectl` harus terkonfigurasi ke cluster yang benar

**Proses:**
```bash
# Script ini menjalankan untuk setiap service:
kubectl exec -n spire spire-server-0 -- \
  /opt/spire/bin/spire-server entry create \
  -spiffeID "spiffe://cluster.local/ns/zta-research/sa/<service>" \
  -parentID "spiffe://cluster.local/ns/spire/sa/spire-agent" \
  -selector "k8s:ns:zta-research" \
  -selector "k8s:sa:<service>" \
  -ttl 3600
```

**Service yang didaftarkan:**
```
frontend, productcatalogservice, cartservice, currencyservice,
checkoutservice, paymentservice, recommendationservice,
graph-analyzer, cache-distributor
```

**Output:**
```
Registering frontend...
Entry ID: 12345678-1234-1234-1234-123456789abc
Registering productcatalogservice...
...
All entries registered. Total: 9
```

**Idempotent:** Ya - menjalankan ulang tidak membuat duplikat entry.

**Kapan digunakan:** Sekali saat setup awal, atau jika SPIRE server di-reset.

---

## 3. `validate-state.sh`

**Tujuan:** Memvalidasi bahwa sistem dalam konfigurasi yang benar sebelum pengukuran dimulai. Mencegah data eksperimen yang salah akibat konfigurasi yang tidak tepat.

**Penggunaan:**
```bash
bash scripts/validate-state.sh <mode>
# mode: pbs | zta-naive | zta-cache
```

**Parameter:**
| Parameter | Posisi | Wajib | Nilai valid |
|---|---|---|---|
| `mode` | $1 | Ya | `pbs`, `zta-naive`, `zta-cache` |

**Contoh:**
```bash
bash scripts/validate-state.sh zta-cache
```

**Pemeriksaan yang dilakukan:**

| # | Pemeriksaan | Kondisi gagal |
|---|---|---|
| 1 | Semua pod Running | Ada pod tidak Running |
| 2 | mTLS STRICT aktif | PeerAuthentication tidak ditemukan |
| 3 | OPA merespons | HTTP selain 200 |
| 4 | Konfigurasi sesuai mode | AuthorizationPolicy/WasmPlugin tidak sesuai |
| 5 | Token aktif > 0 (khusus zta-cache) | Cache Distributor tidak punya token |
| 6 | Prometheus scraping | Jumlah targets UP |

**Output:**
```
=== Validasi State Sistem: zta-cache ===
✅ Semua pod Running
✅ mTLS aktif
✅ OPA sehat
✅ ZTA Cache: AuthorizationPolicy + WasmPlugin aktif
✅ ZTA Cache: 8 token aktif
✅ Prometheus: 12 targets UP

✅✅✅ VALIDASI LULUS - sistem siap untuk pengukuran
```

**Exit code:**
- `0` - validasi lulus, aman untuk mulai pengukuran
- `1` - ada masalah, JANGAN mulai pengukuran

---

## 4. `run-all-scenarios.sh`

**Tujuan:** Menjalankan seluruh 10 skenario eksperimen secara berurutan dengan reset dan validasi otomatis di antara skenario.

**Penggunaan:**
```bash
bash scripts/run-all-scenarios.sh [repetitions]
# Default: 3 repetisi
# atau:
make run-all
```

**Parameter:**
| Parameter | Posisi | Wajib | Default | Keterangan |
|---|---|---|---|---|
| `repetitions` | $1 | Tidak | `3` | Jumlah pengulangan per skenario |

**Alur eksekusi:**
```
Untuk setiap repetisi 1..REPETITIONS:
  Untuk setiap skenario S01..S10:
    1. Log: "Running: SCENARIO (rep N)"
    2. bash reset-light.sh (flush cache + reset graph)
    3. k6 run warmup.js (stabilkan sistem)
    4. sleep 30s (cooldown)
    5. Record START_TIME
    6. k6 run scenarios/SCENARIO.js → results/
    7. Record END_TIME
    8. python3 export-metrics.py (export Prometheus metrics)
    9. sleep 120s (cooldown antar skenario)
```

**Output:** JSON results di `load-testing/results/` dan CSV metrics di `load-testing/results/`.

**Durasi estimasi:** 6–8 jam untuk 10 skenario × 3 pengulangan.

---

## 5. `reset-light.sh` dan `reset-full.sh`

**Tujuan:** Reset state testbed ke kondisi bersih. Mendukung dua mode: ringan (antar pengulangan) dan penuh (antar skenario).

**Penggunaan:**
```bash
bash scripts/reset-light.sh
# atau
bash scripts/reset-full.sh
```

**Parameter:**
| Parameter | Posisi | Wajib | Nilai valid |
|---|---|---|---|
| `mode` | $1 | Ya | `light`, `full` |

**Mode `light` (30 detik):**
```
1. kubectl rollout restart deployment/cache-distributor
2. Tunggu cache-distributor Ready
3. DELETE /graph/reset via HTTP
4. sleep 30
```

**Mode `full` (3–5 menit):**
```
1. kubectl delete authorizationpolicy --all
2. kubectl delete wasmplugin --all
3. kubectl rollout restart deployment --all (namespace zta-research)
4. Tunggu semua pod Ready
5. k6 run warmup.js --duration 2m
6. sleep 120
```

**Kapan menggunakan masing-masing:**
- `light`: Antara pengulangan 1→2→3 dalam skenario yang sama (konfigurasi tidak berubah)
- `full`: Antara S02→S03 (konfigurasi berubah dari ZTA-Naif ke ZTA-Cache)

---

## 6. `snapshot-config.sh`

**Tujuan:** Menyimpan snapshot lengkap konfigurasi sistem ke direktori bertimestamp. Digunakan untuk dokumentasi reproducibility dan audit.

**Penggunaan:**
```bash
bash scripts/snapshot-config.sh
# Output disimpan ke: data/config-snapshot-YYYYMMDD-HHMM/
```

**Parameter:** Tidak ada.

**File yang disimpan:**

| File | Sumber |
|---|---|
| `k8s-resources.yaml` | `kubectl get all -n zta-research -o yaml` |
| `configmaps.yaml` | `kubectl get configmap -n zta-research -o yaml` |
| `authz-policies.yaml` | `kubectl get authorizationpolicy -o yaml` |
| `wasm-plugins.yaml` | `kubectl get wasmplugin -o yaml` |
| `peer-auth.yaml` | `kubectl get peerauthentication -o yaml` |
| `versions.txt` | Output `verify-versions.sh` |
| `graph-analyzer-go.sum` | `control-plane/graph-analyzer/go.sum` |
| `cache-distributor-go.sum` | `control-plane/cache-distributor/go.sum` |
| `authz.rego` | `control-plane/pdp/policies/authz.rego` |
| `data.json` | `control-plane/pdp/policies/data.json` |
| `graph-analyzer-config.yaml` | Config aktif Graph Analyzer |
| `cache-distributor-config.yaml` | Config aktif Cache Distributor |

**Output:**
```
Saving configuration snapshot...
Snapshot saved to: data/config-snapshot-20240115-0930/
Total files: 12
```

**Kapan digunakan:** Wajib dijalankan sekali sebelum sesi eksperimen pertama dan setelah setiap perubahan konfigurasi signifikan.

---

## 7. `export-metrics.py`

**Tujuan:** Mengekspor time-series metrics dari Prometheus ke file CSV per skenario.

**Penggunaan:**
```bash
python3 scripts/export-metrics.py \
  --prometheus-url http://localhost:9090 \
  --scenario S03 \
  --start "2024-01-15T10:00:00Z" \
  --end "2024-01-15T10:05:00Z" \
  --output data/raw/S03/ \
  --repetition 1
```

**Parameter:**

| Parameter | Flag | Wajib | Default | Keterangan |
|---|---|---|---|---|
| Prometheus URL | `--prometheus-url` | Tidak | `http://localhost:9090` | URL Prometheus server |
| Scenario ID | `--scenario` | Ya | - | S01–S10 |
| Start time | `--start` | Ya | - | ISO 8601 UTC |
| End time | `--end` | Ya | - | ISO 8601 UTC |
| Output dir | `--output` | Ya | - | Direktori output |
| Repetition | `--repetition` | Tidak | `1` | Nomor pengulangan |
| Step | `--step` | Tidak | `10s` | Resolusi time-series |

**Metrics yang diekspor:**
```
p50_latency_ms, p90_latency_ms, p99_latency_ms,
throughput_rps, error_rate_pct,
cpu_avg_pct, mem_avg_mb,
cache_hit_ratio, revocation_latency_p99_ms
```

**Output file:** `data/raw/S03/prometheus-S03-rep1.csv`

**Format CSV:**
```csv
timestamp,p50_latency_ms,p90_latency_ms,p99_latency_ms,...
2024-01-15T10:00:00Z,12.3,18.7,24.1,...
2024-01-15T10:00:10Z,12.5,19.1,24.8,...
```

**Error handling:**
- Jika metric tidak tersedia: kolom diisi `None`, script melanjutkan
- Jika Prometheus tidak terjangkau: exit dengan kode 1 dan pesan error jelas
- Jika direktori output tidak ada: dibuat otomatis

**Dependensi Python:**
```
requests>=2.31.0
```

---

## 8. `export-jaeger.py` (Diarsipkan di `dump/retired_scripts/`)

> [!NOTE]
> Skrip ini telah dipindahkan ke `dump/retired_scripts/` karena penarikan metrik utama kini dipusatkan pada `export-metrics.py` (Prometheus) dan `analyze-all-data.py`. Dokumentasi ini dipertahankan sebagai referensi riwayat.

**Tujuan:** Mengekspor distributed traces dari Jaeger REST API ke file JSON per skenario.

**Penggunaan:**
```bash
python3 dump/retired_scripts/export-jaeger.py \
  --jaeger-url http://localhost:16686 \
  --service frontend \
  --scenario S03 \
  --start "2024-01-15T10:00:00Z" \
  --end "2024-01-15T10:05:00Z" \
  --output data/raw/S03/ \
  --limit 5000
```

**Parameter:**

| Parameter | Flag | Wajib | Default | Keterangan |
|---|---|---|---|---|
| Jaeger URL | `--jaeger-url` | Tidak | `http://localhost:16686` | URL Jaeger query API |
| Service | `--service` | Ya | - | Nama service entry point (misal: `frontend`) |
| Scenario | `--scenario` | Ya | - | S01–S10 |
| Start | `--start` | Ya | - | ISO 8601 UTC |
| End | `--end` | Ya | - | ISO 8601 UTC |
| Output | `--output` | Ya | - | Direktori output |
| Limit | `--limit` | Tidak | `5000` | Maksimum trace yang diambil |
| Operation | `--operation` | Tidak | semua | Filter berdasarkan operation name |

**Output file:** `data/raw/S03/jaeger-S03.json`

**Format output (array of spans):**
```json
[
  {
    "trace_id": "abc123",
    "duration_ms": 24.1,
    "start_time": 1705312800000000,
    "scenario_id": "S03",
    "service": "frontend",
    "operation": "HTTP GET /api/products"
  }
]
```

**Error handling:**
- Jika Jaeger tidak terjangkau: exit 1
- Jika tidak ada trace dalam window waktu: simpan file JSON kosong `[]`, log warning

---

## 9. `analyze-all-data.py` (Unified Data Pipeline & Statistical Analysis)

> [!NOTE]
> Skrip ini menggantikan skrip penggabung lama `merge-k6-results.py` dan bekerja berdampingan dengan `statistical-analysis.py`.

**Tujuan:** Membaca seluruh data pengujian mentah (`data/raw/*/k6-*.json` dan log metrik), menggabungkannya ke dataset processed terpadu, serta menjalankan analisis inferensial statistik formal (Shapiro-Wilk, Mann-Whitney U, Cliff's Delta, Kruskal-Wallis).

**Penggunaan:**
```bash
python3 scripts/analyze-all-data.py
# Input:  data/raw/*/k6-*.json, data/raw/*/metrics-*.csv
# Output: data/processed/latency-all-scenarios.csv
#         data/processed/cache-performance.csv
#         data/reports/mann-whitney-results.csv
#         data/reports/kruskal-wallis-results.csv
#         data/reports/statistical-analysis.txt
```

**Parameter:** Otomatis membaca struktur direktori `data/raw/` dan mendeteksi repetisi skenario yang tersedia.

**Proses:**
```
1. Glob semua file: data/raw/*/k6-S??-rep*.json
2. Ekstrak metrik performa (p50..p999, throughput, error rate, durasi revokasi)
3. Gabungkan ke DataFrame dan simpan ke data/processed/latency-all-scenarios.csv
4. Jalankan uji normalitas Shapiro-Wilk
5. Jalankan uji hipotesis Mann-Whitney U dan effect size Cliff's Delta
6. Jalankan Kruskal-Wallis untuk multi-grup comparison
7. Tulis laporan komprehensif ke data/reports/statistical-analysis.txt
```

---

## 10. `export-cache-metrics.py` (Diarsipkan di `dump/retired_scripts/`)

> [!NOTE]
> Skrip ini telah dipindahkan ke `dump/retired_scripts/` karena fungsionalitas penarikan metrik cache telah terintegrasi ke dalam pipeline Prometheus dan `analyze-all-data.py`.

**Tujuan:** Mengekspor cache performance metrics (hit ratio, token count, revocation latency) dari Cache Distributor ke JSON per skenario.

**Penggunaan:**
```bash
python3 dump/retired_scripts/export-cache-metrics.py \
  --distributor-url http://localhost:8081 \
  --scenario S03 \
  --repetition 1 \
  --output data/raw/S03/
```

**Parameter:**

| Parameter | Flag | Wajib | Default |
|---|---|---|---|
| Distributor URL | `--distributor-url` | Tidak | `http://localhost:8081` |
| Scenario | `--scenario` | Ya | - |
| Repetition | `--repetition` | Tidak | `1` |
| Output | `--output` | Ya | - |

**Data yang dikumpulkan:**
- `GET /tokens/active` → jumlah token aktif, TTL tersisa
- `GET /metrics` (Prometheus text format) → cache_hits, cache_misses, hit_ratio, tokens_issued, tokens_expired, revocation_latency

**Output file:** `data/raw/S03/cache-S03-rep1.json`

**Kapan digunakan:** Hanya untuk skenario ZTA-Cache (S03, S05–S10). Tidak relevan untuk S01 (PBS) dan S02/S04 (ZTA-Naif).

---

## 11. `statistical-analysis.py`

**Tujuan:** Menjalankan analisis statistik lengkap terhadap semua data eksperimen dan menghasilkan laporan dengan hasil uji hipotesis.

**Penggunaan:**
```bash
python3 scripts/statistical-analysis.py
# Input:  data/processed/latency-all-scenarios.csv
#         data/processed/cache-performance.csv
#         data/processed/revocation-latency.csv
# Output: data/reports/mann-whitney-results.csv
#         data/reports/kruskal-wallis-results.csv
#         data/reports/statistical-analysis.txt
#         data/reports/figures/ (PNG plots)
```

**Parameter:** Tidak ada.

**Analisis yang dilakukan:**

| Analisis | Uji | Hipotesis |
|---|---|---|
| p99 latency PBS, ZTA-Naif, ZTA-Cache | Mann-Whitney U + Cliff's Delta + TOST ($\Delta = 5\%$) + Bootstrap CI | H1 |
| Revocation latency vs threshold 100ms | Deskriptif + Wilcoxon signed-rank (1-sided) | H2 |
| Pipeline adaptif graph-aware (S07) vs statis (S10) | Mann-Whitney U + Cliff's Delta + TOST ($\Delta = 5\%$) | H3 |
| Topologi (Linear vs Fan-out vs Mesh) & Resource | Kruskal-Wallis + Mann-Whitney U post-hoc (Bonferroni $\alpha' = 0{,}0167$) | H4 & Multi-grup |
| Normalitas distribusi (S01–S10) | Shapiro-Wilk ($n = 30$) | Pre-check |

**Dependensi Python:**
```
pandas>=2.1.4
scipy>=1.11.4
numpy>=1.26.3
matplotlib>=3.8.0
seaborn>=0.13.0
```

**Output ringkasan `statistical-analysis.py` (data kanonikal $n = 30$):**
```
======================================================================
ANALISIS STATISTIK - ZTA Predictive Authorization Caching
======================================================================

[NORMALITAS - SHAPIRO-WILK (n = 30 per skenario)]
Seluruh 10 skenario (S01-S10) menolak H0 normalitas (W = 0.5933 s.d. 0.8460, p < 0.001)
-> Wajib menggunakan statistik non-parametrik (Mann-Whitney U, Cliff's Delta, Hodges-Lehmann, TOST, Bootstrap CI).

[H1] Overhead Otorisasi ZTA & Efektivitas Predictive Caching
- Margin Kesetaraan Praktis TOST: Delta = 5% median baseline (14.79 ms @ 500 RPS; 111.37 ms @ 2000 RPS)
- PBS vs ZTA-Naif @ 500 RPS : Median 295.85 vs 301.36 ms | U = 421.0, p = 0.6735, delta = -0.0644 (negligible), HL = -10.48 ms, p_TOST = 0.3809
- ZTA-Naif vs Cache @ 500 RPS: Median 301.36 vs 293.30 ms | U = 503.0, p = 0.4376, delta = +0.1178 (negligible), HL = +16.63 ms, p_non_inf = 0.0727
- PBS vs ZTA-Cache @ 500 RPS : Median 295.85 vs 293.30 ms | U = 475.0, p = 0.7172, delta = +0.0556 (negligible), HL = +5.79 ms, p_non_inf = 0.1664
  Bootstrap (Cache - PBS)    : Median diff = -2.55 ms | 90% CI: [-51.56, +50.06] ms | 95% CI: [-64.49, +57.78] ms
- ZTA-Naif vs Cache @ 2000 RPS: Median 2227.32 vs 2243.83 ms | U = 428.0, p = 0.7506, delta = -0.0489 (negligible), HL = -17.67 ms
  TOST / Non-Inferiority     : p_upper = 0.0251 < 0.05 (Terbukti non-inferior dan bebas degradasi praktis)

[H2] Latensi Revokasi & Broadcast Control-Plane vs Target Desain 100 ms (S09, n = 30)
- Median: 18.65 ms | Mean: 41.87 ms | SD: 49.30 ms | p90: 102.32 ms | p99: 183.39 ms
- Kepatuhan Run (< 100 ms): 26/30 run (86.7%, batas run ke-26 = 92.40 ms)
- Wilcoxon 1-sided (H1: median < 100 ms): W+ = 43.0, p = 1.18e-05 (< 0.0001) -> H2 TERPENUHI

[H3] Pipeline Adaptif Graph-Aware (S07) vs Pipeline Statis (S10) @ 2000 RPS Fan-out
- Static (S10) Median: 5927.94 ms | Graph-Aware (S07) Median: 6140.55 ms
- Mann-Whitney U: U = 436.0, p = 0.8418, Cliff's delta = -0.0311 (negligible), HL = -40.67 ms, p_TOST = 0.2102
- Reduksi Jendela Keterpaparan (W_exposure steady-state):
  Low-sensitivity (p_s = 0.0): 30.0 s (0% reduksi)
  Medium-sensitivity (p_s = 0.3): <= 21.0 s (30.0% reduksi)
  High-sensitivity (p_s = 0.7): <= 9.0 s (70.0% reduksi)

[H4 & MULTI-GRUP OMNIBUS]
- Konfigurasi @ 500 RPS (PBS vs Naif vs Cache): Kruskal-Wallis H = 0.63, p = 0.7311 (Tidak berbeda signifikan)
- Topologi @ 2000 RPS (Linear vs Fan-out vs Mesh): Kruskal-Wallis H = 63.40, p = 1.71e-14 (Signifikan)
  Post-Hoc Mann-Whitney U (Bonferroni alpha' = 0.0167):
  * Linear vs Fan-out: U = 0.0, p = 3.02e-11, delta = -1.00 (large)
  * Linear vs Mesh   : U = 45.0, p = 2.23e-09, delta = -0.90 (large)
  * Fan-out vs Mesh  : U = 745.0, p = 1.34e-05, delta = +0.66 (large)
- Resource Overhead @ 500 RPS: CPU H = 2.79 (p = 0.2478), Memory H = 0.91 (p = 0.6359)
```

**Error handling:**
- Data kurang dari 5 pengulangan per skenario: peringatan, tetap lanjutkan
- Semua nilai NaN untuk satu skenario: skip analisis skenario tersebut, log error
- Shapiro-Wilk gagal: gunakan Mann-Whitney tanpa pre-check (tetap valid)

---

## Tips Umum

**Menjalankan port-forward sebelum script Python:**
```bash
# Prometheus
kubectl port-forward svc/monitoring-kube-prometheus-prometheus \
  9090:9090 -n monitoring &

# Jaeger
kubectl port-forward svc/jaeger-query 16686:16686 -n monitoring &

# Cache Distributor
kubectl port-forward svc/cache-distributor 8081:8081 -n zta-research &

# Tunggu port-forward ready
sleep 3
```

**Debugging script yang gagal:**
```bash
# Jalankan dengan bash -x untuk trace setiap perintah
bash -x scripts/validate-state.sh zta-cache

# Python dengan verbose error
python3 -v scripts/export-metrics.py ...

# Cek apakah output direktori ada
ls -la data/raw/S03/
```

**Urutan eksekusi yang benar:**
```
1. verify-versions.sh / verify_offline.py ← cek lingkungan & validasi arsitektur
2. setup_vm_2core_4gb.sh                  ← setup cluster lean pada VM testbed
3. snapshot-config.sh                     ← dokumentasi snapshot awal
4. run-all-scenarios.sh                   ← eksekusi beban k6 S01–S10 & export Prometheus
5. analyze-all-data.py                    ← ingest, merge k6/Prometheus, & uji statistik
6. generate-figures.py                    ← generate grafik publikasi
```
