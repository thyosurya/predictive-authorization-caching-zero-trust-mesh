#!/usr/bin/env python3
"""
scripts/audit_dead_code.py
Melakukan audit statis dead code (kode mati / unreachable / unused declarations)
pada seluruh modul Go aktif (services, control-plane, wasm-filter) dan memverifikasi
bahwa tidak ada fungsi, tipe, atau konstanta yang tidak terpakai.
"""

import os
import re
import sys
import subprocess

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Modul Go aktif yang diaudit
ACTIVE_GO_DIRS = [
    "services/service-a",
    "services/service-b",
    "services/service-c",
    "control-plane/cache-distributor",
    "control-plane/graph-analyzer",
    "wasm-filter",
]

# Simbol antarmuka eksternal yang diwajibkan oleh Envoy Proxy-Wasm ABI atau gRPC protobuf dispatch
ALLOWED_EXTERNAL_DISPATCH = {
    "RevokeToken",     # gRPC CacheServiceServer interface implementation
    "OnVMStart",       # Envoy Proxy-Wasm types.VMContext callback
    "NewPluginContext",# Envoy Proxy-Wasm factory callback
    "OnPluginStart",   # Envoy Proxy-Wasm types.PluginContext callback
    "NewHttpContext",  # Envoy Proxy-Wasm factory callback
}

def check_go_vet():
    """Menjalankan go vet pada setiap package Go aktif."""
    print("[1/3] Menjalankan 'go vet ./...' pada seluruh modul Go aktif...")
    failures = []
    for rel_dir in ACTIVE_GO_DIRS:
        abs_dir = os.path.join(ROOT, rel_dir)
        if not os.path.exists(abs_dir):
            continue
        res = subprocess.run(["go", "vet", "./..."], cwd=abs_dir, capture_output=True, text=True)
        if res.returncode != 0:
            failures.append((rel_dir, res.stderr or res.stdout))
        else:
            print(f"  [PASS] {rel_dir}")
    return failures

def collect_go_files():
    """Mengumpulkan berkas Go aktif (mengecualikan protobuf generated .pb.go)."""
    go_files = []
    for rel_dir in ACTIVE_GO_DIRS:
        abs_dir = os.path.join(ROOT, rel_dir)
        for root, dirs, files in os.walk(abs_dir):
            for f in files:
                if f.endswith(".go") and not f.endswith(".pb.go"):
                    go_files.append(os.path.join(root, f))
    return go_files

def audit_symbol_references():
    """Memindai deklarasi fungsi, struct, dan konstanta serta melacak pemanggilnya."""
    print("[2/3] Memindai deklarasi fungsi, struct, dan konstanta pada Go codebase...")
    go_files = collect_go_files()
    
    all_contents = {}
    for p in go_files:
        with open(p, "r", encoding="utf-8", errors="ignore") as fp:
            all_contents[p] = fp.read()

    func_pattern = re.compile(r"func\s+(?:\([^)]+\)\s+)?([A-Za-z0-9_]+)\s*\(")
    type_pattern = re.compile(r"type\s+([A-Za-z0-9_]+)\s+(?:struct|interface|[a-zA-Z0-9]+)")
    const_pattern = re.compile(r"(?:const\s+([A-Za-z0-9_]+)\s*=|([A-Za-z0-9_]+)\s*=\s*iota)")

    declarations = [] # (kind, name, file, line)

    for p, content in all_contents.items():
        # Jangan anggap method di dalam file test sebagai target deklarasi fungsional sistem
        is_test_file = p.endswith("_test.go")
        for i, line in enumerate(content.splitlines()):
            # Fungsi
            mf = func_pattern.search(line)
            if mf:
                name = mf.group(1)
                if not name.startswith(("Test", "Benchmark", "Example")) and name != "main":
                    declarations.append(("func", name, p, i + 1, is_test_file))
            # Tipe
            mt = type_pattern.search(line)
            if mt:
                tname = mt.group(1)
                declarations.append(("type", tname, p, i + 1, is_test_file))
            # Konstanta
            mc = const_pattern.search(line)
            if mc:
                cname = mc.group(1) or mc.group(2)
                declarations.append(("const", cname, p, i + 1, is_test_file))

    print(f"  Total simbol dideklarasikan: {len(declarations)}")
    
    print("[3/3] Melacak keterpakaian simbol di seluruh berkas...")
    dead_code = []

    for kind, name, path, line_no, is_test_file in declarations:
        if name in ALLOWED_EXTERNAL_DISPATCH:
            continue
        
        # Regex pencarian nama simbol sebagai identifier utuh
        pattern = re.compile(r"\b" + re.escape(name) + r"\b")
        total_occurrences = 0
        used_files = set()

        for p, c in all_contents.items():
            matches = pattern.findall(c)
            if matches:
                total_occurrences += len(matches)
                used_files.add(p)

        # Jika simbol hanya muncul 1 kali (hanya deklarasinya sendiri)
        if total_occurrences <= 1:
            rel_p = os.path.relpath(path, ROOT).replace("\\", "/")
            dead_code.append((kind, name, rel_p, line_no, is_test_file))

    return dead_code

def main():
    print("=" * 65)
    print("  AUDIT DEAD CODE & UNUSED DECLARATIONS: GO CODEBASE")
    print("=" * 65)

    vet_failures = check_go_vet()
    if vet_failures:
        print("\n[FAIL] 'go vet' mendeteksi inkonsistensi:")
        for pkg, err in vet_failures:
            print(f"  Package: {pkg}\n  Error: {err}")
        sys.exit(1)

    dead_symbols = audit_symbol_references()
    
    # Filter helper test internal yang mungkin hanya untuk mocking
    production_dead = [s for s in dead_symbols if not s[4]]
    test_dead = [s for s in dead_symbols if s[4]]

    print("=" * 65)
    print(f"Hasil Audit Dead Code:")
    print(f"  - Dead code pada kode produksi : {len(production_dead)}")
    print(f"  - Dead code pada berkas pengujian: {len(test_dead)}")

    if production_dead:
        print("\n[PERINGATAN] Terdeteksi simbol produksi yang tidak pernah dipanggil:")
        for kind, name, rel_p, line_no, _ in production_dead:
            print(f"  [{kind.upper()}] {rel_p}:{line_no} -> {name}")
        sys.exit(1)
    else:
        print("\n[PASS] 100% BERSIH! Tidak ada dead code / unreferenced symbols di kode produksi.")
        print("Seluruh fungsi produksi dipanggil aktif oleh runtime atau test suite.")
        print("=" * 65)

if __name__ == "__main__":
    main()
