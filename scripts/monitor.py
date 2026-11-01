#!/usr/bin/env python3
"""
scripts/monitor.py
Skrip praktis untuk memantau progres eksperimen benchmark k6 di VM dari mesin lokal.
Penggunaan:
  python scripts/monitor.py
"""

import sys
import paramiko

HOST = "172.232.248.69"
USER = "thyo"
PASS = "alkhawarizmi"

def main():
    try:
        ssh = paramiko.SSHClient()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        ssh.connect(HOST, username=USER, password=PASS, timeout=10)
    except Exception as e:
        print(f"[!] Gagal terhubung ke VM ({HOST}): {e}")
        sys.exit(1)

    status_cmd = """
    echo "=================================================================="
    echo "  MONITORING BENCHMARK ZTA PREDICTIVE CACHE (VM: 172.232.248.69)"
    echo "=================================================================="
    echo ""
    echo "--- STATUS SESI TMUX ---"
    tmux ls 2>/dev/null || echo "Tidak ada sesi tmux yang aktif"
    echo ""
    echo "--- PROSES AKTIF ---"
    ps aux | grep -E "run-all|k6" | grep -v grep || echo "Tidak ada proses k6 yang sedang berjalan"
    echo ""
    echo "--- JUMLAH BERKAS HASIL SELESAI ---"
    DIR=$(ls -dt /home/thyo/zta-predictive-cache/load-testing/results/run_full_30rep_* 2>/dev/null | head -n 1)
    if [ -n "$DIR" ]; then
        JSON_COUNT=$(ls -1 "$DIR"/*.json 2>/dev/null | wc -l)
        CSV_COUNT=$(ls -1 "$DIR"/*.csv 2>/dev/null | wc -l)
        echo "Direktori Output: $DIR"
        echo "Selesai JSON k6 : $JSON_COUNT / 300  ($(( JSON_COUNT * 100 / 300 ))%)"
        echo "Selesai CSV Prom: $CSV_COUNT / 300"
    else
        echo "Direktori hasil belum ditemukan."
    fi
    echo ""
    echo "--- 20 BARIS LOG TERAKHIR (REAL-TIME) ---"
    LOG=$(ls -t /home/thyo/zta-predictive-cache/experiment_runner_*.log 2>/dev/null | head -n 1)
    if [ -n "$LOG" ]; then
        echo "Berkas Log: $LOG"
        echo "------------------------------------------------------------------"
        tail -n 20 "$LOG"
        echo "------------------------------------------------------------------"
    fi
    """

    stdin, stdout, stderr = ssh.exec_command(status_cmd)
    output = stdout.read().decode('utf-8', errors='replace')
    safe_output = output.encode(sys.stdout.encoding or 'utf-8', errors='replace').decode(sys.stdout.encoding or 'utf-8')
    print(safe_output)
    ssh.close()

if __name__ == "__main__":
    main()
