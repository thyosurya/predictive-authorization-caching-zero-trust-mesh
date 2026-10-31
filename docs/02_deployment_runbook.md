# Deployment Runbook
## ZTA Predictive Authorization Caching - Prosedur Operasional Eksperimen

---

## Daftar Isi
1. [Checklist Pre-Eksperimen](#1-checklist-pre-eksperimen)
2. [Prosedur Deployment Awal](#2-prosedur-deployment-awal)
3. [Switch Konfigurasi Antar Skenario](#3-switch-konfigurasi-antar-skenario)
4. [Reset Testbed](#4-reset-testbed)
5. [Validasi State Sistem](#5-validasi-state-sistem)
6. [Prosedur Darurat](#6-prosedur-darurat)
7. [Shutdown Bersih](#7-shutdown-bersih)

---

## 1. Checklist Pre-Eksperimen

Lakukan setiap kali sebelum sesi pengujian dimulai.

### Hardware & Network
- [ ] Semua node Kubernetes dalam kondisi `Ready`
- [ ] Tidak ada proses berat lain berjalan di node yang sama
- [ ] Jaringan antar node stabil (ping < 1ms antar node)
- [ ] Disk tersisa > 20GB (untuk log dan metrics)

```bash
# Cek status node
kubectl get nodes -o wide

# Cek resource tersedia
kubectl describe nodes | grep -A5 "Allocated resources"

# Cek disk
df -h /var/lib/rancher
```

### Komponen Sistem
- [ ] Semua pod di `zta-research` dalam status `Running` (0 Pending/CrashLoop)
- [ ] Semua pod di `monitoring` dalam status `Running`
- [ ] Semua pod di `spire` dalam status `Running`
- [ ] Istio control plane (`istiod`) dalam status `Running`
- [ ] OPA merespons health check
- [ ] Graph Analyzer merespons health check
- [ ] Cache Distributor merespons health check
- [ ] Prometheus scraping aktif (target UP)
- [ ] Jaeger collector aktif

```bash
# Satu perintah untuk cek semua pod
kubectl get pods -A | grep -v Running | grep -v Completed | grep -v NAME
# Output kosong = semua baik

# Health check komponen
curl -s http://graph-analyzer.zta-research:8080/health | jq .
curl -s http://cache-distributor.zta-research:8081/health | jq .
curl -s http://opa-service.zta-research:8181/health | jq .
```

### Data & Logging
- [ ] Direktori `load-testing/results/` ada dan writable
- [ ] Prometheus retention cukup (default 15 hari)
- [ ] Jaeger trace buffer tidak penuh

---

## 2. Prosedur Deployment Awal

Jalankan sekali saat pertama kali menyiapkan testbed. Tidak perlu diulang antar skenario.

### Opsi A: Deployment Otomatis pada VM 2 vCPU / 4 GB RAM (Lean Single-Node Testbed)

Jika eksperimen dijalankan pada single-node VM dengan spesifikasi hemat daya (2 vCPU, 4 GB RAM), gunakan skrip otomasi lean berikut:

```bash
# Jalankan skrip otomasi lean
bash scripts/setup_vm_2core_4gb.sh
```

Skrip di atas secara otomatis:
1. Mengonfigurasi 2 GB Linux Swapfile dan kernel sysctl tuning (`vm.swappiness=10`, `somaxconn=2048`, `fs.file-max=2097152`) untuk mencegah Kernel OOM.
2. Menginstal k3s lean dengan menonaktifkan komponen yang tidak diperlukan (`--disable traefik --disable servicelb --disable metrics-server --disable local-storage`).
3. Menginstal Istio menggunakan `profile=minimal` untuk menghemat memori.
4. Menerapkan `k8s/istio/mesh-config.yaml` yang mendefinisikan `extensionProviders` (`opa-ext-authz` port 8181).
5. Menerapkan manifest `k8s/cluster/namespace.yaml`, `k8s/istio/`, dan `k8s/apps/`.

> [!IMPORTANT]
> **Load Generator (k6):** Jalankan pengujian k6 dari luar VM (laptop/workstation penguji) mengarah ke NodePort atau IP VM:PORT agar resource CPU VM 100% dialokasikan untuk Envoy sidecar dan microservice.

---

### Opsi B: Prosedur Manual Multi-Node / Standard Cluster

#### Langkah 1: Deploy infrastruktur dasar

```bash
# Namespace + ServiceAccounts + Secrets
kubectl apply -f k8s/cluster/namespace.yaml
kubectl apply -f k8s/cluster/resource-quota.yaml
kubectl apply -f k8s/cluster/secrets.yaml

# Istio (jika belum)
istioctl install --set profile=demo -y

# SPIRE
kubectl apply -f k8s/spire/
kubectl wait --for=condition=ready pod -l app=spire-server -n spire --timeout=120s
bash scripts/register-spire-entries.sh

# Monitoring stack
helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring \
  --set grafana.adminPassword=ztaresearch \
  --set prometheus.prometheusSpec.scrapeInterval=5s \
  --wait

# Jaeger
kubectl apply -f observability/jaeger/jaeger-deploy.yaml -n monitoring
```

### Langkah 2: Deploy aplikasi testbed

```bash
# Online Boutique
kubectl apply -f k8s/apps/testbed-boutique.yaml -n zta-research

# Tunggu semua pod siap
kubectl wait --for=condition=ready pod --all -n zta-research --timeout=300s

# Verifikasi sidecar ter-inject
kubectl get pods -n zta-research -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[*].name}{"\n"}{end}' | grep istio-proxy | wc -l
# Harus sama dengan jumlah pod
```

### Langkah 3: Konfigurasi Istio mTLS

```bash
kubectl apply -f k8s/istio/peer-auth-strict.yaml
kubectl apply -f k8s/istio/telemetry.yaml

# Verifikasi mTLS aktif
istioctl x describe pod \
  $(kubectl get pod -n zta-research -l app=frontend -o name | head -1) \
  -n zta-research | grep mTLS
# Output harus: mTLS: yes
```

### Langkah 4: Load kebijakan OPA

```bash
kubectl apply -f k8s/apps/opa-deployment.yaml -n zta-research

kubectl create configmap opa-policies \
  --from-file=authz.rego=control-plane/pdp/policies/authz.rego \
  --from-file=data.json=control-plane/pdp/policies/data.json \
  -n zta-research \
  --dry-run=client -o yaml | kubectl apply -f -

# Verifikasi kebijakan dimuat
curl -s -X POST http://opa-service.zta-research:8181/v1/data/authz/allow \
  -H "Content-Type: application/json" \
  -d '{"input":{"source_identity":"spiffe://cluster.local/ns/zta-research/sa/frontend","target_identity":"spiffe://cluster.local/ns/zta-research/sa/productcatalogservice","action":"GET"}}' \
  | jq .result
# Output harus: true
```

### Langkah 5: Deploy control plane algoritma

```bash
kubectl apply -f k8s/apps/graph-analyzer.yaml -n zta-research
kubectl apply -f k8s/apps/cache-distributor.yaml -n zta-research

kubectl wait --for=condition=ready pod -l app=graph-analyzer -n zta-research --timeout=60s
kubectl wait --for=condition=ready pod -l app=cache-distributor -n zta-research --timeout=60s
```

### Langkah 6: Warmup (wajib sebelum pengukuran)

```bash
# Warmup 5 menit untuk mengisi graph topologi dan stabilkan sistem
k6 run --duration 5m --vus 20 load-testing/warmup.js

echo "Warmup selesai. Tunggu 60 detik sebelum skenario pertama."
sleep 60
```

---

## 3. Switch Konfigurasi Antar Skenario

Setiap skenario membutuhkan konfigurasi otorisasi yang berbeda. Gunakan prosedur ini untuk switch secara bersih.

### Konfigurasi 1: PBS Baseline (Skenario S01)

Tanpa otorisasi per-call. Hanya perimeter firewall via NetworkPolicy.

```bash
# Hapus semua AuthorizationPolicy
kubectl delete authorizationpolicy --all -n zta-research

# Hapus WASM filter
kubectl delete wasmplugin --all -n zta-research 2>/dev/null || true

# Terapkan NetworkPolicy PBS (allow-all intra-namespace)
kubectl apply -f k8s/cluster/network-policy-pbs.yaml

# Verifikasi
kubectl get authorizationpolicy -n zta-research
# Output: No resources found
echo "PBS Baseline siap"
```

### Konfigurasi 2: ZTA Naif - PDP Central per call (Skenario S02, S04)

Setiap East-West call query ke OPA tanpa caching.

```bash
# Hapus WASM filter jika ada
kubectl delete wasmplugin --all -n zta-research 2>/dev/null || true

# Terapkan AuthorizationPolicy yang redirect ke OPA (tanpa WASM cache)
kubectl apply -f k8s/istio/authz-policy-zta.yaml

# Verifikasi
kubectl get authorizationpolicy -n zta-research
# Output harus: zta-ext-authz + zta-deny-default
kubectl get wasmplugin -n zta-research
# Output harus: No resources found

bash scripts/validate-state.sh zta-naive
echo "ZTA Naif siap"
```

### Konfigurasi 3: ZTA + Predictive Cache (Skenario S03, S05–S10)

Cache delegation aktif dengan graph-aware TTL.

```bash
# Pastikan AuthorizationPolicy ZTA naif masih aktif sebagai fallback
kubectl apply -f k8s/istio/authz-policy-zta.yaml -n zta-research

# Deploy WASM filter
kubectl apply -f k8s/istio/wasm-filter.yaml

# Tunggu filter terdeploy di semua sidecar
sleep 10

# Trigger distribusi token awal
curl -s -X POST http://cache-distributor.zta-research:8081/distribute \
  -H "Content-Type: application/json" \
  -d @control-plane/cache-distributor/seed-candidates.json | jq .tokens_created

# Verifikasi cache mulai terisi
curl -s http://cache-distributor.zta-research:8081/tokens/active | jq 'length'
# Harus > 0

echo "ZTA + Predictive Cache siap"
```

---

## 4. Reset Testbed

Jalankan antara setiap skenario untuk memastikan state bersih.

### Reset ringan (antar pengulangan dalam skenario yang sama)

```bash
#!/bin/bash
# scripts/reset-light.sh

echo "[RESET RINGAN] Flushing cache dan counter..."

# Flush local cache semua sidecar via restart Cache Distributor
kubectl rollout restart deployment/cache-distributor -n zta-research
kubectl wait --for=condition=ready pod -l app=cache-distributor -n zta-research --timeout=60s

# Reset graph topologi
curl -s -X DELETE http://graph-analyzer.zta-research:8080/graph/reset \
  -H "X-Admin-Token: ${ADMIN_TOKEN}" | jq .

echo "[RESET RINGAN] Selesai. Cooldown 30 detik..."
sleep 30
```

### Reset penuh (antar skenario berbeda)

```bash
#!/bin/bash
# scripts/reset-full.sh

echo "[RESET PENUH] Memulai..."

# 1. Hapus semua konfigurasi otorisasi
kubectl delete authorizationpolicy --all -n zta-research 2>/dev/null
kubectl delete wasmplugin --all -n zta-research 2>/dev/null

# 2. Restart seluruh deployment aplikasi
kubectl rollout restart deployment -n zta-research

# 3. Tunggu semua pod kembali siap
kubectl wait --for=condition=ready pod --all -n zta-research --timeout=180s

# 4. Reset graph
curl -s -X DELETE http://graph-analyzer.zta-research:8080/graph/reset \
  -H "X-Admin-Token: ${ADMIN_TOKEN}"

# 5. Bersihkan Prometheus (opsional - jangan hapus data historis)
# Hanya reset in-memory state

# 6. Jalankan warmup singkat
k6 run --duration 2m --vus 10 load-testing/warmup.js

echo "[RESET PENUH] Selesai. Cooldown 120 detik..."
sleep 120
```

---

## 5. Validasi State Sistem

Jalankan sebelum memulai pengukuran untuk memastikan sistem dalam kondisi yang benar.

### Script validasi lengkap

```bash
#!/bin/bash
# scripts/validate-state.sh

SCENARIO=$1  # pbs | zta-naive | zta-cache
ERRORS=0

echo "=== Validasi State Sistem: $SCENARIO ==="

# --- Cek 1: Semua pod running ---
NOT_RUNNING=$(kubectl get pods -n zta-research --no-headers | grep -v Running | wc -l)
if [ "$NOT_RUNNING" -gt 0 ]; then
  echo "❌ Ada $NOT_RUNNING pod tidak Running"
  kubectl get pods -n zta-research | grep -v Running
  ERRORS=$((ERRORS+1))
else
  echo "✅ Semua pod Running"
fi

# --- Cek 2: mTLS aktif ---
MTLS_STATUS=$(istioctl x describe pod \
  $(kubectl get pod -n zta-research -l app=frontend -o name | head -1) \
  -n zta-research 2>/dev/null | grep -c "mTLS: yes")
if [ "$MTLS_STATUS" -lt 1 ]; then
  echo "❌ mTLS tidak aktif"
  ERRORS=$((ERRORS+1))
else
  echo "✅ mTLS aktif"
fi

# --- Cek 3: OPA merespons ---
OPA_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" http://opa-service.zta-research:8181/health)
if [ "$OPA_HEALTH" != "200" ]; then
  echo "❌ OPA tidak merespons (HTTP $OPA_HEALTH)"
  ERRORS=$((ERRORS+1))
else
  echo "✅ OPA sehat"
fi

# --- Cek 4: Konfigurasi otorisasi sesuai skenario ---
AUTH_POLICY_COUNT=$(kubectl get authorizationpolicy -n zta-research --no-headers 2>/dev/null | wc -l)
WASM_COUNT=$(kubectl get wasmplugin -n zta-research --no-headers 2>/dev/null | wc -l)

case $SCENARIO in
  pbs)
    if [ "$AUTH_POLICY_COUNT" -gt 0 ] || [ "$WASM_COUNT" -gt 0 ]; then
      echo "❌ Masih ada AuthorizationPolicy/WasmPlugin (PBS harus bersih)"
      ERRORS=$((ERRORS+1))
    else
      echo "✅ PBS: Tidak ada otorisasi per-call"
    fi
    ;;
  zta-naive)
    if [ "$AUTH_POLICY_COUNT" -lt 1 ]; then
      echo "❌ ZTA Naif: AuthorizationPolicy tidak ditemukan"
      ERRORS=$((ERRORS+1))
    elif [ "$WASM_COUNT" -gt 0 ]; then
      echo "❌ ZTA Naif: WasmPlugin seharusnya tidak ada"
      ERRORS=$((ERRORS+1))
    else
      echo "✅ ZTA Naif: AuthorizationPolicy aktif, tanpa WASM cache"
    fi
    ;;
  zta-cache)
    if [ "$AUTH_POLICY_COUNT" -lt 1 ]; then
      echo "❌ ZTA Cache: AuthorizationPolicy tidak ditemukan"
      ERRORS=$((ERRORS+1))
    elif [ "$WASM_COUNT" -lt 1 ]; then
      echo "❌ ZTA Cache: WasmPlugin tidak ditemukan"
      ERRORS=$((ERRORS+1))
    else
      echo "✅ ZTA Cache: AuthorizationPolicy + WasmPlugin aktif"
    fi
    ;;
esac

# --- Cek 5: Token aktif (khusus ZTA Cache) ---
if [ "$SCENARIO" = "zta-cache" ]; then
  TOKEN_COUNT=$(curl -s http://cache-distributor.zta-research:8081/tokens/active | jq 'length')
  if [ "$TOKEN_COUNT" -lt 1 ]; then
    echo "❌ ZTA Cache: Tidak ada token aktif"
    ERRORS=$((ERRORS+1))
  else
    echo "✅ ZTA Cache: $TOKEN_COUNT token aktif"
  fi
fi

# --- Cek 6: Prometheus scraping ---
PROM_TARGETS_UP=$(curl -s http://localhost:9090/api/v1/targets | \
  jq '[.data.activeTargets[] | select(.health=="up")] | length')
echo "✅ Prometheus: $PROM_TARGETS_UP targets UP"

# --- Hasil ---
echo ""
if [ "$ERRORS" -eq 0 ]; then
  echo "✅✅✅ VALIDASI LULUS - sistem siap untuk pengukuran"
  exit 0
else
  echo "❌❌❌ VALIDASI GAGAL - $ERRORS masalah ditemukan. Selesaikan sebelum melanjutkan."
  exit 1
fi
```

### Penggunaan

```bash
# Validasi sebelum skenario PBS
bash scripts/validate-state.sh pbs

# Validasi sebelum skenario ZTA Naif
bash scripts/validate-state.sh zta-naive

# Validasi sebelum skenario ZTA Cache
bash scripts/validate-state.sh zta-cache
```

---

## 6. Prosedur Darurat

### Pod crash loop selama eksperimen

```bash
# Identifikasi pod bermasalah
kubectl get pods -n zta-research | grep -v Running

# Lihat log
kubectl logs <pod-name> -n zta-research --previous

# Jika tidak bisa recovery: tandai skenario sebagai invalid dan reset penuh
echo "SKENARIO INVALID: $(date)" >> load-testing/results/invalid-runs.log
bash scripts/reset-full.sh
```

### OPA tidak merespons saat pengukuran

```bash
# Restart OPA
kubectl rollout restart deployment/opa -n zta-research
kubectl wait --for=condition=ready pod -l app=opa -n zta-research --timeout=60s

# Verifikasi
curl -s http://opa-service.zta-research:8181/health | jq .
```

### gRPC stream Cache Distributor terputus

```bash
# Cek jumlah koneksi aktif
curl -s http://cache-distributor.zta-research:8081/health | jq .connected_sidecars

# Jika 0, restart Cache Distributor (sidecar akan reconnect otomatis)
kubectl rollout restart deployment/cache-distributor -n zta-research
kubectl wait --for=condition=ready pod -l app=cache-distributor -n zta-research --timeout=60s

# Monitor reconnect
kubectl logs -f deployment/cache-distributor -n zta-research | grep "stream"
```

### Prometheus kehilangan data

```bash
# Cek status Prometheus
kubectl get pods -n monitoring | grep prometheus

# Jika pod restart, data dalam window sebelum restart hilang
# Tandai gap di catatan eksperimen
echo "PROMETHEUS RESTART: $(date) - data gap kemungkinan ada" \
  >> load-testing/results/incident-log.txt
```

---

## 7. Shutdown Bersih

Jalankan setelah semua skenario eksperimen selesai.

```bash
#!/bin/bash
# scripts/shutdown.sh

echo "=== Shutdown Bersih Testbed ==="

# 1. Export semua data sebelum shutdown
echo "Mengekspor data Prometheus..."
python3 scripts/export-metrics.py \
  --prometheus-url http://localhost:9090 \
  --output data/raw/final-export-$(date +%Y%m%d)/

# 2. Export Jaeger traces
echo "Mengekspor Jaeger traces..."
curl -s "http://localhost:16686/api/traces?service=frontend&limit=10000" \
  > data/raw/final-export-$(date +%Y%m%d)/jaeger-traces.json

# 3. Hapus resource eksperimen (tapi biarkan monitoring)
kubectl delete -f k8s/apps/ -n zta-research
kubectl delete -f k8s/istio/ -n zta-research 2>/dev/null

echo "=== Shutdown selesai. Data tersimpan di data/raw/ ==="
```
