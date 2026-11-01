#!/usr/bin/env python3
"""
bundle-replication-repo.py
===========================
Automated generator for the clean, standalone replication package & artifact
evaluation suite for the academic paper:
"Predictive Authorization Caching with Microservice Graph Prioritization in Zero-Trust Service Mesh"
(Moh. Radithyo Surya Martuah, Universitas Negeri Gorontalo)

Target GitHub Repo:
https://github.com/thyosurya/predictive-authorization-caching-zero-trust-mesh
"""

import os
import sys
import shutil
import subprocess
import stat
from pathlib import Path

SRC = Path(__file__).resolve().parent.parent
REPO_NAME = "predictive-authorization-caching-zero-trust-mesh"
DEST = SRC / REPO_NAME

print("=" * 70)
print("BUILDING CLEAN, FRESH STANDALONE REPLICATION PACKAGE")
print("=" * 70)
print(f"Source Directory : {SRC}")
print(f"Target Directory : {DEST}\n")


def handle_remove_readonly(func, path, exc):
    try:
        os.chmod(path, stat.S_IWRITE)
        func(path)
    except Exception:
        pass


# 1. Clean previous destination directory
if DEST.exists():
    print(f"[*] Removing existing destination folder: {DEST}")
    try:
        shutil.rmtree(DEST, onexc=handle_remove_readonly)
    except TypeError:
        shutil.rmtree(DEST, onerror=handle_remove_readonly)
DEST.mkdir(parents=True)
print("[+] Target folder initialized.\n")

# 2. Copy system source components
CORE_DIRS = [
    'control-plane',
    'wasm-filter',
    'services',
    'k8s',
    'observability',
]

for d in CORE_DIRS:
    src_d = SRC / d
    dst_d = DEST / d
    if src_d.exists():
        shutil.copytree(src_d, dst_d, ignore=shutil.ignore_patterns('__pycache__', '*.pyc', '.git'))
        print(f"  [+] Copied system component: {d}/")

# 3. Copy technical documentation (strictly omitting internal author notes)
DOCS_SRC = SRC / 'docs'
DOCS_DEST = DEST / 'docs'
DOCS_DEST.mkdir(parents=True, exist_ok=True)

OMIT_DOCS = {
    '13_remediation_and_patch_specification.md',
    'final_submission_metadata.md',
    'instrumen_penelitian_zta_predictive_cache.md',
    'panduan-publikasi-repo-replikasi.md',
    'rencana-publikasi-dan-kolaborasi.md',
    'response_to_reviewers.md',
    'tanggapan_reviewer.md',
}

doc_count = 0
for f in sorted(DOCS_SRC.glob('*.md')):
    if f.name not in OMIT_DOCS:
        shutil.copy2(f, DOCS_DEST / f.name)
        doc_count += 1
print(f"  [+] Copied {doc_count} technical architecture and methodology docs to docs/ (omitted {len(OMIT_DOCS)} internal author docs)")

# 4. Copy load-testing scenarios & ONLY canonical raw results
LT_DEST = DEST / 'load-testing'
LT_DEST.mkdir(parents=True, exist_ok=True)

# Copy scenarios and mini-scenarios
if (SRC / 'load-testing' / 'scenarios').exists():
    shutil.copytree(SRC / 'load-testing' / 'scenarios', LT_DEST / 'scenarios')
if (SRC / 'load-testing' / 'mini-scenarios').exists():
    shutil.copytree(SRC / 'load-testing' / 'mini-scenarios', LT_DEST / 'mini-scenarios')
for f in ['run-mini-suite.sh', 'warmup.js']:
    if (SRC / 'load-testing' / f).exists():
        shutil.copy2(SRC / 'load-testing' / f, LT_DEST / f)

# Copy ONLY canonical results: run_full_30rep_20261028_022212 and ablation
RESULTS_DEST = LT_DEST / 'results'
RESULTS_DEST.mkdir(parents=True, exist_ok=True)

canonical_30rep = SRC / 'load-testing' / 'results' / 'run_full_30rep_20261028_022212'
if canonical_30rep.exists():
    shutil.copytree(canonical_30rep, RESULTS_DEST / 'run_full_30rep_20261028_022212')
    print("  [+] Copied canonical 30-rep raw dataset: run_full_30rep_20261028_022212 (600 files)")

ablation_results = SRC / 'load-testing' / 'results' / 'ablation'
if ablation_results.exists():
    shutil.copytree(ablation_results, RESULTS_DEST / 'ablation')
    print("  [+] Copied canonical ablation raw dataset: ablation/ (11 files)")

# 5. Copy processed datasets and reports
shutil.copytree(SRC / 'data' / 'processed', DEST / 'data' / 'processed', dirs_exist_ok=True, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
print("  [+] Copied consolidated datasets to data/processed/")

shutil.copytree(SRC / 'data' / 'reports', DEST / 'data' / 'reports', dirs_exist_ok=True, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
print("  [+] Copied statistical reports and SHA-256 manifest to data/reports/")

# 6. Copy 10 official figures (Indonesian and English)
(DEST / 'figures').mkdir(parents=True, exist_ok=True)
for f in (SRC / 'draft' / 'figures').glob('*.png'):
    shutil.copy2(f, DEST / 'figures' / f.name)
print("  [+] Copied 10 canonical figures to figures/")

if (SRC / 'draft' / 'en' / 'figures').exists():
    (DEST / 'figures' / 'en').mkdir(parents=True, exist_ok=True)
    for f in (SRC / 'draft' / 'en' / 'figures').glob('*.png'):
        shutil.copy2(f, DEST / 'figures' / 'en' / f.name)
    print("  [+] Copied 10 English canonical figures to figures/en/")

# 7. Copy ONLY essential public replication scripts
PUBLIC_SCRIPTS_WHITELIST = {
    'verify_traceability.py',
    'validate-data.py',
    'analyze-all-data.py',
    'statistical-analysis.py',
    'analyze_ablation_results.py',
    'generate-figures.py',
    'generate-figures-en.py',
    'generate_ablation_figure.py',
    'scalability-analysis.py',
    'test_analyze_ablation.py',
    'test_local_end_to_end.py',
    'generate_replication_manifest.py',
    'verify_offline.py',
    'export-metrics.py',
}

SCRIPTS_DEST = DEST / 'scripts'
SCRIPTS_DEST.mkdir(parents=True, exist_ok=True)

for script_name in PUBLIC_SCRIPTS_WHITELIST:
    src_script = SRC / 'scripts' / script_name
    if src_script.exists():
        shutil.copy2(src_script, SCRIPTS_DEST / script_name)

print(f"  [+] Copied {len(PUBLIC_SCRIPTS_WHITELIST)} public replication scripts to scripts/ (omitted internal author scripts)")

# 9. Adapt paths dynamically in copied scripts
print("  [*] Configuring portable relative paths across replication scripts...")

# verify_traceability.py
vt_file = SCRIPTS_DEST / 'verify_traceability.py'
if vt_file.exists():
    text = vt_file.read_text(encoding='utf-8')
    lines = text.splitlines()
    new_lines = []
    for line in lines:
        if line.strip().startswith('os.chdir(r"d:\\Penelitian') or line.strip().startswith('os.chdir(r"D:\\Penelitian'):
            new_lines.append("from pathlib import Path")
            new_lines.append("BASE_DIR = Path(__file__).resolve().parent.parent")
            new_lines.append("os.chdir(BASE_DIR)")
        else:
            new_lines.append(line)
    vt_file.write_text("\n".join(new_lines) + "\n", encoding='utf-8')

# analyze-all-data.py
aad_file = SCRIPTS_DEST / 'analyze-all-data.py'
if aad_file.exists():
    lines = aad_file.read_text(encoding='utf-8').splitlines()
    new_lines = []
    for line in lines:
        if 'ORIGINAL_DIR = r"d:\\Penelitian' in line or 'ORIGINAL_DIR = r"D:\\Penelitian' in line:
            new_lines.append("BASE_DIR = Path(__file__).resolve().parent.parent")
            new_lines.append('ORIGINAL_DIR = str(BASE_DIR / "load-testing" / "results" / "run_full_30rep_20261028_022212")')
        elif 'RERUN_DIR =' in line:
            new_lines.append('RERUN_DIR = ""')
        elif 'PROCESSED_DIR = r"d:\\Penelitian' in line or 'PROCESSED_DIR = r"D:\\Penelitian' in line:
            new_lines.append('PROCESSED_DIR = str(BASE_DIR / "data" / "processed")')
        elif 'REPORTS_DIR = r"d:\\Penelitian' in line or 'REPORTS_DIR = r"D:\\Penelitian' in line:
            new_lines.append('REPORTS_DIR = str(BASE_DIR / "data" / "reports")')
        else:
            new_lines.append(line)
    aad_file.write_text("\n".join(new_lines) + "\n", encoding='utf-8')

# validate-data.py
vd_file = SCRIPTS_DEST / 'validate-data.py'
if vd_file.exists():
    lines = vd_file.read_text(encoding='utf-8').splitlines()
    new_lines = []
    for line in lines:
        if 'ORIGINAL_DIR = r"d:\\Penelitian' in line or 'ORIGINAL_DIR = r"D:\\Penelitian' in line:
            new_lines.append("BASE_DIR = Path(__file__).resolve().parent.parent")
            new_lines.append('ORIGINAL_DIR = str(BASE_DIR / "load-testing" / "results" / "run_full_30rep_20261028_022212")')
        elif 'RERUN_DIR =' in line:
            new_lines.append('RERUN_DIR = ""')
        else:
            new_lines.append(line)
    vd_file.write_text("\n".join(new_lines) + "\n", encoding='utf-8')

# generate-figures.py
gf_file = SCRIPTS_DEST / 'generate-figures.py'
if gf_file.exists():
    text = gf_file.read_text(encoding='utf-8')
    text = text.replace('FIGS = BASE / "draft" / "figures"', 'FIGS = BASE / "figures"')
    gf_file.write_text(text, encoding='utf-8')

# generate-figures-en.py
gfe_file = SCRIPTS_DEST / 'generate-figures-en.py'
if gfe_file.exists():
    text = gfe_file.read_text(encoding='utf-8')
    text = text.replace('FIGS = BASE / "draft" / "en" / "figures"', 'FIGS = BASE / "figures" / "en"')
    gfe_file.write_text(text, encoding='utf-8')

# generate_ablation_figure.py
gaf_file = SCRIPTS_DEST / 'generate_ablation_figure.py'
if gaf_file.exists():
    text = gaf_file.read_text(encoding='utf-8')
    text = text.replace('FIGS_ID = BASE / "draft" / "figures"', 'FIGS_ID = BASE / "figures"')
    text = text.replace('FIGS_EN = BASE / "draft" / "en" / "figures"', 'FIGS_EN = BASE / "figures" / "en"')
    gaf_file.write_text(text, encoding='utf-8')

# 10. Copy root configuration files
for root_f in ['requirements.txt', 'docker-compose.yml', 'Makefile']:
    if (SRC / root_f).exists():
        shutil.copy2(SRC / root_f, DEST / root_f)

# Write clean .gitignore
gitignore_content = """# Python
__pycache__/
*.py[cod]
*$py.class
.venv/
venv/

# OS & IDE
.DS_Store
Thumbs.db
.idea/
.vscode/
*.swp
*.swo

# Temporary
*.tmp
*.log
"""
(DEST / '.gitignore').write_text(gitignore_content, encoding='utf-8')

# Write LICENSE (GPLv3)
gplv3_content = """                    GNU GENERAL PUBLIC LICENSE
                       Version 3, 29 June 2007

 Copyright (C) 2007 Free Software Foundation, Inc. <https://fsf.org/>
 Everyone is permitted to copy and distribute verbatim copies
 of this license document, but changing it is not allowed.

                            Preamble

  The GNU General Public License is a free, copyleft license for
software and other kinds of works.

  The licenses for most software and other practical works are designed
to take away your freedom to share and change the works.  By contrast,
the GNU General Public License is intended to guarantee your freedom to
share and change all versions of a program--to make sure it remains free
software for all its users.  We, the Free Software Foundation, use the
GNU General Public License for most of our software; it applies also to
any other work released this way by its authors.  You can apply it to
your programs, too.

Copyright (c) 2026 Moh. Radithyo Surya Martuah <thyo.surya@gmail.com>
Universitas Negeri Gorontalo

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
"""
(DEST / 'LICENSE').write_text(gplv3_content, encoding='utf-8')

# Write professional, comprehensive README.md
readme_content = """# Replication Package: Predictive Authorization Caching with Microservice Graph Prioritization in Zero-Trust Service Mesh

[![Artifact Evaluation](https://img.shields.io/badge/Artifact-Available%20%26%20Reproducible-success.svg)](#)
[![Python](https://img.shields.io/badge/Python-3.10%2B-blue.svg)](https://www.python.org/)
[![Go](https://img.shields.io/badge/Go-1.22%2B-blue.svg)](https://golang.org/)
[![Traceability](https://img.shields.io/badge/Traceability%20Audit-119%2F119%20PASS-brightgreen.svg)](#1-quick-start-1-click-traceability-verification-10-seconds)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

Official, standalone **Replication Package & Artifact Evaluation Suite** for the academic paper:

> **"Predictive Authorization Caching with Microservice Graph Prioritization in Zero-Trust Service Mesh"**  
> *Moh. Radithyo Surya Martuah (Universitas Negeri Gorontalo)*

This repository contains the complete experimental testbed, WebAssembly filter source code, Go control-plane microservices, raw benchmark datasets (600+ test runs), data processing pipelines, inferential statistical test engines, and automated figure generators.

---

## Repository Structure

```
├── README.md                      # Replication guide and artifact evaluation runbook
├── LICENSE                        # GNU General Public License v3.0 (GPLv3)
├── Makefile                       # One-click execution targets
├── requirements.txt               # Python analytical dependencies
├── docker-compose.yml             # Local multi-service simulation testbed
│
├── data/                          # Canonical experimental datasets
│   ├── processed/                 # Consolidated CSV datasets (latency, resource, revocation, ablation)
│   └── reports/                   # Statistical reports (Mann-Whitney U, Cliff's Delta, TOST, Kruskal-Wallis)
│
├── load-testing/                  # Load generator and canonical raw files
│   ├── scenarios/                 # k6 load testing scripts (S01-S10 and multi-hop ablation)
│   └── results/                   # Raw benchmark exports:
│       ├── run_full_30rep_20261028_022212/  # 600 raw canonical data files (300 k6 JSON + 300 Prom CSV)
│       └── ablation/                        # 11 raw ablation data files (LFU vs Graph-Aware)
│
├── figures/                       # Official canonical paper figures
│   ├── en/                        # 10 English figures (Figures 1-10)
│   └── *.png                      # 10 Indonesian figures (Figures 1-10)
│
├── scripts/                       # Reproduction, verification, and analytical scripts
│   ├── verify_traceability.py     # 1-click verification of all 119 paper claims
│   ├── validate-data.py           # 10-step raw data completeness and integrity audit
│   ├── analyze-all-data.py        # Raw data extraction and CSV consolidation
│   ├── statistical-analysis.py    # Non-parametric hypothesis testing engine
│   ├── analyze_ablation_results.py# Inferential statistics for multi-hop ablation study
│   ├── generate-figures.py        # Regenerates Indonesian figures
│   ├── generate-figures-en.py     # Regenerates English figures
│   ├── generate_ablation_figure.py# Regenerates Figure 10 (LFU vs Graph-Aware boxplot)
│   ├── scalability-analysis.py    # Algorithmic complexity evaluation
│   ├── test_analyze_ablation.py   # Unit test for ablation statistical calculations
│   └── test_local_end_to_end.py   # Local end-to-end integration smoke test
│
├── control-plane/                 # Go microservices: Graph Analyzer & Cache Distributor
├── wasm-filter/                   # Envoy WebAssembly authorization filter (Go/TinyGo)
├── services/                      # Modified Online Boutique microservices
├── k8s/                           # Kubernetes manifests (deployments, Istio, SPIFFE/SPIRE)
├── observability/                 # Monitoring configurations (Prometheus, Grafana, Jaeger)
└── docs/                          # Comprehensive technical architecture & methodology documentation
```

---

## 1. Quick Start: 1-Click Traceability Verification (< 10 seconds)

To verify that **every single empirical number, statistical test statistic, and mathematical formula tag in the paper** matches the underlying experimental data:

```bash
pip install -r requirements.txt
python scripts/verify_traceability.py
```

**Expected output:**
```
======================================================================
HASIL: 119/119 PASS, 0/119 FAIL
✅ SELURUH ANGKA DI NASKAH DAPAT DIREPRODUKSI DARI DATA CSV
======================================================================
```

---

## 2. Reproducing the Full Analytical Pipeline (< 2 minutes)

Execute the complete data processing, statistical validation, and figure regeneration pipeline from the raw canonical telemetry files:

```bash
# Step 1: Audit raw data integrity across all 600 benchmark files
python scripts/validate-data.py

# Step 2: Consolidate 300 k6 JSON + 300 Prometheus CSV into unified datasets
python scripts/analyze-all-data.py

# Step 3: Run non-parametric statistical tests (Mann-Whitney U, Cliff's Delta, TOST, Kruskal-Wallis)
python scripts/statistical-analysis.py

# Step 4: Regenerate all canonical figures (Figures 1 through 10 + supplementary figure)
python scripts/generate-figures.py
python scripts/generate-figures-en.py

# Step 5: Execute inferential statistics and visualization for multi-hop ablation study (Figure 10)
python scripts/analyze_ablation_results.py
python scripts/generate_ablation_figure.py
python scripts/test_analyze_ablation.py

# Step 6: Evaluate algorithmic complexity models (O(1) data plane, O(V^2) control plane)
python scripts/scalability-analysis.py
```

All regenerated CSV reports and figure PNG files will be updated in `data/reports/` and `figures/`.

---

## 3. Verifying the Go Codebase and Unit Tests

All control-plane and data-plane WebAssembly Go modules include comprehensive unit and integration test suites:

```bash
# Test services
go test -v ./services/service-a/...
go test -v ./services/service-b/...
go test -v ./services/service-c/...

# Test control plane
go test -v ./control-plane/graph-analyzer/...
go test -v ./control-plane/cache-distributor/...

# Test WebAssembly filter
go test -v ./wasm-filter/...
```

---

## 4. Hardware and Software Environment

- **Cluster Testbed:** Kubernetes (k3s v1.28.5+k3s1, containerd 1.7.11), 2 vCPU AMD EPYC 7713, 4 GB RAM, Debian GNU/Linux 12 (bookworm, kernel 6.1.0-47-amd64).
- **Service Mesh:** Istio 1.20.1 (Envoy Proxy 1.28.0).
- **Policy Engine:** Open Policy Agent (OPA) 0.60.0.
- **Load Generator:** k6 v1.6.1 (constant-arrival-rate executor).
- **Observability:** Prometheus 2.48.1 (5-second scrape interval).
- **Analytical Stack:** Python 3.10+, pandas 2.2, scipy 1.14, numpy 2.2, matplotlib 3.10.

---

## 5. License & Commercial Adoption

This project is licensed under the **GNU General Public License v3.0 (GPLv3)**.  
It is completely free to read, replicate, evaluate, modify, and use for academic, research, and open-source purposes.

### Commercial Licensing & Proprietary Adoption
If your organization wishes to adopt, integrate, or commercialize this predictive authorization caching algorithm into proprietary, closed-source, or commercial enterprise products without being subject to the viral copyleft terms of GPLv3, a **separate commercial license** is required.

Please contact the author directly for commercial licensing agreements, custom enterprise integration, or consultancy:
- **Author:** Moh. Radithyo Surya Martuah
- **Email:** thyo.surya@gmail.com
"""
(DEST / 'README.md').write_text(readme_content, encoding='utf-8')
print("  [+] Installed clean README.md (Artifact Evaluation standard)")

# 11. Initialize fresh Git repository
print("\n[*] Initializing fresh Git repository on main branch...")
subprocess.run(["git", "init", "-b", "main"], cwd=DEST, check=True, stdout=subprocess.DEVNULL)
subprocess.run(["git", "remote", "add", "origin", f"https://github.com/thyosurya/{REPO_NAME}.git"], cwd=DEST, check=True, stdout=subprocess.DEVNULL)
subprocess.run(["git", "add", "."], cwd=DEST, check=True, stdout=subprocess.DEVNULL)
subprocess.run(["git", "commit", "-m", "feat: initial release of clean, standalone replication package under GPLv3"], cwd=DEST, check=True, stdout=subprocess.DEVNULL)

print("\n" + "=" * 70)
print(f"SUCCESS: Clean replication repository generated at:")
print(f"         {DEST}")
print(f"Remote : https://github.com/thyosurya/{REPO_NAME}.git")
print("=" * 70)
