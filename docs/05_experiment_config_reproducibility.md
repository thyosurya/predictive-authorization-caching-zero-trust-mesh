# Konfigurasi Eksperimen & Reproducibility
## ZTA Predictive Authorization Caching - Parameter Lengkap untuk Replikasi

Dokumen ini mendokumentasikan seluruh parameter eksperimen secara presisi sehingga peneliti lain dapat mereplikasi hasil dengan kondisi yang identik.

---

## Daftar Isi
1. [Versi Software (Pinned)](#1-versi-software-pinned)
2. [Spesifikasi Hardware Testbed](#2-spesifikasi-hardware-testbed)
3. [Parameter Algoritma](#3-parameter-algoritma)
4. [Parameter Eksperimen](#4-parameter-eksperimen)
5. [Seed Data & Kebijakan Awal](#5-seed-data--kebijakan-awal)
6. [Konfigurasi Lengkap Kubernetes & Istio](#6-konfigurasi-lengkap-kubernetes--istio)
7. [Konfigurasi Load Generator](#7-konfigurasi-load-generator)
8. [Checklist Reproducibility](#8-checklist-reproducibility)

---

## 1. Versi Software (Pinned)

Semua versi di-pin secara eksplisit. Jangan gunakan `latest` - bisa menghasilkan hasil berbeda.

### Platform

| Komponen | Versi | Image / Source |
|---|---|---|
| OS Host | Debian GNU/Linux 12 (bookworm) (kompatibel Ubuntu 22.04 LTS) | `debian:12` / `ubuntu:22.04` |
| Kernel | 6.1.0-47-amd64 | - |
| Docker | 24.0.7 | `docker.io/library/docker:24.0.7` |
| k3s | v1.28.5+k3s1 | `rancher/k3s:v1.28.5-k3s1` |
| Istio | 1.20.1 | `docker.io/istio/pilot:1.20.1` |
| Envoy (via Istio) | 1.28.0 | `docker.io/istio/proxyv2:1.20.1` |

### Aplikasi & Tools

| Komponen | Versi | Image |
|---|---|---|
| Open Policy Agent | 0.60.0 | `openpolicyagent/opa:0.60.0` |
| SPIRE Server | 1.8.5 | `ghcr.io/spiffe/spire-server:1.8.5` |
| SPIRE Agent | 1.8.5 | `ghcr.io/spiffe/spire-agent:1.8.5` |
| Online Boutique | 0.9.0 | `gcr.io/google-samples/microservices-demo` |
| Prometheus | 2.48.1 | `prom/prometheus:v2.48.1` |
| Grafana | 10.2.3 | `grafana/grafana:10.2.3` |
| Jaeger | 1.52.0 | `jaegertracing/all-in-one:1.52.0` |
| k6 | v1.6.1 (kompatibel >= 0.48.0) | `grafana/k6:1.6.1` |

### Bahasa & Library

| Bahasa / Library | Versi |
|---|---|
| Go | 1.22.0 |
| TinyGo | 0.31.0 |
| proxy-wasm-go-sdk | 0.23.0 |
| gonum/graph | 0.14.0 |
| google.golang.org/grpc | 1.61.0 |
| prometheus/client_golang | 1.18.0 |
| Python | 3.11+ (diverifikasi s.d. 3.14) |
| scipy | >= 1.11.4 |
| pandas | >= 2.1.4 |
| numpy | >= 1.26.3 |

### Go module checksums (go.sum excerpt)

```
gonum.org/v1/gonum v0.14.0 h1:2NiG67LD1tEH0D7kM+ps2V+fXmsAnpUeec7n8tcr4S0=
google.golang.org/grpc v1.61.0 h1:TOvOcuXn30kRao+gfcvsebNEa5iZIiLkisYEkf7R7guY=
github.com/tetratelabs/proxy-wasm-go-sdk v0.23.0 h1:...
```

File `go.sum` lengkap tersedia di `control-plane/graph-analyzer/go.sum`.

---

## 2. Spesifikasi Hardware Testbed

### 2.1 Testbed Empiris Utama: Single-Node VM Lean (AMD EPYC 7713, 2 vCPU, 4 GB RAM)

Seluruh data eksperimen empiris (Skenario S01 hingga S10, 30 repetisi per skenario) yang dipublikasikan dalam naskah dieksekusi pada testbed virtual machine tunggal dengan konfigurasi lean:

| Parameter | Spesifikasi Empiris | Catatan Operasional |
|---|---|---|
| Node | 1 Virtual Machine | Single-node Kubernetes (k3s) testbed |
| CPU | 2 vCPU (AMD EPYC 7713 64-Core Processor) | Dialokasikan untuk OS, k3s, Envoy sidecars, dan microservices |
| RAM | 4 GB DDR4 (3915 MB terdeteksi) | Ditopang 2 GB swapfile untuk stabilitas alokasi memori |
| Swap | 2 GB Linux Swap (`/swapfile`) | Mencegah kernel OOM killer pada beban puncak |
| Disk | 50 GB SSD | Penyimpanan container images, temporary build artifacts, dan logs |
| OS Host | Debian GNU/Linux 12 (bookworm) | Kernel 6.1.0-47-amd64 |
| Kubernetes | k3s v1.28.5+k3s1 (containerd 1.7.11) | Flag lean: `--disable traefik --disable servicelb --disable metrics-server --disable local-storage` |
| Istio | 1.20.1 (Envoy proxy 1.28.0) | Profile `minimal` (`k8s/istio/mesh-config.yaml`) |
| Load Generator | k6 v1.6.1 | Dijalankan dari luar VM agar kapasitas CPU VM teralokasi murni untuk target beban |

Otomasi konfigurasi lingkungan ini disediakan pada skrip: `bash scripts/setup_vm_2core_4gb.sh`.

### 2.2 Arsitektur Klaster Multi-Node (Scale-Out Target)

Untuk perbandingan atau evaluasi skala produksi enterprise pada klaster multi-node, konfigurasi referensi berikut dapat digunakan:

| Node | Role | CPU | RAM | Disk | OS |
|---|---|---|---|---|---|
| node-01 | control-plane + worker | 4 vCPU (Intel Xeon E5-2680v4) | 8 GB DDR4 | 50 GB SSD | Ubuntu 22.04 LTS |
| node-02 | worker | 4 vCPU (Intel Xeon E5-2680v4) | 8 GB DDR4 | 50 GB SSD | Ubuntu 22.04 LTS |
| node-03 | worker + monitoring | 4 vCPU (Intel Xeon E5-2680v4) | 8 GB DDR4 | 50 GB SSD | Ubuntu 22.04 LTS |

**Catatan:** Jika menggunakan VM cloud, instance yang setara:
- GCP: `n2-standard-4` (4 vCPU, 16 GB RAM)
- AWS: `t3.xlarge` (4 vCPU, 16 GB RAM)
- DigitalOcean: `s-4vcpu-8gb`

### 2.3 Jaringan

| Parameter | Nilai |
|---|---|
| Bandwidth antar node | 1 Gbps (virtual NIC) |
| Latency antar node | < 1 ms (lokal / datacenter yang sama) |
| MTU | 1500 bytes |
| CNI | Calico v3.27.0 / Flannel (default k3s) |

**Penting:** Pastikan tidak ada traffic signifikan lain di jaringan saat pengukuran.

---

## 3. Parameter Algoritma

### Graph Topology Analyzer

```yaml
# Nilai eksak yang digunakan dalam eksperimen
graph_analyzer:
  sampling_window_seconds: 60        # Window historis untuk menghitung frekuensi
  min_calls_per_min: 10              # Threshold minimum untuk kandidat cache
  max_edges: 500                     # Maksimum edge dalam graph (mencegah memory bloat)
  centrality_algorithm: betweenness  # Algoritma centrality untuk ranking
  stale_edge_timeout_seconds: 120    # Edge dihapus jika tidak ada panggilan > 2 menit
  analysis_interval_seconds: 30      # Frekuensi analisis graph
```

### TTL Calculator

```yaml
ttl_calculator:
  base_ttl_ms: 30000          # 30 detik
  max_ttl_ms: 30000           # 30 detik (batas atas absolut empiris)
  min_ttl_ms: 5000            # 5 detik (batas bawah absolut)
  freq_weight_divisor: 100    # Pembagi untuk normalisasi frekuensi
  sensitivity_penalties:
    low: 0.0
    medium: 0.3
    high: 0.7
  # Formula:
  # TTL_candidate = base_ttl * min(1.0, calls/divisor) * (1 - penalty) * cert_factor
  # TTL = max(min_ttl, min(min(max_ttl, t_remaining), TTL_candidate))
```

### Cache Distributor

```yaml
cache_distributor:
  distribution_interval_seconds: 30   # Seberapa sering token baru didistribusikan
  grpc_port: 50051
  rest_port: 8081
  max_token_per_pair: 1               # Hanya satu token aktif per pasangan
  nonce_blacklist_duration_ms: 60000  # 2x MaxTTL (2 x 30 detik)
  grpc_reconnect_initial_ms: 100
  grpc_reconnect_max_ms: 5000
  grpc_reconnect_multiplier: 2.0
```

### WASM Filter (Local Cache)

```yaml
wasm_filter:
  max_cache_entries: 1000             # Maksimum entri di in-memory cache (thread-isolated)
  signature_algorithm: HMAC-SHA256 / Ed25519
  eviction_strategy: proactive_stale_first
  ingress_sanitization: remove_x_zta_cache_verified
  validation_order:
    - check_signature                 # 1. Verifikasi Ed25519 / HMAC
    - check_expiry                    # 2. Cek exp field
    - check_nonce                     # 3. Validasi nonce
    - check_action                    # 4. Cek allowed_actions
  fallback_on_cache_miss: ext_authz_pdp # Forward ke OPA tanpa header x-zta-cache-verified
```

---

## 4. Parameter Eksperimen

### Skenario lengkap

| Kode | Konfigurasi | RPS | Topologi | Durasi | Pengulangan | Cooldown |
|---|---|---|---|---|---|---|
| S01 | PBS Baseline | 500 | Linear | 5 menit | 30 | 120 detik |
| S02 | ZTA-Naif | 500 | Linear | 5 menit | 30 | 120 detik |
| S03 | ZTA-Cache | 500 | Linear | 5 menit | 30 | 120 detik |
| S04 | ZTA-Naif | 2000 | Linear | 5 menit | 30 | 120 detik |
| S05 | ZTA-Cache | 2000 | Linear | 5 menit | 30 | 120 detik |
| S06 | ZTA-Cache | 5000 | Linear | 5 menit | 30 | 180 detik |
| S07 | ZTA-Cache | 2000 | Fan-out | 5 menit | 30 | 120 detik |
| S08 | ZTA-Cache | 2000 | Mesh penuh | 5 menit | 30 | 120 detik |
| S09 | ZTA-Cache | 500 | Linear | 5 menit | 30 | 120 detik |
| S10 | ZTA-Cache (TTL static) | 2000 | Fan-out | 5 menit | 30 | 120 detik |

**Warmup:** 5 menit @ 50 RPS sebelum skenario pertama, 2 menit @ 20 RPS antar kelompok skenario.

### Parameter statistik

```
Jumlah pengulangan per skenario : 30 (studi ablasi multi-hop: n = 5)
Taraf signifikansi (α)           : 0.05
Uji normalitas                   : Shapiro-Wilk
Uji hipotesis utama              : Mann-Whitney U + TOST (margin Δ = 5%) + Wilcoxon signed-rank
Effect size                      : Cliff's Delta + Hodges-Lehmann + Percentile Bootstrap CI (B = 10.000, seed = 42)
Uji multi-grup                   : Kruskal-Wallis + Mann-Whitney U post-hoc (Bonferroni α' = 0.0167)
```

### Topologi service mesh

**Linear (S01–S06, S09):**
```
Load Generator → frontend → productcatalogservice → recommendationservice
```
3 hop, panggilan sequential.

**Fan-out (S07, S10):**
```
                    ┌── productcatalogservice
Load Generator → frontend ─┼── cartservice
                    └── currencyservice
```
1 service → 3 downstream (simulasi high-fan-out).

**Mesh penuh (S08):**
```
frontend ←→ productcatalogservice ←→ cartservice ←→ checkoutservice ←→ paymentservice
```
5 service, masing-masing memanggil semua tetangganya (N×(N-1)/2 = 10 pasangan).

---

## 5. Seed Data & Kebijakan Awal

### OPA policy seed (data.json)

```json
{
  "service_rules": [
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "allowed_actions": ["GET"],
      "sensitivity": "low"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/cartservice",
      "allowed_actions": ["GET", "POST", "DELETE"],
      "sensitivity": "medium"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/currencyservice",
      "allowed_actions": ["POST"],
      "sensitivity": "low"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
      "target": "spiffe://cluster.local/ns/zta-research/sa/cartservice",
      "allowed_actions": ["GET", "DELETE"],
      "sensitivity": "medium"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
      "target": "spiffe://cluster.local/ns/zta-research/sa/paymentservice",
      "allowed_actions": ["POST"],
      "sensitivity": "high"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
      "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "allowed_actions": ["GET"],
      "sensitivity": "low"
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/recommendationservice",
      "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "allowed_actions": ["GET"],
      "sensitivity": "low"
    }
  ]
}
```

### SPIRE entry seeds

```bash
# Script: scripts/register-spire-entries.sh
ENTRIES=(
  "frontend"
  "productcatalogservice"
  "cartservice"
  "currencyservice"
  "checkoutservice"
  "paymentservice"
  "recommendationservice"
)

for svc in "${ENTRIES[@]}"; do
  kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID "spiffe://cluster.local/ns/zta-research/sa/${svc}" \
    -parentID "spiffe://cluster.local/ns/spire/sa/spire-agent" \
    -selector "k8s:ns:zta-research" \
    -selector "k8s:sa:${svc}" \
    -ttl 3600
done
```

### Seed Candidates Cache Delegation (`seed-candidates.json`)

Untuk inisialisasi awal delegasi cache (`POST /distribute`), digunakan konfigurasi kandidat awal yang telah diselaraskan dengan formula TTL dinamis ($v_c = 1{,}0$):

```json
[
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "allowed_actions": ["GET"],
    "suggested_ttl_ms": 30000,
    "sensitivity": "low"
  },
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/cartservice",
    "allowed_actions": ["GET", "POST", "DELETE"],
    "suggested_ttl_ms": 21000,
    "sensitivity": "medium"
  },
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/currencyservice",
    "allowed_actions": ["POST"],
    "suggested_ttl_ms": 30000,
    "sensitivity": "low"
  },
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
    "target": "spiffe://cluster.local/ns/zta-research/sa/paymentservice",
    "allowed_actions": ["POST"],
    "suggested_ttl_ms": 9000,
    "sensitivity": "high"
  }
]
```

---

## 6. Konfigurasi Lengkap Kubernetes & Istio

### ResourceQuota (membatasi resource agar konsisten)

```yaml
# k8s/cluster/resource-quota.yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: zta-research-quota
  namespace: zta-research
spec:
  hard:
    requests.cpu: "16"
    requests.memory: "16Gi"
    limits.cpu: "32"
    limits.memory: "32Gi"
    pods: "50"
```

### Resource request/limit semua deployment (nilai eksak)

```yaml
# Berlaku untuk SEMUA service dalam eksperimen
resources:
  requests:
    cpu: "100m"
    memory: "128Mi"
  limits:
    cpu: "500m"
    memory: "256Mi"

# Envoy sidecar (di-inject Istio)
# resources:
#   requests:
#     cpu: "10m"
#     memory: "40Mi"
#   limits:
#     cpu: "200m"
#     memory: "128Mi"
```

### Istio mesh config (eksak)

```yaml
# k8s/istio/mesh-config.yaml
meshConfig:
  accessLogFile: "/dev/stdout"
  accessLogEncoding: JSON
  enableTracing: true
  defaultConfig:
    tracing:
      zipkin:
        address: "jaeger-collector.monitoring:9411"
      sampling: 100  # 100% sampling selama eksperimen
    concurrency: 2   # Worker threads Envoy (di-pin untuk konsistensi)
  outboundTrafficPolicy:
    mode: REGISTRY_ONLY  # Blokir traffic ke service yang tidak terdaftar
  extensionProviders:
    - name: opa-ext-authz
      envoyExtAuthzHttp:
        service: opa-service.zta-research.svc.cluster.local
        port: 8181
        pathPrefix: /v1/data/authz_allow
        includeRequestHeadersInCheck:
          - "x-forwarded-client-cert"
          - "x-source-identity"
          - "x-zta-cache-verified"
          - ":authority"
          - ":method"
          - ":path"
          - "host"
        failureModeAllow: false
```

### Prometheus scrape interval

```yaml
# kube-prometheus-stack values
prometheus:
  prometheusSpec:
    scrapeInterval: "5s"        # Interval scraping
    evaluationInterval: "5s"
    retention: "15d"
    retentionSize: "10GB"
```

---

## 7. Konfigurasi Load Generator

### Parameter k6 yang di-pin untuk semua skenario

```javascript
// Bagian yang SAMA di semua skrip skenario
export const commonOptions = {
    discardResponseBodies: false,
    userAgent: "zta-research-k6/0.48.0",
    timeout: "10s",            // Request timeout
    throw: false,              // Jangan throw, catat sebagai error
    insecureSkipTLSVerify: false,
    tlsAuth: [],               // mTLS via Istio - tidak dikonfigurasi di k6
    summaryTrendStats: ["min", "avg", "med", "p(90)", "p(95)", "p(99)", "p(99.9)", "max"],
};

// Payload request standar (1 KB)
const PAYLOAD = "x".repeat(1024);  // 1 KB constant payload
```

### Warmup script

```javascript
// load-testing/warmup.js
import http from "k6/http";
import { sleep } from "k6";

export const options = {
    vus: 10,
    duration: "5m",
    thresholds: {},  // Tidak ada threshold saat warmup
};

export default function () {
    http.get("http://frontend.zta-research.svc.cluster.local/api/products");
    sleep(0.5);
}
```

---

## 8. Checklist Reproducibility

Gunakan checklist ini sebelum menjalankan eksperimen untuk memastikan kondisi identik dengan paper.

### Verifikasi versi

```bash
#!/bin/bash
# scripts/verify-versions.sh

echo "=== Verifikasi Versi Komponen ==="

# k3s
K3S_VER=$(kubectl version --short 2>/dev/null | grep Server | awk '{print $3}')
[[ "$K3S_VER" == "v1.28.5+k3s1" ]] && echo "✅ k3s: $K3S_VER" || echo "⚠️  k3s: $K3S_VER (expected v1.28.5+k3s1)"

# Istio
ISTIO_VER=$(istioctl version 2>/dev/null | grep "control plane version" | awk '{print $NF}')
[[ "$ISTIO_VER" == "1.20.1" ]] && echo "✅ Istio: $ISTIO_VER" || echo "⚠️  Istio: $ISTIO_VER (expected 1.20.1)"

# OPA
OPA_VER=$(kubectl exec -n zta-research deploy/opa -- opa version 2>/dev/null | head -1 | awk '{print $2}')
[[ "$OPA_VER" == "0.60.0" ]] && echo "✅ OPA: $OPA_VER" || echo "⚠️  OPA: $OPA_VER (expected 0.60.0)"

# k6
K6_VER=$(k6 version 2>/dev/null | awk '{print $2}')
[[ "$K6_VER" == "v0.48.0" ]] && echo "✅ k6: $K6_VER" || echo "⚠️  k6: $K6_VER (expected v0.48.0)"

# Python
PY_VER=$(python3 --version | awk '{print $2}')
echo "ℹ️  Python: $PY_VER"
```

### Verifikasi parameter algoritma

```bash
# Verifikasi config Graph Analyzer
kubectl exec -n zta-research deploy/graph-analyzer -- \
  cat /etc/config/graph-analyzer.yaml

# Verifikasi config Cache Distributor
kubectl exec -n zta-research deploy/cache-distributor -- \
  cat /etc/config/cache-distributor.yaml
```

### Verifikasi hardware identik

```bash
# CPU model
lscpu | grep "Model name"

# RAM
free -h | grep Mem

# Network
ip link show | grep -E "mtu|ether"
```

### Kondisi yang harus dipenuhi sebelum tiap pengulangan

- [ ] Graph Analyzer menunjukkan 0 edges (reset bersih)
- [ ] Cache Distributor menunjukkan 0 token aktif
- [ ] Tidak ada proses k6 lain berjalan
- [ ] CPU usage semua node < 10% (sistem idle)
- [ ] Memory usage sidecar dalam rentang baseline ± 5 MB
- [ ] Log Istio tidak menunjukkan error dalam 60 detik terakhir
- [ ] Prometheus menunjukkan semua target UP

### Snapshot konfigurasi (jalankan sekali sebelum eksperimen)

```bash
#!/bin/bash
# scripts/snapshot-config.sh
# Simpan seluruh konfigurasi untuk reproducibility

SNAPSHOT_DIR="data/config-snapshot-$(date +%Y%m%d-%H%M)"
mkdir -p "$SNAPSHOT_DIR"

# Semua resource Kubernetes
kubectl get all -n zta-research -o yaml > "$SNAPSHOT_DIR/k8s-resources.yaml"
kubectl get configmap -n zta-research -o yaml > "$SNAPSHOT_DIR/configmaps.yaml"
kubectl get authorizationpolicy -n zta-research -o yaml > "$SNAPSHOT_DIR/authz-policies.yaml"
kubectl get wasmplugin -n zta-research -o yaml > "$SNAPSHOT_DIR/wasm-plugins.yaml"
kubectl get peerauthentication -n zta-research -o yaml > "$SNAPSHOT_DIR/peer-auth.yaml"

# Versi komponen
bash scripts/verify-versions.sh > "$SNAPSHOT_DIR/versions.txt" 2>&1

# Go module checksums
cp control-plane/graph-analyzer/go.sum "$SNAPSHOT_DIR/graph-analyzer-go.sum"
cp control-plane/cache-distributor/go.sum "$SNAPSHOT_DIR/cache-distributor-go.sum"

# OPA policies
cp control-plane/pdp/policies/authz.rego "$SNAPSHOT_DIR/authz.rego"
cp control-plane/pdp/policies/data.json "$SNAPSHOT_DIR/data.json"

echo "Snapshot tersimpan di: $SNAPSHOT_DIR"
```

---

## Ringkasan Parameter Kunci

Tabel ini merangkum semua parameter kritis yang harus identik untuk mereplikasi hasil:

| Parameter | Nilai |
|---|---|
| Kubernetes | v1.28.5+k3s1 |
| Istio | 1.20.1 |
| OPA | 0.60.0 |
| k6 | v1.6.1 (kompatibel >= 0.48.0) |
| Go | 1.22.0 |
| Node spec (Testbed Empiris Utama) | Single-Node VM Lean: 2 vCPU AMD EPYC 7713 / 4 GB RAM (Referensi Multi-Node: 4 vCPU / 8 GB RAM) |
| Jaringan antar node | < 1 ms latency, 1 Gbps |
| Tracing sampling | 100% |
| Prometheus scrape interval | 5 detik |
| Durasi skenario | 5 menit steady-state |
| Pengulangan per skenario | 30 |
| Cooldown antar skenario | 120–180 detik |
| Warmup sebelum eksperimen | 5 menit @ 50 RPS |
| Payload request | 1 KB (konstan) |
| BaseTTL | 30.000 ms |
| MaxTTL | 30.000 ms |
| MinTTL | 5.000 ms |
| Min calls/min untuk kandidat cache | 10 |
| Taraf signifikansi (α) | 0.05 |
| Uji hipotesis | Mann-Whitney U + Cliff's Delta |
