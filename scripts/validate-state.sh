#!/bin/bash
# scripts/validate-state.sh

# Auto-detect K3s kubeconfig
if [ -f /etc/rancher/k3s/k3s.yaml ]; then
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

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
FRONTEND_POD=$(kubectl get pod -n zta-research -l app=frontend -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$FRONTEND_POD" ]; then
  echo "❌ mTLS tidak aktif (frontend pod tidak ditemukan)"
  ERRORS=$((ERRORS+1))
else
  MTLS_STATUS=$(istioctl x describe pod ${FRONTEND_POD}.zta-research --kubeconfig /etc/rancher/k3s/k3s.yaml 2>/dev/null | grep -c -E "mTLS mode: (STRICT|PERMISSIVE)")
  if [ "$MTLS_STATUS" -lt 1 ]; then
    echo "❌ mTLS tidak aktif"
    ERRORS=$((ERRORS+1))
  else
    echo "✅ mTLS aktif (STRICT atau PERMISSIVE)"
  fi
fi

# --- Cek 3: OPA merespons ---
OPA_READY=$(kubectl get pod -n zta-research -l app=opa -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null || echo "false")
if [ "$OPA_READY" != "true" ]; then
  echo "❌ OPA tidak merespons (not ready)"
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
  DIST_POD=$(kubectl get pod -n zta-research -l app=cache-distributor -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  TOKEN_COUNT=$(kubectl exec -n zta-research ${DIST_POD} -c distributor -- wget -qO- http://localhost:8081/tokens/active 2>/dev/null | jq 'length' 2>/dev/null || echo "0")
  if [ -z "$TOKEN_COUNT" ] || [ "$TOKEN_COUNT" -eq 0 ]; then
    echo "❌ ZTA Cache: Tidak ada token aktif"
    ERRORS=$((ERRORS+1))
  else
    echo "✅ ZTA Cache: $TOKEN_COUNT token aktif"
  fi
fi

# --- Cek 6: Prometheus scraping ---
PROM_IP=$(kubectl get svc monitoring-kube-prometheus-prometheus -n monitoring -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "127.0.0.1")
PROM_TARGETS_UP=$(curl -s http://${PROM_IP}:9090/api/v1/targets | \
  jq '[.data.activeTargets[] | select(.health=="up")] | length' 2>/dev/null || echo "0")
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
