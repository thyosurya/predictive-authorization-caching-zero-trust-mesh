#!/bin/bash
# ==============================================================================
# scripts/setup_vm_2core_4gb.sh
# Skrip Otomasi Deployment Lean Testbed untuk VM 2 vCPU / 4 GB RAM
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

export PATH="/usr/local/go/bin:/usr/local/bin:/usr/sbin:/sbin:$PATH"
export KUBECONFIG="/home/thyo/.kube/config"

echo "=========================================================="
echo "  ZTA Predictive Cache - VM 2-Core 4GB Lean Setup"
echo "=========================================================="

# 1. Konfigurasi Swap (2 GB) & Sysctl OS
echo "[1/7] Menyiapkan Swap 2GB & Kernel Tuning..."
if [ ! -f /swapfile ]; then
    echo "  -> Membuat /swapfile 2GB..."
    sudo fallocate -l 2G /swapfile || sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
    sudo chmod 600 /swapfile
    sudo mkswap /swapfile
    sudo swapon /swapfile
    if ! grep -q "/swapfile" /etc/fstab; then
        echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
    fi
    echo "  -> Swap 2GB berhasil diaktifkan."
else
    echo "  -> /swapfile sudah ada."
    if ! swapon --show | grep -q "/swapfile"; then
        sudo swapon /swapfile 2>/dev/null || true
    fi
fi

sudo sysctl -w vm.swappiness=10
sudo sysctl -w net.core.somaxconn=2048
sudo sysctl -w fs.file-max=2097152

# 2. Instalasi K3s Lean (Tanpa Traefik, ServiceLB, Storage)
echo "[2/7] Menyiapkan K3s Lean..."
if ! command -v k3s &> /dev/null; then
    curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik --disable servicelb --disable metrics-server --disable local-storage" sh -
    mkdir -p /home/thyo/.kube
    sudo cp /etc/rancher/k3s/k3s.yaml /home/thyo/.kube/config
    sudo chown thyo:thyo /home/thyo/.kube/config
    chmod 600 /home/thyo/.kube/config
    if ! grep -q "KUBECONFIG" /home/thyo/.bashrc; then
        echo 'export KUBECONFIG=~/.kube/config' >> /home/thyo/.bashrc
    fi
    echo "  -> K3s berhasil diinstal."
else
    echo "  -> K3s sudah terpasang."
    mkdir -p /home/thyo/.kube
    sudo cp -u /etc/rancher/k3s/k3s.yaml /home/thyo/.kube/config 2>/dev/null || true
    sudo chown thyo:thyo /home/thyo/.kube/config 2>/dev/null || true
    chmod 600 /home/thyo/.kube/config 2>/dev/null || true
fi

echo "  -> Menunggu K3s Node Ready..."
until kubectl get nodes 2>/dev/null | grep -q 'Ready'; do
    sleep 2
done
echo "  -> Node Ready!"

echo "  -> Mengimpor Docker images ke K3s containerd..."
IMAGES=(
    "thyosurya/frontend:latest"
    "thyosurya/productcatalogservice:latest"
    "thyosurya/cartservice:latest"
    "thyosurya/graph-analyzer:latest"
    "thyosurya/cache-distributor:latest"
    "openpolicyagent/opa:0.60.0"
)

for img in "${IMAGES[@]}"; do
    if docker image inspect "$img" > /dev/null 2>&1; then
        echo "     Importing $img..."
        docker save "$img" | sudo /usr/local/bin/k3s ctr images import - > /dev/null
    else
        echo "     WARNING: Image $img tidak ditemukan di Docker!"
    fi
done

# 3. Instalasi Istio Minimal Profile
echo "[3/7] Menginstal Istio (Minimal Profile)..."
if ! command -v istioctl &> /dev/null; then
    ISTIO_VERSION="1.20.2"
    echo "  -> Mengunduh Istio ${ISTIO_VERSION}..."
    curl -sL https://istio.io/downloadIstio | ISTIO_VERSION=$ISTIO_VERSION sh -
    sudo mv istio-${ISTIO_VERSION}/bin/istioctl /usr/local/bin/
    rm -rf istio-${ISTIO_VERSION}
fi

istioctl install --set profile=minimal -y
echo "  -> Menunggu Istiod Control Plane siap..."
kubectl wait --namespace istio-system --for=condition=ready pod -l app=istiod --timeout=180s || true

# 4. Buat Namespace, Secrets & Enable Istio Sidecar Injection
echo "[4/7] Menyiapkan Namespace & Secrets..."
kubectl apply -f k8s/cluster/namespace.yaml
kubectl apply -f k8s/cluster/secrets.yaml
kubectl label namespace zta-research istio-injection=enabled --overwrite

# 5. Pasang MeshConfig, WASM Filter & ExtensionProviders
echo "[5/7] Menerapkan MeshConfig & WASM Filter..."
WASM_PATH="$REPO_ROOT/wasm-filter/authz-cache.wasm"
if [ -f "$WASM_PATH" ]; then
    echo "  -> Mendaftarkan ConfigMap wasm-filter dari $WASM_PATH..."
    kubectl delete configmap wasm-filter -n zta-research --ignore-not-found
    kubectl create configmap wasm-filter \
      --from-file=authz-cache.wasm="$WASM_PATH" \
      -n zta-research
fi

kubectl apply -f k8s/istio/mesh-config.yaml
kubectl apply -f k8s/istio/authz-policy-zta.yaml
kubectl apply -f k8s/istio/wasm-filter.yaml

# 6. Deploy Microservices + Control Plane
echo "[6/7] Menyiapkan Deployment Pods..."
if [ -d "k8s/apps" ]; then
    kubectl apply -f k8s/apps/ -n zta-research
fi

# 7. Menunggu Pods Siap
echo "[7/7] Menunggu semua pod siap di namespace zta-research..."
kubectl wait --namespace zta-research \
  --for=condition=ready pod \
  --selector=app \
  --timeout=300s || true

echo ""
echo "=========================================================="
echo "  Setup Testbed Selesai!"
echo "=========================================================="
echo "Status Pod:"
kubectl get pods -n zta-research -o wide
echo ""
echo "Node Resources:"
kubectl top nodes 2>/dev/null || free -h
echo ""
echo "Instruksi eksekusi k6:"
echo "  k6 run load-testing/scenarios/s01-pbs-baseline-500rps.js"
echo "=========================================================="
