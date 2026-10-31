# Developer Guide
## Predictive Authorization Caching for Zero Trust Service Mesh

Panduan ini membantu developer membangun dan menjalankan sistem **Predictive Authorization Caching** berbasis Zero Trust Architecture (ZTA) di atas Kubernetes + Istio dari nol.

---

## Prasyarat

Sebelum memulai, pastikan kamu memahami konsep dasar berikut:

- **Container & Docker** - cara build dan run container
- **REST API** - familiar dengan HTTP request/response
- **Terminal / CLI** - nyaman menggunakan command line Linux/macOS

Tidak perlu pengalaman Kubernetes atau Istio sebelumnya - panduan ini akan menjelaskan setiap langkah.

---

## Daftar Isi

1. [Gambaran Sistem](#1-gambaran-sistem)
2. [Setup Environment](#2-setup-environment)
3. [Deploy Kubernetes Cluster](#3-deploy-kubernetes-cluster)
4. [Install Istio Service Mesh](#4-install-istio-service-mesh)
5. [Deploy Aplikasi Testbed](#5-deploy-aplikasi-testbed)
6. [Setup Identity (SPIFFE/SPIRE)](#6-setup-identity-spiffespire)
7. [Deploy OPA sebagai PDP Central](#7-deploy-opa-sebagai-pdp-central)
8. [Implementasi Graph Topology Analyzer](#8-implementasi-graph-topology-analyzer)
9. [Implementasi Cache Policy Distributor](#9-implementasi-cache-policy-distributor)
10. [Envoy WASM Filter (Local Cache)](#10-envoy-wasm-filter-local-cache)
11. [Setup Load Testing dengan k6](#11-setup-load-testing-dengan-k6)
12. [Setup Observabilitas](#12-setup-observabilitas)
13. [Menjalankan Eksperimen](#13-menjalankan-eksperimen)
14. [Troubleshooting](#14-troubleshooting)

---

## 1. Gambaran Sistem

Sistem terdiri dari tiga layer utama yang bekerja bersama:

```
┌─────────────────────────────────────────┐
│ CONTROL PLANE                           │
│  PDP (OPA) → Graph Analyzer → Cache     │
│              Distributor                │
└──────────────────┬──────────────────────┘
                   │ delegation token
┌──────────────────▼──────────────────────┐
│ DATA PLANE (Envoy Sidecar)              │
│  Sidecar A │ Sidecar B │ Sidecar C      │
│  (local cache per sidecar)              │
└──────────────────┬──────────────────────┘
                   │
┌──────────────────▼──────────────────────┐
│ SERVICE LAYER                           │
│  Service A │ Service B │ Service C      │
└─────────────────────────────────────────┘
```

**Alur kerja singkat:**

1. PDP Central (OPA) menyimpan semua kebijakan otorisasi
2. Graph Analyzer memantau pola panggilan antar-service
3. Cache Distributor mengirim delegation token ke sidecar untuk pasangan service yang sering berkomunikasi
4. Sidecar menggunakan token lokal untuk membuat keputusan otorisasi tanpa query ke PDP - ini yang mengurangi latensi
5. Jika ada pencabutan akses, PDP langsung push ke semua sidecar via gRPC

---

## 2. Setup Environment

### 2.1 Install tools yang diperlukan

**Docker**
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker

# Verifikasi
docker --version
# Output: Docker version 24.x.x
```

**kubectl** (CLI untuk Kubernetes)
```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Verifikasi
kubectl version --client
```

**Helm** (package manager untuk Kubernetes)
```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Verifikasi
helm version
```

**Go** (untuk implementasi algoritma)
```bash
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verifikasi
go version
# Output: go version go1.22.0 linux/amd64
```

**istioctl** (CLI untuk Istio)
```bash
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.20.0 sh -
sudo mv istio-1.20.0/bin/istioctl /usr/local/bin/

# Verifikasi
istioctl version
```

### 2.2 Struktur direktori proyek

Buat struktur folder berikut sebelum mulai coding:

```
zta-predictive-cache/
├── k8s/                        # Kubernetes manifests
│   ├── cluster/
│   ├── istio/
│   ├── spire/
│   └── apps/
├── services/                   # Source code microservices
│   ├── service-a/
│   ├── service-b/
│   └── service-c/
├── control-plane/              # Algoritma utama
│   ├── graph-analyzer/
│   ├── cache-distributor/
│   └── pdp/
├── wasm-filter/                # Envoy WASM filter
├── load-testing/               # Skrip k6
├── observability/              # Konfigurasi monitoring
└── scripts/                    # Helper scripts
```

```bash
mkdir -p zta-predictive-cache/{k8s/{cluster,istio,spire,apps},services/{service-a,service-b,service-c},control-plane/{graph-analyzer,cache-distributor,pdp},wasm-filter,load-testing,observability,scripts}
cd zta-predictive-cache
```

---

## 3. Deploy Kubernetes Cluster

Gunakan **k3s** - versi Kubernetes ringan yang cocok untuk development dan testbed penelitian.

### 3.1 Install k3s

```bash
# Install k3s (single-node untuk development)
curl -sfL https://get.k3s.io | sh -

# Konfigurasi kubectl
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config

# Verifikasi cluster berjalan
kubectl get nodes
# Output:
# NAME       STATUS   ROLES                  AGE   VERSION
# hostname   Ready    control-plane,master   1m    v1.28.x
```

> [!TIP]
> **Testbed Hemat Resource (VM 2 vCPU / 4 GB RAM):**
> Jika menginstal di VM spesifikasi hemat daya, gunakan opsi instalasi k3s lean:
> `curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik --disable servicelb --disable metrics-server --disable local-storage" sh -`
> dan install Istio dengan `profile=minimal`: `istioctl install --set profile=minimal -y`.
> Seluruh langkah otomatis ini sudah dikemas di: `bash scripts/setup_vm_2core_4gb.sh`.

### 3.2 Tambah worker node (opsional, untuk multi-node testbed)

Jika tersedia beberapa VM:
```bash
# Di master node, ambil token
sudo cat /var/lib/rancher/k3s/server/node-token

# Di worker node, jalankan:
curl -sfL https://get.k3s.io | K3S_URL=https://<MASTER_IP>:6443 \
  K3S_TOKEN=<TOKEN> sh -

# Verifikasi dari master
kubectl get nodes
```

### 3.3 Buat namespace untuk penelitian

```bash
kubectl create namespace zta-research
kubectl create namespace monitoring
kubectl create namespace spire

# Label namespace agar Istio bisa inject sidecar otomatis
kubectl label namespace zta-research istio-injection=enabled
```

---

## 4. Install Istio Service Mesh

Istio adalah service mesh yang mengelola traffic antar-service, mTLS, dan sidecar injection.

### 4.1 Install Istio

```bash
# Install dengan profil demo (cocok untuk penelitian)
istioctl install --set profile=demo -y

# Verifikasi Istio berjalan
kubectl get pods -n istio-system
# Output (semua harus Running):
# istiod-xxx          Running
# istio-ingressgateway-xxx   Running
# istio-egressgateway-xxx    Running
```

### 4.2 Aktifkan mTLS strict mode

mTLS memastikan semua komunikasi antar-service dienkripsi dan diautentikasi. Ini adalah salah satu prinsip inti ZTA.

```yaml
# k8s/istio/peer-auth-strict.yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: zta-research
spec:
  mtls:
    mode: STRICT
```

```bash
kubectl apply -f k8s/istio/peer-auth-strict.yaml

# Verifikasi
kubectl get peerauthentication -n zta-research
```

### 4.3 Aktifkan distributed tracing

```yaml
# k8s/istio/telemetry.yaml
apiVersion: telemetry.istio.io/v1alpha1
kind: Telemetry
metadata:
  name: mesh-default
  namespace: istio-system
spec:
  tracing:
    - providers:
        - name: "jaeger"
      randomSamplingPercentage: 100
```

```bash
kubectl apply -f k8s/istio/telemetry.yaml
```

---

## 5. Deploy Aplikasi Testbed

Gunakan **Google Online Boutique** - aplikasi e-commerce dengan 11 microservice, cocok untuk mensimulasikan East-West traffic yang kompleks.

### 5.1 Clone dan deploy Online Boutique

```bash
git clone https://github.com/GoogleCloudPlatform/microservices-demo.git
cd microservices-demo

# Deploy ke namespace penelitian
kubectl apply -f release/kubernetes-manifests.yaml -n zta-research

# Tunggu semua pod siap (bisa 3-5 menit)
kubectl wait --for=condition=ready pod --all -n zta-research --timeout=300s

# Verifikasi
kubectl get pods -n zta-research
```

### 5.2 Verifikasi sidecar ter-inject

Setiap pod seharusnya memiliki 2 container (1 app + 1 Envoy sidecar):

```bash
kubectl get pods -n zta-research -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{","}{end}{"\n"}{end}'

# Output yang benar (ada istio-proxy di setiap pod):
# frontend-xxx    server,istio-proxy,
# cartservice-xxx cartservice,istio-proxy,
```

---

## 6. Setup Identity (SPIFFE/SPIRE)

SPIFFE/SPIRE memberikan identitas kriptografis kepada setiap service. Identitas ini digunakan untuk membuat dan memvalidasi delegation token.

### 6.1 Konsep dasar SPIFFE

Setiap service mendapat **SVID** (SPIFFE Verifiable Identity Document) berformat:
```
spiffe://cluster.local/ns/zta-research/sa/<service-account-name>
```

### 6.2 Deploy SPIRE

```bash
# Clone SPIRE K8s quickstart
git clone https://github.com/spiffe/spire-tutorials.git
cd spire-tutorials/k8s/quickstart

# Deploy SPIRE server dan agent
kubectl apply -f spire-namespace.yaml
kubectl apply -f server-account.yaml
kubectl apply -f spire-bundle-configmap.yaml
kubectl apply -f server-cluster-role.yaml
kubectl apply -f server-configmap.yaml
kubectl apply -f server-statefulset.yaml
kubectl apply -f server-service.yaml
kubectl apply -f agent-account.yaml
kubectl apply -f agent-cluster-role.yaml
kubectl apply -f agent-configmap.yaml
kubectl apply -f agent-daemonset.yaml

# Verifikasi
kubectl get pods -n spire
# spire-server-0   Running
# spire-agent-xxx  Running (satu per node)
```

### 6.3 Daftarkan service entries

Setiap service perlu didaftarkan ke SPIRE agar mendapat SVID:

```bash
# Daftarkan service-a
kubectl exec -n spire spire-server-0 -- \
  /opt/spire/bin/spire-server entry create \
  -spiffeID spiffe://cluster.local/ns/zta-research/sa/service-a \
  -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
  -selector k8s:ns:zta-research \
  -selector k8s:sa:service-a

# Ulangi untuk service-b dan service-c
```

---

## 7. Deploy OPA sebagai PDP Central

OPA (Open Policy Agent) adalah mesin kebijakan yang mengevaluasi apakah suatu service boleh memanggil service lain.

### 7.1 Deploy OPA

```bash
# Tambah Helm repo OPA
helm repo add open-policy-agent https://open-policy-agent.github.io/gatekeeper/charts
helm repo update

# Install OPA standalone (bukan Gatekeeper, karena kita butuh external PDP)
kubectl apply -f https://raw.githubusercontent.com/open-policy-agent/opa/main/docs/website/static/deployments/kubernetes/opa.yaml -n zta-research
```

### 7.2 Buat kebijakan otorisasi (Rego)

```rego
# control-plane/pdp/policies/authz.rego
package authz

import future.keywords.if
import future.keywords.in

# Default tolak semua
default allow := false

# Izinkan jika ada entri eksplisit
allow if {
    some rule in data.service_rules
    rule.source == input.source_identity
    rule.target == input.target_identity
    input.action in rule.allowed_actions
}
```

```json
# control-plane/pdp/policies/data.json
{
  "service_rules": [
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "allowed_actions": ["GET"]
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/cartservice",
      "allowed_actions": ["GET", "POST", "DELETE"]
    },
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
      "target": "spiffe://cluster.local/ns/zta-research/sa/paymentservice",
      "allowed_actions": ["POST"]
    }
  ]
}
```

```bash
# Load kebijakan ke OPA
kubectl create configmap opa-policies \
  --from-file=authz.rego=control-plane/pdp/policies/authz.rego \
  --from-file=data.json=control-plane/pdp/policies/data.json \
  -n zta-research

# Test kebijakan
curl -X POST http://<OPA_SERVICE_IP>:8181/v1/data/authz/allow \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "source_identity": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target_identity": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "action": "GET"
    }
  }'
# Output: {"result": true}
```

---

## 8. Implementasi Graph Topology Analyzer

Komponen ini memantau pola panggilan antar-service dan menentukan pasangan mana yang layak mendapat cache delegation.

### 8.1 Inisialisasi Go module

```bash
cd control-plane/graph-analyzer
go mod init github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer
go get gonum.org/v1/gonum/graph
go get gonum.org/v1/gonum/graph/network
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### 8.2 Implementasi graph builder

```go
// control-plane/graph-analyzer/graph/topology.go
package graph

import (
    "sync"
    "time"

    "gonum.org/v1/gonum/graph/simple"
)

// ServiceEdge merepresentasikan panggilan antar dua service
type ServiceEdge struct {
    Source    string
    Target    string
    CallCount int64
    LastSeen  time.Time
}

// TopologyGraph menyimpan struktur graph service mesh
type TopologyGraph struct {
    mu    sync.RWMutex
    edges map[string]*ServiceEdge         // key: "source->target"
    graph *simple.WeightedDirectedGraph
    nodes map[string]int64               // serviceID -> nodeID
}

func NewTopologyGraph() *TopologyGraph {
    return &TopologyGraph{
        edges: make(map[string]*ServiceEdge),
        graph: simple.NewWeightedDirectedGraph(0, 0),
        nodes: make(map[string]int64),
    }
}

// RecordCall mencatat satu panggilan antar service
func (g *TopologyGraph) RecordCall(source, target string) {
    g.mu.Lock()
    defer g.mu.Unlock()

    key := source + "->" + target
    if edge, exists := g.edges[key]; exists {
        edge.CallCount++
        edge.LastSeen = time.Now()
    } else {
        g.edges[key] = &ServiceEdge{
            Source:    source,
            Target:    target,
            CallCount: 1,
            LastSeen:  time.Now(),
        }
    }
}

// GetHighFrequencyPairs mengembalikan pasangan service dengan frekuensi
// panggilan di atas threshold - kandidat untuk cache delegation
func (g *TopologyGraph) GetHighFrequencyPairs(minCallsPerMin int64) []*ServiceEdge {
    g.mu.RLock()
    defer g.mu.RUnlock()

    cutoff := time.Now().Add(-1 * time.Minute)
    var result []*ServiceEdge

    for _, edge := range g.edges {
        if edge.LastSeen.After(cutoff) && edge.CallCount >= minCallsPerMin {
            result = append(result, edge)
        }
    }
    return result
}
```

### 8.3 Implementasi TTL calculator

```go
// control-plane/graph-analyzer/cache/ttl.go
package cache

import "time"

// SensitivityLevel menentukan seberapa ketat TTL dikurangi
type SensitivityLevel int

const (
    SensitivityLow    SensitivityLevel = iota // penalty 0%
    SensitivityMedium                         // penalty 30%
    SensitivityHigh                           // penalty 70%
)

const (
    BaseTTL = 30 * time.Second
    MaxTTL  = 60 * time.Second
    MinTTL  = 5 * time.Second
)

// CalculateTTL menghitung TTL optimal untuk delegation token
// berdasarkan frekuensi panggilan, sensitivitas data, dan validitas sertifikat
func CalculateTTL(
    callsPerMin    int64,
    sensitivity    SensitivityLevel,
    certRemaining  time.Duration,
    certTotal      time.Duration,
) time.Duration {

    // freq_weight: makin tinggi frekuensi, makin panjang TTL (max 1.0)
    freqWeight := float64(callsPerMin) / 100.0
    if freqWeight > 1.0 {
        freqWeight = 1.0
    }

    // sensitivity_penalty: data sensitif dapat TTL lebih pendek
    var sensitivityPenalty float64
    switch sensitivity {
    case SensitivityLow:
        sensitivityPenalty = 0.0
    case SensitivityMedium:
        sensitivityPenalty = 0.3
    case SensitivityHigh:
        sensitivityPenalty = 0.7
    }

    // cert_validity_factor: TTL tidak boleh melebihi sisa validitas sertifikat
    certFactor := float64(certRemaining) / float64(certTotal)
    if certFactor > 1.0 {
        certFactor = 1.0
    }

    ttl := float64(BaseTTL) * freqWeight * (1 - sensitivityPenalty) * certFactor

    result := time.Duration(ttl)
    if result > MaxTTL {
        result = MaxTTL
    }
    if result < MinTTL {
        result = MinTTL
    }
    return result
}
```

### 8.4 Main service Graph Analyzer

```go
// control-plane/graph-analyzer/main.go
package main

import (
    "encoding/json"
    "log"
    "net/http"
    "time"

    "github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/cache"
    "github.com/thyosurya/zta-predictive-cache/control-plane/graph-analyzer/graph"
)

type AnalyzerService struct {
    topology *graph.TopologyGraph
}

type CacheCandidateResponse struct {
    Source      string        `json:"source"`
    Target      string        `json:"target"`
    CallsPerMin int64         `json:"calls_per_min"`
    SuggestedTTL time.Duration `json:"suggested_ttl_ms"`
}

func (s *AnalyzerService) handleGetCandidates(w http.ResponseWriter, r *http.Request) {
    pairs := s.topology.GetHighFrequencyPairs(10)

    var candidates []CacheCandidateResponse
    for _, pair := range pairs {
        ttl := cache.CalculateTTL(
            pair.CallCount,
            cache.SensitivityMedium,
            24*time.Hour,  // simulasi sertifikat berlaku 24 jam
            72*time.Hour,
        )
        candidates = append(candidates, CacheCandidateResponse{
            Source:       pair.Source,
            Target:       pair.Target,
            CallsPerMin:  pair.CallCount,
            SuggestedTTL: ttl / time.Millisecond,
        })
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(candidates)
}

// handleRecordCall menerima notifikasi panggilan antar service
// dipanggil oleh Envoy via access log filter
func (s *AnalyzerService) handleRecordCall(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        Source string `json:"source"`
        Target string `json:"target"`
    }
    json.NewDecoder(r.Body).Decode(&payload)
    s.topology.RecordCall(payload.Source, payload.Target)
    w.WriteHeader(http.StatusAccepted)
}

func main() {
    svc := &AnalyzerService{
        topology: graph.NewTopologyGraph(),
    }

    http.HandleFunc("/candidates", svc.handleGetCandidates)
    http.HandleFunc("/record", svc.handleRecordCall)

    log.Println("Graph Analyzer listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

```bash
# Build dan push image
docker build -t graph-analyzer:latest control-plane/graph-analyzer/
```

---

## 9. Implementasi Cache Policy Distributor

Komponen ini mengambil kandidat dari Graph Analyzer, membuat delegation token bertanda tangan, dan mengirimkannya ke sidecar.

### 9.1 Struktur delegation token

```go
// control-plane/cache-distributor/token/delegation.go
package token

import (
    "crypto/rand"
    "encoding/hex"
    "time"
)

// DelegationToken adalah token yang dikirim ke sidecar
// berisi keputusan otorisasi yang sudah ditanda tangani PDP
type DelegationToken struct {
    Issuer          string    `json:"iss"`
    SourceIdentity  string    `json:"sub"`
    TargetIdentity  string    `json:"aud"`
    AllowedActions  []string  `json:"allowed_actions"`
    Sensitivity     string    `json:"sensitivity"`
    ExpiresAt       int64     `json:"exp"`
    IssuedAt        int64     `json:"iat"`
    RevocationNonce string    `json:"revocation_nonce"`
    CacheTTLMs      int64     `json:"cache_ttl_ms"`
}

// GenerateNonce membuat revocation nonce acak 8 byte
func GenerateNonce() (string, error) {
    b := make([]byte, 8)
    _, err := rand.Read(b)
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(b), nil
}

// NewDelegationToken membuat token baru untuk pasangan service
func NewDelegationToken(source, target string, actions []string, ttl time.Duration) (*DelegationToken, error) {
    nonce, err := GenerateNonce()
    if err != nil {
        return nil, err
    }

    now := time.Now()
    return &DelegationToken{
        Issuer:          "pdp-central.spiffe.io",
        SourceIdentity:  source,
        TargetIdentity:  target,
        AllowedActions:  actions,
        Sensitivity:     "medium",
        ExpiresAt:       now.Add(ttl).Unix(),
        IssuedAt:        now.Unix(),
        RevocationNonce: nonce,
        CacheTTLMs:      ttl.Milliseconds(),
    }, nil
}
```

### 9.2 Push token ke sidecar via gRPC

```protobuf
// control-plane/cache-distributor/proto/cache.proto
syntax = "proto3";
package cache;
option go_package = "./proto";

service CacheDistributor {
  // PushToken mengirim delegation token ke sidecar
  rpc PushToken(TokenPayload) returns (Ack);
  // RevokeToken mencabut token dan invalidasi nonce
  rpc RevokeToken(RevocationRequest) returns (Ack);
  // StreamRevocations stream revokasi ke sidecar secara real-time
  rpc StreamRevocations(SubscribeRequest) returns (stream RevocationEvent);
}

message TokenPayload {
  string token_json = 1;
  string signature  = 2;
}

message RevocationRequest {
  string source_identity = 1;
  string target_identity = 2;
  string old_nonce       = 3;
}

message RevocationEvent {
  string source_identity = 1;
  string target_identity = 2;
  string new_nonce       = 3;
  int64  timestamp       = 4;
}

message SubscribeRequest {
  string sidecar_id = 1;
}

message Ack {
  bool   success = 1;
  string message = 2;
}
```

```bash
# Generate Go code dari proto
cd control-plane/cache-distributor
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc --go_out=. --go-grpc_out=. proto/cache.proto
```

---

## 10. Envoy WASM Filter (Local Cache)

WASM filter berjalan di dalam Envoy sidecar dan mengambil keputusan otorisasi lokal menggunakan delegation token yang sudah di-push.

### 10.1 Konsep WASM filter

Envoy mendukung ekstensi berbasis WebAssembly. Filter ini akan:
1. Intersep setiap request masuk ke service
2. Cek delegation token di local cache
3. Jika token valid dan belum kedaluwarsa: izinkan request langsung
4. Jika tidak ada token atau tidak valid: forward ke PDP central

### 10.2 Implementasi filter (TinyGo)

```bash
# Install TinyGo
wget https://github.com/tinygo-org/tinygo/releases/download/v0.31.0/tinygo_0.31.0_amd64.deb
sudo dpkg -i tinygo_0.31.0_amd64.deb

# Install proxy-wasm-go-sdk
cd wasm-filter
go mod init github.com/thyosurya/zta-predictive-cache/wasm-filter
go get github.com/tetratelabs/proxy-wasm-go-sdk
```

```go
// wasm-filter/main.go
package main

import (
    "encoding/json"
    "time"

    "github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
    "github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

func main() {
    proxywasm.SetVMContext(&vmContext{})
}

type vmContext struct{}
func (*vmContext) OnVMStart(vmConfigurationSize int) types.OnVMStartStatus {
    return types.OnVMStartStatusOK
}
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
    return &pluginContext{}
}

type pluginContext struct{}
func (*pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
    return types.OnPluginStartStatusOK
}
func (*pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
    return &httpContext{contextID: contextID}
}

// CachedToken adalah representasi token di cache lokal in-memory (thread-isolated)
type CachedToken struct {
    Source          string   `json:"source"`
    Target          string   `json:"target"`
    AllowedActions  []string `json:"allowed_actions"`
    ExpiresAt       int64    `json:"exp"`
    RevocationNonce string   `json:"revocation_nonce"`
    Signature       string   `json:"signature"`
}

type httpContext struct {
    types.DefaultHttpContext
    contextID uint32
    plugin    *pluginContext
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
    // 1. Sanitasi header x-zta-cache-verified dari ingress untuk mencegah spoofing bypass
    _ = proxywasm.RemoveHttpRequestHeader("x-zta-cache-verified")

    // Ambil identitas source dari header mTLS (di-set oleh Istio)
    sourceID, _ := proxywasm.GetHttpRequestHeader("x-forwarded-client-cert")
    targetID, _ := proxywasm.GetHttpRequestHeader("host")
    method, _   := proxywasm.GetHttpRequestHeader(":method")

    now := time.Now().Unix()

    // 2. Evaluasi via in-memory pointer cache (zero-deserialization hot path)
    decision := ctx.plugin.cache.Decide(sourceID, targetID, method, now)

    switch decision {
    case token.DecisionAllow:
        // Cache hit authorized: sematkan header bypass OPA fail-closed
        proxywasm.LogInfof("Cache hit: authorized %s -> %s [%s]", sourceID, targetID, method)
        _ = proxywasm.AddHttpRequestHeader("x-zta-cache-verified", "true")
        return types.ActionContinue

    case token.DecisionDeny:
        // Explicit deny dari valid cached token: tolak langsung di sidecar
        proxywasm.LogWarnf("Cache deny: unauthorized action %s for %s -> %s", method, sourceID, targetID)
        _ = proxywasm.SendHttpResponse(403, nil, []byte("Forbidden by ZTA Cache Policy"), -1)
        return types.ActionPause

    default: // DecisionMiss atau expired
        // Cache miss: biarkan request mengalir ke central PDP (ext_authz OPA)
        proxywasm.LogInfof("Cache miss for %s -> %s, forwarding to PDP", sourceID, targetID)
        return types.ActionContinue
    }
}
```

### 10.3 Build dan deploy WASM filter

```bash
# Build WASM binary
cd wasm-filter
tinygo build -o authz-cache.wasm -scheduler=none -target=wasi .

# Buat ConfigMap dari WASM binary
kubectl create configmap wasm-filter \
  --from-file=authz-cache.wasm=authz-cache.wasm \
  -n zta-research
```

```yaml
# k8s/istio/wasm-filter.yaml
apiVersion: extensions.istio.io/v1alpha1
kind: WasmPlugin
metadata:
  name: authz-cache-filter
  namespace: zta-research
spec:
  selector:
    matchLabels:
      app: productcatalogservice
  url: file:///etc/wasm/authz-cache.wasm
  phase: AUTHZ
  pluginConfig:
    pdp_endpoint: "http://opa-service:8181/v1/data/authz/allow"
```

```bash
kubectl apply -f k8s/istio/wasm-filter.yaml
```

---

## 11. Setup Load Testing dengan k6

### 11.1 Install k6

```bash
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69

echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/k6.list

sudo apt-get update && sudo apt-get install k6

# Verifikasi
k6 version
```

### 11.2 Skrip load test per skenario

```javascript
// load-testing/scenarios/s03-zta-cache-500rps.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

// Custom metrics
const p99Latency = new Trend('p99_latency', true);
const authErrors  = new Counter('auth_errors');

export const options = {
    scenarios: {
        constant_load: {
            executor: 'constant-arrival-rate',
            rate: 500,               // 500 RPS
            timeUnit: '1s',
            duration: '5m',          // 5 menit steady-state
            preAllocatedVUs: 50,
            maxVUs: 100,
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],  // p99 harus di bawah 500ms
        http_req_failed:   ['rate<0.01'],  // error rate < 1%
    },
};

const BASE_URL = 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    // Simulasi browsing: list produk -> detail -> tambah ke cart
    const listRes = http.get(`${BASE_URL}/api/products`);
    check(listRes, {
        'list products 200': (r) => r.status === 200,
    });

    if (listRes.status !== 200) {
        authErrors.add(1);
    }

    p99Latency.add(listRes.timings.duration);

    sleep(0.1);
}

export function handleSummary(data) {
    return {
        [`results/s03-${new Date().toISOString()}.json`]: JSON.stringify(data, null, 2),
    };
}
```

### 11.3 Jalankan skenario

```bash
# Buat direktori hasil
mkdir -p load-testing/results

# Jalankan skenario S03
k6 run \
  --out json=load-testing/results/s03-raw.json \
  --out prometheus-rw=http://prometheus:9090/api/v1/write \
  load-testing/scenarios/s03-zta-cache-500rps.js

# Lihat ringkasan hasil
k6 run load-testing/scenarios/s03-zta-cache-500rps.js --summary-trend-stats="min,avg,med,p(90),p(95),p(99),max"
```

### 11.4 Helper script untuk menjalankan semua skenario

```bash
#!/bin/bash
# scripts/run-all-scenarios.sh

SCENARIOS=(
    "s01-pbs-baseline-500rps"
    "s02-zta-naive-500rps"
    "s03-zta-cache-500rps"
    "s04-zta-naive-2000rps"
    "s05-zta-cache-2000rps"
    "s06-zta-cache-5000rps"
)

for scenario in "${SCENARIOS[@]}"; do
    echo "=============================="
    echo "Running: $scenario"
    echo "=============================="

    # Reset cache sebelum tiap skenario
    kubectl rollout restart deployment -n zta-research

    # Cooldown
    sleep 30

    # Jalankan skenario
    k6 run \
      --out json=load-testing/results/${scenario}-$(date +%Y%m%d-%H%M).json \
      load-testing/scenarios/${scenario}.js

    echo "Done: $scenario"
    sleep 120  # 2 menit cooldown antar skenario
done

echo "All scenarios complete."
```

---

## 12. Setup Observabilitas

### 12.1 Deploy Prometheus + Grafana

```bash
# Tambah Helm repo monitoring
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install kube-prometheus-stack (Prometheus + Grafana + Alertmanager)
helm install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring \
  --set grafana.adminPassword=admin \
  --set prometheus.prometheusSpec.scrapeInterval=5s

# Verifikasi
kubectl get pods -n monitoring
```

### 12.2 Deploy Jaeger (distributed tracing)

```bash
kubectl apply -f https://raw.githubusercontent.com/jaegertracing/jaeger-operator/main/deploy/crds/jaegertracing.io_jaegers.yaml

kubectl apply -n monitoring -f - <<EOF
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
  name: jaeger
spec:
  strategy: allInOne
  allInOne:
    image: jaegertracing/all-in-one:latest
  storage:
    type: memory
    options:
      memory:
        max-traces: 100000
EOF
```

### 12.3 Custom metrics untuk cache

Tambahkan Prometheus metrics pada Cache Distributor:

```go
// control-plane/cache-distributor/metrics/metrics.go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    CacheHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "zta_cache_hits_total",
            Help: "Total authorization decisions served from local cache",
        },
        []string{"source", "target"},
    )

    CacheMisses = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "zta_cache_misses_total",
            Help: "Total authorization decisions that required PDP query",
        },
        []string{"source", "target"},
    )

    RevocationLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "zta_revocation_latency_ms",
            Help:    "Time from revocation event to sidecar rejection (ms)",
            Buckets: []float64{10, 25, 50, 75, 100, 150, 200, 500},
        },
    )

    TokenTTL = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "zta_token_ttl_ms",
            Help:    "Delegation token TTL distribution (ms)",
            Buckets: []float64{5000, 10000, 20000, 30000, 45000, 60000},
        },
        []string{"source", "target"},
    )
)

func init() {
    prometheus.MustRegister(CacheHits, CacheMisses, RevocationLatency, TokenTTL)
}
```

### 12.4 Akses dashboard

```bash
# Grafana
kubectl port-forward svc/monitoring-grafana 3000:80 -n monitoring
# Buka: http://localhost:3000 (admin/admin)

# Jaeger
kubectl port-forward svc/jaeger-query 16686:16686 -n monitoring
# Buka: http://localhost:16686

# Prometheus
kubectl port-forward svc/monitoring-kube-prometheus-prometheus 9090:9090 -n monitoring
# Buka: http://localhost:9090
```

### 12.5 Query Prometheus yang berguna

```promql
# p99 latency end-to-end (dari Jaeger/Istio)
histogram_quantile(0.99,
  sum(rate(istio_request_duration_milliseconds_bucket{
    destination_service_namespace="zta-research"
  }[1m])) by (le)
)

# Cache hit ratio
rate(zta_cache_hits_total[1m]) /
(rate(zta_cache_hits_total[1m]) + rate(zta_cache_misses_total[1m]))

# CPU usage Envoy sidecar
sum(rate(container_cpu_usage_seconds_total{
  container="istio-proxy",
  namespace="zta-research"
}[1m])) by (pod)

# Revocation latency p99
histogram_quantile(0.99, rate(zta_revocation_latency_ms_bucket[5m]))
```

---

## 13. Menjalankan Eksperimen

### 13.1 Urutan deployment lengkap

```bash
# 1. Verifikasi semua komponen siap
kubectl get pods -n zta-research
kubectl get pods -n spire
kubectl get pods -n monitoring

# 2. Verifikasi mTLS aktif
istioctl x describe pod <pod-name> -n zta-research

# 3. Verifikasi OPA merespons
kubectl exec -n zta-research deploy/opa -- \
  curl -s http://localhost:8181/health

# 4. Deploy Graph Analyzer
kubectl apply -f k8s/apps/graph-analyzer.yaml -n zta-research

# 5. Deploy Cache Distributor
kubectl apply -f k8s/apps/cache-distributor.yaml -n zta-research

# 6. Apply WASM filter
kubectl apply -f k8s/istio/wasm-filter.yaml

# 7. Jalankan warm-up (5 menit) sebelum eksperimen
k6 run --duration 5m load-testing/warmup.js

# 8. Jalankan semua skenario
bash scripts/run-all-scenarios.sh
```

### 13.2 Export data untuk analisis statistik

```bash
# Export dari Prometheus ke CSV
python3 scripts/export_metrics.py \
  --prometheus-url http://localhost:9090 \
  --start "2024-01-01T00:00:00Z" \
  --end "2024-01-01T12:00:00Z" \
  --output data/raw/

# Analisis dengan Python
python3 scripts/statistical_analysis.py \
  --data-dir data/raw/ \
  --output reports/
```

```python
# scripts/statistical_analysis.py
import pandas as pd
from scipy import stats
import numpy as np

def mann_whitney_comparison(group_a, group_b, label):
    """Uji Mann-Whitney U untuk dua kelompok pengukuran."""
    u_stat, p_value = stats.mannwhitneyu(group_a, group_b, alternative='two-sided')

    # Cliff's Delta
    n1, n2 = len(group_a), len(group_b)
    dominance = sum(1 if a > b else (-1 if a < b else 0)
                    for a in group_a for b in group_b)
    cliffs_d = dominance / (n1 * n2)

    print(f"\n{label}")
    print(f"  U statistic : {u_stat:.2f}")
    print(f"  p-value     : {p_value:.4f} {'(significant)' if p_value < 0.05 else '(not significant)'}")
    print(f"  Cliff's d   : {cliffs_d:.3f}")
    print(f"  Median A    : {np.median(group_a):.2f} ms")
    print(f"  Median B    : {np.median(group_b):.2f} ms")

# Contoh penggunaan
zta_naive  = pd.read_csv('data/raw/s02-latency.csv')['p99'].values
zta_cache  = pd.read_csv('data/raw/s03-latency.csv')['p99'].values

mann_whitney_comparison(zta_naive, zta_cache, "ZTA-Naif vs ZTA-Cache @ 500 RPS")
```

---

## 14. Troubleshooting

### Pod tidak mau start

```bash
# Cek event
kubectl describe pod <pod-name> -n zta-research

# Cek log
kubectl logs <pod-name> -n zta-research --previous
```

### mTLS gagal / connection refused antar service

```bash
# Cek PeerAuthentication
kubectl get peerauthentication -n zta-research

# Cek apakah sidecar ter-inject
kubectl get pod <pod-name> -n zta-research -o jsonpath='{.spec.containers[*].name}'
# Harus ada "istio-proxy"

# Debug dengan istioctl
istioctl proxy-config listeners <pod-name> -n zta-research
```

### WASM filter tidak ter-load

```bash
# Cek log Envoy sidecar
kubectl logs <pod-name> -n zta-research -c istio-proxy | grep -i wasm

# Cek WasmPlugin status
kubectl get wasmplugin -n zta-research
kubectl describe wasmplugin authz-cache-filter -n zta-research
```

### Cache tidak ter-update

```bash
# Cek log Cache Distributor
kubectl logs deploy/cache-distributor -n zta-research

# Cek gRPC stream aktif
kubectl exec -n zta-research deploy/cache-distributor -- \
  grpc_health_probe -addr=:50051
```

### Prometheus tidak scrape metrics

```bash
# Verifikasi ServiceMonitor
kubectl get servicemonitor -n zta-research

# Test endpoint metrics manual
kubectl exec -n zta-research deploy/cache-distributor -- \
  curl http://localhost:2112/metrics | grep zta_
```

---

## Referensi Cepat

| Perintah | Fungsi |
|---|---|
| `kubectl get pods -n zta-research` | Cek status semua pod |
| `istioctl proxy-status` | Status sidecar seluruh mesh |
| `kubectl logs -f deploy/graph-analyzer -n zta-research` | Live log Graph Analyzer |
| `k6 run --dry-run script.js` | Validasi skrip k6 tanpa eksekusi |
| `kubectl rollout restart deploy -n zta-research` | Restart semua deployment (flush cache) |
| `istioctl analyze -n zta-research` | Deteksi masalah konfigurasi Istio |
