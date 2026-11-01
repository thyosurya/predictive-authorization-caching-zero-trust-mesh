#!/bin/bash
# scripts/verify-versions.sh
set -uo pipefail

echo "=== Verifikasi Versi Komponen ==="
ERRORS=0

# k3s / kubectl
K3S_VER=$(kubectl version -o json 2>/dev/null | grep -o '"gitVersion":"[^"]*"' | head -1 | cut -d'"' -f4)
[[ "$K3S_VER" == "v1.28.5+k3s1" ]] && echo "✅ k3s: $K3S_VER" || { echo "⚠️  k3s: $K3S_VER (expected v1.28.5+k3s1)"; ERRORS=$((ERRORS+1)); }

# Istio
ISTIO_VER=$(istioctl version --remote=false 2>/dev/null | head -1)
[[ "$ISTIO_VER" == "1.20.1" ]] && echo "✅ Istio: $ISTIO_VER" || { echo "⚠️  Istio: $ISTIO_VER (expected 1.20.1)"; ERRORS=$((ERRORS+1)); }

# OPA
OPA_VER=$(kubectl exec -n zta-research deploy/opa -- opa version 2>/dev/null | head -1 | awk '{print $2}')
[[ "$OPA_VER" == "0.60.0" ]] && echo "✅ OPA: $OPA_VER" || { echo "⚠️  OPA: $OPA_VER (expected 0.60.0)"; ERRORS=$((ERRORS+1)); }

# k6
K6_VER=$(k6 version 2>/dev/null | awk '{print $2}')
[[ "$K6_VER" == "v0.48.0" ]] && echo "✅ k6: $K6_VER" || { echo "⚠️  k6: $K6_VER (expected v0.48.0)"; ERRORS=$((ERRORS+1)); }

# Go
GO_VER=$(go version 2>/dev/null | awk '{print $3}')
[[ "$GO_VER" == "go1.22.0" ]] && echo "✅ Go: $GO_VER" || echo "ℹ️  Go: $GO_VER (expected go1.22.0)"

# Python
PY_VER=$(python3 --version 2>/dev/null | awk '{print $2}')
echo "ℹ️  Python: $PY_VER"

echo ""
if [ "$ERRORS" -eq 0 ]; then
  echo "[RESULT] Semua versi sesuai. ✅"
  exit 0
else
  echo "[RESULT] $ERRORS komponen tidak sesuai versi. Perbaiki sebelum eksperimen."
  exit 1
fi
