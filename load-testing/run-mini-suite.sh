#!/bin/bash
# load-testing/run-mini-suite.sh
# Menjalankan mini smoke test k6 untuk seluruh 10 skenario penelitian (S01 - S10).
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="$DIR/mini-scenarios"
LOG_DIR="$DIR/results/mini_logs"
mkdir -p "$LOG_DIR"

echo "================================================================"
echo "  MINI SMOKE TEST SUITE K6: SKENARIO S01 - S10"
echo "================================================================"

# Verifikasi host mapping untuk cluster domain
FRONTEND_IP=$(kubectl get svc -n zta-research frontend -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
DISTRIBUTOR_IP=$(kubectl get svc -n zta-research cache-distributor -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")

if [ -n "$FRONTEND_IP" ] && [ -n "$DISTRIBUTOR_IP" ]; then
    echo "[*] Sinkronisasi IP Cluster ke /etc/hosts..."
    sudo sed -i '/zta-research/d' /etc/hosts
    for svc in frontend cache-distributor cartservice productcatalogservice graph-analyzer opa; do
        ip=$(kubectl get svc -n zta-research "$svc" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
        if [ -n "$ip" ]; then
            echo "$ip $svc.zta-research.svc.cluster.local $svc.zta-research $svc" | sudo tee -a /etc/hosts >/dev/null
        fi
    done
fi

SCENARIOS=(
    "mini-s01-pbs-baseline.js:S01 PBS Baseline"
    "mini-s02-zta-naive.js:S02 ZTA Naive"
    "mini-s03-zta-cache.js:S03 ZTA Cache (500 RPS)"
    "mini-s04-zta-naive-2000rps.js:S04 ZTA Naive (2000 RPS)"
    "mini-s05-zta-cache-2000rps.js:S05 ZTA Cache (2000 RPS)"
    "mini-s06-zta-cache-5000rps.js:S06 ZTA Cache Stress"
    "mini-s07-zta-cache-fanout.js:S07 Cache Fan-out"
    "mini-s08-zta-cache-mesh.js:S08 Cache Mesh"
    "mini-s09-revocation-test.js:S09 Revocation Lifecycle"
    "mini-s10-ttl-comparison.js:S10 TTL Comparison"
    "mini-ablation-multihop.js:Ablation Multi-hop (Contention)"
)

TOTAL=${#SCENARIOS[@]}
PASSED=0
FAILED=0

printf "\n%-30s | %-12s | %-10s\n" "SKENARIO" "STATUS" "DURATION"
echo "----------------------------------------------------------------"

for item in "${SCENARIOS[@]}"; do
    FILE="${item%%:*}"
    LABEL="${item##*:}"
    LOG_FILE="$LOG_DIR/${FILE%.js}.log"
    
    START=$(date +%s)
    if k6 run --quiet "$SCENARIOS_DIR/$FILE" > "$LOG_FILE" 2>&1; then
        END=$(date +%s)
        DUR=$((END - START))
        printf "%-30s | \e[32mPASS\e[0m         | %ds\n" "$LABEL" "$DUR"
        PASSED=$((PASSED + 1))
    else
        END=$(date +%s)
        DUR=$((END - START))
        printf "%-30s | \e[31mFAIL\e[0m         | %ds\n" "$LABEL" "$DUR"
        echo "  [!] Log error pada $LOG_FILE:"
        tail -n 10 "$LOG_FILE" | sed 's/^/      /'
        FAILED=$((FAILED + 1))
    fi
done

echo "----------------------------------------------------------------"
echo "HASIL AKHIR: $PASSED / $TOTAL LULUS, $FAILED GAGAL"
echo "================================================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
exit 0
