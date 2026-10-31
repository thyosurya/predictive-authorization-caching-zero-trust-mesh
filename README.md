# Replication Package: Predictive Authorization Caching with Microservice Graph Prioritization in Zero-Trust Service Mesh

[![Artifact Evaluation](https://img.shields.io/badge/Artifact-Available%20%26%20Reproducible-success.svg)](#)
[![Python](https://img.shields.io/badge/Python-3.10%2B-blue.svg)](https://www.python.org/)
[![Go](https://img.shields.io/badge/Go-1.22%2B-blue.svg)](https://golang.org/)
[![Traceability](https://img.shields.io/badge/Traceability%20Audit-117%2F117%20PASS-brightgreen.svg)](#1-quick-start-1-click-traceability-verification-10-seconds)
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
│   ├── verify_traceability.py     # 1-click verification of all 117 paper claims
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
HASIL: 117/117 PASS, 0/117 FAIL
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

# Step 4: Regenerate all canonical figures (Figures 1 through 9)
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

- **Cluster Testbed:** Kubernetes (k3s v1.28.2), 2 vCPU AMD EPYC 7713, 4 GB RAM, Ubuntu 22.04 LTS.
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
