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
| `generate-figures.py` | Python | Pembuatan visualisasi grafik publikasi ilmiah |
| *(Arsip di `dump/`)* | Shell/Py | `audit_code_consistency.py`, `deploy_to_vm.py`, `fetch_results_from_vm.py`, `run_mini_k6_suite.py`, `setup_vm_hosts.py`, `vm_runner.py`, `merge-k6-results.py`, `export-jaeger.py`, `export-cache-metrics.py`, `azure/*` |

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

## 5. `reset-testbed.sh`

**Tujuan:** Reset state testbed ke kondisi bersih. Mendukung dua mode: ringan (antar pengulangan) dan penuh (antar skenario).

**Penggunaan:**
```bash
bash scripts/reset-testbed.sh <mode>
# mode: light | full
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
> Skrip ini menyatukan dan menggantikan berkas lama `merge-k6-results.py` serta `statistical-analysis.py` (keduanya kini telah diarsipkan di `dump/retired_scripts/`).

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
| p99 latency ZTA-Naif vs ZTA-Cache | Mann-Whitney U + Cliff's Delta | H1 |
| Revocation latency vs threshold 100ms | Deskriptif + one-sample test | H2 |
| Cache hit ratio: graph-aware vs static TTL | Mann-Whitney U + Cliff's Delta | H3 |
| PBS vs ZTA-Naif vs ZTA-Cache | Kruskal-Wallis + Dunn post-hoc | Multi-grup |
| Normalitas distribusi | Shapiro-Wilk | Pre-check |

**Dependensi Python:**
```
pandas>=2.1.4
scipy>=1.11.4
numpy>=1.26.3
matplotlib>=3.8.0
seaborn>=0.13.0
```

**Output laporan `statistical-analysis.txt`:**
```
============================================================
ANALISIS STATISTIK: ZTA Predictive Authorization Caching
============================================================
Generated: 2026-10-15T18:00:00Z

[NORMALITAS]
Shapiro-Wilk: ZTA-Naif @ 500 RPS: W=0.9102, p=0.018 -> Non-normal
Shapiro-Wilk: ZTA-Cache @ 500 RPS: W=0.9341, p=0.042 -> Non-normal
-> Pengujian non-parametrik (Mann-Whitney U, Hodges-Lehmann, TOST) diwajibkan.

[H1] TOST Equivalence & Latency Reduction
- Equivalence Margin: [-0.50 ms, +0.50 ms]
- ZTA-Cache vs PBS Baseline:
  Hodges-Lehmann median diff: -0.04 ms (90% CI: [-0.18 ms, +0.12 ms])
  TOST Upper Bound t1: p < 0.001, Lower Bound t2: p < 0.001
  -> EKUIVALENSI TERBUKTI (ZTA-Cache memulihkan performa setara PBS)
- Latensi vs ZTA-Naif:
  @ 500 RPS:  Naif=14.80 ms -> Cache=1.85 ms (Reduksi: 87.5%, Cliff's Delta d=-0.94)
  @ 2000 RPS: Naif=48.20 ms -> Cache=2.12 ms (Reduksi: 95.6%, Cliff's Delta d=-0.98)
  @ 5000 RPS: Naif=124.5 ms -> Cache=3.40 ms (Reduksi: 97.3%, Cliff's Delta d=-0.99)
-> H1 DITERIMA PENUH

[H2] Revocation Invalidation Latency (Threshold SLA = 100 ms)
- Sample N = 30 event revokasi terdistribusi
- Median: 1.72 ms | p95: 2.45 ms | p99: 3.81 ms | Max: 4.95 ms
- Safety Margin vs SLA: 96.19%
-> H2 DITERIMA (p99 = 3.81 ms << 100 ms)

[H3] Graph-Aware Predictive vs Static Cache Hit Ratio
- Static TTL Median: 61.2% | Graph-Aware Median: 88.4%
- Mann-Whitney U: W = 892.0, p < 0.001, Cliff's Delta d = +0.82 (Large)
-> H3 DITERIMA

[H4] Dynamic Exposure Window Risk Reduction
- Low-sensitivity endpoints: Reduksi window eksposur sebesar 70.0%
- High-sensitivity endpoints: Reduksi window eksposur sebesar 30.0%
-> H4 DITERIMA

[MULTI-GRUP OMNIBUS]
- Kruskal-Wallis: H = 79.12, p < 0.001 -> Signifikan antar 3 arsitektur
- Dunn's Post-Hoc (Bonferroni adjusted):
  PBS vs ZTA-Cache: p = 0.842 (Tidak berbeda secara signifikan / setara)
  PBS vs ZTA-Naif : p < 0.001 (Signifikan)
  ZTA-Cache vs ZTA-Naif: p < 0.001 (Signifikan)
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
