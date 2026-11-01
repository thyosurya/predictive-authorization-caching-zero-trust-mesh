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
