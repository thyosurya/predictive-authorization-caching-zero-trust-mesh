#!/bin/bash
# scripts/reset-light.sh
# Reset ringan testbed sebelum setiap repetisi pengujian beban.

export KUBECONFIG="${KUBECONFIG:-/home/thyo/.kube/config}"

echo "[RESET RINGAN] Flushing cache dan counter..."

# Pastikan ADMIN_TOKEN terisi
ADMIN_TOKEN=${ADMIN_TOKEN:-$(kubectl get secret zta-admin-secret -n zta-research -o jsonpath='{.data.admin-token}' 2>/dev/null | base64 -d 2>/dev/null || echo "research-admin-token-2024")}

# Flush local cache semua sidecar via restart Cache Distributor
kubectl rollout restart deployment/cache-distributor -n zta-research 2>/dev/null || true
kubectl wait --for=condition=ready pod -l app=cache-distributor -n zta-research --timeout=60s 2>/dev/null || true

# Reset graph topologi
curl -s -X DELETE http://graph-analyzer.zta-research:8080/graph/reset \
  -H "X-Admin-Token: ${ADMIN_TOKEN}" 2>/dev/null || true

echo "[RESET RINGAN] Selesai. Cooldown 15 detik..."
sleep 15
