#!/usr/bin/env python3
"""
verify_offline.py - verifikasi statis temuan 1-5 pada repo zta-predictive-cache.
Tidak butuh cluster. Hanya membaca file repo (kode, manifest, CSV, laporan).

Pemakaian:
    python verify_offline.py [path-ke-repo]        # default: direktori saat ini

Dependensi: pandas, scipy, pyyaml  (pip install pandas scipy pyyaml)

Status per pemeriksaan:
    FLAG  = bukti di repo mendukung kekhawatiran (perlu dicek di cluster/Azure)
    OK    = tidak ditemukan masalah pada pemeriksaan ini
    INFO  = konteks, bukan verdict
Skrip ini membaca REPO, bukan kondisi deploy. Image Docker / manifest di Azure
bisa berbeda; konfirmasi akhir lewat verify_cluster.sh.
"""
import re
import sys
from pathlib import Path

import numpy as np
import pandas as pd
import yaml
from scipy import stats

def detect_root():
    if len(sys.argv) > 1:
        return Path(sys.argv[1]).resolve()
    cwd = Path.cwd().resolve()
    if (cwd / "services").exists() and (cwd / "k8s").exists():
        return cwd
    if (cwd.parent / "services").exists() and (cwd.parent / "k8s").exists():
        return cwd.parent
    return cwd

ROOT = detect_root()
RESULTS = []  # (no, judul, status, ringkasan)


def rd(rel):
    p = ROOT / rel
    return p.read_text(encoding="utf-8", errors="replace") if p.exists() else ""


def sh(title):
    print("\n" + "=" * 78 + "\n" + title + "\n" + "=" * 78)


def rec(no, title, status, summary):
    RESULTS.append((no, title, status, summary))
    print(f"[{status}] {summary}")


# --------------------------------------------------------------------------
# 1. Apakah trafik k6 melewati ext_authz / WASM?
# --------------------------------------------------------------------------
def check_path():
    sh("1. Jalur otorisasi: apakah target k6 dicakup policy ZTA?")
    # a) selector AuthorizationPolicy + WasmPlugin
    selectors = {}
    for f in sorted((ROOT / "k8s" / "istio").glob("*.yaml")):
        for doc in yaml.safe_load_all(f.read_text(encoding="utf-8")):
            if not doc or doc.get("kind") not in ("AuthorizationPolicy", "WasmPlugin"):
                continue
            app = (doc.get("spec", {}).get("selector", {}) or {}).get("matchLabels", {}).get("app")
            selectors.setdefault(doc["kind"], set()).add(app)
    for k, v in selectors.items():
        print(f"  {k:<20} selector app = {sorted(x for x in v if x)}")
    covered = set().union(*selectors.values()) if selectors else set()

    # b) host yang ditembak k6
    hosts = {}
    for f in sorted((ROOT / "load-testing" / "scenarios").glob("*.js")):
        for h in set(re.findall(r"http://([a-z0-9-]+)[.:/]", rd(f.relative_to(ROOT).as_posix()))):
            hosts.setdefault(h, []).append(f.name.split("-")[0])
    print("  Host target k6      :", {h: sorted(set(s)) for h, s in hosts.items()})

    # c) apakah service melakukan panggilan keluar (east-west)?
    outbound = {}
    for d in sorted((ROOT / "services").glob("service-*")):
        src = rd((d / "main.go").relative_to(ROOT).as_posix())
        outbound[d.name] = bool(re.search(r"http\.(Get|Post|NewRequest|Client)\b|grpc\.Dial", src))
    print("  Panggilan keluar    :", outbound)

    hosts = {h: s for h, s in hosts.items() if h != "cache-distributor"}  # pemicu /revoke, bukan trafik uji
    uncovered = sorted(h for h in hosts if h not in covered)
    any_outbound = any(outbound.values())
    if uncovered and not any_outbound:
        rec(1, "Jalur ZTA", "FLAG",
            f"k6 menembak {sorted(hosts)}; policy hanya memilih {sorted(x for x in covered if x)}; "
            f"tidak ada service yang memanggil service lain. Request k6 kemungkinan tidak melewati ext_authz/WASM.")
    elif uncovered:
        rec(1, "Jalur ZTA", "INFO",
            f"Host {uncovered} tidak dipilih policy, tetapi ada panggilan keluar; periksa apakah hop berikutnya dicakup.")
    else:
        rec(1, "Jalur ZTA", "OK", "Semua host target k6 dicakup selector policy.")

    # d) extension provider didefinisikan di manifest?
    provider_names = set(re.findall(r"name:\s*(opa-[\w-]+)", "".join(
        f.read_text(encoding="utf-8", errors="replace") for f in (ROOT / "k8s").rglob("*.yaml"))))
    used = set(re.findall(r"provider:\s*\n\s*name:\s*(\S+)", rd("k8s/istio/authz-policy-zta.yaml")))
    defined = bool(re.search(r"extensionProviders", "".join(
        f.read_text(encoding="utf-8", errors="replace") for f in (ROOT / "k8s").rglob("*.yaml"))))
    docs_name = set(re.findall(r"- name:\s*(opa-[\w-]+)", rd("docs/12_performance_baseline_and_error_handling.md")))
    print(f"  Provider dipakai policy: {sorted(used)} | nama di docs/12: {sorted(docs_name)} | "
          f"extensionProviders ada di manifest k8s/: {defined}")
    if used and not defined:
        rec(1, "Provider ext_authz", "FLAG",
            "extensionProviders tidak ada di manifest repo"
            + (f"; nama di docs/12 ({sorted(docs_name)}) berbeda dari yang dipakai policy ({sorted(used)})."
               if docs_name and docs_name != used else "."))


# --------------------------------------------------------------------------
# 2. Apakah cache sidecar pernah diisi?
# --------------------------------------------------------------------------
def check_wasm():
    sh("2. WASM filter: apakah ada jalur yang mengisi cache / nonce?")
    files = [p for p in (ROOT / "wasm-filter").rglob("*.go") if not p.name.endswith("_test.go")]
    code = {p.relative_to(ROOT).as_posix(): p.read_text(encoding="utf-8", errors="replace") for p in files}

    def callers(pattern, skip_def=None):
        hits = []
        for name, src in code.items():
            for m in re.finditer(pattern, src):
                line = src[: m.start()].count("\n") + 1
                text = src.splitlines()[line - 1].strip()
                if skip_def and re.search(skip_def, text):
                    continue
                if text.startswith("//"):
                    continue
                hits.append(f"{name}:{line}")
        return hits

    writers = callers(r"\.Set\(") + callers(r"SetSharedData|RegisterSharedQueue|DequeueSharedQueue")
    nonce_calls = callers(r"RegisterNonce\(", skip_def=r"^func ")
    pdp_nil = bool(re.search(r"NewDecisionEngine\([^)]*nil\)", code.get("wasm-filter/main.go", "")))
    grpc_client = bool(re.search(r"grpc\.|StreamRevocations|DispatchHttpCall", "".join(code.values())))
    print("  Pemanggil cache.Set / shared queue (non-test):", writers or "TIDAK ADA")
    print("  Pemanggil Validator.RegisterNonce (non-test) :", nonce_calls or "TIDAK ADA")
    print("  DecisionEngine dibuat dengan pdp=nil          :", pdp_nil)
    print("  Ada klien gRPC / DispatchHttpCall di filter   :", grpc_client)

    dist = rd("control-plane/cache-distributor/main.go")
    push_comment = "_ = sig" in dist
    proto_has_push = bool(re.search(r"rpc\s+PushToken\s*\(", rd("control-plane/cache-distributor/proto/cache.proto")))
    server_impl_push = bool(re.search(r"func\s+\(\w+\s+\*grpcServer\)\s+PushToken\(", dist))
    grpc_stub = "placeholder" in rd("control-plane/cache-distributor/proto/cache_grpc.pb.go").lower()
    print("  Distributor: token hanya 'digunakan saat push' (komentar _ = sig):", push_comment)
    print("  proto mendefinisikan rpc PushToken                              :", proto_has_push)
    print("  grpcServer mengimplementasikan PushToken                        :", server_impl_push)
    print("  cache_grpc.pb.go masih placeholder (belum di-generate)          :", grpc_stub)

    if not writers and not nonce_calls:
        rec(2, "Cache sidecar", "FLAG",
            "Tidak ada kode non-test yang mengisi cache atau mendaftarkan nonce di filter. "
            "Semua request akan menjadi Miss -> ActionContinue; kalaupun token masuk, "
            "activeNonces kosong sehingga akan dinilai Revoked.")
    else:
        rec(2, "Cache sidecar", "OK", "Ada jalur pengisi cache/nonce di filter.")
    allow_continue = "types.ActionContinue" in code.get("wasm-filter/main.go", "")
    print(f"  Catatan: Allow -> ActionContinue (tidak melewati ext_authz) : {allow_continue}")


# --------------------------------------------------------------------------
# 3. H2: apa yang sebenarnya diukur sebagai 'revocation latency'?
# --------------------------------------------------------------------------
def check_h2():
    sh("3. H2 revokasi: revocation_latency_ms vs p99_ms (data mentah)")
    p = ROOT / "data" / "processed" / "revocation-latency.csv"
    if not p.exists():
        rec(3, "H2 revokasi", "INFO", "revocation-latency.csv tidak ditemukan.")
        return
    d = pd.read_csv(p)
    same = np.allclose(d["revocation_latency_ms"], d["p99_ms"])
    frac_fail = (d["revocation_fail_count"] >= d["revocations_triggered"]).mean()
    print(f"  n={len(d)} | revocation_latency_ms == p99_ms pada semua baris : {same}")
    print(f"  revocations_triggered rata-rata = {d['revocations_triggered'].mean():.1f}; "
          f"revocation_fail_count rata-rata = {d['revocation_fail_count'].mean():.1f}; "
          f"baris dengan fail >= triggered: {frac_fail:.0%}")
    s01 = pd.read_csv(ROOT / "data" / "processed" / "latency-all-scenarios.csv")
    m09 = s01.loc[s01.scenario_id.str.startswith("s09"), "p99_ms"].median()
    m01 = s01.loc[s01.scenario_id.str.startswith("s01"), "p99_ms"].median()
    print(f"  Median p99 S09 = {m09:.3f} ms vs S01 (tanpa ZTA) = {m01:.3f} ms")
    s09 = rd("load-testing/scenarios/s09-revocation-test.js")
    print("  s09 mengukur waktu sampai sidecar menolak token? "
          "Skrip hanya memanggil /revoke dan mencatat timing request GET biasa:",
          "revocation_trigger" in s09 and "p99Latency.add(res.timings.duration)" in s09)
    if same:
        rec(3, "H2 revokasi", "FLAG",
            "revocation_latency_ms identik dengan p99 latensi request k6; ini bukan waktu propagasi revokasi. "
            + ("Selain itu revocation_fail_count >= revocations_triggered pada semua run (cek respons /revoke)."
               if frac_fail == 1 else ""))
    else:
        rec(3, "H2 revokasi", "OK", "Kolom revokasi berbeda dari p99 request.")


# --------------------------------------------------------------------------
# 4. TTL: nilai di kode vs seed vs draf
# --------------------------------------------------------------------------
def ttl_go(calls, pen, cf, base=30.0, lo=5.0, hi=60.0):
    return max(lo, min(hi, base * min(calls / 100.0, 1.0) * (1 - pen) * min(cf, 1.0)))


def check_ttl():
    sh("4. TTL: CalculateTTL (kode) vs seed-candidates.json vs draf")
    cand = rd("control-plane/graph-analyzer/cache/candidate.go")
    m = re.search(r"CalculateTTL\([^)]*?(\d+)\*time\.Hour,\s*(\d+)\*time\.Hour\)", cand)
    cf = (int(m.group(1)) / int(m.group(2))) if m else None
    print(f"  candidate.go memanggil CalculateTTL dengan sisa/total sertifikat = "
          f"{m.group(1)}h/{m.group(2)}h -> certFactor = {cf:.3f}" if m else "  pola pemanggilan tidak ditemukan")
    pens = {"low": 0.0, "medium": 0.3, "high": 0.7}
    print("  TTL hasil CalculateTTL @ >=100 calls/min:",
          {k: f"{ttl_go(100, v, cf):.1f}s" for k, v in pens.items()} if cf else "n/a")
    print("  TTL jika certFactor = 1                 :",
          {k: f"{ttl_go(100, v, 1.0):.1f}s" for k, v in pens.items()})
    seed = rd("control-plane/cache-distributor/seed-candidates.json")
    if seed:
        import json
        rows = json.loads(seed)
        print("  seed-candidates.json (dipakai `make config-zta-cache`):")
        for r in rows:
            print(f"    {r['sensitivity']:<6} {r['target'].split('/')[-1]:<22} {r['suggested_ttl_ms']/1000:.0f}s")
    draft = rd("draft/naskah-artikel.md")
    nums = sorted(set(re.findall(r"\b(21\.000|30\.000)\s*ms", draft)))
    print("  Angka TTL yang disebut di draf          :", nums or "tidak ditemukan")
    mk = rd("Makefile")
    uses_seed = "seed-candidates.json" in mk
    default_policy = re.search(r"Default policyFn.*", rd("control-plane/graph-analyzer/main.go"))
    print("  Makefile config-zta-cache memakai seed statis :", uses_seed)
    print("  graph-analyzer policyFn default                :", default_policy.group(0).strip() if default_policy else "n/a")
    if cf is not None and cf < 1 and uses_seed:
        rec(4, "TTL adaptif", "FLAG",
            f"Tiga set TTL tidak konsisten: kode (certFactor={cf:.2f}) -> 10/7/5 s; seed statis -> 45/20/40/10 s; "
            "draf -> 30/21 s (hanya benar jika certFactor=1). TTL yang dipakai eksperimen kemungkinan dari seed statis, "
            "bukan hasil CalculateTTL.")
    else:
        rec(4, "TTL adaptif", "INFO", "Periksa manual konsistensi TTL.")


# --------------------------------------------------------------------------
# 5. Reproduksi statistik: laporan vs CSV vs hitung ulang
# --------------------------------------------------------------------------
def check_stats():
    sh("5. Konsistensi statistik: hitung ulang dari data mentah")
    d = pd.read_csv(ROOT / "data" / "processed" / "latency-all-scenarios.csv")
    g = lambda s: d.loc[d.scenario_id.str.startswith(s), "p99_ms"].to_numpy()
    sw_csv = pd.read_csv(ROOT / "data" / "reports" / "shapiro-wilk-results.csv")
    kw_csv = pd.read_csv(ROOT / "data" / "reports" / "kruskal-wallis-results.csv")
    report = rd("analysis_report.md")

    W, p = stats.shapiro(g("s03"))
    row = sw_csv[sw_csv.label == "ZTA-Cache 500 RPS"].iloc[0]
    m = re.search(r"\| ZTA-Cache 500 RPS \| ([\d.]+) \| ([\d.]+) \|", report)
    print("  Shapiro-Wilk ZTA-Cache 500 RPS")
    print(f"    hitung ulang : W={W:.4f}  p={p:.4f}")
    print(f"    CSV reports  : W={row.W:.4f}  p={row.p_value:.4f}")
    print(f"    analysis_report.md : " + (f"W={m.group(1)}  p={m.group(2)}" if m else "tidak ditemukan"))

    H, pk = stats.kruskal(g("s01"), g("s02"), g("s03"))
    krow = kw_csv[kw_csv.comparison.str.contains("Config @ 500", na=False)].iloc[0]
    mk = re.search(r"H=([\d.]+)\s*\|\s*p=([\d.]+)", report)
    print("  Kruskal-Wallis PBS|Naif|Cache @500 RPS")
    print(f"    hitung ulang : H={H:.2f}  p={pk:.4f}")
    print(f"    CSV reports  : H={krow.h_statistic:.2f}  p={krow.p_value:.4f}")
    print(f"    analysis_report.md : " + (f"H={mk.group(1)}  p={mk.group(2)}" if mk else "tidak ditemukan"))

    dup = sw_csv[sw_csv.duplicated(keep=False)]
    print(f"  Baris duplikat di shapiro-wilk-results.csv: {len(dup)//2 if len(dup) else 0} ({sorted(set(dup.label)) if len(dup) else '-'})")

    csv_ok = abs(W - row.W) < 5e-4 and abs(H - krow.h_statistic) < 5e-3
    if not csv_ok:
        rec(5, "Konsistensi statistik", "FLAG", "CSV reports tidak cocok dengan hitung ulang dari data mentah.")
    elif report:
        rep_ok = bool(m and abs(float(m.group(1)) - W) < 5e-4) and bool(mk and abs(float(mk.group(1)) - H) < 5e-3)
        if not rep_ok:
            rec(5, "Konsistensi statistik", "FLAG",
                "CSV reports cocok dengan hitung ulang, tetapi analysis_report.md memuat angka lama (usang); "
                "pastikan naskah dan tabel mengacu ke CSV terbaru. Ada juga baris duplikat di shapiro-wilk-results.csv."
                if len(dup) else
                "CSV reports cocok dengan hitung ulang, tetapi analysis_report.md memuat angka lama (usang).")
        else:
            rec(5, "Konsistensi statistik", "OK", "Laporan, CSV, dan hitung ulang konsisten.")
    else:
        rec(5, "Konsistensi statistik", "OK", "Laporan CSV dan hitung ulang data mentah konsisten (analysis_report.md telah dibersihkan).")


if __name__ == "__main__":
    print(f"Repo: {ROOT}")
    for fn in (check_path, check_wasm, check_h2, check_ttl, check_stats):
        try:
            fn()
        except Exception as e:  # satu pemeriksaan gagal tidak menghentikan yang lain
            print(f"  [ERROR] {fn.__name__}: {type(e).__name__}: {e}")
    sh("RINGKASAN")
    for no, title, st, summ in RESULTS:
        print(f"  #{no} {title:<22} {st}")
    print("\nCatatan: ini hasil dari isi repo. Lanjutkan dengan verify_cluster.sh untuk bukti runtime.")
