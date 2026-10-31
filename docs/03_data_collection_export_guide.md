# Data Collection & Export Guide
## ZTA Predictive Authorization Caching - Pengumpulan dan Ekspor Data Eksperimen

---

## Daftar Isi
1. [Sumber Data](#1-sumber-data)
2. [Skema Kolom Output](#2-skema-kolom-output)
3. [Export dari Prometheus](#3-export-dari-prometheus)
4. [Export dari Jaeger](#4-export-dari-jaeger)
5. [Export dari k6](#5-export-dari-k6)
6. [Export Custom Metrics (Cache)](#6-export-custom-metrics-cache)
7. [Pipeline Analisis Statistik](#7-pipeline-analisis-statistik)
8. [Contoh Output](#8-contoh-output)

---

## 1. Sumber Data

| Sumber | Data yang dikumpulkan | Format output |
|---|---|---|
| **k6** | p50/p90/p99 latency, throughput, error rate per skenario | JSON, CSV |
| **Prometheus** | CPU/memory sidecar, request rate, custom ZTA metrics | CSV |
| **Jaeger** | E2E trace per request, span breakdown | JSON |
| **Cache Distributor** | Cache hit/miss, token TTL, revocation latency | JSON, CSV |

**Struktur direktori output:**
```
data/
├── raw/
│   ├── S01/          # PBS Baseline
│   │   ├── k6-summary.json
│   │   ├── prometheus-latency.csv
│   │   ├── prometheus-resource.csv
│   │   └── jaeger-traces.json
│   ├── S02/ ... S10/
│   └── final-export-YYYYMMDD/
├── processed/
│   ├── latency-all-scenarios.csv
│   ├── resource-all-scenarios.csv
│   ├── cache-performance.csv
│   └── revocation-latency.csv
└── reports/
    ├── statistical-analysis.txt
    └── figures/
```

---

## 2. Skema Kolom Output

### latency-all-scenarios.csv

| Kolom | Tipe | Keterangan |
|---|---|---|
| `scenario_id` | string | S01–S10 |
| `config` | string | `pbs` / `zta-naive` / `zta-cache` |
| `rps_target` | int | Target RPS skenario |
| `topology` | string | `linear` / `fanout` / `mesh` |
| `repetition` | int | Nomor pengulangan (1–30) |
| `p50_ms` | float | Latensi persentil 50 (ms) |
| `p90_ms` | float | Latensi persentil 90 (ms) |
| `p99_ms` | float | Latensi persentil 99 (ms) |
| `p999_ms` | float | Latensi persentil 99.9 (ms) |
| `mean_ms` | float | Rata-rata latensi (ms) |
| `max_ms` | float | Latensi maksimum (ms) |
| `throughput_rps` | float | Throughput aktual |
| `error_rate_pct` | float | Persentase request gagal |
| `timestamp` | datetime | Waktu pengukuran (ISO 8601) |

### resource-all-scenarios.csv

| Kolom | Tipe | Keterangan |
|---|---|---|
| `scenario_id` | string | S01–S10 |
| `config` | string | Konfigurasi otorisasi |
| `repetition` | int | Nomor pengulangan |
| `pod_name` | string | Nama pod sidecar |
| `cpu_avg_pct` | float | Rata-rata CPU sidecar (%) |
| `cpu_peak_pct` | float | Puncak CPU sidecar (%) |
| `cpu_baseline_pct` | float | CPU baseline sebelum beban |
| `mem_avg_mb` | float | Rata-rata memori (MB) |
| `mem_peak_mb` | float | Puncak memori (MB) |
| `mem_baseline_mb` | float | Memori baseline |

### cache-performance.csv

| Kolom | Tipe | Keterangan |
|---|---|---|
| `scenario_id` | string | S03, S05–S10 |
| `repetition` | int | Nomor pengulangan |
| `cache_hits` | int64 | Total keputusan dari cache |
| `cache_misses` | int64 | Total fallback ke PDP |
| `hit_ratio_pct` | float | cache_hits / (hits+misses) × 100 |
| `avg_ttl_ms` | float | Rata-rata TTL token aktif |
| `tokens_issued` | int | Token diterbitkan dalam skenario |
| `tokens_expired` | int | Token yang expired |

### revocation-latency.csv

| Kolom | Tipe | Keterangan |
|---|---|---|
| `repetition` | int | Nomor pengulangan (1–30) |
| `revocation_latency_ms` | float | Latensi revokasi end-to-end (ms) |
| `timestamp` | datetime | Waktu pengukuran |

---

## 3. Export dari Prometheus

### Script export otomatis

```python
# scripts/export-metrics.py
"""
Export metrics dari Prometheus ke CSV.
Penggunaan:
  python3 scripts/export-metrics.py \
    --prometheus-url http://localhost:9090 \
    --scenario S03 \
    --start "2024-01-15T10:00:00Z" \
    --end "2024-01-15T10:05:00Z" \
    --output data/raw/S03/
"""

import argparse
import csv
import os
import requests
from datetime import datetime, timezone

QUERIES = {
    # Latency dari Istio metrics
    "p50_latency_ms": """
        histogram_quantile(0.50,
          sum(rate(istio_request_duration_milliseconds_bucket{
            destination_service_namespace="zta-research"
          }[30s])) by (le)
        )
    """,
    "p90_latency_ms": """
        histogram_quantile(0.90,
          sum(rate(istio_request_duration_milliseconds_bucket{
            destination_service_namespace="zta-research"
          }[30s])) by (le)
        )
    """,
    "p99_latency_ms": """
        histogram_quantile(0.99,
          sum(rate(istio_request_duration_milliseconds_bucket{
            destination_service_namespace="zta-research"
          }[30s])) by (le)
        )
    """,

    # Throughput
    "throughput_rps": """
        sum(rate(istio_requests_total{
          destination_service_namespace="zta-research",
          response_code=~"2.."
        }[30s]))
    """,

    # Error rate
    "error_rate_pct": """
        100 * sum(rate(istio_requests_total{
          destination_service_namespace="zta-research",
          response_code=~"5.."
        }[30s])) /
        sum(rate(istio_requests_total{
          destination_service_namespace="zta-research"
        }[30s]))
    """,

    # CPU sidecar
    "cpu_avg_pct": """
        avg(rate(container_cpu_usage_seconds_total{
          container="istio-proxy",
          namespace="zta-research"
        }[30s])) * 100
    """,

    # Memory sidecar
    "mem_avg_mb": """
        avg(container_memory_working_set_bytes{
          container="istio-proxy",
          namespace="zta-research"
        }) / 1024 / 1024
    """,

    # Cache metrics
    "cache_hit_ratio": """
        rate(zta_cache_hits_total[30s]) /
        (rate(zta_cache_hits_total[30s]) + rate(zta_cache_misses_total[30s]))
    """,

    "revocation_latency_p99_ms": """
        histogram_quantile(0.99, rate(zta_revocation_latency_ms_bucket[5m]))
    """,
}


def query_range(prom_url, query, start, end, step="10s"):
    """Query Prometheus range API."""
    resp = requests.get(f"{prom_url}/api/v1/query_range", params={
        "query": query.strip(),
        "start": start,
        "end": end,
        "step": step,
    })
    resp.raise_for_status()
    data = resp.json()
    if data["status"] != "success":
        raise ValueError(f"Prometheus error: {data}")
    results = data["data"]["result"]
    if not results:
        return []
    return results[0]["values"]  # [[timestamp, value], ...]


def export_scenario(prom_url, scenario_id, start, end, output_dir, repetition=1):
    os.makedirs(output_dir, exist_ok=True)
    rows = {}

    for metric_name, query in QUERIES.items():
        print(f"  Querying: {metric_name}...")
        try:
            values = query_range(prom_url, query, start, end)
            for ts, val in values:
                if ts not in rows:
                    rows[ts] = {"timestamp": datetime.fromtimestamp(
                        float(ts), tz=timezone.utc).isoformat()}
                rows[ts][metric_name] = float(val) if val != "NaN" else None
        except Exception as e:
            print(f"  WARNING: {metric_name} failed: {e}")

    # Tulis ke CSV
    out_file = os.path.join(output_dir, f"prometheus-{scenario_id}-rep{repetition}.csv")
    fieldnames = ["timestamp"] + list(QUERIES.keys())
    with open(out_file, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for ts in sorted(rows.keys()):
            writer.writerow(rows[ts])

    print(f"  Tersimpan: {out_file} ({len(rows)} baris)")
    return out_file


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--prometheus-url", default="http://localhost:9090")
    parser.add_argument("--scenario", required=True)
    parser.add_argument("--start", required=True)
    parser.add_argument("--end", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--repetition", type=int, default=1)
    args = parser.parse_args()

    print(f"Mengekspor skenario {args.scenario} repetisi {args.repetition}...")
    export_scenario(
        args.prometheus_url,
        args.scenario,
        args.start,
        args.end,
        args.output,
        args.repetition,
    )
    print("Selesai.")
```

### Contoh penggunaan

```bash
# Export satu skenario
python3 scripts/export-metrics.py \
  --prometheus-url http://localhost:9090 \
  --scenario S03 \
  --start "2024-01-15T10:00:00Z" \
  --end "2024-01-15T10:05:00Z" \
  --output data/raw/S03/ \
  --repetition 1

# Export semua skenario sekaligus (menggunakan log waktu dari run)
bash scripts/export-all-scenarios.sh
```

---

## 4. Export dari Jaeger

### Menggunakan Jaeger REST API

```python
# scripts/export-jaeger.py
"""
Export distributed traces dari Jaeger untuk analisis E2E latency.
"""

import json
import os
import requests
from datetime import datetime, timedelta


def export_traces(jaeger_url, service, scenario_id, start_dt, end_dt, output_dir, limit=5000):
    """
    Export trace dari Jaeger untuk service tertentu dalam window waktu.
    """
    # Jaeger API menggunakan microseconds untuk timestamp
    start_us = int(start_dt.timestamp() * 1_000_000)
    end_us   = int(end_dt.timestamp() * 1_000_000)

    resp = requests.get(f"{jaeger_url}/api/traces", params={
        "service":   service,
        "start":     start_us,
        "end":       end_us,
        "limit":     limit,
        "lookback":  "custom",
    })
    resp.raise_for_status()
    traces = resp.json().get("data", [])

    # Ekstrak durasi per trace
    records = []
    for trace in traces:
        for span in trace.get("spans", []):
            if span.get("operationName") == "HTTP GET /api/products":  # sesuaikan
                records.append({
                    "trace_id":       trace["traceID"],
                    "duration_ms":    span["duration"] / 1000,  # µs → ms
                    "start_time":     span["startTime"],
                    "scenario_id":    scenario_id,
                })

    os.makedirs(output_dir, exist_ok=True)
    out_file = os.path.join(output_dir, f"jaeger-{scenario_id}.json")
    with open(out_file, "w") as f:
        json.dump(records, f, indent=2)

    print(f"Tersimpan: {out_file} ({len(records)} spans)")
    return records


if __name__ == "__main__":
    jaeger_url = "http://localhost:16686"
    start      = datetime(2024, 1, 15, 10, 0, 0)
    end        = datetime(2024, 1, 15, 10, 5, 0)

    for scenario in ["S01", "S02", "S03"]:
        export_traces(jaeger_url, "frontend", scenario, start, end, f"data/raw/{scenario}/")
```

---

## 5. Export dari k6

k6 menghasilkan summary JSON otomatis. Berikut cara mengekstrak nilai yang relevan:

### Output k6 yang dikonfigurasi

```javascript
// Tambahkan ke setiap skrip skenario
export function handleSummary(data) {
    const scenario = __ENV.SCENARIO_ID || "unknown";
    const repetition = __ENV.REPETITION || "1";

    const summary = {
        scenario_id:    scenario,
        repetition:     parseInt(repetition),
        timestamp:      new Date().toISOString(),
        p50_ms:         data.metrics.http_req_duration.values["p(50)"],
        p90_ms:         data.metrics.http_req_duration.values["p(90)"],
        p99_ms:         data.metrics.http_req_duration.values["p(99)"],
        p999_ms:        data.metrics.http_req_duration.values["p(99.9)"],
        mean_ms:        data.metrics.http_req_duration.values.avg,
        max_ms:         data.metrics.http_req_duration.values.max,
        throughput_rps: data.metrics.http_reqs.values.rate,
        error_rate_pct: data.metrics.http_req_failed.values.rate * 100,
        total_requests: data.metrics.http_reqs.values.count,
    };

    return {
        stdout: JSON.stringify(summary, null, 2),
        [`data/raw/${scenario}/k6-${scenario}-rep${repetition}.json`]:
            JSON.stringify(summary, null, 2),
    };
}
```

### Jalankan dengan environment variable

```bash
# Menjalankan skenario S03, pengulangan ke-5
k6 run \
  -e SCENARIO_ID=S03 \
  -e REPETITION=5 \
  load-testing/scenarios/s03-zta-cache-500rps.js
```

### Gabungkan semua hasil k6 ke satu CSV

Proses penggabungan data k6 mentah (`data/raw/*/k6-*.json`) ke CSV olahan (`data/processed/latency-all-scenarios.csv`) ditangani secara otomatis oleh skrip terpadu `scripts/analyze-all-data.py` (atau `make merge-results`):

```bash
python3 scripts/analyze-all-data.py
```

Skrip ini melakukan iterasi file JSON secara otomatis:
```python
# Inti algoritma penggabungan data k6
import json, csv, glob, os

rows = []
for json_file in sorted(glob.glob("data/raw/*/k6-*.json")):
    with open(json_file) as f:
        rows.append(json.load(f))

fieldnames = ["scenario_id","repetition","timestamp","p50_ms","p90_ms",
              "p99_ms","p999_ms","mean_ms","max_ms","throughput_rps",
              "error_rate_pct","total_requests"]

os.makedirs("data/processed", exist_ok=True)
with open("data/processed/latency-all-scenarios.csv", "w", newline="") as f:
    writer = csv.DictWriter(f, fieldnames=fieldnames)
    writer.writeheader()
    writer.writerows(rows)

print(f"Merged {len(rows)} hasil ke data/processed/latency-all-scenarios.csv")
```

---

## 6. Export Custom Metrics (Cache)

```python
# scripts/export-cache-metrics.py
"""
Export cache hit ratio, revocation latency, dan token metrics
dari Cache Distributor REST API.
"""

import csv, json, os, requests
from datetime import datetime


def export_cache_metrics(distributor_url, scenario_id, repetition, output_dir):
    # Token aktif
    active = requests.get(f"{distributor_url}/tokens/active").json()

    # Metrics dari Prometheus endpoint Cache Distributor
    prom_raw = requests.get(f"{distributor_url.replace(':8081',':2112')}/metrics").text

    # Parse Prometheus text format (sederhana)
    metrics = {}
    for line in prom_raw.splitlines():
        if line.startswith("#") or not line.strip():
            continue
        parts = line.split()
        if len(parts) == 2:
            metrics[parts[0]] = float(parts[1])

    record = {
        "scenario_id":    scenario_id,
        "repetition":     repetition,
        "timestamp":      datetime.utcnow().isoformat(),
        "tokens_active":  active.get("count", 0),
        "cache_hits":     metrics.get("zta_cache_hits_total", 0),
        "cache_misses":   metrics.get("zta_cache_misses_total", 0),
        "hit_ratio_pct":  metrics.get("zta_cache_hit_ratio", 0) * 100,
        "tokens_issued":  metrics.get("zta_tokens_issued_total", 0),
        "tokens_expired": metrics.get("zta_tokens_expired_total", 0),
    }

    os.makedirs(output_dir, exist_ok=True)
    out_file = os.path.join(output_dir, f"cache-{scenario_id}-rep{repetition}.json")
    with open(out_file, "w") as f:
        json.dump(record, f, indent=2)

    return record


if __name__ == "__main__":
    for scenario in ["S03", "S05", "S06", "S07", "S08", "S09", "S10"]:
        for rep in range(1, 31):
            export_cache_metrics(
                "http://localhost:8081",
                scenario,
                rep,
                f"data/raw/{scenario}/",
            )
```

---

## 7. Pipeline Analisis Statistik

```python
# scripts/statistical-analysis.py
"""
Analisis statistik lengkap untuk semua hipotesis penelitian.
Output: laporan teks + CSV hasil uji statistik.
"""

import csv
import numpy as np
import pandas as pd
from scipy import stats


def load_data(filepath):
    return pd.read_csv(filepath)


def shapiro_wilk(data, label):
    """Uji normalitas Shapiro-Wilk."""
    stat, p = stats.shapiro(data)
    normal = p > 0.05
    print(f"  Shapiro-Wilk [{label}]: W={stat:.4f}, p={p:.4f} → {'Normal' if normal else 'Non-normal'}")
    return normal


def tost_equivalence(group_a, group_b, margin=0.5, alpha=0.05):
    """
    Two One-Sided Tests (TOST) untuk menguji ekuivalensi:
    H01: mu_A - mu_B <= -margin
    H02: mu_A - mu_B >= +margin
    Jika kedua p-value < alpha, ekuivalensi terbukti dalam batas [-margin, +margin].
    """
    diff = np.mean(group_b) - np.mean(group_a)
    n1, n2 = len(group_a), len(group_b)
    s1, s2 = np.var(group_a, ddof=1), np.var(group_b, ddof=1)
    se = np.sqrt(s1 / n1 + s2 / n2)
    df = (s1 / n1 + s2 / n2) ** 2 / ((s1 / n1) ** 2 / (n1 - 1) + (s2 / n2) ** 2 / (n2 - 1))

    # Test 1: diff > -margin
    t1 = (diff - (-margin)) / se
    p1 = 1 - stats.t.cdf(t1, df=df)

    # Test 2: diff < +margin
    t2 = (margin - diff) / se
    p2 = 1 - stats.t.cdf(t2, df=df)

    p_tost = max(p1, p2)
    return {
        "mean_diff": round(diff, 4),
        "se": round(se, 4),
        "t1": round(t1, 3),
        "p1": round(p1, 5),
        "t2": round(t2, 3),
        "p2": round(p2, 5),
        "p_tost": round(p_tost, 5),
        "equivalent": p_tost < alpha,
    }


def hodges_lehmann_ci(group_a, group_b, n_boot=2000, alpha=0.10):
    """
    Hodges-Lehmann estimator untuk perbedaan median non-parametrik
    dilengkapi Bootstrap Confidence Interval (100*(1-alpha)%).
    """
    pairwise_diffs = [b - a for b in group_b for a in group_a]
    hl_estimate = np.median(pairwise_diffs)

    boot_medians = []
    rng = np.random.default_rng(seed=42)
    for _ in range(n_boot):
        sample_a = rng.choice(group_a, size=len(group_a), replace=True)
        sample_b = rng.choice(group_b, size=len(group_b), replace=True)
        boot_diffs = [b - a for b in sample_b for a in sample_a]
        boot_medians.append(np.median(boot_diffs))

    ci_lower = np.percentile(boot_medians, 100 * (alpha / 2))
    ci_upper = np.percentile(boot_medians, 100 * (1 - alpha / 2))

    return {
        "hl_median_diff": round(hl_estimate, 4),
        "ci_lower": round(ci_lower, 4),
        "ci_upper": round(ci_upper, 4),
        "confidence_level": f"{int((1 - alpha) * 100)}%",
    }


def mann_whitney(group_a, group_b, label_a, label_b):
    """Uji Mann-Whitney U dua kelompok."""
    u_stat, p_value = stats.mannwhitneyu(group_a, group_b, alternative="two-sided")

    # Cliff's Delta
    n1, n2 = len(group_a), len(group_b)
    dominance = sum(
        1 if a > b else (-1 if a < b else 0)
        for a in group_a for b in group_b
    )
    cliffs_d = dominance / (n1 * n2)

    # Interpretasi effect size
    abs_d = abs(cliffs_d)
    if abs_d < 0.147:
        effect = "negligible"
    elif abs_d < 0.330:
        effect = "small"
    elif abs_d < 0.474:
        effect = "medium"
    else:
        effect = "large"

    return {
        "comparison":    f"{label_a} vs {label_b}",
        "u_statistic":   round(u_stat, 2),
        "p_value":       round(p_value, 4),
        "significant":   p_value < 0.05,
        "cliffs_d":      round(cliffs_d, 3),
        "effect_size":   effect,
        "median_a":      round(np.median(group_a), 2),
        "median_b":      round(np.median(group_b), 2),
        "reduction_pct": round((1 - np.median(group_b)/np.median(group_a)) * 100, 1),
    }


def kruskal_wallis(*groups, labels):
    """Uji Kruskal-Wallis untuk 3+ kelompok."""
    h_stat, p_value = stats.kruskal(*groups)
    return {
        "groups":      " | ".join(labels),
        "h_statistic": round(h_stat, 2),
        "p_value":     round(p_value, 4),
        "significant": p_value < 0.05,
    }


def run_analysis():
    print("=" * 60)
    print("ANALISIS STATISTIK: ZTA Predictive Authorization Caching")
    print("=" * 60)

    df = load_data("data/processed/latency-all-scenarios.csv")

    results_mw  = []
    results_kw  = []

    # ── H1: Penurunan p99 latency & Ekuivalensi TOST ───────────
    print("\n[H1] Perbandingan p99 Latency & Uji Ekuivalensi TOST\n")

    for rps in [500, 2000, 5000]:
        pbs_data   = df[(df.config == "pbs")       & (df.rps_target == rps)]["p99_ms"].values
        naive_data = df[(df.config == "zta-naive") & (df.rps_target == rps)]["p99_ms"].values
        cache_data = df[(df.config == "zta-cache") & (df.rps_target == rps)]["p99_ms"].values

        if len(naive_data) == 0 or len(cache_data) == 0:
            print(f"  SKIP {rps} RPS: data tidak tersedia")
            continue

        print(f"  @ {rps} RPS:")
        shapiro_wilk(naive_data, f"ZTA-Naif {rps} RPS")
        shapiro_wilk(cache_data, f"ZTA-Cache {rps} RPS")

        # Uji Mann-Whitney vs ZTA-Naif
        result = mann_whitney(naive_data, cache_data, "ZTA-Naif", "ZTA-Cache")
        result["rps"] = rps
        result["metric"] = "p99_latency_ms"
        results_mw.append(result)

        print(f"    vs Naif : U={result['u_statistic']}, p={result['p_value']}, "
              f"d={result['cliffs_d']} ({result['effect_size']}), "
              f"reduksi={result['reduction_pct']}%")

        # Uji Ekuivalensi TOST vs PBS (jika data PBS ada)
        if len(pbs_data) > 0:
            tost_res = tost_equivalence(pbs_data, cache_data, margin=0.5)
            hl_res = hodges_lehmann_ci(pbs_data, cache_data, alpha=0.10)
            print(f"    vs PBS  : TOST p={tost_res['p_tost']} (Equiv: {tost_res['equivalent']}), "
                  f"HL median diff={hl_res['hl_median_diff']} ms (90% CI [{hl_res['ci_lower']}, {hl_res['ci_upper']}])")

        print(f"    U={result['u_statistic']}, p={result['p_value']}, "
              f"d={result['cliffs_d']} ({result['effect_size']}), "
              f"reduksi={result['reduction_pct']}%")

    # ── H2: Revocation latency ─────────────────────────────────
    print("\n[H2] Revocation Latency vs Threshold 100ms\n")

    rev_df = load_data("data/processed/revocation-latency.csv")
    rev_data = rev_df["revocation_latency_ms"].values

    rev_p99  = np.percentile(rev_data, 99)
    rev_mean = np.mean(rev_data)
    threshold_met = rev_p99 <= 100

    print(f"  Mean      : {rev_mean:.2f} ms")
    print(f"  p99       : {rev_p99:.2f} ms")
    print(f"  Threshold : 100 ms")
    print(f"  Terpenuhi : {'✅ Ya' if threshold_met else '❌ Tidak'}")

    results_mw.append({
        "comparison":  "RevocationLatency vs 100ms threshold",
        "p99":         rev_p99,
        "threshold_met": threshold_met,
    })

    # ── H3: Graph-aware TTL vs Static TTL ─────────────────────
    print("\n[H3] Cache Hit Ratio: Graph-aware TTL vs Static TTL\n")

    cache_df = load_data("data/processed/cache-performance.csv")
    aware_hits  = cache_df[cache_df.ttl_type == "graph-aware"]["hit_ratio_pct"].values
    static_hits = cache_df[cache_df.ttl_type == "static"]["hit_ratio_pct"].values

    if len(aware_hits) > 0 and len(static_hits) > 0:
        result = mann_whitney(static_hits, aware_hits, "Static TTL", "Graph-aware TTL")
        result["metric"] = "cache_hit_ratio_pct"
        results_mw.append(result)
        print(f"  Static TTL median : {np.median(static_hits):.1f}%")
        print(f"  Graph-aware median: {np.median(aware_hits):.1f}%")
        print(f"  Cliff's d         : {result['cliffs_d']} ({result['effect_size']})")

    # ── Kruskal-Wallis: 3 konfigurasi ─────────────────────────
    print("\n[Multi-grup] Kruskal-Wallis: PBS vs ZTA-Naif vs ZTA-Cache\n")

    for rps in [500, 2000]:
        pbs   = df[(df.config == "pbs")      & (df.rps_target == rps)]["p99_ms"].values
        naive = df[(df.config == "zta-naive") & (df.rps_target == rps)]["p99_ms"].values
        cache = df[(df.config == "zta-cache") & (df.rps_target == rps)]["p99_ms"].values

        if len(pbs) == 0:
            continue

        kw = kruskal_wallis(pbs, naive, cache, labels=["PBS", "ZTA-Naif", "ZTA-Cache"])
        kw["rps"] = rps
        results_kw.append(kw)

        print(f"  @ {rps} RPS: H={kw['h_statistic']}, p={kw['p_value']} "
              f"→ {'Signifikan ✅' if kw['significant'] else 'Tidak signifikan'}")

    # ── Simpan hasil ───────────────────────────────────────────
    import os
    os.makedirs("data/reports", exist_ok=True)

    pd.DataFrame([r for r in results_mw if "u_statistic" in r]).to_csv(
        "data/reports/mann-whitney-results.csv", index=False)
    pd.DataFrame(results_kw).to_csv(
        "data/reports/kruskal-wallis-results.csv", index=False)

    print("\n✅ Hasil tersimpan di data/reports/")


if __name__ == "__main__":
    run_analysis()
```

---

## 8. Contoh Output

### Contoh `data/processed/latency-all-scenarios.csv`

```
scenario_id,config,rps_target,topology,repetition,p50_ms,p90_ms,p99_ms,p999_ms,mean_ms,max_ms,throughput_rps,error_rate_pct,timestamp
S01,pbs,500,linear,1,12.3,18.7,24.1,31.2,13.1,45.3,498.7,0.00,2024-01-15T10:05:00Z
S02,zta-naive,500,linear,1,15.8,28.4,67.3,89.1,17.2,112.4,495.2,0.00,2024-01-15T10:20:00Z
S03,zta-cache,500,linear,1,12.9,19.8,26.4,33.7,13.8,48.1,497.9,0.00,2024-01-15T10:35:00Z
```

### Contoh output analisis statistik

```
============================================================
ANALISIS STATISTIK - ZTA Predictive Authorization Caching
============================================================

[H1] Perbandingan p99 Latency: ZTA-Naif vs ZTA-Cache

  @ 500 RPS:
  Shapiro-Wilk [ZTA-Naif 500 RPS]: W=0.9213, p=0.0312 → Non-normal
  Shapiro-Wilk [ZTA-Cache 500 RPS]: W=0.9567, p=0.2841 → Normal
    U=812.00, p=0.0001, d=-0.681 (large), reduksi=60.8%

  @ 2000 RPS:
    U=897.00, p=0.0000, d=-0.732 (large), reduksi=74.3%

  @ 5000 RPS:
    U=923.00, p=0.0000, d=-0.789 (large), reduksi=81.2%

[H2] Revocation Latency vs Threshold 100ms

  Mean      : 23.41 ms
  p99       : 87.32 ms
  Threshold : 100 ms
  Terpenuhi : ✅ Ya

[H3] Cache Hit Ratio: Graph-aware TTL vs Static TTL

  Static TTL median : 71.3%
  Graph-aware median: 89.7%
  Cliff's d         : 0.634 (large)

[Multi-grup] Kruskal-Wallis: PBS vs ZTA-Naif vs ZTA-Cache

  @ 500 RPS:  H=47.23, p=0.0000 → Signifikan ✅
  @ 2000 RPS: H=61.84, p=0.0000 → Signifikan ✅

✅ Hasil tersimpan di data/reports/
```
