#!/bin/bash
# scripts/run-all-scenarios.sh
# Menjalankan semua 10 skenario eksperimen dengan repetisi
# Usage: bash scripts/run-all-scenarios.sh [repetitions]

set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-/home/thyo/.kube/config}"

REPETITIONS=${1:-30}
START_REP=${2:-1}
RUN_ID=$(date +%Y%m%d_%H%M%S)
RESULTS_DIR=${3:-"load-testing/results/run_${RUN_ID}"}
mkdir -p "$RESULTS_DIR"

SCENARIOS=(
    "s01-pbs-baseline-500rps"
    "s02-zta-naive-500rps"
    "s03-zta-cache-500rps"
    "s04-zta-naive-2000rps"
    "s05-zta-cache-2000rps"
    "s06-zta-cache-5000rps"
    "s07-zta-cache-fanout"
    "s08-zta-cache-mesh"
    "s09-revocation-test"
    "s10-ttl-comparison"
)

echo "=========================================="
echo "ZTA Predictive Cache - Experiment Runner"
echo "Scenarios: ${#SCENARIOS[@]}"
echo "Repetitions: $REPETITIONS (Starting from $START_REP)"
echo "Output Directory: $RESULTS_DIR"
echo "=========================================="

for rep in $(seq $START_REP $REPETITIONS); do
    echo ""
    echo ">>> REPETITION $rep / $REPETITIONS"
    echo ""
    # Flush OS page caches antar-repetisi untuk mencegah akumulasi memori pada VM 4GB
    sync && echo 3 | sudo tee /proc/sys/vm/drop_caches >/dev/null 2>&1 || true

    for scenario in "${SCENARIOS[@]}"; do
        # Idempotency check: skip HANYA jika file .json dan .csv keduanya lengkap
        EXISTING_JSON=$(ls "${RESULTS_DIR}/${scenario}-rep${rep}-"*.json 2>/dev/null | head -n 1 || true)
        EXISTING_CSV=$(ls "${RESULTS_DIR}/prometheus-${scenario}-rep${rep}.csv" 2>/dev/null | head -n 1 || true)
        if [ -n "$EXISTING_JSON" ] && [ -n "$EXISTING_CSV" ]; then
            echo "  [SKIP] Sudah lengkap sebelumnya (JSON & CSV): $scenario rep $rep"
            continue
        fi

        TIMESTAMP=$(date +%Y%m%d-%H%M%S)
        OUTFILE="${RESULTS_DIR}/${scenario}-rep${rep}-${TIMESTAMP}.json"

        echo "=============================="
        echo "Running: $scenario (rep $rep)"
        echo "=============================="

        # 1. Reset testbed - clear cache dan state
        echo "  Resetting testbed..."
        bash scripts/reset-light.sh 2>/dev/null || true

        # 2. Warmup (5 menit, low rate)
        echo "  Warming up..."
        k6 run --quiet load-testing/warmup.js 2>/dev/null || true

        # 3. Cooldown sebelum test (30 detik)
        echo "  Cooling down (30s)..."
        sleep 30

        # 4. Record start time untuk metric export
        START_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

        # 4b. Jika skenario adalah s10, pastikan graph-analyzer di-scale ke 0 untuk mode Static TTL
        if [[ "$scenario" == "s10-ttl-comparison" ]]; then
            echo "  [S10 detected] Scaling down graph-analyzer to 0 for Static TTL mode..."
            kubectl scale deploy graph-analyzer -n zta-research --replicas=0 2>/dev/null || true
            sleep 15
        else
            # Untuk skenario lain, pastikan graph-analyzer aktif (1 replica)
            # Terutama setelah S10 selesai atau untuk skenario awal
            CURRENT_REPLICAS=$(kubectl get deploy graph-analyzer -n zta-research -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")
            if [[ "$CURRENT_REPLICAS" -eq 0 ]]; then
                echo "  Scaling graph-analyzer back up to 1 replica..."
                kubectl scale deploy graph-analyzer -n zta-research --replicas=1 2>/dev/null || true
                sleep 10
                kubectl wait --for=condition=ready pod -l app=graph-analyzer -n zta-research --timeout=120s 2>/dev/null || true
            fi
        fi

        # 5. Jalankan skenario
        echo "  Executing k6 scenario..."
        k6 run \
          --summary-export="$OUTFILE" \
          "load-testing/scenarios/${scenario}.js" || true

        # 6. Record end time
        END_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

        # 6b. Kembalikan replica jika baru selesai menjalankan S10
        if [[ "$scenario" == "s10-ttl-comparison" ]]; then
            echo "  Restoring graph-analyzer deployment to 1 replica..."
            kubectl scale deploy graph-analyzer -n zta-research --replicas=1 2>/dev/null || true
            sleep 10
        fi

        # 7. Export Prometheus metrics
        echo "  Exporting Prometheus metrics..."
        PROM_URL="${PROMETHEUS_URL:-http://prometheus:9090}"
        python3 scripts/export-metrics.py \
          --prometheus-url "$PROM_URL" \
          --scenario "$scenario" \
          --start "$START_TIME" \
          --end "$END_TIME" \
          --output "$RESULTS_DIR" \
          --repetition "$rep" 2>/dev/null || echo "  WARNING: metric export failed"

        echo "  Done: $scenario (rep $rep) -> $OUTFILE"

        # 8. Cooldown antar skenario (2 menit)
        echo "  Cooldown (120s)..."
        sleep 120
    done
done

echo ""
echo "=========================================="
echo "All ${#SCENARIOS[@]} scenarios x $REPETITIONS repetitions complete."
echo "Results in: $RESULTS_DIR/"
echo ""
echo "Next steps:"
echo "  1. Ingest & analyze all data: python3 scripts/analyze-all-data.py"
echo "  2. Generate publication figures: python3 scripts/generate-figures.py"
echo "=========================================="
