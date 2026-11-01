#!/bin/bash
# ==============================================================================
# scripts/build_docker_images.sh
# Build Docker images dari precompiled binaries dan import ke K3s containerd
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== 1. Compiling Native Go Binaries ==="
echo "  Compiling service-a (frontend)..."
(cd "$REPO_ROOT/services/service-a" && CGO_ENABLED=0 go build -ldflags="-s -w" -o service-a .)
echo "  Compiling service-b (productcatalogservice)..."
(cd "$REPO_ROOT/services/service-b" && CGO_ENABLED=0 go build -ldflags="-s -w" -o service-b .)
echo "  Compiling service-c (cartservice)..."
(cd "$REPO_ROOT/services/service-c" && CGO_ENABLED=0 go build -ldflags="-s -w" -o service-c .)
echo "  Compiling graph-analyzer..."
(cd "$REPO_ROOT/control-plane/graph-analyzer" && CGO_ENABLED=0 go build -ldflags="-s -w" -o graph-analyzer .)
echo "  Compiling cache-distributor..."
(cd "$REPO_ROOT/control-plane/cache-distributor" && CGO_ENABLED=0 go build -ldflags="-s -w" -o cache-distributor .)

echo "=== 2. Building Docker Images from Native Binaries ==="

# 1. Frontend (service-a)
cd "$REPO_ROOT/services/service-a"
cat << 'EOF' > Dockerfile.deploy
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY service-a /app/main
EXPOSE 8080
ENTRYPOINT ["/app/main"]
EOF
docker build -t thyosurya/frontend:latest -f Dockerfile.deploy .

# 2. Productcatalogservice (service-b)
cd "$REPO_ROOT/services/service-b"
cat << 'EOF' > Dockerfile.deploy
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY service-b /app/main
EXPOSE 8080
ENTRYPOINT ["/app/main"]
EOF
docker build -t thyosurya/productcatalogservice:latest -f Dockerfile.deploy .

# 3. Cartservice (service-c)
cd "$REPO_ROOT/services/service-c"
cat << 'EOF' > Dockerfile.deploy
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY service-c /app/main
EXPOSE 8080
ENTRYPOINT ["/app/main"]
EOF
docker build -t thyosurya/cartservice:latest -f Dockerfile.deploy .

# 4. Graph Analyzer
cd "$REPO_ROOT/control-plane/graph-analyzer"
cat << 'EOF' > Dockerfile.deploy
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY graph-analyzer /app/main
EXPOSE 8080 2112
ENTRYPOINT ["/app/main"]
EOF
docker build -t thyosurya/graph-analyzer:latest -f Dockerfile.deploy .

# 5. Cache Distributor
cd "$REPO_ROOT/control-plane/cache-distributor"
cat << 'EOF' > Dockerfile.deploy
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY cache-distributor /app/main
EXPOSE 8081 50051 2112
ENTRYPOINT ["/app/main"]
EOF
docker build -t thyosurya/cache-distributor:latest -f Dockerfile.deploy .

echo "=== Importing Updated Images to K3s containerd ==="
for img in thyosurya/frontend:latest thyosurya/productcatalogservice:latest thyosurya/cartservice:latest thyosurya/graph-analyzer:latest thyosurya/cache-distributor:latest; do
    echo "  Importing $img..."
    docker save "$img" | sudo /usr/local/bin/k3s ctr images import -
done

echo "=== Restarting Deployments in zta-research ==="
export KUBECONFIG=/home/thyo/.kube/config
kubectl rollout restart deployment -n zta-research
kubectl wait --namespace zta-research --for=condition=ready pod --selector=app --timeout=180s || true

echo "=== Current Pod Status ==="
kubectl get pods -n zta-research -o wide
