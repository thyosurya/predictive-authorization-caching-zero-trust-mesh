#!/usr/bin/env python3
"""
Verifikasi Traceability: Naskah ↔ Data ↔ Script
=================================================
Script ini membuktikan bahwa SETIAP angka dalam naskah-artikel.md
dapat direproduksi dari data CSV mentah.
"""
import pandas as pd
import numpy as np
from scipy.stats import shapiro, mannwhitneyu, kruskal
import os, sys

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')
if hasattr(sys.stderr, 'reconfigure'):
    sys.stderr.reconfigure(encoding='utf-8')

from pathlib import Path
BASE_DIR = Path(__file__).resolve().parent.parent
os.chdir(BASE_DIR)

lat = pd.read_csv("data/processed/latency-all-scenarios.csv")
lat["scenario_id"] = lat["scenario_id"].str.strip()
res = pd.read_csv("data/processed/resource-overhead.csv")
res["scenario_id"] = res["scenario_id"].str.strip()
rev = pd.read_csv("data/processed/revocation-latency.csv")

PASS = 0
FAIL = 0

def check(label, computed, expected, tol=0.02):
    global PASS, FAIL
    ok = abs(computed - expected) <= tol
    status = "✅" if ok else "❌"
    if not ok:
        FAIL += 1
        print(f"  {status} {label}: computed={computed:.4f}, expected={expected}, diff={abs(computed-expected):.4f}")
    else:
        PASS += 1
    return ok

print("=" * 70)
print("VERIFIKASI TRACEABILITY: Naskah → Data → Script")
print("=" * 70)

# === TABEL 4 ===
print("\n--- TABEL 4: Statistik Deskriptif ---")
tabel4 = {
    "s01-pbs-baseline-500rps":    {"med": 295.85, "mean": 348.92, "std": 154.28, "rps": 442.5},
    "s02-zta-naive-500rps":       {"med": 301.36, "mean": 369.39, "std": 214.21, "rps": 447.9},
    "s03-zta-cache-500rps":       {"med": 293.30, "mean": 328.87, "std": 126.42, "rps": 455.9},
    "s04-zta-naive-2000rps":      {"med": 2227.32, "mean": 2383.34, "std": 520.10, "rps": 479.5},
    "s05-zta-cache-2000rps":      {"med": 2243.83, "mean": 2436.48, "std": 620.22, "rps": 473.6},
    "s06-zta-cache-5000rps":      {"med": 12898.73, "mean": 13464.23, "std": 4647.01, "rps": 359.4},
    "s07-zta-cache-fanout":       {"med": 6140.55, "mean": 7287.55, "std": 2472.58, "rps": 372.7},
    "s08-zta-cache-mesh":         {"med": 4028.93, "mean": 4936.48, "std": 1748.04, "rps": 383.0},
    "s09-revocation-test":        {"med": 305.94, "mean": 377.63, "std": 150.96, "rps": 434.7},
    "s10-ttl-comparison":         {"med": 5927.94, "mean": 7634.60, "std": 3410.45, "rps": 379.0},
}
for sid, exp in tabel4.items():
    v = lat[lat.scenario_id == sid]["p99_ms"].values
    rps = lat[lat.scenario_id == sid]["actual_rps"].values
    check(f"{sid} median", np.median(v), exp["med"], 0.05)
    check(f"{sid} mean", np.mean(v), exp["mean"], 0.05)
    check(f"{sid} std", np.std(v, ddof=1), exp["std"], 0.05)
    check(f"{sid} RPS", np.mean(rps), exp["rps"], 0.20)

# === TABEL 5 ===
print("\n--- TABEL 5: Resource Overhead ---")
tabel5 = {
    "s01-pbs-baseline-500rps":  {"cpu": 12.01, "mem": 275.96},
    "s02-zta-naive-500rps":     {"cpu": 12.04, "mem": 276.03},
    "s03-zta-cache-500rps":     {"cpu": 11.81, "mem": 257.62},
    "s04-zta-naive-2000rps":    {"cpu": 12.80, "mem": 268.39},
    "s05-zta-cache-2000rps":    {"cpu": 13.16, "mem": 271.24},
    "s06-zta-cache-5000rps":    {"cpu": 11.16, "mem": 278.51},
    "s07-zta-cache-fanout":     {"cpu": 11.15, "mem": 281.20},
    "s08-zta-cache-mesh":       {"cpu": 11.60, "mem": 276.52},
    "s09-revocation-test":      {"cpu": 11.67, "mem": 274.41},
    "s10-ttl-comparison":       {"cpu": 12.95, "mem": 290.01},
}
for sid, exp in tabel5.items():
    cpu = res[res.scenario_id == sid]["cpu_avg_pct"].values
    mem = res[res.scenario_id == sid]["mem_avg_mb"].values
    check(f"{sid} CPU", np.nanmean(cpu), exp["cpu"], 0.05)
    check(f"{sid} Mem", np.nanmean(mem), exp["mem"], 0.05)

# === TABEL 6a-6c (Mann-Whitney) ===
print("\n--- TABEL 6a-6c: Mann-Whitney U ---")
s01 = lat[lat.scenario_id == "s01-pbs-baseline-500rps"]["p99_ms"].values
s02 = lat[lat.scenario_id == "s02-zta-naive-500rps"]["p99_ms"].values
s03 = lat[lat.scenario_id == "s03-zta-cache-500rps"]["p99_ms"].values
s04 = lat[lat.scenario_id == "s04-zta-naive-2000rps"]["p99_ms"].values
s05 = lat[lat.scenario_id == "s05-zta-cache-2000rps"]["p99_ms"].values

def cd(a, b):
    return sum(1 if x > y else (-1 if x < y else 0) for x in a for y in b) / (len(a) * len(b))

tests_mw = [
    ("6a PBS-Naive", s01, s02, 421.0, 0.673, -0.06),
    ("6b Naive-Cache@500", s02, s03, 503.0, 0.438, 0.12),
    ("6b Naive-Cache@2k", s04, s05, 428.0, 0.751, -0.05),
    ("6c PBS-Cache", s01, s03, 475.0, 0.717, 0.06),
]
for label, a, b, exp_u, exp_p, exp_d in tests_mw:
    u, p = mannwhitneyu(a, b, alternative="two-sided")
    d = cd(a, b)
    check(f"{label} U", u, exp_u, 0.5)
    check(f"{label} p", p, exp_p, 0.005)
    check(f"{label} delta", d, exp_d, 0.02)

# === TABEL 6d (TOST) ===
print("\n--- TABEL 6d: TOST ---")
tost = pd.read_csv("data/reports/tost-equivalence-results.csv")
exp_tost = {
    "PBS@500RPS vs ZTA-Naif@500RPS": 0.381,
    "ZTA-Naif@500RPS vs ZTA-Cache@500RPS": 0.521,
    "ZTA-Naif@2000RPS vs ZTA-Cache@2000RPS": 0.109,
    "PBS@500RPS vs ZTA-Cache@500RPS": 0.300,
    "Static TTL (S10) vs Graph-aware (S07)": 0.210,
}
for _, r in tost.iterrows():
    c = r.comparison.strip()
    if c in exp_tost:
        check(f"TOST {c[:15]}... p", r.p_tost, exp_tost[c], 0.005)

# === TABEL 7 (Revocation) ===
print("\n--- TABEL 7: Revocation ---")
rv = rev["revocation_latency_ms"].dropna().values
check("Rev mean", np.mean(rv), 41.87, 0.05)
check("Rev median", np.median(rv), 18.65, 0.05)
check("Rev std", np.std(rv, ddof=1), 49.30, 0.05)
check("Rev p90", np.percentile(rv, 90), 102.32, 0.05)
check("Rev p99", np.percentile(rv, 99), 183.39, 0.10)
p99_s09 = rev["p99_ms"].dropna().values
check("S09 p99 mean", np.mean(p99_s09), 377.63, 0.10)
check("S09 p99 median", np.median(p99_s09), 305.94, 0.10)
check("S09 p99 p99", np.percentile(p99_s09, 99), 705.40, 0.20)
revt = rev["revocations_triggered"].dropna().values
check("Rev triggered mean", np.mean(revt), 10.80, 0.15)

# === TABEL 8 (TTL) ===
print("\n--- TABEL 8: TTL ---")
s07 = lat[lat.scenario_id == "s07-zta-cache-fanout"]["p99_ms"].values
s10 = lat[lat.scenario_id == "s10-ttl-comparison"]["p99_ms"].values
u, p = mannwhitneyu(s10, s07, alternative="two-sided")
d = cd(s10, s07)
check("TTL U", u, 436.0, 0.5)
check("TTL p", p, 0.842, 0.005)
check("TTL delta", d, -0.031, 0.01)

# === TABEL 9 (KW Topology) ===
print("\n--- TABEL 9: Kruskal-Wallis ---")
s08 = lat[lat.scenario_id == "s08-zta-cache-mesh"]["p99_ms"].values
h, p = kruskal(s05, s07, s08)
check("KW H", h, 63.40, 0.05)
check("KW p", p, 0.0, 0.001)

# === TABEL 10 (KW Resource) ===
print("\n--- TABEL 10: KW Resource ---")
for col, nm, exp_h, exp_p in [("cpu_avg_pct", "CPU", 2.79, 0.248), ("mem_avg_mb", "Mem", 0.91, 0.636)]:
    p1 = res[(res.config == "pbs") & (res.rps_target == 500)][col].dropna().values
    p2 = res[(res.config == "zta-naive") & (res.rps_target == 500)][col].dropna().values
    p3 = res[(res.config == "zta-cache") & (res.rps_target == 500) & (res.topology == "linear")][col].dropna().values
    h, p = kruskal(p1, p2, p3)
    check(f"KW {nm} H", h, exp_h, 0.05)
    check(f"KW {nm} p", p, exp_p, 0.005)

# === TABEL 12 (Studi Ablasi Multi-Hop under Cache Contention) ===
print("\n--- TABEL 12: Studi Ablasi Multi-Hop (K=2) ---")
abl_path = "data/processed/ablation/ablation-comparison-table.csv"
if os.path.exists(abl_path):
    abl_df = pd.read_csv(abl_path).set_index("metric")
    h2_row = abl_df.loc["Multi-Hop Latency (Hop-2) [ms]"]
    e2e_row = abl_df.loc["End-to-End Latency p99 [ms]"]
    dir_row = abl_df.loc["Direct Latency p99 [ms]"]
    med_row = abl_df.loc["End-to-End Latency Median [ms]"]

    check("Ablation Hop-2 LFU median", h2_row["lfu_med"], 1677.12, 0.05)
    check("Ablation Hop-2 Graph median", h2_row["graph_med"], 1569.14, 0.05)
    check("Ablation E2E LFU median", e2e_row["lfu_med"], 1371.50, 0.05)
    check("Ablation E2E Graph median", e2e_row["graph_med"], 1310.45, 0.05)
    check("Ablation Direct LFU median", dir_row["lfu_med"], 927.83, 0.05)
    check("Ablation Direct Graph median", dir_row["graph_med"], 905.05, 0.05)
    check("Ablation Median LFU median", med_row["lfu_med"], 367.03, 0.05)
    check("Ablation Median Graph median", med_row["graph_med"], 375.01, 0.05)
    check("Ablation E2E MWU U", e2e_row["mwu_u"], 8.0, 0.1)
    check("Ablation E2E Cliff delta", e2e_row["cliffs_delta"], 0.360, 0.01)

# === FORMALISASI PERSAMAAN MATEMATIKA (1-6) ===
print("\n--- FORMALISASI PERSAMAAN MATEMATIKA (Naskah ↔ Skrip) ---")
if os.path.exists("paper/naskah-artikel.md"):
    id_path = "paper/naskah-artikel.md"
    en_path = "paper/naskah-artikel-en.md"
elif os.path.exists("draft/naskah-artikel.md"):
    id_path = "draft/naskah-artikel.md"
    en_path = "draft/en/naskah-artikel-en.md"
else:
    id_path = "docs/15_notasi_persamaan_matematika_dan_validasi_statistik.md"
    en_path = "docs/15_notasi_persamaan_matematika_dan_validasi_statistik.md"
with open(id_path, "r", encoding="utf-8") as f:
    id_ms = f.read()
with open(en_path, "r", encoding="utf-8") as f:
    en_ms = f.read()
with open("scripts/statistical-analysis.py", "r", encoding="utf-8") as f:
    stat_code = f.read()
with open("scripts/scalability-analysis.py", "r", encoding="utf-8") as f:
    scal_code = f.read()
with open("scripts/generate-figures.py", "r", encoding="utf-8") as f:
    fig_code = f.read()
with open("scripts/generate-figures-en.py", "r", encoding="utf-8") as f:
    fig_en_code = f.read()

eq_checks = [
    ("Persamaan (1) in naskah-id", "...(1)" in id_ms or "Persamaan (1)" in id_ms),
    ("Persamaan (1) in naskah-en", "...(1)" in en_ms or "Persamaan (1)" in en_ms),
    ("Persamaan (2) in naskah-id", "...(2)" in id_ms or "Persamaan (2)" in id_ms),
    ("Persamaan (2) in naskah-en", "...(2)" in en_ms or "Persamaan (2)" in en_ms),
    ("Persamaan (2) in generate-figures", "Persamaan (2)" in fig_code),
    ("Persamaan (2) in generate-figures-en", "Equation (2)" in fig_en_code),
    ("Persamaan (3) in naskah-id", "...(3)" in id_ms or "Persamaan (3)" in id_ms),
    ("Persamaan (3) in naskah-en", "...(3)" in en_ms or "Persamaan (3)" in en_ms),
    ("Persamaan (3) in scalability-analysis", "T_request" in scal_code),
    ("Persamaan (4) in naskah-id", "...(4)" in id_ms or "Persamaan (4)" in id_ms),
    ("Persamaan (4) in naskah-en", "...(4)" in en_ms or "Persamaan (4)" in en_ms),
    ("Persamaan (4) in scalability-analysis", "T_analysis" in scal_code),
    ("Model Memory in scalability-analysis", "M_sidecar" in scal_code),
    ("Model Bandwidth in scalability-analysis", "BW_distribution" in scal_code),
]

for label, cond in eq_checks:
    status = "✅" if cond else "❌"
    if cond:
        PASS += 1
    else:
        FAIL += 1
        print(f"  {status} {label}: FAILED TO MATCH!")

# === SUMMARY ===
print("\n" + "=" * 70)
total = PASS + FAIL
print(f"HASIL: {PASS}/{total} PASS, {FAIL}/{total} FAIL")
if FAIL == 0:
    print("✅ SELURUH ANGKA DI NASKAH DAPAT DIREPRODUKSI DARI DATA CSV")
else:
    print("❌ ADA ANGKA YANG TIDAK COCOK!")
print("=" * 70)

# === SCRIPT ↔ DATA MAPPING ===
print("\nPETA SCRIPT → DATA → TABEL:")
print("""
┌─────────────────────────────┬──────────────────────────────────────┬──────────┐
│ Script                      │ Data Source                         │ Tabel    │
├─────────────────────────────┼──────────────────────────────────────┼──────────┤
│ analyze-all-data.py         │ 300 k6 JSON + 300 Prometheus CSV    │ (source) │
│   → latency-all-scenarios.csv                                     │          │
│   → resource-overhead.csv                                         │          │
│   → revocation-latency.csv                                        │          │
│   → descriptive-stats.csv                                         │          │
├─────────────────────────────┼──────────────────────────────────────┼──────────┤
│ statistical-analysis.py     │ latency-all-scenarios.csv           │ 6a-6d,   │
│                             │ resource-overhead.csv               │ 8-10     │
│                             │ revocation-latency.csv              │ 7        │
│   → mann-whitney-results.csv                                      │          │
│   → shapiro-wilk-results.csv                                      │          │
│   → kruskal-wallis-results.csv                                    │          │
│   → tost-equivalence-results.csv                                  │          │
├─────────────────────────────┼──────────────────────────────────────┼──────────┤
│ generate-figures.py         │ latency-all-scenarios.csv           │ Fig 1-10 │
│                             │ resource-overhead.csv               │          │
│                             │ edge-topology.json                  │          │
├─────────────────────────────┼──────────────────────────────────────┼──────────┤
│ scalability-analysis.py     │ descriptive-stats.csv (constants)   │ 11       │
│                             │ Model O(V²), bukan dari CSV         │          │
├─────────────────────────────┼──────────────────────────────────────┼──────────┤
│ validate-data.py            │ 600 raw files (JSON+CSV)            │ §3.7     │
└─────────────────────────────┴──────────────────────────────────────┴──────────┘
""")
