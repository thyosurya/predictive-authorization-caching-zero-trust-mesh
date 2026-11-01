#!/bin/bash
# scripts/register-spire-entries.sh
set -euo pipefail

ENTRIES=(
  "frontend"
  "productcatalogservice"
  "cartservice"
  "currencyservice"
  "checkoutservice"
  "paymentservice"
  "recommendationservice"
  "graph-analyzer"
  "cache-distributor"
)

for svc in "${ENTRIES[@]}"; do
  echo "Registering $svc..."
  kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID "spiffe://cluster.local/ns/zta-research/sa/${svc}" \
    -parentID "spiffe://cluster.local/ns/spire/sa/spire-agent" \
    -selector "k8s:ns:zta-research" \
    -selector "k8s:sa:${svc}" \
    -ttl 3600 2>/dev/null || echo "  (entry may already exist)"
done

echo "All entries registered. Total: ${#ENTRIES[@]}"
