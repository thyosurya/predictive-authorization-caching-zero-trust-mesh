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
