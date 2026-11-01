#!/bin/bash
# scripts/shutdown.sh
set -euo pipefail

echo "=== Shutdown Bersih Testbed ==="

EXPORT_DIR="data/raw/final-export-$(date +%Y%m%d)"
mkdir -p "$EXPORT_DIR"

# 1. Export Jaeger traces
echo "Mengekspor Jaeger traces..."
python3 scripts/export-jaeger.py \
  --jaeger-url http://localhost:16686 \
  --service frontend \
  --scenario FINAL \
  --start "$(date -u -d '12 hours ago' +%Y-%m-%dT%H:%M:%SZ)" \
  --end "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --output "$EXPORT_DIR" 2>/dev/null || echo "WARNING: Jaeger export failed"

# 2. Snapshot config sebelum hapus
echo "Menyimpan snapshot konfigurasi..."
bash scripts/snapshot-config.sh

# 3. Hapus resource eksperimen (tapi biarkan monitoring)
echo "Menghapus resource eksperimen..."
kubectl delete -f k8s/apps/ -n zta-research 2>/dev/null || true
kubectl delete -f k8s/istio/ -n zta-research 2>/dev/null || true

echo "=== Shutdown selesai. Data tersimpan di $EXPORT_DIR ==="
