#!/usr/bin/env python3
"""
scripts/run_ablation_experiment.py
Orkestrator Otomatis Eksperimen Ablasi: Graph-Aware vs Frequency-Only (LFU)

Skrip ini mengotomasi eksekusi pengujian ablasi secara terisolasi untuk membuktikan
hipotesis keunggulan Graph-Aware Prioritization pada kondisi cache contention (K=2):
1. Menghubungi control plane Graph Analyzer untuk verifikasi kesehatan dan reset topologi.
2. Menjalankan skenario beban k6 multi-hop contention (500 RPS) secara berulang (5-10 repetisi).
3. Mengekspor metrik latensi terperinci (end-to-end, multi-hop, direct, throughput, errors).
4. Menyimpan rekapitulasi komparatif ke load-testing/results/ablation/.
"""

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path
from urllib import request, error


def parse_args():
    parser = argparse.ArgumentParser(
        description="Jalankan eksperimen ablasi multi-hop: Graph-Aware vs Frequency-Only (LFU)"
    )
    parser.add_argument(
        "--mode",
        choices=["lfu", "graph", "both"],
        default="both",
        help="Mode prioritisasi yang diuji: lfu, graph, atau both (default: both)",
    )
    parser.add_argument(
        "--repetitions",
        type=int,
        default=5,
        help="Jumlah repetisi pengujian per mode (default: 5)",
    )
    parser.add_argument(
        "--mini",
        action="store_true",
        help="Gunakan skenario mini (15s @ 50 RPS) untuk verifikasi pipeline cepat",
    )
    parser.add_argument(
        "--target-url",
        default="http://frontend.zta-research.svc.cluster.local",
        help="URL basis frontend entry point (default: http://frontend.zta-research.svc.cluster.local)",
    )
    parser.add_argument(
        "--analyzer-url",
        default="http://graph-analyzer.zta-research.svc.cluster.local:8080",
        help="URL basis control plane graph-analyzer (default: http://graph-analyzer.zta-research.svc.cluster.local:8080)",
    )
    parser.add_argument(
        "--admin-token",
        default=os.getenv("ADMIN_TOKEN", "research-admin-token-2024"),
        help="Token otentikasi admin untuk endpoint reset graf",
    )
    parser.add_argument(
        "--output-dir",
        default="load-testing/results/ablation",
        help="Direktori penyimpanan data metrik mentah dan ringkasan",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Hanya tampilkan rencana perintah tanpa mengeksekusi k6 secara fisik",
    )
    return parser.parse_args()


def check_analyzer_health(analyzer_url):
    """Memeriksa kesehatan analyzer dan membaca mode aktif saat ini."""
    health_url = f"{analyzer_url}/health"
    try:
        req = request.Request(health_url, headers={"Accept": "application/json"})
        with request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data
    except Exception as e:
        print(f"[WARN] Tidak dapat menghubungi {health_url}: {e}")
        return None


def reset_analyzer_graph(analyzer_url, admin_token):
    """Mereset state topologi graf di control plane sebelum memulai repetisi baru."""
    reset_url = f"{analyzer_url}/graph/reset"
    try:
        req = request.Request(
            reset_url,
            headers={"X-Admin-Token": admin_token},
            method="DELETE",
        )
        with request.urlopen(req, timeout=5) as resp:
            return resp.status == 200
    except Exception as e:
        print(f"[WARN] Gagal mereset topologi di {reset_url}: {e}")
        return False


def run_single_k6_test(scenario_path, target_url, output_json, dry_run=False):
    """Menjalankan satu eksekusi k6 dengan summary export ke file JSON."""
    cmd = [
        "k6",
        "run",
        f"-e", f"BASE_URL={target_url}",
        f"--summary-export={output_json}",
        scenario_path,
    ]
    print(f"[EXEC] {' '.join(cmd)}")
    if dry_run:
        print("[DRY-RUN] Melewati eksekusi k6 sebenarnya.")
        mock_summary = {
            "metrics": {
                "http_req_duration": {"values": {"med": 2.45, "p(90)": 3.8, "p(95)": 4.5, "p(99)": 7.2}},
                "multihop_latency": {"values": {"med": 3.2, "p(90)": 4.9, "p(95)": 5.8, "p(99)": 8.9}},
                "direct_latency": {"values": {"med": 1.8, "p(90)": 2.9, "p(95)": 3.4, "p(99)": 5.1}},
                "http_reqs": {"values": {"count": 90000, "rate": 499.8}},
                "auth_errors": {"values": {"count": 0}},
            }
        }
        with open(output_json, "w", encoding="utf-8") as f:
            json.dump(mock_summary, f, indent=2)
        return True

    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        print(f"[ERROR] k6 gagal (kode {res.returncode}):\n{res.stderr}")
        return False
    return True


def configure_prioritization_mode(mode, dry_run=False):
    """Mengubah variabel PRIORITIZATION_MODE pada deployment graph-analyzer dan me-restart cache."""
    print(f"\n[CONFIG] Menyetel PRIORITIZATION_MODE={mode} di graph-analyzer...")
    if dry_run:
        print("[DRY-RUN] Melewati perubahan mode k8s.")
        return True

    # 1. Update environment variable pada deployment graph-analyzer
    res = subprocess.run([
        "kubectl", "set", "env", "deploy/graph-analyzer",
        "-n", "zta-research", f"PRIORITIZATION_MODE={mode}"
    ], capture_output=True, text=True)
    if res.returncode != 0:
        print(f"[WARN] kubectl set env gagal: {res.stderr}")

    # 2. Tunggu deployment graph-analyzer siap
    subprocess.run([
        "kubectl", "rollout", "status", "deploy/graph-analyzer",
        "-n", "zta-research", "--timeout=60s"
    ], capture_output=True, text=True)

    # 3. Restart cache-distributor agar seluruh local cache dan counter bersih total
    print("[CONFIG] Merestart cache-distributor agar cache bersih...")
    subprocess.run([
        "kubectl", "rollout", "restart", "deploy/cache-distributor",
        "-n", "zta-research"
    ], capture_output=True, text=True)
    subprocess.run([
        "kubectl", "rollout", "status", "deploy/cache-distributor",
        "-n", "zta-research", "--timeout=60s"
    ], capture_output=True, text=True)

    time.sleep(5)
    print(f"[CONFIG] Kluster siap untuk pengujian mode: {mode.upper()}\n")
    return True


def _get_metric_dict(metrics, key):
    """Mengekstrak dictionary nilai metrik k6 (mendukung format flat maupun nested 'values')."""
    obj = metrics.get(key, {})
    if isinstance(obj, dict) and "values" in obj:
        return obj["values"]
    return obj if isinstance(obj, dict) else {}


def parse_k6_summary(json_path):
    """Mengekstrak metrik kunci dari file ringkasan JSON k6."""
    try:
        with open(json_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        metrics = data.get("metrics", {})
        e2e = _get_metric_dict(metrics, "http_req_duration")
        multihop = _get_metric_dict(metrics, "multihop_latency")
        direct = _get_metric_dict(metrics, "direct_latency")
        reqs = _get_metric_dict(metrics, "http_reqs")
        errs = _get_metric_dict(metrics, "auth_errors")

        return {
            "e2e_med_ms": e2e.get("med", 0.0),
            "e2e_p95_ms": e2e.get("p(95)", 0.0),
            "e2e_p99_ms": e2e.get("p(99)", 0.0),
            "multihop_med_ms": multihop.get("med", 0.0),
            "multihop_p99_ms": multihop.get("p(99)", 0.0),
            "direct_med_ms": direct.get("med", 0.0),
            "direct_p99_ms": direct.get("p(99)", 0.0),
            "throughput_rps": reqs.get("rate", 0.0),
            "total_reqs": reqs.get("count", 0),
            "auth_errors": errs.get("count", 0),
        }
    except Exception as e:
        print(f"[ERROR] Gagal mem-parse {json_path}: {e}")
        return None


def main():
    args = parse_args()
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    if args.mini:
        scenario = "load-testing/mini-scenarios/mini-ablation-multihop.js"
    else:
        scenario = "load-testing/scenarios/s-ablation-multihop-contention.js"

    if not Path(scenario).exists():
        print(f"[FATAL] Berkas skenario {scenario} tidak ditemukan!")
        sys.exit(1)

    modes_to_test = ["lfu", "graph"] if args.mode == "both" else [args.mode]
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    experiment_id = f"ablation_{timestamp}"
    run_log = out_dir / f"{experiment_id}_summary.json"

    print("=" * 70)
    print("ZTA PREDICTIVE CACHING: MULTI-HOP ABLATION EXPERIMENT RUNNER")
    print("=" * 70)
    print(f"ID Eksperimen   : {experiment_id}")
    print(f"Mode yang diuji : {', '.join(modes_to_test)}")
    print(f"Repetisi/mode   : {args.repetitions}")
    print(f"Skenario k6     : {scenario}")
    print(f"Target Frontend : {args.target_url}")
    print(f"Graph Analyzer  : {args.analyzer_url}")
    print(f"Output Directory: {out_dir}")
    print("=" * 70)

    all_results = {}

    for mode in modes_to_test:
        print(f"\n>>> MEMULAI SERI PENGUJIAN MODE: {mode.upper()} ({args.repetitions} Repetisi) <<<")
        configure_prioritization_mode(mode, dry_run=args.dry_run)
        all_results[mode] = []

        for rep in range(1, args.repetitions + 1):
            run_name = f"{mode}_rep{rep:02d}"
            output_json = out_dir / f"{experiment_id}_{run_name}.json"
            print(f"\n--- Repetisi {rep}/{args.repetitions} ({mode.upper()}) ---")

            # 1. Reset state topologi sebelum pengujian
            if not args.dry_run:
                reset_analyzer_graph(args.analyzer_url, args.admin_token)
                time.sleep(1)

            # 2. Jalankan k6
            success = run_single_k6_test(scenario, args.target_url, str(output_json), dry_run=args.dry_run)
            if not success:
                print(f"[WARN] Pengujian {run_name} gagal atau dihentikan.")
                continue

            # 3. Parse dan catat hasil
            metrics = parse_k6_summary(str(output_json))
            if metrics:
                metrics["repetition"] = rep
                metrics["mode"] = mode
                all_results[mode].append(metrics)
                print(f"    E2E Latency     : Med = {metrics['e2e_med_ms']:.2f} ms | p99 = {metrics['e2e_p99_ms']:.2f} ms")
                print(f"    Multi-Hop Hop-2 : Med = {metrics['multihop_med_ms']:.2f} ms | p99 = {metrics['multihop_p99_ms']:.2f} ms")
                print(f"    Throughput      : {metrics['throughput_rps']:.1f} RPS | Errors: {metrics['auth_errors']}")

            # Beri jeda pendinginan singkat antar repetisi
            if rep < args.repetitions and not args.dry_run:
                time.sleep(3)

    # Simpan ringkasan lengkap
    with open(run_log, "w", encoding="utf-8") as f:
        json.dump({
            "experiment_id": experiment_id,
            "timestamp": timestamp,
            "repetitions": args.repetitions,
            "scenario": scenario,
            "results": all_results,
        }, f, indent=2)

    print("\n" + "=" * 70)
    print("REKAPITULASI HASIL EKSPERIMEN ABLASI")
    print("=" * 70)
    for mode, runs in all_results.items():
        if not runs:
            continue
        med_e2e = [r["e2e_med_ms"] for r in runs]
        p99_e2e = [r["e2e_p99_ms"] for r in runs]
        med_multi = [r["multihop_med_ms"] for r in runs]
        p99_multi = [r["multihop_p99_ms"] for r in runs]

        avg_med = sum(med_e2e) / len(med_e2e)
        avg_p99 = sum(p99_e2e) / len(p99_e2e)
        avg_multi_p99 = sum(p99_multi) / len(p99_multi)

        print(f"Mode {mode.upper():<10}: E2E Med = {avg_med:.2f} ms | E2E p99 = {avg_p99:.2f} ms | Multi-Hop p99 = {avg_multi_p99:.2f} ms ({len(runs)} runs)")

    print(f"\n[INFO] Seluruh data berhasil disimpan ke: {run_log}")
    print("=" * 70)


if __name__ == "__main__":
    main()
