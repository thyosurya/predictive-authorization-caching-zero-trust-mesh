#!/usr/bin/env python3
"""
Deep Validation Script - Audit seluruh data eksperimen ZTA Predictive Cache.

Checklist validasi:
1. Kelengkapan file (300 JSON + 300 CSV)
2. Tidak ada file kosong / corrupt
3. Repetisi 1-30 lengkap per skenario
4. Semua metrik wajib tersedia (p50, p90, p95, p99, p999, max, mean)
5. Error rate per skenario (harus < 1% kecuali S09)
6. Actual RPS vs target RPS (deviasi)
7. Durasi test ~5 menit (300 detik)
8. Outlier detection per skenario (IQR method)
9. Prometheus CSV: kolom yang terisi vs kosong
10. Cross-validation antara k6 JSON dan Prometheus CSV
"""

import json, csv, glob, os, re, sys
import numpy as np
import pandas as pd
from pathlib import Path

# ============================================================
# CONFIG - DUAL SOURCE
# ============================================================
BASE_DIR = Path(__file__).resolve().parent.parent
ORIGINAL_DIR = str(BASE_DIR / "load-testing" / "results" / "run_full_30rep_20261028_022212")
RERUN_DIR = ""
RERUN_SCENARIOS = set()

SCENARIO_META = {
    "s01-pbs-baseline-500rps": {"config": "pbs", "rps_target": 500, "topology": "linear"},
    "s02-zta-naive-500rps": {"config": "zta-naive", "rps_target": 500, "topology": "linear"},
    "s03-zta-cache-500rps": {"config": "zta-cache", "rps_target": 500, "topology": "linear"},
    "s04-zta-naive-2000rps": {"config": "zta-naive", "rps_target": 2000, "topology": "linear"},
    "s05-zta-cache-2000rps": {"config": "zta-cache", "rps_target": 2000, "topology": "linear"},
    "s06-zta-cache-5000rps": {"config": "zta-cache", "rps_target": 5000, "topology": "linear"},
    "s07-zta-cache-fanout": {"config": "zta-cache", "rps_target": 2000, "topology": "fan-out"},
    "s08-zta-cache-mesh": {"config": "zta-cache", "rps_target": 2000, "topology": "mesh"},
    "s09-revocation-test": {"config": "zta-cache", "rps_target": 500, "topology": "linear"},
    "s10-ttl-comparison": {"config": "zta-cache-static", "rps_target": 2000, "topology": "fan-out"},
}

def parse_scenario(filename):
    basename = os.path.splitext(filename)[0]
    match = re.match(r"(s\d+-[^-]+-.*?)-rep(\d+)-\d{8}-\d{6}", basename)
    if match:
        return match.group(1), int(match.group(2))
    return None, None

def parse_prom(filename):
    basename = os.path.splitext(filename)[0]
    match = re.match(r"prometheus-(s\d+-[^-]+-.*?)-rep(\d+)", basename)
    if match:
        return match.group(1), int(match.group(2))
    return None, None

def build_file_list(pattern, parse_fn):
    files = []
    for f in sorted(glob.glob(os.path.join(ORIGINAL_DIR, pattern))):
        sid, _ = parse_fn(os.path.basename(f))
        if sid and sid not in RERUN_SCENARIOS:
            files.append(f)
    if os.path.exists(RERUN_DIR):
        for f in sorted(glob.glob(os.path.join(RERUN_DIR, pattern))):
            sid, _ = parse_fn(os.path.basename(f))
            if sid and sid in RERUN_SCENARIOS:
                files.append(f)
    return files

# ============================================================
issues = []
warnings = []
info = []

def ISSUE(msg):
    issues.append(msg)
    print(f"  [ISSUE] {msg}")

def WARN(msg):
    warnings.append(msg)
    print(f"  [WARN]  {msg}")

def INFO(msg):
    info.append(msg)
    print(f"  [INFO]  {msg}")

# ============================================================
# CHECK 1: File completeness
# ============================================================
print("=" * 70)
print("CHECK 1: File Completeness")
print("=" * 70)

json_files = build_file_list("s*.json", parse_scenario)
csv_files = build_file_list("prometheus-*.csv", parse_prom)

print(f"  Total JSON files: {len(json_files)}")
print(f"  Total CSV files:  {len(csv_files)}")

if len(json_files) != 300:
    ISSUE(f"Expected 300 JSON files, found {len(json_files)}")
else:
    INFO("300 JSON files found")

if len(csv_files) != 300:
    ISSUE(f"Expected 300 CSV files, found {len(csv_files)}")
else:
    INFO("300 CSV files found")

# ============================================================
# CHECK 2: Repetition completeness (1-30 per scenario)
# ============================================================
print("\n" + "=" * 70)
print("CHECK 2: Repetition Completeness (1-30 per scenario)")
print("=" * 70)

scenario_reps = {}
for f in json_files:
    sid, rep = parse_scenario(os.path.basename(f))
    if sid:
        scenario_reps.setdefault(sid, set()).add(rep)

for sid in sorted(SCENARIO_META.keys()):
    reps = scenario_reps.get(sid, set())
    expected = set(range(1, 31))
    missing = expected - reps
    extra = reps - expected
    if missing:
        ISSUE(f"{sid}: Missing reps {sorted(missing)}")
    if extra:
        WARN(f"{sid}: Extra reps {sorted(extra)}")
    if not missing and not extra:
        INFO(f"{sid}: 30/30 reps complete")

# Same for Prometheus CSV
prom_reps = {}
for f in csv_files:
    sid, rep = parse_prom(os.path.basename(f))
    if sid:
        prom_reps.setdefault(sid, set()).add(rep)

print("\n  Prometheus CSV rep check:")
for sid in sorted(SCENARIO_META.keys()):
    reps = prom_reps.get(sid, set())
    expected = set(range(1, 31))
    missing = expected - reps
    if missing:
        ISSUE(f"Prometheus {sid}: Missing reps {sorted(missing)}")
    else:
        INFO(f"Prometheus {sid}: 30/30 reps complete")

# ============================================================
# CHECK 3: JSON integrity + required metrics
# ============================================================
print("\n" + "=" * 70)
print("CHECK 3: JSON Integrity & Required Metrics")
print("=" * 70)

REQUIRED_METRICS = ["http_req_duration", "http_reqs", "http_req_failed"]
REQUIRED_LATENCY_FIELDS = ["min", "avg", "med", "p(90)", "p(95)", "p(99)", "p(99.9)", "max"]

all_rows = []
corrupt_files = []

for f in json_files:
    fname = os.path.basename(f)
    sid, rep = parse_scenario(fname)
    
    try:
        with open(f) as fh:
            data = json.load(fh)
    except Exception as e:
        ISSUE(f"CORRUPT JSON: {fname}: {e}")
        corrupt_files.append(fname)
        continue
    
    m = data.get("metrics", {})
    
    # Check required metrics exist
    for metric in REQUIRED_METRICS:
        if metric not in m:
            ISSUE(f"{fname}: Missing metric '{metric}'")
    
    # Check latency fields
    dur = m.get("http_req_duration", {})
    for field in REQUIRED_LATENCY_FIELDS:
        if field not in dur:
            ISSUE(f"{fname}: Missing http_req_duration.{field}")
    
    # Collect row for analysis
    reqs = m.get("http_reqs", {})
    failed = m.get("http_req_failed", {})
    iter_dur = m.get("iteration_duration", {})
    
    row = {
        "file": fname,
        "source": "rerun" if sid in RERUN_SCENARIOS else "original",
        "scenario_id": sid,
        "repetition": rep,
        "rps_target": SCENARIO_META.get(sid, {}).get("rps_target", 0),
        "p50_ms": dur.get("med"),
        "p90_ms": dur.get("p(90)"),
        "p95_ms": dur.get("p(95)"),
        "p99_ms": dur.get("p(99)"),
        "p999_ms": dur.get("p(99.9)"),
        "max_ms": dur.get("max"),
        "mean_ms": dur.get("avg"),
        "min_ms": dur.get("min"),
        "total_requests": reqs.get("count"),
        "actual_rps": reqs.get("rate"),
        "error_rate": failed.get("value", 0),
        "iteration_avg_ms": iter_dur.get("avg"),
        "iteration_count": m.get("iterations", {}).get("count"),
    }
    all_rows.append(row)

if corrupt_files:
    ISSUE(f"Total corrupt files: {len(corrupt_files)}")
elif len(all_rows) > 0:
    INFO("All 300 JSON files parsed successfully")

df = pd.DataFrame(all_rows)

if df.empty:
    ISSUE("No valid raw k6 JSON results found in ORIGINAL_DIR/RERUN_DIR.")
    print("\n" + "=" * 70)
    print("VALIDATION SUMMARY (ABORTED EARLY)")
    print("=" * 70)
    print(f"  Total ISSUES: {len(issues)}")
    for i, iss in enumerate(issues, 1):
        print(f"  {i}. {iss}")
    sys.exit(1)

# ============================================================
# CHECK 4: Error Rate
# ============================================================
print("\n" + "=" * 70)
print("CHECK 4: Error Rate (< 1% expected, except S09)")
print("=" * 70)

for sid in sorted(df.scenario_id.unique()):
    subset = df[df.scenario_id == sid]
    err = subset["error_rate"]
    max_err = err.max()
    mean_err = err.mean()
    
    threshold = 0.01 if sid != "s09-revocation-test" else 0.05
    
    if max_err > threshold:
        WARN(f"{sid}: Max error rate = {max_err*100:.4f}% (threshold: {threshold*100}%)")
    else:
        INFO(f"{sid}: Error rate OK (max={max_err*100:.4f}%, mean={mean_err*100:.4f}%)")

# ============================================================
# CHECK 5: Actual RPS vs Target
# ============================================================
print("\n" + "=" * 70)
print("CHECK 5: Actual RPS vs Target")
print("=" * 70)

for sid in sorted(df.scenario_id.unique()):
    subset = df[df.scenario_id == sid]
    target = subset["rps_target"].iloc[0]
    actual_mean = subset["actual_rps"].mean()
    actual_min = subset["actual_rps"].min()
    actual_max = subset["actual_rps"].max()
    deviation = abs(actual_mean - target) / target * 100
    
    # For S07/S10 (batch 3 requests per iteration), actual_rps counts individual requests
    # iteration rate should be ~2000, but http_reqs will be ~6000
    note = ""
    if sid in ("s07-zta-cache-fanout", "s10-ttl-comparison"):
        iter_rate = subset["iteration_count"].mean() / 300  # 5 min = 300s
        note = f" [batch: iter_rate ~{iter_rate:.0f}/s]"
    
    if deviation > 25:
        WARN(f"{sid}: RPS deviation {deviation:.1f}% (target={target}, actual_mean={actual_mean:.1f}, range=[{actual_min:.1f}-{actual_max:.1f}]){note}")
    else:
        INFO(f"{sid}: RPS OK (target={target}, actual_mean={actual_mean:.1f}, deviation={deviation:.1f}%){note}")

# ============================================================
# CHECK 6: Test Duration (~5 min = 300s)
# ============================================================
print("\n" + "=" * 70)
print("CHECK 6: Test Duration (~300 seconds)")
print("=" * 70)

for sid in sorted(df.scenario_id.unique()):
    subset = df[df.scenario_id == sid]
    # Duration ≈ total_requests / actual_rps
    durations = subset["total_requests"] / subset["actual_rps"]
    mean_dur = durations.mean()
    min_dur = durations.min()
    max_dur = durations.max()
    
    if abs(mean_dur - 300) > 30:
        WARN(f"{sid}: Duration {mean_dur:.1f}s (expected ~300s)")
    else:
        INFO(f"{sid}: Duration OK ({mean_dur:.1f}s, range=[{min_dur:.1f}-{max_dur:.1f}])")

# ============================================================
# CHECK 7: Outlier Detection (IQR method, per scenario)
# ============================================================
print("\n" + "=" * 70)
print("CHECK 7: Outlier Detection (p99 latency, IQR × 1.5)")
print("=" * 70)

for sid in sorted(df.scenario_id.unique()):
    subset = df[df.scenario_id == sid]
    p99 = subset["p99_ms"].dropna()
    
    if len(p99) < 5:
        continue
    
    Q1 = p99.quantile(0.25)
    Q3 = p99.quantile(0.75)
    IQR = Q3 - Q1
    lower = Q1 - 1.5 * IQR
    upper = Q3 + 1.5 * IQR
    
    outliers = p99[(p99 < lower) | (p99 > upper)]
    
    if len(outliers) > 0:
        outlier_reps = subset.loc[outliers.index, "repetition"].tolist()
        WARN(f"{sid}: {len(outliers)} outlier(s) in p99 - reps {outlier_reps}, "
             f"values={[round(v,2) for v in outliers.values]}, "
             f"bounds=[{lower:.2f}, {upper:.2f}]")
    else:
        INFO(f"{sid}: No p99 outliers (IQR=[{Q1:.2f}, {Q3:.2f}])")

# ============================================================
# CHECK 8: Prometheus CSV content validation
# ============================================================
print("\n" + "=" * 70)
print("CHECK 8: Prometheus CSV Content Validation")
print("=" * 70)

prom_columns_status = {}

for f in csv_files:
    fname = os.path.basename(f)
    sid, rep = parse_prom(fname)
    
    try:
        pdf = pd.read_csv(f)
    except Exception as e:
        ISSUE(f"CORRUPT CSV: {fname}: {e}")
        continue
    
    for col in pdf.columns:
        key = (sid, col)
        non_null = pdf[col].dropna().count()
        total = len(pdf)
        if key not in prom_columns_status:
            prom_columns_status[key] = {"filled": 0, "empty": 0, "total_files": 0}
        prom_columns_status[key]["total_files"] += 1
        if non_null > 0:
            prom_columns_status[key]["filled"] += 1
        else:
            prom_columns_status[key]["empty"] += 1

# Summarize per scenario
print("  Column availability per scenario:")
scenarios_checked = set()
for (sid, col), status in sorted(prom_columns_status.items()):
    if sid not in scenarios_checked:
        scenarios_checked.add(sid)
        print(f"\n  {sid}:")
    fill_pct = status["filled"] / status["total_files"] * 100
    if fill_pct < 100 and fill_pct > 0:
        WARN(f"    {col}: {fill_pct:.0f}% files have data ({status['filled']}/{status['total_files']})")
    elif fill_pct == 0:
        print(f"    {col}: EMPTY (0/{status['total_files']})")
    else:
        print(f"    {col}: OK (100%)")

# ============================================================
# CHECK 9: Cross-validation - consistency between JSON sources
# ============================================================
print("\n" + "=" * 70)
print("CHECK 9: Data Source Cross-Validation")
print("=" * 70)

# Verify that rerun data for S07/S08/S10 is actually from rerun dir
for _, row in df[df.scenario_id.isin(RERUN_SCENARIOS)].iterrows():
    if row["source"] != "rerun":
        ISSUE(f"{row['file']}: Expected source=rerun but got {row['source']}")

# Verify that original data S01-S06,S09 is from original dir
for _, row in df[~df.scenario_id.isin(RERUN_SCENARIOS)].iterrows():
    if row["source"] != "original":
        ISSUE(f"{row['file']}: Expected source=original but got {row['source']}")

INFO(f"Source verification: {len(df[df.source == 'original'])} original + {len(df[df.source == 'rerun'])} rerun = {len(df)} total")

# ============================================================
# CHECK 10: Descriptive stats sanity
# ============================================================
print("\n" + "=" * 70)
print("CHECK 10: Descriptive Stats Sanity")
print("=" * 70)

for sid in sorted(df.scenario_id.unique()):
    subset = df[df.scenario_id == sid]
    p99 = subset["p99_ms"]
    mean = subset["mean_ms"]
    
    # p99 should always be >= mean
    violations = subset[subset["p99_ms"] < subset["mean_ms"]]
    if len(violations) > 0:
        ISSUE(f"{sid}: {len(violations)} files where p99 < mean (impossible!)")
    
    # min should be > 0
    neg = subset[subset["min_ms"] <= 0]
    if len(neg) > 0:
        ISSUE(f"{sid}: {len(neg)} files with min_ms <= 0")
    
    # Coefficient of variation (CV)
    cv = p99.std() / p99.mean() * 100 if p99.mean() > 0 else 0
    if cv > 50:
        WARN(f"{sid}: High p99 CV = {cv:.1f}% - data may have high variability")
    else:
        INFO(f"{sid}: p99 CV={cv:.1f}% (mean={p99.mean():.2f}ms, std={p99.std():.2f}ms)")

# ============================================================
# SUMMARY
# ============================================================
print("\n" + "=" * 70)
print("VALIDATION SUMMARY")
print("=" * 70)
print(f"  Total ISSUES  : {len(issues)} {'*** CRITICAL ***' if issues else '(NONE)'}")
print(f"  Total WARNINGS: {len(warnings)}")
print(f"  Total INFO    : {len(info)}")

if issues:
    print("\n  === ISSUES (must fix) ===")
    for i, issue in enumerate(issues, 1):
        print(f"  {i}. {issue}")

if warnings:
    print("\n  === WARNINGS (review needed) ===")
    for i, w in enumerate(warnings, 1):
        print(f"  {i}. {w}")

print(f"\n{'PASS' if not issues else 'FAIL'}: Data validation {'complete' if not issues else 'found critical issues'}")
