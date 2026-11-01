#!/usr/bin/env python3
"""
scripts/monitor_ablation.py
Pemantau live progres eksperimen ablasi multi-hop (LFU vs Graph) di VM.
Penggunaan:
  python scripts/monitor_ablation.py
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

    cmd = """
    echo "=================================================================="
    echo "  MONITORING ABLATION EXPERIMENT (VM: 172.232.248.69)"
    echo "=================================================================="
    echo ""
    echo "--- STATUS SESI TMUX ---"
    tmux ls 2>/dev/null || echo "Tidak ada sesi tmux yang aktif"
    echo ""
    echo "--- PROSES AKTIF ---"
    ps aux | grep -E "run_ablation|k6" | grep -v grep || echo "Tidak ada proses k6 ablasi yang sedang berjalan"
    echo ""
    echo "--- JUMLAH BERKAS ABLASI SELESAI ---"
    ls -1 /home/thyo/zta-predictive-cache/load-testing/results/ablation/*.json 2>/dev/null | wc -l | awk '{print "Selesai JSON: " $1 " / 10 runs"}'
    echo ""
    echo "--- LOG ABLASI TERAKHIR ---"
    tail -n 15 /home/thyo/zta-predictive-cache/ablation_runner.log 2>/dev/null || echo "Belum ada log"
    echo "=================================================================="
    """

    _, stdout, _ = ssh.exec_command(cmd)
    output = stdout.read().decode("utf-8", errors="replace")
    ssh.close()

    print(output)

if __name__ == "__main__":
    main()

