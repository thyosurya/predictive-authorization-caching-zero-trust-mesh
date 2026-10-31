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
